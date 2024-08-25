package handler

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/go-chi/chi/v5"
	hpack "github.com/tatsuhiro-t/go-http2-hpack"

	"httpServer/internal/http2/frame"
	"httpServer/internal/http2/structs"
	"httpServer/internal/logging"
	http11 "httpServer/internal/request/http1.1"
	"httpServer/internal/request/http2"
	http11Response "httpServer/internal/response/http1.1"
	http2Response "httpServer/internal/response/http2"
	proxystructs "httpServer/internal/reverseproxy/structs"
)

var Proxy proxystructs.ProxyHandler

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	Proxy.Log(logging.LogLevelWarn, "Not Found: %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusNotFound)
	_, err := w.Write([]byte("Not Found"))
	if err != nil {
		Proxy.Log(logging.LogLevelError, "Response writer failed in NotFoundHandler: %s", err)
	}
}

func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	Proxy.Log(logging.LogLevelWarn, "Method Not Allowed: %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, err := w.Write([]byte("Method Not Allowed"))
	if err != nil {
		Proxy.Log(logging.LogLevelError, "Response writer failed in MethodNotAllowedHandler: %s", err)
	}
}

func resolveRoute(route []proxystructs.ProxyRoute, path string) *url.URL {
	for _, route := range route {
		if route.Path == path {
			return route.Target
		}
	}

	return nil
}

/* TODO: Finish
func loadSystemCAs() *x509.CertPool {
	Proxy.Log(logging.LogLevelDebug, "Loading system certificates")
	var certPool *x509.CertPool
	var err error

	if runtime.GOOS == "windows" {
		// On Windows, use the system certificate pool
		certPool, err = x509.SystemCertPool()
		if err != nil {
			Proxy.Log(logging.LogLevelError, "Failed to load system CA certificates on Windows: %v", err)
			return nil
		}
		Proxy.Log(logging.LogLevelDebug, "System certificate pool loaded on Windows")
	} else {
		// On Unix-like systems, manually load the certificate bundle
		systemCerts, err := os.ReadFile("/etc/ssl/certs/ca-certificates.crt")
		if err != nil {
			if os.IsNotExist(err) {
				Proxy.Log(logging.LogLevelError, "System CA cert file not found, please ensure your system has CA certificates installed.")
			} else {
				Proxy.Log(logging.LogLevelError, "Failed to load system CA certificates: %v", err)
			}
			return nil
		}

		certPool = x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(systemCerts); !ok {
			Proxy.Log(logging.LogLevelError, "Failed to append system CA certificates")
			return nil
		}
	}

	if certPool == nil || len(certPool.Subjects()) == 0 {
		Proxy.Log(logging.LogLevelError, "Loaded certificate pool is empty")
		return nil // Return nil in case of failure
	}

	Proxy.Log(logging.LogLevelDebug, "Successfully loaded system CA certificates")
	return certPool
} */

