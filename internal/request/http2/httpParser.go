package http2

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/tatsuhiro-t/go-http2-hpack"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Frame struct {
	Length   uint32
	Type     uint8
	Flags    uint8
	StreamID uint32
	Payload  []byte
}

const (
	DATA_FRAME_TYPE = iota
	HEADER_FRAME_TYPE
	PRIORITY_FRAME_TYPE
	RST_STREAM_FRAME_TYPE
	SETTINGS_FRAME_TYPE
	PUSH_PROMISE_FRAME_TYPE
	PING_FRAME_TYPE
	GOAWAY_FRAME_TYPE
	WINDOW_UPDATE_FRAME_TYPE
	CONTINUATION_FRAME_TYPE
)

const (
	PADDED           = 0x08
	END_STREAM       = 0x01
	END_HEADERS      = 0x04
	HEADERS_PRIORITY = 0x20
	ACK              = 0x01
)

func ParseFrame(reader *bufio.Reader) (*Frame, error) {
	newFrame := new(Frame)

	var buffer bytes.Buffer
	_, err := io.CopyN(&buffer, reader, 9)
	if err != nil {
		return nil, fmt.Errorf("cannot read frame data: %v", err)
	}

	var length []byte
	length = append(length, 0)
	length = append(length, buffer.Next(3)...)

	newFrame.Length = binary.BigEndian.Uint32(length)
	newFrame.Type = buffer.Next(1)[0]
	newFrame.Flags = buffer.Next(1)[0]
	newFrame.StreamID = binary.BigEndian.Uint32(buffer.Next(4))

	// Clears the first bit (Reserved)
	newFrame.StreamID &^= 1 << 31

	_, err = io.CopyN(&buffer, reader, int64(newFrame.Length))
	if err != nil {
		return nil, fmt.Errorf("cannot read frame data: %v", err)
	}
	newFrame.Payload = buffer.Bytes()

	return newFrame, nil
}

func parseHeader(key string, value string, r *http.Request) error {
	if key == ":method" {
		r.Method = value
	} else if key == ":path" {
		r.RequestURI = value
		u, err := url.ParseRequestURI(r.RequestURI)
		if err != nil {
			return fmt.Errorf("invalid request URI: %v", r.RequestURI)
		}
		r.URL = u
	} else if key == ":authority" {
		r.Host = value
	}

	if key[0] == ':' {
		return nil
	}
	values := strings.Split(value, ",")

	for _, v := range values {
		r.Header.Add(key, strings.TrimSpace(v))
	}

	return nil
}

func parseHeaders(frame *Frame, r *http.Request, dec *hpack.Decoder) error {
	var buffer bytes.Buffer
	var paddingLength uint8
	var bytesReadAlready uint8

	r.Header = make(http.Header)

	// bytes.NewBuffer mit frame.Payload
	bodyReader := bufio.NewReader(bytes.NewReader(frame.Payload))

	// Padding flag set
	if frame.Flags&PADDED != 0 {
		_, err := io.CopyN(&buffer, bodyReader, 1)
		if err != nil {
			return fmt.Errorf("cannot read header padding length: %v", err)
		}
		paddingLength = buffer.Next(1)[0]
		bytesReadAlready++
	}

	if uint32(paddingLength) >= frame.Length {
		return fmt.Errorf("invalid header padding length: %v", paddingLength)
	}

	// Priority flag set
	if frame.Flags&HEADERS_PRIORITY != 0 {
		_, err := bodyReader.Discard(5)
		if err != nil {
			return fmt.Errorf("cannot read header priority: %v", err)
		}
		bytesReadAlready += 5
	}

	_, err := io.CopyN(&buffer, bodyReader, int64(frame.Length)-int64(bytesReadAlready))
	if err != nil {
		return fmt.Errorf("cannot read header payload: %v", err)
	}

	pos := 0
	bufferBytes := buffer.Bytes()
	bufferBytes = bufferBytes[:len(bufferBytes)-int(paddingLength)]

	// TODO: decode all header payloads as one
	for {
		headerContent, nPos, err := dec.Decode(bufferBytes[pos:], true)
		if err != nil {
			fmt.Println(fmt.Sprintf("cannot read header content: %v", err))
			break
		}

		if headerContent == nil {
			break
		}

		pos += nPos

		err = parseHeader(headerContent.Name, headerContent.Value, r)
		if err != nil {
			return err
		}
	}

	r.Proto = "HTTP/2.0"
	r.ProtoMajor = 2
	r.ProtoMinor = 0

	return nil
}

