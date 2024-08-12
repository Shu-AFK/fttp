package httpParser

import (
	"bytes"
	"httpServer/internal/helper"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

/****** HTTP1.1 ******/
func parseStartLine(reader *bytes.Reader, req *http.Request) error {
	// Read Method
	method, err := helper.ReadUntil(reader, ' ')
	if err != nil {
		return err
	}
	req.Method = string(method)

	// Read URI
	uri, err := helper.ReadUntil(reader, ' ')
	if err != nil {
		return err
	}
	u, err := url.Parse(string(uri))
	if err != nil {
		return err
	}
	req.URL = u

	// Read PROTO version
	version, err := helper.ReadUntil(reader, ' ')
	if err != nil {
		return err
	}
	req.Proto = string(version)
	req.ProtoMinor = int(version[len(version)-1])
	req.ProtoMajor = int(version[len(version)-3])

	return nil
}

func parseHeader(reader *bytes.Reader, req *http.Request) error {
	var header = new(http.Header)
	firstIteration := true
	recurringNewLines := 0

	for {
		err := helper.CheckForNewLines(reader)
		if err != nil {
			return err
		}
		recurringNewLines++
		if recurringNewLines >= 2 {
			break
		}

		// To check for an empty header
		if firstIteration {
			err = helper.PeekForNewLines(reader)
			if err != nil {
				return err
			}
			firstIteration = false
		}

		recurringNewLines = 0

		// Get key value pairs
		key, err := helper.ReadUntil(reader, ':')
		if err != nil {
			return err
		}
		value, err := helper.ReadUntil(reader, '\n')
		if err != nil {
			return err
		}
		// Removes the \r
		value = value[:len(value)-1]

		// Split up after each comma for header insertion
		// TODO: Not correct, need to finish
		var values []string
		prev := 0
		for i := 0; i < len(string(value)); i++ {
			if value[i] == ',' {
				values = append(values, string(value[prev:i-1-prev]))
				prev = i + 1
			}
		}
		values = append(values, string(value[prev:len(value)-1-prev]))

		header.Add(string(key), string(value))

		recurringNewLines++
	}

	req.Header = *header
	return nil
}

func parseBody(reader *bytes.Reader, req *http.Request) error {
	req.Body = io.NopCloser(reader)

	// Unused
	req.GetBody = func() (io.ReadCloser, error) { return nil, nil }

	contentLength, err := strconv.ParseInt(req.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return err
	}
	req.ContentLength = contentLength

	// TODO: finish rest of header items

	req.Host = req.Header.Get("Host")
	if req.Host != "" {
		delete(req.Header, "Host")
	}

	return nil
}

/****** end of HTTP1.1 ******/

func HttpRequestParser(input []byte) (*http.Request, error) {
	r := http.Request{}
	reader := bytes.NewReader(input)

	/****** HTTP1.1 ******/
	err := parseStartLine(reader, &r)
	if err != nil {
		return nil, err
	}

	err = parseHeader(reader, &r)
	if err != nil {
		return nil, err
	}

	err = parseBody(reader, &r)
	if err != nil {
		return nil, err
	}
	/****** end of HTTP1.1 ******/

	return &r, nil
}
