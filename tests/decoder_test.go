package tests

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	tested_hpack "github.com/tatsuhiro-t/go-http2-hpack"
	"httpServer/internal/hpack"
	"testing"
)

func TestDecoderStatic(t *testing.T) {
	headersPre := []*tested_hpack.Header{
		tested_hpack.NewHeader(":method", "GET", false),
		tested_hpack.NewHeader(":scheme", "https", false),
		tested_hpack.NewHeader(":path", "/", false),
	}

	dec := hpack.NewDecoder()
	enc := tested_hpack.NewEncoder(0)

	encoded := &bytes.Buffer{}
	enc.Encode(encoded, headersPre)

	encBytes := encoded.Bytes()
	t.Logf("Encoded headers as hex   : 0x%s", hex.EncodeToString(encBytes))
	t.Logf("Encoded headers as string: %s", encBytes)

	headersAfter, err := dec.Decode(bufio.NewReader(bytes.NewReader(encBytes)))
	assert.NoError(t, err, "Error decoding headers after encoded payload")
	assert.Len(t, headersAfter, len(headersPre))

	for i, header := range headersAfter {
		assert.Equal(t, headersPre[i].Name, header.HeaderFieldName)
		assert.Equal(t, headersPre[i].Value, header.HeaderFieldValue)
	}
}

func TestDecoderHeaderLiterals(t *testing.T) {
	encoded, err := hex.DecodeString("040c2f73616d706c652f70617468")
	if err != nil {
		t.Fatalf("Error decoding headers after encoded payload: %s", err)
	}

	headersPref := []*hpack.HeaderField{
		{":path", "/sample/path", false},
	}

	dec := hpack.NewDecoder()

	headersAfter, err := dec.Decode(bufio.NewReader(bytes.NewReader(encoded)))
	assert.NoError(t, err, "Error decoding headers after encoded payload")
	assert.Len(t, headersAfter, 1)

	assert.Equal(t, headersPref[0], headersAfter[0])
}
