package http2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	hpack "github.com/tatsuhiro-t/go-http2-hpack"
	"httpServer/internal/http2/frame"
	"httpServer/internal/http2/structs"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type Response struct {
	header             http.Header
	body               []byte
	essential          structs.ResponseEssential
	lastStreamID       uint32
	headerWritten      bool
	preventFutureReads bool
	maxTableSze        int
}

const CONTENT_SIZE_MIN = 1_024 * 5
const MAX_DATA_BODY_LENGTH = 16_000_000

const MAX_HEADER_BODY_LENGTH = 8_000

func SendFrame(conn net.Conn, iType uint8, flags uint8, streamID uint32, data []byte) error {
	var message bytes.Buffer

	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
	message.Write(lengthBytes[1:])

	message.WriteByte(iType)
	message.WriteByte(flags)

	// Sets the reserved bit to 0
	streamID &^= 1 << 31
	streamIDBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(streamIDBytes, streamID)
	message.Write(streamIDBytes)

	message.Write(data)

	_, err := conn.Write(message.Bytes())
	if err != nil {
		return fmt.Errorf("send frame failed: %w", err)
	}

	return nil
}

func NewResponse(conn net.Conn, streamID uint32, essential structs.ResponseEssential) *Response {
	return &Response{
		header:             http.Header{},
		essential:          essential,
		headerWritten:      false,
		preventFutureReads: false,
		lastStreamID:       streamID,
	}
}

func (r *Response) SetMaxTableSize(s int) {
	r.maxTableSze = s
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
		if len(data) < CONTENT_SIZE_MIN {
			r.header.Set("Content-Length", strconv.Itoa(len(data)))
		}

		r.WriteHeader(http.StatusOK)
	}

	r.preventFutureReads = true

	r.body = append(r.body, data...)

	var wrote int
	var toWrite = len(r.body)

	for toWrite != 0 {
		if toWrite > MAX_DATA_BODY_LENGTH {
			r.essential.FrameChan <- frame.NewFrame(structs.DATA_FRAME_TYPE, 0x00, r.lastStreamID, data[wrote:wrote+MAX_DATA_BODY_LENGTH])
			toWrite -= MAX_DATA_BODY_LENGTH
			wrote += MAX_DATA_BODY_LENGTH
		} else {
			r.essential.FrameChan <- frame.NewFrame(structs.DATA_FRAME_TYPE, structs.END_STREAM, r.lastStreamID, data[wrote:toWrite])
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
			headers = append(headers, &hpack.Header{Name: strings.ToLower(key), Value: value})
		}
	}

	headers = append(headers, &hpack.Header{Name: ":status", Value: strconv.Itoa(statusCode)})

	var encodedHeaders bytes.Buffer
	r.essential.Enc.Encode(&encodedHeaders, headers)

	r.essential.FrameChan <- frame.NewFrame(structs.HEADER_FRAME_TYPE, structs.END_HEADERS, r.lastStreamID, encodedHeaders.Bytes())

	r.headerWritten = true
}

func SendFrames(essential structs.ResponseEssential) {
	for f := range essential.FrameChan {
		err := SendFrame(essential.Connection, f.Type, f.Flags, f.StreamID, f.Payload)
		if err != nil {
			fmt.Printf("send frame failed: %v\n", err)
			return
		}
	}
}
