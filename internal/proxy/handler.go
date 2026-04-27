package proxy

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"httpServer/internal/cache"
	"httpServer/internal/logging"
)

func (p *Proxy) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	body := []byte("Not Found")
	p.Log(logging.LogLevelWarn, "Not Found: %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusNotFound)
	if _, err := w.Write(body); err != nil {
		p.Log(logging.LogLevelError, "Response writer failed in NotFoundHandler: %s", err)
	}
}

func (p *Proxy) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	body := []byte("Method Not Allowed")
	p.Log(logging.LogLevelWarn, "Method Not Allowed: %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusMethodNotAllowed)
	if _, err := w.Write(body); err != nil {
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

	resp, err := forwardRoute.Client.Do(req)
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

	if _, err = io.Copy(w, resp.Body); err != nil {
		p.Log(logging.LogLevelError, "Response body stream failed in ReverseProxyHandler: %v", err)
		return
	}

	p.Log(logging.LogLevelDebug, "Reverse proxy handler finished")
}
