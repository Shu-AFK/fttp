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
	t.Logf("Encoded headers: 0x%s", hex.EncodeToString(encBytes))

	headersAfter, err := dec.Decode(bufio.NewReader(bytes.NewReader(encBytes)))
	assert.NoError(t, err, "Error decoding headers after encoded payload")
	assert.Len(t, headersAfter, len(headersPre))

	for i, header := range headersAfter {
		assert.Equal(t, headersPre[i].Name, header.HeaderFieldName)
		assert.Equal(t, headersPre[i].Value, header.HeaderFieldValue)
	}
}
