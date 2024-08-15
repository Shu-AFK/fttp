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
	body               []byte
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
	if !r.headerWritten {
		length := min(len(data), 512)

		if r.Header().Get("Content-Type") == "" {
			r.Header().Set("Content-Type", http.DetectContentType(data[:length]))
		}
		if len(data) < CONTENTSIZEMIN {
			r.header.Set("Content-Length", strconv.Itoa(len(data)))
		}

		r.WriteHeader(http.StatusOK)
	}

	r.preventFutureReads = true
	r.body = append(r.body, data...)
	wrote, err := r.connection.Write(data)
	if err != nil {
		return 0, err
	}

	return wrote, nil
}

func (r *Response) WriteHeader(statusCode int) {
	if statusCode < 100 || statusCode >= 600 || r.headerWritten {
		return
	}

	var responseLine = fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, http.StatusText(statusCode))
	// Check if wrote correct
	_, err := r.connection.Write([]byte(responseLine))
	if err != nil {
		return
	}

	for key, values := range r.header {
		headerEntry := fmt.Sprintf("%s: %s\r\n", key, strings.Join(values, ", "))
		fmt.Print(headerEntry)
		_, err := r.connection.Write([]byte(headerEntry))
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
