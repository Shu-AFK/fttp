package http2

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
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

var frames []Frame

var ConnectionPreface = "PRI * HTTP/2.0\\r\\n\\r\\nSM\\r\\n\\r\\n"

func verifyConnectionPreface(reader *bufio.Reader) error {
	preface, err := reader.ReadBytes(24)
	if err != nil {
		return err
	}
	if string(preface) != ConnectionPreface {
		return fmt.Errorf("invalid connection preface")
	}

	return nil
}

func parseLength(frame *Frame, reader *bufio.Reader) error {
	var buffer bytes.Buffer
	_, err := io.CopyN(&buffer, reader, 3)
	if err != nil {
		return fmt.Errorf("cannot read frame length: %v", err)
	}

	frame.Length = binary.BigEndian.Uint32(buffer.Bytes())
	return nil
}

func parseType(frame *Frame, reader *bufio.Reader) error {
	var buffer bytes.Buffer

	_, err := io.CopyN(&buffer, reader, 1)
	if err != nil {
		return fmt.Errorf("cannot read frame type: %v", err)
	}

	frame.Type = buffer.Bytes()[0]
	return nil
}

func parseFlags(frame *Frame, reader *bufio.Reader) error {
	var buffer bytes.Buffer
	_, err := io.CopyN(&buffer, reader, 1)
	if err != nil {
		return fmt.Errorf("cannot read frame flags: %v", err)
	}

	frame.Flags = buffer.Bytes()[0]
	return nil
}

func parseStreamID(frame *Frame, reader *bufio.Reader) error {
	var buffer bytes.Buffer
	_, err := io.CopyN(&buffer, reader, 4)
	if err != nil {
		return fmt.Errorf("cannot read frame stream id: %v", err)
	}

	frame.StreamID = binary.BigEndian.Uint32(buffer.Bytes())
	return nil
}

// TODO: Implement different types of frame structs
func parseContent(reader *bufio.Reader, frame *Frame) error {
	var buffer bytes.Buffer
	_, err := io.CopyN(&buffer, reader, int64(frame.Length))
	if err != nil {
		return fmt.Errorf("cannot read frame data: %v", err)
	}

	frame.Payload = buffer.Bytes()
	return nil
}

func parseFrame(reader *bufio.Reader) error {
	newFrame := new(Frame)

	err := parseLength(newFrame, reader)
	if err != nil {
		return fmt.Errorf("cannot parse frame length: %v", err)
	}

	err = parseType(newFrame, reader)
	if err != nil {
		return fmt.Errorf("cannot parse frame type: %v", err)
	}

	err = parseFlags(newFrame, reader)
	if err != nil {
		return fmt.Errorf("cannot parse frame flags: %v", err)
	}

	err = parseStreamID(newFrame, reader)
	if err != nil {
		return fmt.Errorf("cannot parse frame stream id: %v", err)
	}

	err = parseContent(reader, newFrame)
	if err != nil {
		return fmt.Errorf("cannot parse content: %v", err)
	}

	frames = append(frames, *newFrame)
	return nil
}

// TODO: Finish functions
func parseFrames(reader *bufio.Reader) error {
	return nil
}

func turnFramesIntoRequest(r *http.Request) error {
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
	err = parseFrames(iReader)
	if err != nil {
		return nil, err
	}

	err = turnFramesIntoRequest(&r)
	if err != nil {
		return nil, err
	}

	return &r, nil
}