func validateHeaderFrames(reader *bufio.Reader, r *http.Request, dec *hpack.Decoder) (bool, uint32, error) {
	var lastStreamID uint32

	firstIteration := true
	endStream := false

	for {
		frame, err := ParseFrame(reader)
		if err != nil {
			return false, 0, err
		}

		if frame.Type == DATA_FRAME_TYPE {
			return false, 0, fmt.Errorf("invalid frame type: %v", frame.Type)
		}

		if firstIteration && frame.Type != HEADER_FRAME_TYPE {
			continue
		} else if !firstIteration && frame.Type != CONTINUATION_FRAME_TYPE {
			return !endStream, lastStreamID, nil
		}

		if frame.StreamID == 0x0 {
			return false, 0, fmt.Errorf("invalid frame stream id: %v", frame.StreamID)
		}

		firstIteration = false
		lastStreamID = frame.StreamID

		// End stream
		if frame.Flags&END_STREAM != 0 && frame.Type == HEADER_FRAME_TYPE {
			err := parseHeaders(frame, r, dec)
			if err != nil {
				return false, 0, err
			}
			endStream = true
			continue
		}

		// End headers
		if frame.Flags&END_HEADERS != 0 && endStream {
			err := parseHeaders(frame, r, dec)
			if err != nil {
				return false, lastStreamID, err
			}
			return false, 0, nil
		} else if frame.Flags&END_HEADERS != 0 && !endStream {
			err := parseHeaders(frame, r, dec)
			if err != nil {
				return false, 0, err
			}
			return true, lastStreamID, nil
		}
	}
}

func getDataFrameContent(frame *Frame) ([]byte, error) {
	var paddingLength uint8
	var bodyContentBuffer bytes.Buffer
	var bytesReadAlready uint8

	bodyReader := bufio.NewReader(bytes.NewReader(frame.Payload))

	if frame.Flags&PADDED != 0 {
		_, err := io.CopyN(&bodyContentBuffer, bodyReader, 1)
		if err != nil {
			return nil, fmt.Errorf("cannot read header padding length: %v", err)
		}
		paddingLength = bodyContentBuffer.Next(1)[0]
		bytesReadAlready++
	}

	_, err := io.CopyN(&bodyContentBuffer, bodyReader, int64(frame.Length)-int64(bytesReadAlready))
	if err != nil {
		return nil, fmt.Errorf("cannot read header payload: %v", err)
	}
	bodyContent := bodyContentBuffer.Bytes()
	bodyContent = bodyContent[:len(bodyContent)-int(paddingLength)]

	return bodyContent, nil
}

// TODO: Check if header but stream not over yet (need to parse footer)
func parseDataFrames(reader *bufio.Reader, r *http.Request) (uint32, error) {
	var dataContentBuffer bytes.Buffer
	var lastStreamID uint32

	for {
		frame, err := ParseFrame(reader)
		if err != nil {
			return 0, fmt.Errorf("cannot read frame data: %v", err)
		}

		if frame.Type != DATA_FRAME_TYPE {
			continue
		}

		if frame.StreamID == 0x0 {
			return 0, fmt.Errorf("invalid frame stream id: %v", frame.StreamID)
		}

		content, err := getDataFrameContent(frame)
		if err != nil {
			return 0, fmt.Errorf("cannot read frame content: %v", err)
		}
		dataContentBuffer.Write(content)
		lastStreamID = frame.StreamID

		// End stream
		if frame.Flags&END_STREAM != 0 {
			break
		}
	}

	dataContent := dataContentBuffer.String()
	r.Body = io.NopCloser(strings.NewReader(dataContent))

	return lastStreamID, nil
}

func ParseFrames(reader *bufio.Reader, r *http.Request, dec *hpack.Decoder) (uint32, error) {
	var lastStreamID uint32
	var err error

	dataToRead, lastStreamID, err := validateHeaderFrames(reader, r, dec)
	if err != nil {
		return 0, fmt.Errorf("cannot validate headers frame: %v", err)
	}

	if dataToRead {
		lastStreamID, err = parseDataFrames(reader, r)
		if err != nil {
			return 0, fmt.Errorf("cannot parse data frames: %v", err)
		}
	}

	return lastStreamID, nil
}

// TODO: Implement??
func ParseSettingsFrameContent(frame *Frame) ([]int, error) {
	return nil, nil
}

func Parser(reader io.Reader, dec *hpack.Decoder) (*http.Request, uint32, error) {
	r := http.Request{}
	// To avoid nil dereference
	r.Body = io.NopCloser(strings.NewReader(""))
	iReader := bufio.NewReader(reader)

	// Settings frame
	lastStreamID, err := ParseFrames(iReader, &r, dec)
	if err != nil {
		return nil, 0, err
	}

	return &r, lastStreamID, nil
}