func ReverseProxyHandler(w http.ResponseWriter, r *http.Request) {
	forwardTo := resolveRoute(Proxy.GetRoutes(), r.URL.Path)
	if forwardTo == nil {
		Proxy.Log(logging.LogLevelWarn, "Forwarding to nil target: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	req, err := http.NewRequest(r.Method, forwardTo.String(), r.Body)
	if err != nil {
		Proxy.Log(logging.LogLevelError, "New request creation failed in ReverseProxyHandler: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	req.Header = r.Header.Clone()

	// Setting up HTTP client with system CA certificates
	/* TODO: Uncomment when loadSystemCAs works
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: loadSystemCAs(),
			},
		},
	}
	*/

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		Proxy.Log(logging.LogLevelError, "Request forwarding failed in ReverseProxyHandler: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	Proxy.Log(logging.LogLevelDebug, "Received status code: %d", resp.StatusCode)

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, resp.Body)
	if err != nil {
		Proxy.Log(logging.LogLevelError, "Response body copy failed in ReverseProxyHandler: %v", err)
		return
	}

	_, err = w.Write(buffer.Bytes())
	if err != nil {
		Proxy.Log(logging.LogLevelError, "Response writer failed in ReverseProxyHandler: %v", err)
		return
	}

	Proxy.Log(logging.LogLevelDebug, "Reverse proxy handler finished")
}

func HandleHttp2(reader io.Reader, essential *structs.ParsingEssential, respEssential structs.ResponseEssential) {
	Proxy.Log(logging.LogLevelInfo, "Starting HTTP/2 handling")

	iReader := bufio.NewReader(reader)

	err := HandleStreamMultiplexing(iReader, essential, respEssential)
	if err != nil {
		Proxy.Log(logging.LogLevelError, "Error in stream multiplexing: %v", err)
		return
	}

	Proxy.Log(logging.LogLevelInfo, "Completed HTTP/2 handling")
	return
}

func HandleStreamMultiplexing(reader *bufio.Reader, essential *structs.ParsingEssential, respEssential structs.ResponseEssential) error {
	Proxy.Log(logging.LogLevelInfo, "Starting stream multiplexing")

	for {
		f, err := frame.ParseFrame(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			Proxy.Log(logging.LogLevelError, "Cannot parse frame data: %v", err)
			return fmt.Errorf("cannot parse frame data: %v", err)
		}

		if f.StreamID == 0 {
			Proxy.Log(logging.LogLevelDebug, "Skipping frame with StreamID 0")
			continue
		}

		newChannel := false

		if _, exists := essential.Channels[f.StreamID]; !exists {
			Proxy.Log(logging.LogLevelDebug, "Creating new channel for StreamID: %d", f.StreamID)
			comm := structs.NewCommunication(essential.Dec, essential.Mutex)

			essential.Channels[f.StreamID] = comm
			newChannel = true
		}

		if newChannel {
			Proxy.Log(logging.LogLevelDebug, "Launching handler for new channel StreamID: %d", f.StreamID)
			go http2.HandleMultiplexedFrameParsing(essential.Channels[f.StreamID], essential.Router, essential.Conn, respEssential)
		}

		select {
		case <-essential.Channels[f.StreamID].Frames: // Channel is closed
			Proxy.Log(logging.LogLevelDebug, "Channel closed for StreamID: %d", f.StreamID)
			continue
		default:
		}

		Proxy.Log(logging.LogLevelDebug, "Handling frame for StreamID: %d", f.StreamID)
		essential.Channels[f.StreamID].Frames <- *f
	}
}

func HandleAccept(conn net.Conn, proxy proxystructs.ProxyHandler, r chi.Router) {
	Proxy = proxy

	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			proxy.Log(logging.LogLevelError, "Error closing connection from %v: %v", conn.RemoteAddr(), err)
		}
	}(conn)

	proxy.Log(logging.LogLevelInfo, "New connection from %v", conn.RemoteAddr())

	tlsConn, ok := conn.(*tls.Conn)
	if ok {
		proxy.Log(logging.LogLevelDebug, "Performing TLS handshake with %v", conn.RemoteAddr())
		err := tlsConn.Handshake()
		if err != nil {
			proxy.Log(logging.LogLevelError, "TLS handshake failed with %v: %v", conn.RemoteAddr(), err)
			return
		}
	}

	if !ok || tlsConn.ConnectionState().NegotiatedProtocol == "http/1.1" {
		HandleHTTP11(conn, r)
	} else if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
		HandleHTTP2(tlsConn, r)
	}

	proxy.Log(logging.LogLevelInfo, "Handled connection from %v", conn.RemoteAddr())
}

func HandleHTTP11(conn net.Conn, r chi.Router) {
	proxy := Proxy // Use global Proxy
	requestReader := bufio.NewReader(conn)
	sendBadRequest := false
	moreRequests := false
	var req *http.Request
	var err error

	for {
		req, err, moreRequests = http11.Parser(requestReader)
		if err != nil {
			if errors.Is(err, http11.ChunkEncodingError) {
				sendBadRequest = true
				proxy.Log(logging.LogLevelWarn, "Chunk encoding error for %v: %v", conn.RemoteAddr(), err)
			} else {
				proxy.Log(logging.LogLevelError, "Failed to parse request from %v: %v", conn.RemoteAddr(), err)
				return
			}
		}

		req.RemoteAddr = conn.RemoteAddr().String()
		responseWriter := http11Response.NewResponse(conn)
		if sendBadRequest {
			responseWriter.WriteHeader(http.StatusBadRequest)
			proxy.Log(logging.LogLevelWarn, "[BAD REQUEST] Chunked encoding issue for %v", conn.RemoteAddr())
		} else {
			proxy.Log(logging.LogLevelDebug, "Serving HTTP/1.1 request from %v", conn.RemoteAddr())
			r.ServeHTTP(responseWriter, req)
		}

		if !moreRequests {
			break
		}
	}
}

func HandleHTTP2(tlsConn *tls.Conn, r chi.Router) {
	proxy := Proxy // Use global Proxy
	requestReader := bufio.NewReader(tlsConn)
	dec := hpack.NewDecoder()

	// Validate settings frame
	err := http2Response.VerifyConnectionPreface(requestReader)
	if err != nil {
		proxy.Log(logging.LogLevelError, "Failed to verify connection preface for %v: %v", tlsConn.RemoteAddr(), err)
		return
	}

	err = http2Response.SendSettingsFrame(tlsConn)
	if err != nil {
		proxy.Log(logging.LogLevelError, "Failed to send settings frame for %v: %v", tlsConn.RemoteAddr(), err)
		return
	}

	proxy.Log(logging.LogLevelDebug, "Established HTTP/2 connection with %v", tlsConn.RemoteAddr())

	respEssential := structs.NewResponseEssential(tlsConn, hpack.NewEncoder(4096))
	go http2Response.SendFrames(*respEssential)

	HandleHttp2(requestReader, structs.NewParsingEssential(dec, new(sync.Mutex), r, tlsConn), *respEssential)
}
