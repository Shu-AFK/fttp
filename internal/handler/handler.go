package handler

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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

// ReverseProxyHandler TODO: Not implemented
func ReverseProxyHandler(w http.ResponseWriter, r *http.Request) {
	Proxy.Log(logging.LogLevelWarn, "Not implemented yet")
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
