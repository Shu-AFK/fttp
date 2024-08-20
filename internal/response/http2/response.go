package http2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	hpack "github.com/tatsuhiro-t/go-http2-hpack"
	"httpServer/internal/request/http2"
	"net"
	"net/http"
	"strconv"
)

type Response struct {
	header             http.Header
	body               []byte
	connection         net.Conn
	headerWritten      bool
	preventFutureReads bool
	maxTableSze        int
}

const CONTENTSIZEMIN = 1_024 * 5
const MAX_DATA_BODY_LENGTH = 16_000_000
const MAX_HEADER_BODY_LENGTH = 8_000

func SendFrame(conn net.Conn, iType uint8, flags uint8, streamID uint32, data []byte) error {
	var message bytes.Buffer

	lengthBytes := make([]byte, 3)
	binary.BigEndian.PutUint32(append([]byte{0}, lengthBytes...), uint32(len(data)))
	message.Write(lengthBytes)

	message.WriteByte(iType)
	message.WriteByte(flags)

	// Sets the reserved bit to 0
	streamID &^= 1 << 31
	message.Write([]byte(strconv.Itoa(int(streamID))))

	message.Write(data)

	_, err := conn.Write(message.Bytes())
	if err != nil {
		return fmt.Errorf("send frame failed: %w", err)
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

func (r *Response) SetMaxTableSize(s int) {
	r.maxTableSze = s
}

func (r *Response) Header() http.Header {
	return r.header
}

// TODO: fix curl: (16) Error in the HTTP2 framing layer
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

		r.headerWritten = true
	}

	r.preventFutureReads = true

	// Terminate stream early
	if len(data) == 0 {
		err := SendFrame(r.connection, http2.DATA_FRAME_TYPE, http2.END_STREAM, 0x02, nil)
		if err != nil {
			return 0, fmt.Errorf("send frame failed: %w", err)
		}
		return 0, nil
	}

	r.body = append(r.body, data...)

	var wrote int
	var toWrite = len(r.body)

	for toWrite != 0 {
		if toWrite > MAX_DATA_BODY_LENGTH {
			err := SendFrame(r.connection, http2.DATA_FRAME_TYPE, 0x00, 0x02, data[wrote:MAX_DATA_BODY_LENGTH])
			if err != nil {
				return wrote, fmt.Errorf("send data frame failed: %w", err)
			}
			toWrite -= MAX_DATA_BODY_LENGTH
			wrote += MAX_DATA_BODY_LENGTH
		} else {
			err := SendFrame(r.connection, http2.DATA_FRAME_TYPE, http2.END_STREAM, 0x02, data[wrote:toWrite])
			if err != nil {
				return wrote, fmt.Errorf("send data frame failed: %w", err)
			}
			wrote += toWrite
			toWrite -= toWrite
		}
	}

	return wrote, nil
}

func (r *Response) WriteHeader(statusCode int) {
	var headers []*hpack.Header

	for key, values := range r.header {
		for _, value := range values {
			headers = append(headers, &hpack.Header{Name: key, Value: value})
		}
	}

	headers = append(headers, &hpack.Header{Name: ":status", Value: strconv.Itoa(statusCode)})

	// TODO: Pull max table size from settings??
	enc := hpack.NewEncoder(4096)
	var encodedHeaders bytes.Buffer
	enc.Encode(&encodedHeaders, headers)

	err := SendFrame(r.connection, http2.HEADER_FRAME_TYPE, http2.END_HEADERS, 0x02, encodedHeaders.Bytes())
	if err != nil {
		fmt.Printf("send frame failed: %v\n", err)
		return
	}
}
