package http2

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"httpServer/internal/request/http2"
	"io"
	"net"
)

const (
	SETTINGS_HEADER_TABLE_SIZE = iota + 1
	SETTINGS_ENABLE_PUSH
	SETTINGS_MAX_CONCURRENT_STREAMS
	SETTINGS_INITIAL_WINDOW_SIZE
	SETTINGS_MAX_FRAME_SIZE
	SETTINGS_MAX_HEADER_LIST_SIZE
)

var ConnectionPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
var parseNewFrame = fmt.Errorf("got window_update frame, need settings frame")

func SendSettingsFrame(conn net.Conn, reader *bufio.Reader) error {
	// Construct settings data
	data := make([]byte, 6)
	binary.BigEndian.PutUint16(data[:2], uint16(SETTINGS_HEADER_TABLE_SIZE))
	binary.BigEndian.PutUint32(data[2:], 4096)

	err := SendFrame(conn, http2.SETTINGS_FRAME_TYPE, 0, 0, data)
	if err != nil {
		return fmt.Errorf("error writing settings frame: %w", err)
	}

	err = SendFrame(conn, http2.SETTINGS_FRAME_TYPE, http2.ACK, 0, nil)
	if err != nil {
		return fmt.Errorf("error writing second settings frame: %w", err)
	}

	return nil
}

func validateSettingsFrame(frame *http2.Frame, ackExpected bool) error {
	if frame.Type == http2.WINDOW_UPDATE_FRAME_TYPE {
		return parseNewFrame
	} else if frame.Type != http2.SETTINGS_FRAME_TYPE {
		return fmt.Errorf("invalid frame type, needs to be a settings frame: %v", frame.Type)
	}

	if frame.StreamID != 0x0 {
		return fmt.Errorf("invalid frame stream id: %v", frame.StreamID)
	}

	if len(frame.Payload)%6 != 0 {
		return fmt.Errorf("invalid frame payload length: %v", len(frame.Payload))
	}

	if ackExpected {
		if frame.Flags&http2.ACK == 0 {
			return fmt.Errorf("invalid, ack flag expected: %v", frame.Flags)
		}
	}

	return nil
}

func VerifyConnectionPreface(reader *bufio.Reader) error {
	var preface bytes.Buffer
	_, err := io.CopyN(&preface, reader, 24)
	if err != nil {
		return err
	}
	if preface.String() != ConnectionPreface {
		return fmt.Errorf("invalid connection preface: %v", preface.String())
	}

	frame, err := http2.ParseFrame(reader)
	if err != nil {
		return fmt.Errorf("cannot parse frames: %v", err)
	}

	err = validateSettingsFrame(frame, false)
	if err != nil {
		return fmt.Errorf("cannot validate settings frame: %v", err)
	}

	return nil
}
