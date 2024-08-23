package hpack

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

type Decoder struct {
	Table      IndexAddressSpace
	HeaderList []HeaderField
}

func NewDecoder() *Decoder {
	return &Decoder{
		Table: *initIndexAddressSpace(),
	}
}

func (dec *Decoder) Decode(reader *bufio.Reader) ([]HeaderField, error) {
	headers := new([]HeaderField)

	for {
		readByte, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return *headers, fmt.Errorf("decoder error: %w", err)
		}

		if readByte&0x80 == 1 {
			index := readByte & 0x7F
			*headers = append(*headers, dec.Table[index])
		} else {
			return *headers, fmt.Errorf("the decoder only supports static table indexing")
		}
	}

	return *headers, nil
}
