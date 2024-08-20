package http2

import (
	"fmt"
	"net"
	"net/http"
)

var ConnectionPreface = "PRI * HTTP/2.0\\r\\n\\r\\nSM\\r\\n\\r\\n"

type Response struct {
	header             http.Header
	body               []byte
	connection         net.Conn
	headerWritten      bool
	preventFutureReads bool
}

const CONTENTSIZEMIN = 1024 * 5

func SendSettings(conn net.Conn) error {
	_, err := conn.Write([]byte(ConnectionPreface))
	if err != nil {
		return fmt.Errorf("error writing connection preface: %w", err)
	}

	return nil
}

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
