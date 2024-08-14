package httpResponse

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type Response struct {
	header             http.Header
	connection         net.Conn
	headerWritten      bool
	preventFutureReads bool
}

const CONTENTSIZEMIN = 1024 * 5

func NewResponse(conn net.Conn) *Response {
	res := &Response{
		header:             http.Header{},
		connection:         conn,
		headerWritten:      false,
		preventFutureReads: false,
	}

	return res
}

func (r *Response) Header() http.Header {
	return r.header
}

func (r *Response) Write(data []byte) (int, error) {
	var length int
	if len(data) < 512 {
		length = len(data)
	} else {
		length = 512
	}

	if r.Header().Get("Content-Type") == "" {
		r.Header().Set("Content-Type", http.DetectContentType(data[:length]))
	}
	if len(data) < CONTENTSIZEMIN {
		r.header.Set("Content-Length", strconv.Itoa(len(data)))
	}
	if !r.headerWritten {
		r.WriteHeader(http.StatusOK)
	}

	r.preventFutureReads = true
	wrote, err := r.connection.Write(data)
	if err != nil {
		return 0, err
	}

	return wrote, nil
}

func (r *Response) WriteHeader(statusCode int) {
	if statusCode < 100 || statusCode >= 600 {
		return
	}

	_, err := r.connection.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, http.StatusText(statusCode))))
	if err != nil {
		return
	}

	for key, values := range r.header {
		_, err := r.connection.Write([]byte(fmt.Sprintf("%s: %s\r\n", key, strings.Join(values, ", "))))
		if err != nil {
			return
		}
	}

	_, err = r.connection.Write([]byte("\r\n"))
	if err != nil {
		return
	}

	r.headerWritten = true
}
