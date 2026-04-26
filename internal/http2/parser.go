package http2

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/tatsuhiro-t/go-http2-hpack"
)

func parseHeader(key string, value string, r *http.Request) error {
	if key == ":method" {
		r.Method = value
	} else if key == ":path" {
		r.RequestURI = value
		u, err := url.ParseRequestURI(r.RequestURI)
		if err != nil {
			return fmt.Errorf("invalid request URI: %v", r.RequestURI)
		}
		r.URL = u
	} else if key == ":authority" {
		r.Host = value
	}

	if key[0] == ':' {
		return nil
	}
	values := strings.Split(value, ",")

	for _, v := range values {
		r.Header.Add(key, strings.TrimSpace(v))
	}

	return nil
}

func parseHeaders(frame *Frame, r *http.Request, dec *hpack.Decoder, mutex *sync.Mutex) error {
	var buffer bytes.Buffer
	var paddingLength uint8
	var bytesReadAlready uint8

	r.Header = make(http.Header)

	bodyReader := bufio.NewReader(bytes.NewReader(frame.Payload))

	if frame.Flags&PADDED != 0 {
		_, err := io.CopyN(&buffer, bodyReader, 1)
		if err != nil {
			return fmt.Errorf("cannot read header padding length: %v", err)
		}
		paddingLength = buffer.Next(1)[0]
		bytesReadAlready++
	}

	if uint32(paddingLength) >= frame.Length {
		return fmt.Errorf("invalid header padding length: %v", paddingLength)
	}

	if frame.Flags&HEADERS_PRIORITY != 0 {
		_, err := bodyReader.Discard(5)
		if err != nil {
			return fmt.Errorf("cannot read header priority: %v", err)
		}
		bytesReadAlready += 5
	}

	_, err := io.CopyN(&buffer, bodyReader, int64(frame.Length)-int64(bytesReadAlready))
	if err != nil {
		return fmt.Errorf("cannot read header payload: %v", err)
	}

	pos := 0
	bufferBytes := buffer.Bytes()
	bufferBytes = bufferBytes[:len(bufferBytes)-int(paddingLength)]

	// TODO: decode all header payloads as one
	for {
		mutex.Lock()
		headerContent, nPos, err := dec.Decode(bufferBytes[pos:], true)
		mutex.Unlock()
		if err != nil {
			fmt.Println(fmt.Sprintf("cannot read header content: %v", err))
			break
		}

		if headerContent == nil {
			break
		}

		pos += nPos

		err = parseHeader(headerContent.Name, headerContent.Value, r)
		if err != nil {
			return err
		}
	}

	r.Proto = "HTTP/2.0"
	r.ProtoMajor = 2
	r.ProtoMinor = 0

	return nil
}

func getDataFrameContent(frame *Frame) ([]byte, error) {
	var paddingLength uint8
	var bodyContentBuffer bytes.Buffer
	var bytesReadAlready uint8

	bodyReader := bufio.NewReader(bytes.NewReader(frame.Payload))

	if frame.Flags&PADDED != 0 {
		_, err := io.CopyN(&bodyContentBuffer, bodyReader, 1)
		if err != nil {
			return nil, fmt.Errorf("cannot read header padding length: %v", err)
		}
		paddingLength = bodyContentBuffer.Next(1)[0]
		bytesReadAlready++
	}

	_, err := io.CopyN(&bodyContentBuffer, bodyReader, int64(frame.Length)-int64(bytesReadAlready))
	if err != nil {
		return nil, fmt.Errorf("cannot read header payload: %v", err)
	}
	bodyContent := bodyContentBuffer.Bytes()
	bodyContent = bodyContent[:len(bodyContent)-int(paddingLength)]

	return bodyContent, nil
}

func parseHeaderFrame(frame Frame, r *http.Request, dec hpack.Decoder, mutex *sync.Mutex) (bool, error) {
	var endStreamSet bool

	if frame.Flags&END_STREAM != 0 && frame.Type == HEADER_FRAME_TYPE {
		endStreamSet = true
	} else if frame.Flags&END_STREAM != 0 && frame.Type == CONTINUATION_FRAME_TYPE {
		return false, fmt.Errorf("invalid frame, continuation frame with end stream flag")
	}

	err := parseHeaders(&frame, r, &dec, mutex)
	if err != nil {
		return false, fmt.Errorf("cannot parse headers: %v", err)
	}

	return !endStreamSet, nil
}

func parseDataFrame(frame Frame, bodyContent *string) (bool, error) {
	content, err := getDataFrameContent(&frame)
	if err != nil {
		return false, fmt.Errorf("cannot read frame content: %v", err)
	}

	*bodyContent += string(content)

	if frame.Flags&END_STREAM != 0 {
		return false, nil
	}

	return true, nil
}

func HandleMultiplexedFrameParsing(comm *Communication, router chi.Router, conn *tls.Conn, respEssential ResponseEssential) {
	r := new(http.Request)
	var bodyContent string
	var streamID uint32

	dec := comm.Dec

	defer close(comm.Done)

Loop:
	for frame := range comm.Frames {
		if streamID == 0 {
			streamID = frame.StreamID
		}
		switch frame.Type {
		case CONTINUATION_FRAME_TYPE:
			fallthrough
		case HEADER_FRAME_TYPE:
			moreFrames, err := parseHeaderFrame(frame, r, *dec, comm.Mutex)
			if err != nil {
				fmt.Println("cannot parse header frame")
				return
			}
			if !moreFrames {
				break Loop
			}
		case DATA_FRAME_TYPE:
			moreFrames, err := parseDataFrame(frame, &bodyContent)
			if err != nil {
				fmt.Printf("cannot parse frame content: %v", err)
				return
			}
			if !moreFrames {
				break Loop
			}
		default:
			continue
		}
	}

	r.Body = io.NopCloser(strings.NewReader(bodyContent))
	r.RemoteAddr = conn.RemoteAddr().String()
	r.TLS = &tls.ConnectionState{} // signal HTTPS to downstream handlers
	if r.URL == nil {
		r.URL = &url.URL{}
	}
	responseWriter := NewResponse(conn, streamID, respEssential)
	router.ServeHTTP(responseWriter, r)
	responseWriter.EndStream()
}
