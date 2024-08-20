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

var ConnectionPreface = "PRI * HTTP/2.0\\r\\n\\r\\nSM\\r\\n\\r\\n"

func verifyConnectionPreface(reader *bufio.Reader) error {
	preface, err := reader.ReadBytes(24)
	if err != nil {
		return err
	}
	if string(preface) != ConnectionPreface {
		return fmt.Errorf("invalid connection preface")
	}

	frame, err := parseFrame(reader)
	if err != nil {
		return fmt.Errorf("cannot parse frames: %v", err)
	}

	err = validateSettingsFrame(frame)
	if err != nil {
		return fmt.Errorf("cannot validate settings frame: %v", err)
	}

	return nil
}

func parseFrame(reader *bufio.Reader) (*Frame, error) {
	newFrame := new(Frame)

	var buffer bytes.Buffer
	_, err := io.CopyN(&buffer, reader, 10)
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

func validateSettingsFrame(frame *Frame) error {
	if frame.Type != SETTINGS_FRAME_TYPE {
		return fmt.Errorf("invalid first frame type, needs to be a settings frame: %v", frame.Type)
	}
	if frame.StreamID != 0x0 {
		return fmt.Errorf("invalid first frame stream id: %v", frame.StreamID)
	}
	if len(frame.Payload)%6 != 0 {
		return fmt.Errorf("invalid frame payload length: %v", len(frame.Payload))
	}

	return nil
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
	}

	values := strings.Split(value, ",")

	for _, v := range values {
		r.Header.Add(key, strings.TrimSpace(v))
	}

	return nil
}

func parseHeaders(frame *Frame, r *http.Request) error {
	var buffer bytes.Buffer
	var paddingLength uint8
	var bytesReadAlready uint8
	dec := hpack.NewDecoder()

	bodyReader := bufio.NewReader(bytes.NewReader(frame.Payload))

	// Padding flag set
	if frame.Flags&0x08 != 0 {
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
	if frame.Flags&0x20 != 0 {
		_, err := io.CopyN(&buffer, bodyReader, 5)
		if err != nil {
			return fmt.Errorf("cannot read header priority information: %v", err)
		}
		buffer.Next(5)
		bytesReadAlready += 5
	}

	_, err := io.CopyN(&buffer, bodyReader, int64(frame.Length)-int64(bytesReadAlready))
	if err != nil {
		return fmt.Errorf("cannot read header payload: %v", err)
	}

	pos := 0
	bufferBytes := buffer.Bytes()
	// correct?
	bufferBytes = bufferBytes[:len(bufferBytes)-int(paddingLength)]

	for {
		headerContent, nPos, err := dec.Decode(bufferBytes[:pos], true)
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

func validateHeaderFrames(reader *bufio.Reader, r *http.Request) (bool, error) {
	firstIteration := true

	for {
		frame, err := parseFrame(reader)
		if err != nil {
			return false, err
		}

		if firstIteration && frame.Type != HEADER_FRAME_TYPE {
			return false, fmt.Errorf("invalid first header frame type, needs to be a header frame: %v", frame.Type)
		} else if !firstIteration && frame.Type != HEADER_FRAME_TYPE && frame.Type != CONTINUATION_FRAME_TYPE {
			return false, nil
		}

		if frame.StreamID == 0x0 {
			return false, fmt.Errorf("invalid frame stream id: %v", frame.StreamID)
		}

		firstIteration = false

		// End stream
		if frame.Flags&0x01 != 0 && frame.Type == HEADER_FRAME_TYPE {
			err := parseHeaders(frame, r)
			if err != nil {
				return false, err
			}
			return false, nil
		}
		// End headers
		if frame.Flags&0x04 != 0 {
			err := parseHeaders(frame, r)
			if err != nil {
				return false, err
			}
			return true, nil
		}
	}
}

func getDataFrameContent(frame *Frame) ([]byte, error) {
	var paddingLength uint8
	var bodyContentBuffer bytes.Buffer
	var bytesReadAlready uint8

	bodyReader := bufio.NewReader(bytes.NewReader(frame.Payload))

	if frame.Flags&0x08 != 0 {
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

func parseDataFrames(reader *bufio.Reader, r *http.Request) error {
	var dataContentBuffer bytes.Buffer

	for {
		frame, err := parseFrame(reader)
		if err != nil {
			return fmt.Errorf("cannot read frame data: %v", err)
		}

		if frame.Type != DATA_FRAME_TYPE {
			return fmt.Errorf("invalid frame type, needs to be a data frame: %v", frame.Type)
		}

		if frame.StreamID == 0x0 {
			return fmt.Errorf("invalid frame stream id: %v", frame.StreamID)
		}

		content, err := getDataFrameContent(frame)
		if err != nil {
			return fmt.Errorf("cannot read frame content: %v", err)
		}
		dataContentBuffer.Write(content)

		// End stream
		if frame.Flags&0x01 != 0 {
			break
		}
	}

	dataContent := dataContentBuffer.String()
	r.Body = io.NopCloser(io.LimitReader(strings.NewReader(dataContent), int64(len(dataContent))))

	return nil
}

func parseFrames(reader *bufio.Reader, r *http.Request) error {
	dataToRead, err := validateHeaderFrames(reader, r)
	if err != nil {
		return fmt.Errorf("cannot validate headers frame: %v", err)
	}

	if dataToRead {
		err = parseDataFrames(reader, r)
		if err != nil {
			return fmt.Errorf("cannot parse data frames: %v", err)
		}
	}

	return nil
}

func Parser(reader io.Reader) (*http.Request, error) {
	r := http.Request{}
	iReader := bufio.NewReader(reader)

	err := verifyConnectionPreface(iReader)
	if err != nil {
		return nil, err
	}

	// Settings frame
	err = parseFrames(iReader, &r)
	if err != nil {
		return nil, err
	}

	return &r, nil
}
