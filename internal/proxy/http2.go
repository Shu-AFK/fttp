package proxy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/go-chi/chi/v5"
	hpack "github.com/tatsuhiro-t/go-http2-hpack"

	"httpServer/internal/http2"
	"httpServer/internal/logging"
)

func (p *Proxy) handleHTTP2(tlsConn *tls.Conn, r chi.Router) {
	requestReader := bufio.NewReader(tlsConn)
	dec := hpack.NewDecoder()

	if err := http2.VerifyConnectionPreface(requestReader); err != nil {
		p.Log(logging.LogLevelError, "Failed to verify connection preface for %v: %v", tlsConn.RemoteAddr(), err)
		return
	}

	if err := http2.SendSettingsFrame(tlsConn); err != nil {
		p.Log(logging.LogLevelError, "Failed to send settings frame for %v: %v", tlsConn.RemoteAddr(), err)
		return
	}

	p.Log(logging.LogLevelDebug, "Established HTTP/2 connection with %v", tlsConn.RemoteAddr())

	respEssential := http2.NewResponseEssential(tlsConn, hpack.NewEncoder(4096))
	go http2.SendFrames(*respEssential)

	p.http2IntermediateHandler(requestReader, http2.NewParsingEssential(dec, new(sync.Mutex), r, tlsConn), *respEssential)
}

func (p *Proxy) http2IntermediateHandler(reader io.Reader, essential *http2.ParsingEssential, respEssential http2.ResponseEssential) {
	p.Log(logging.LogLevelInfo, "Starting HTTP/2 handling")

	iReader := bufio.NewReader(reader)
	if err := p.handleStreamMultiplexing(iReader, essential, respEssential); err != nil {
		p.Log(logging.LogLevelError, "Error in stream multiplexing: %v", err)
		return
	}

	p.Log(logging.LogLevelInfo, "Completed HTTP/2 handling")
}

func (p *Proxy) handleStreamMultiplexing(reader *bufio.Reader, essential *http2.ParsingEssential, respEssential http2.ResponseEssential) error {
	p.Log(logging.LogLevelInfo, "Starting stream multiplexing")

	for {
		f, err := http2.ParseFrame(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			p.Log(logging.LogLevelError, "Cannot parse frame data: %v", err)
			return fmt.Errorf("cannot parse frame data: %v", err)
		}

		if f.StreamID == 0 {
			p.Log(logging.LogLevelDebug, "Skipping frame with StreamID 0")
			continue
		}

		comm, exists := essential.Channels[f.StreamID]
		if !exists {
			p.Log(logging.LogLevelDebug, "Creating new channel for StreamID: %d", f.StreamID)
			comm = http2.NewCommunication(essential.Dec, essential.Mutex)
			essential.Channels[f.StreamID] = comm

			p.Log(logging.LogLevelDebug, "Launching handler for new channel StreamID: %d", f.StreamID)
			go http2.HandleMultiplexedFrameParsing(comm, essential.Router, essential.Conn, respEssential)
		}

		select {
		case <-comm.Done:
			p.Log(logging.LogLevelDebug, "Stream %d already finished, dropping frame", f.StreamID)
			continue
		default:
		}

		p.Log(logging.LogLevelDebug, "Handling frame for StreamID: %d", f.StreamID)
		select {
		case comm.Frames <- *f:
		case <-comm.Done:
			p.Log(logging.LogLevelDebug, "Stream %d finished while sending frame, dropping", f.StreamID)
		}
	}
}
