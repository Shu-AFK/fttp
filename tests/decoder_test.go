package tests

import (
	"bufio"
	"bytes"
	tested_hpack "github.com/tatsuhiro-t/go-http2-hpack"
	"httpServer/internal/hpack"
	"testing"
)

func TestDecoderStatic(t *testing.T) {
	t.Log("Testing Decoder Static...")

	headersPre := []*tested_hpack.Header{
		tested_hpack.NewHeader(":method", "GET", false),
		tested_hpack.NewHeader(":scheme", "https", false),
		tested_hpack.NewHeader(":authority", "example.org", false),
	}

	dec := hpack.NewDecoder()
	enc := tested_hpack.NewEncoder(0)

	encoded := &bytes.Buffer{}
	enc.Encode(encoded, headersPre)

	headersAfter, _ := dec.Decode(bufio.NewReader(bytes.NewReader(encoded.Bytes())))
	for i, header := range headersAfter {
		if header.HeaderFieldName != headersPre[i].Name {
			t.Errorf("got header field name %s instead of %s", header.HeaderFieldName, headersPre[i].Name)
		}
		if header.HeaderFieldValue != headersPre[i].Value {
			t.Errorf("got header field value %s instead of %s", header.HeaderFieldValue, headersPre[i].Value)
		}
	}
}
