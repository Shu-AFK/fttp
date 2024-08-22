package hpack

import "io"

type Decoder struct {
	Table      *IndexAddressSpace
	HeaderList []HeaderField
}

func NewDecoder() *Decoder {
	return &Decoder{
		Table: initIndexAddressSpace(),
	}
}

func DecodeInteger(r *io.Reader) (int64, error) {
	var ret int64

	return ret, nil
}

func (dec *Decoder) Decode(reader *io.Reader) error {

	return nil
}
