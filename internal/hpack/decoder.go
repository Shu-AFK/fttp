package hpack

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

type Decoder struct {
	Table          IndexAddressSpace
	LastIndexField int
}

func NewDecoder() *Decoder {
	return &Decoder{
		Table:          *initIndexAddressSpace(),
		LastIndexField: -1,
	}
}

func decodeStringLiteral(reader *bufio.Reader) (string, error) {
	length, err := reader.ReadByte()
	if err != nil {
		return "", err
	}

	if length&0x80 == 0x80 {
		return "", errors.New("huffman decoding is not supported")
	}
	length &= 0x7F

	ret := make([]byte, length)
	_, err = reader.Read(ret)
	if err != nil {
		return "", err
	}

	return string(ret), nil
}

func literalHeaderFieldDecoding(reader *bufio.Reader) (*HeaderField, error) {
	name, err := decodeStringLiteral(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode name: %w", err)
	}
	value, err := decodeStringLiteral(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode value: %w", err)
	}

	return NewHeaderField(name, value, false), nil
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

		if readByte&0x80 != 0 { // If in static table
			index := readByte & 0x7F
			if index == 0 {
				return *headers, fmt.Errorf("static indexes can't be 0")
			}

			*headers = append(*headers, dec.Table[index-1])
		} else if readByte&0xC0 == 0x40 { // If literal header field with incremental indexing
			index := readByte & 0x20

			if index == 0 {
				header, err := literalHeaderFieldDecoding(reader)
				if err != nil {
					return *headers, fmt.Errorf("failed to decode header: %w", err)
				}
				*headers = append(*headers, *header)
			}

		} else if readByte&0xF0 == 0 { // If literal header field without indexing
			index := readByte & 0xF

			if index == 0 {
				header, err := literalHeaderFieldDecoding(reader)
				if err != nil {
					return *headers, fmt.Errorf("failed to decode header: %w", err)
				}
				*headers = append(*headers, *header)
			}

		} else {
			return *headers, fmt.Errorf("the decoder only supports static table indexing")
		}
	}

	return *headers, nil
}
