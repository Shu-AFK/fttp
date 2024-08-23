package hpack

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

type Decoder struct {
	Table IndexAddressSpace
}

func NewDecoder() *Decoder {
	return &Decoder{
		Table: *initIndexAddressSpace(),
	}
}

func (dec *Decoder) Decode(reader *bufio.Reader) ([]HeaderField, error) {
	headers := new([]HeaderField)
	_, err := reader.Discard(1)
	if err != nil {
		return nil, err
	}

	for {
		readByte, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return *headers, fmt.Errorf("decoder error: %w", err)
		}

		if readByte&0x80 != 0 {
			index := readByte & 0x7F
			if index == 0 {
				return *headers, fmt.Errorf("static indexes can't be 0")
			}

			*headers = append(*headers, dec.Table[index-1])
		} else {
			return *headers, fmt.Errorf("the decoder only supports static table indexing")
		}
	}

	return *headers, nil
}
