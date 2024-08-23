package frame

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"httpServer/internal/http2/structs"
	"io"
)

func ParseFrame(reader *bufio.Reader) (*structs.Frame, error) {
	newFrame := new(structs.Frame)

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

func NewFrame(iType uint8, flags uint8, streamID uint32, data []byte) *structs.Frame {
	return &structs.Frame{
		Type:     iType,
		Flags:    flags,
		StreamID: streamID &^ 1 << 31,
		Payload:  data,
	}
}
