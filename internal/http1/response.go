package http1

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Response is an http.ResponseWriter that streams bytes to the underlying
// connection. When the handler does not set Content-Length before the first
// Write, the body is sent as Transfer-Encoding: chunked so the connection
// stays self-framing and reusable for keep-alive.
type Response struct {
	header         http.Header
	statusCode     int
	connection     net.Conn
	statusCodeSet  bool
	headersFlushed bool
	chunked        bool
	finalized      bool
}

func NewResponse(conn net.Conn) *Response {
	return &Response{
		header:     http.Header{},
		statusCode: http.StatusOK,
		connection: conn,
	}
}

func (r *Response) Header() http.Header {
	return r.header
}

func (r *Response) WriteHeader(statusCode int) {
	if r.statusCodeSet || statusCode < 100 || statusCode >= 600 {
		return
	}
	r.statusCode = statusCode
	r.statusCodeSet = true
}

func (r *Response) Write(p []byte) (int, error) {
	if !r.statusCodeSet {
		r.WriteHeader(http.StatusOK)
	}
	if !r.headersFlushed {
		if err := r.flushHeaders(); err != nil {
			return 0, err
		}
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.chunked {
		return r.writeChunk(p)
	}
	return r.connection.Write(p)
}

// flushHeaders emits the status line and headers, deciding between
// Content-Length passthrough and Transfer-Encoding: chunked.
func (r *Response) flushHeaders() error {
	if r.headersFlushed {
		return nil
	}

	if r.header.Get("Content-Length") == "" && r.header.Get("Transfer-Encoding") == "" {
		r.header.Set("Transfer-Encoding", "chunked")
		r.chunked = true
	}

	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", r.statusCode, http.StatusText(r.statusCode))
	if _, err := r.connection.Write([]byte(statusLine)); err != nil {
		return err
	}
	for key, values := range r.header {
		line := fmt.Sprintf("%s: %s\r\n", key, strings.Join(values, ", "))
		if _, err := r.connection.Write([]byte(line)); err != nil {
			return err
		}
	}
	if _, err := r.connection.Write([]byte("\r\n")); err != nil {
		return err
	}

	r.headersFlushed = true
	return nil
}

func (r *Response) writeChunk(p []byte) (int, error) {
	head := fmt.Sprintf("%x\r\n", len(p))
	if _, err := r.connection.Write([]byte(head)); err != nil {
		return 0, err
	}
	n, err := r.connection.Write(p)
	if err != nil {
		return n, err
	}
	if _, err := r.connection.Write([]byte("\r\n")); err != nil {
		return n, err
	}
	return n, nil
}

// Finalize completes the response: emits the chunked terminator if the
// body was chunk-framed, or writes a Content-Length: 0 head if no body
// was ever written. Called once per request after the handler returns.
func (r *Response) Finalize() error {
	if r.finalized {
		return nil
	}
	r.finalized = true

	if !r.statusCodeSet {
		r.WriteHeader(http.StatusOK)
	}
	if !r.headersFlushed {
		if r.header.Get("Content-Length") == "" && r.header.Get("Transfer-Encoding") == "" {
			r.header.Set("Content-Length", "0")
		}
		return r.flushHeaders()
	}
	if r.chunked {
		_, err := r.connection.Write([]byte("0\r\n\r\n"))
		return err
	}
	return nil
}
