package proxy

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"httpServer/internal/cache"
	"httpServer/internal/logging"
)

func (p *Proxy) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	p.Log(logging.LogLevelWarn, "Not Found: %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusNotFound)
	if _, err := w.Write([]byte("Not Found")); err != nil {
		p.Log(logging.LogLevelError, "Response writer failed in NotFoundHandler: %s", err)
	}
}

func (p *Proxy) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	p.Log(logging.LogLevelWarn, "Method Not Allowed: %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusMethodNotAllowed)
	if _, err := w.Write([]byte("Method Not Allowed")); err != nil {
		p.Log(logging.LogLevelError, "Response writer failed in MethodNotAllowedHandler: %s", err)
	}
}

func resolveRoute(routes []ProxyRoute, path string) *ProxyRoute {
	var best *ProxyRoute
	bestLen := -1
	for i := range routes {
		prefix := strings.TrimRight(routes[i].Path, "/")
		if path == prefix || path == prefix+"/" || strings.HasPrefix(path, prefix+"/") {
			if len(prefix) > bestLen {
				best = &routes[i]
				bestLen = len(prefix)
			}
		}
	}
	return best
}

// hopHeaders are stripped per RFC 7230 sec 6.1 before forwarding.
var hopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func stripHopHeaders(h http.Header) {
	for _, c := range h.Values("Connection") {
		for _, name := range strings.Split(c, ",") {
			if name = strings.TrimSpace(name); name != "" {
				h.Del(name)
			}
		}
	}
	for _, name := range hopHeaders {
		h.Del(name)
	}
}

// TODO: Finish — currently unused; kept for the planned switch from
// InsecureSkipVerify to a real CA-rooted client.
func (p *Proxy) loadSystemCAs() *x509.CertPool {
	p.Log(logging.LogLevelDebug, "Loading system certificates")

	if runtime.GOOS == "windows" {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			p.Log(logging.LogLevelError, "Failed to load system CA certificates on Windows: %v", err)
			return nil
		}
		p.Log(logging.LogLevelDebug, "System certificate pool loaded on Windows")
		return certPool
	}

	systemCerts, err := os.ReadFile("/etc/ssl/certs/ca-certificates.crt")
	if err != nil {
		if os.IsNotExist(err) {
			p.Log(logging.LogLevelError, "System CA cert file not found, please ensure your system has CA certificates installed.")
		} else {
			p.Log(logging.LogLevelError, "Failed to load system CA certificates: %v", err)
		}
		return nil
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(systemCerts); !ok {
		p.Log(logging.LogLevelError, "Failed to append system CA certificates")
		return nil
	}

	if len(certPool.Subjects()) == 0 {
		p.Log(logging.LogLevelError, "Loaded certificate pool is empty")
		return nil
	}

	p.Log(logging.LogLevelDebug, "Successfully loaded system CA certificates")
	return certPool
}

func (p *Proxy) ReverseProxyHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := http.StatusOK
	upstream := ""
	defer func() {
		cacheState := cache.CacheStatus(r)
		if cacheState == "" {
			cacheState = "off"
		}
		p.Log(logging.LogLevelInfo,
			"request method=%s path=%q status=%d cache=%s dur=%s upstream=%q",
			r.Method, r.URL.Path, status, cacheState, time.Since(start), upstream)
	}()

	forwardRoute := resolveRoute(p.Routes, r.URL.Path)
	if forwardRoute == nil {
		p.Log(logging.LogLevelWarn, "Forwarding to nil target: %s %s", r.Method, r.URL.Path)
		status = http.StatusBadGateway
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	matchedPrefix := strings.TrimRight(forwardRoute.Path, "/")
	tail := strings.TrimPrefix(r.URL.Path, matchedPrefix)
	targetPath := strings.TrimRight(forwardRoute.TargetPath, "/") + tail
	if targetPath == "" {
		targetPath = "/"
	}

	targetURL := *forwardRoute.Host
	targetURL.Path = targetPath
	targetURL.RawQuery = r.URL.RawQuery
	upstream = targetURL.String()

	req, err := http.NewRequest(r.Method, upstream, r.Body)
	if err != nil {
		p.Log(logging.LogLevelError, "New request creation failed in ReverseProxyHandler: %v", err)
		status = http.StatusInternalServerError
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	req.Header = r.Header.Clone()
	stripHopHeaders(req.Header)

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		p.Log(logging.LogLevelWarn, "Failed to parse remote address %q: %v", r.RemoteAddr, err)
		ip = r.RemoteAddr
	}
	if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
		ip = prior + ", " + ip
	}
	req.Header.Set("X-Forwarded-For", ip)

	if req.Header.Get("X-Forwarded-Host") == "" && r.Host != "" {
		req.Header.Set("X-Forwarded-Host", r.Host)
	}
	if req.Header.Get("X-Forwarded-Proto") == "" {
		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}
		req.Header.Set("X-Forwarded-Proto", proto)
	}

	for key, values := range p.AddedHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		p.Log(logging.LogLevelError, "Request forwarding failed in ReverseProxyHandler: %v", err)
		status = http.StatusBadGateway
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	status = resp.StatusCode

	stripHopHeaders(resp.Header)
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	p.Log(logging.LogLevelDebug, "Received status code: %d", resp.StatusCode)

	var buffer bytes.Buffer
	if _, err = io.Copy(&buffer, resp.Body); err != nil {
		p.Log(logging.LogLevelError, "Response body copy failed in ReverseProxyHandler: %v", err)
		return
	}
	if _, err = w.Write(buffer.Bytes()); err != nil {
		p.Log(logging.LogLevelError, "Response writer failed in ReverseProxyHandler: %v", err)
		return
	}

	p.Log(logging.LogLevelDebug, "Reverse proxy handler finished")
}
