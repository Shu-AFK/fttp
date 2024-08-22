package structs

import (
	"crypto/tls"
	"github.com/go-chi/chi/v5"
	hpack "github.com/tatsuhiro-t/go-http2-hpack"
	"sync"
)

//goland:noinspection ALL
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

const (
	PADDED           = 0x08
	END_STREAM       = 0x01
	END_HEADERS      = 0x04
	HEADERS_PRIORITY = 0x20
	ACK              = 0x01
)

type ParsingEssential struct {
	Mutex    *sync.Mutex
	Dec      *hpack.Decoder
	Channels map[uint32]*Communication
	Router   chi.Router
	Conn     *tls.Conn
}

type Frame struct {
	Length   uint32
	Type     uint8
	Flags    uint8
	StreamID uint32
	Payload  []byte
}

type Communication struct {
	Frames chan Frame

	Mutex *sync.Mutex
	Dec   *hpack.Decoder
}

func NewCommunication(dec *hpack.Decoder, mut *sync.Mutex) *Communication {
	return &Communication{
		Frames: make(chan Frame),
		Mutex:  mut,
		Dec:    dec,
	}
}

func NewParsingEssential(dec *hpack.Decoder, mut *sync.Mutex, r chi.Router, conn *tls.Conn) *ParsingEssential {
	return &ParsingEssential{
		Dec:      dec,
		Channels: make(map[uint32]*Communication),
		Mutex:    mut,
		Router:   r,
		Conn:     conn,
	}
}
