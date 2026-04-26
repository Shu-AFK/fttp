package http1

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type Response struct {
	header        http.Header
	body          []byte
	statusCode    int
	connection    net.Conn
	headerWritten bool
	finalized     bool
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
	if r.headerWritten || statusCode < 100 || statusCode >= 600 {
		return
	}
	r.statusCode = statusCode
	r.headerWritten = true
}

func (r *Response) Write(data []byte) (int, error) {
	if !r.headerWritten {
		r.WriteHeader(http.StatusOK)
	}
	r.body = append(r.body, data...)
	return len(data), nil
}

// Finalize flushes the buffered status line, headers, and body to the
// underlying connection. Called once per request after the handler returns.
// Buffering means Content-Length is always set correctly, so the response
// is self-framing and the connection can be reused.
func (r *Response) Finalize() error {
	if r.finalized {
		return nil
	}
	r.finalized = true

	if r.header.Get("Content-Type") == "" && len(r.body) > 0 {
		sniff := r.body
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		r.header.Set("Content-Type", http.DetectContentType(sniff))
	}
	if r.header.Get("Content-Length") == "" {
		r.header.Set("Content-Length", strconv.Itoa(len(r.body)))
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
	if len(r.body) > 0 {
		if _, err := r.connection.Write(r.body); err != nil {
			return err
		}
	}
	return nil
}
