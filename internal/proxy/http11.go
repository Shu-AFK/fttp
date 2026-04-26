package proxy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"httpServer/internal/http1"
	"httpServer/internal/logging"
)

func (p *Proxy) handleAccept(conn net.Conn, r chi.Router) {
	defer func() {
		if err := conn.Close(); err != nil {
			p.Log(logging.LogLevelError, "Error closing connection from %v: %v", conn.RemoteAddr(), err)
		}
	}()

	p.Log(logging.LogLevelInfo, "New connection from %v", conn.RemoteAddr())

	tlsConn, ok := conn.(*tls.Conn)
	if ok {
		p.Log(logging.LogLevelDebug, "Performing TLS handshake with %v", conn.RemoteAddr())
		if err := tlsConn.Handshake(); err != nil {
			p.Log(logging.LogLevelError, "TLS handshake failed with %v: %v", conn.RemoteAddr(), err)
			return
		}
	}

	if !ok || tlsConn.ConnectionState().NegotiatedProtocol == "http/1.1" {
		p.handleHTTP11(conn, r)
	} else if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
		p.handleHTTP2(tlsConn, r)
	}

	p.Log(logging.LogLevelInfo, "Handled connection from %v", conn.RemoteAddr())
}

func (p *Proxy) handleHTTP11(conn net.Conn, r chi.Router) {
	requestReader := bufio.NewReader(conn)

	for {
		// TODO: Turn into a go routine in order
		req, err, moreRequests := http1.Parser(requestReader)
		sendBadRequest := false
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				p.Log(logging.LogLevelDebug, "Client %v closed the connection", conn.RemoteAddr())
				return
			}
			if errors.Is(err, http1.ChunkEncodingError) {
				sendBadRequest = true
				p.Log(logging.LogLevelWarn, "Chunk encoding error for %v: %v", conn.RemoteAddr(), err)
			} else {
				p.Log(logging.LogLevelError, "Failed to parse request from %v: %v", conn.RemoteAddr(), err)
				return
			}
		}

		responseWriter := http1.NewResponse(conn)
		if sendBadRequest {
			responseWriter.WriteHeader(http.StatusBadRequest)
			if ferr := responseWriter.Finalize(); ferr != nil {
				p.Log(logging.LogLevelError, "Failed to flush response to %v: %v", conn.RemoteAddr(), ferr)
			}
			p.Log(logging.LogLevelWarn, "[BAD REQUEST] Chunked encoding issue for %v", conn.RemoteAddr())
			return
		}

		req.RemoteAddr = conn.RemoteAddr().String()
		p.Log(logging.LogLevelDebug, "Serving HTTP/1.1 request from %v", conn.RemoteAddr())
		r.ServeHTTP(responseWriter, req)
		if ferr := responseWriter.Finalize(); ferr != nil {
			p.Log(logging.LogLevelError, "Failed to flush response to %v: %v", conn.RemoteAddr(), ferr)
			return
		}

		if !moreRequests {
			break
		}
	}
}
