package http2

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/tatsuhiro-t/go-http2-hpack"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Communication struct {
	Frames chan Frame
	Req    chan *http.Request
	Err    chan error
	Done   chan bool

	Mutex *sync.Mutex
	Dec   *hpack.Decoder
}

type Frame struct {
	Length   uint32
	Type     uint8
	Flags    uint8
	StreamID uint32
	Payload  []byte
}

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

var Channels = make(map[uint32]*Communication)

func ParseFrame(reader *bufio.Reader) (*Frame, error) {
	newFrame := new(Frame)

	var buffer bytes.Buffer
	_, err := io.CopyN(&buffer, reader, 9)
	if err != nil {
		return nil, fmt.Errorf("cannot read frame data: %v", err)
	}

	var length []byte
	length = append(length, 0)
	length = append(length, buffer.Next(3)...)

	newFrame.Length = binary.BigEndian.Uint32(length)
	newFrame.Type = buffer.Next(1)[0]
	newFrame.Flags = buffer.Next(1)[0]
	newFrame.StreamID = binary.BigEndian.Uint32(buffer.Next(4))

	// Clears the first bit (Reserved)
	newFrame.StreamID &^= 1 << 31

	_, err = io.CopyN(&buffer, reader, int64(newFrame.Length))
	if err != nil {
		return nil, fmt.Errorf("cannot read frame data: %v", err)
	}
	newFrame.Payload = buffer.Bytes()

	return newFrame, nil
}

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

	// bytes.NewBuffer mit frame.Payload
	bodyReader := bufio.NewReader(bytes.NewReader(frame.Payload))

	// Padding flag set
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

	// Priority flag set
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
	} else if frame.Flags&END_HEADERS != 0 && frame.Type == CONTINUATION_FRAME_TYPE {
		return false, fmt.Errorf("invalid frame, continuation frame with end headers flag")
	}

	err := parseHeaders(&frame, r, &dec, mutex)
	if err != nil {
		return false, fmt.Errorf("cannot parse headers: %v", err)
	}

	if endStreamSet {
		return false, nil
	}
	return true, nil
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

func checkIfDone() bool {
	done := 0

	for _, value := range Channels {
		select {
		case <-value.Done:
			done = 1
			continue
		default:
			done = 0
			break
		}
	}

	if done == 0 {
		return false
	} else {
		return true
	}
}

func handleMultiplexedFrameParsing(comm *Communication) {
	r := new(http.Request)
	var bodyContent string

	dec := comm.Dec

Loop:
	for frame := range comm.Frames {
		switch frame.Type {
		case CONTINUATION_FRAME_TYPE:
			fallthrough
		case HEADER_FRAME_TYPE:
			moreFrames, err := parseHeaderFrame(frame, r, *dec, comm.Mutex)
			if err != nil {
				comm.Err <- fmt.Errorf("cannot parse header frame: %v", err)
				return
			}
			if !moreFrames {
				break Loop
			}
			break

		case DATA_FRAME_TYPE:
			moreFrames, err := parseDataFrame(frame, &bodyContent)
			if err != nil {
				comm.Err <- fmt.Errorf("cannot parse data frame: %v", err)
				return
			}
			if !moreFrames {
				break Loop
			}
			break
		default:
			continue
		}
	}

	r.Body = io.NopCloser(strings.NewReader(bodyContent))
	comm.Req <- r
	close(comm.Done)
}

func handleStreamMultiplexing(reader *bufio.Reader, dec *hpack.Decoder) ([]*http.Request, error) {
	mutex := new(sync.Mutex)

	for {
		done := checkIfDone()
		if done {
			break
		}

		frame, err := ParseFrame(reader)
		if err != nil {
			return nil, fmt.Errorf("cannot parse frame data: %v", err)
		}

		if frame.StreamID == 0 {
			continue
		}

		newChannel := false

		if _, exists := Channels[frame.StreamID]; !exists {
			comm := new(Communication)
			comm.Frames = make(chan Frame)
			comm.Req = make(chan *http.Request)
			comm.Err = make(chan error)
			comm.Done = make(chan bool)

			comm.Dec = dec
			comm.Mutex = mutex

			Channels[frame.StreamID] = comm
			newChannel = true
		}

		go func() {
			Channels[frame.StreamID].Frames <- *frame
		}()
		if newChannel {
			go handleMultiplexedFrameParsing(Channels[frame.StreamID])
		}
	}
	requests := *new([]*http.Request)

	for _, value := range Channels {
		err := <-value.Err
		if err != nil {
			return nil, err
		}

		req := <-value.Req
		requests = append(requests, req)
	}

	return requests, nil
}

func Parser(reader io.Reader, dec *hpack.Decoder) ([]*http.Request, error) {
	iReader := bufio.NewReader(reader)

	// Settings frame
	r, err := handleStreamMultiplexing(iReader, dec)
	if err != nil {
		return nil, err
	}

	return r, nil
}
