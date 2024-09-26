package http1_1

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var ChunkEncodingError = errors.New("chunked encoding was not at the end of the transfer encodings")

func parseStartLine(reader *bufio.Reader, req *http.Request) error {
	startLine, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("can't scan start-line: %v", err)
	}

	splitStartLine := strings.Split(startLine, " ")
	if len(splitStartLine) != 3 {
		return fmt.Errorf("format error: %v", startLine)
	}

	req.Method = splitStartLine[0]

	uri := splitStartLine[1]
	req.RequestURI = uri

	u, err := url.ParseRequestURI(uri)
	if err != nil {
		return fmt.Errorf("error parsing start-line uri: %v", err)
	}
	req.URL = u

	version := strings.Trim(splitStartLine[2], "\r\n")

	if version[:5] != "HTTP/" {
		return fmt.Errorf("proto format error: %v", version)
	}
	if len(version) != len("HTTP/x.x") {
		return fmt.Errorf("proto format error: %v", version)
	}

	req.Proto = version
	protoMajor, err := strconv.Atoi(string(version[len(version)-3]))
	if err != nil {
		return fmt.Errorf("proto format error: %v", err)
	}
	protoMinor, err := strconv.Atoi(string(version[len(version)-1]))
	if err != nil {
		return fmt.Errorf("proto format error: %v", err)
	}
	if string(version[len(version)-2]) != "." {
		return fmt.Errorf("proto format error: %v", version)
	}

	req.ProtoMinor = protoMinor
	req.ProtoMajor = protoMajor

	return nil
}

func readHeader(reader *bufio.Reader) (http.Header, error) {
	header := make(http.Header)

	for {
		row, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("can't scan header: %v", err)
		}
		row = strings.Trim(row, "\r\n")

		if row == "" {
			break
		}

		// Get key value pairs
		key, value, found := strings.Cut(row, ":")
		if !found {
			fmt.Println(header)
			return nil, fmt.Errorf("header seperator not found: %q", row)
		}

		// Split up after each comma for header insertion
		values := strings.Split(value, ",")

		for _, v := range values {
			header.Add(key, strings.TrimSpace(v))
		}
	}

	return header, nil
}

func parseHeader(reader *bufio.Reader, req *http.Request) error {
	header, err := readHeader(reader)
	if err != nil {
		fmt.Println(req)
		return err
	}

	req.Header = header
	return nil
}

func parseChunkedData(reader *bufio.Reader, req *http.Request) (string, error) {
	var bodyContentBuffer string

	// Respect \n
	for {
		chunkSizeStr, err := reader.ReadString('\n')
		if err != nil {
			return bodyContentBuffer, fmt.Errorf("can't read chunk size: %v", err)
		}

		chunkSizeStr = strings.Trim(chunkSizeStr, "\r\n")

		// Disregard any chunk extensions
		chunkSizeCut, _, found := strings.Cut(chunkSizeStr, ";")
		if found {
			chunkSizeStr = chunkSizeCut
		}

		chunkSize, err := strconv.ParseInt(chunkSizeStr, 16, 64)
		if err != nil {
			return bodyContentBuffer, fmt.Errorf("can't read chunk size: %v", err)
		}

		if chunkSize == 0 {
			break
		}

		buffer, err := io.ReadAll(io.LimitReader(reader, chunkSize+2))
		if err != nil {
			return bodyContentBuffer, fmt.Errorf("can't read chunk content: %v", err)
		}
		buffer = bytes.Trim(buffer, "\r\n")

		bodyContentBuffer += string(buffer)
	}

	// checking if a trailer section is present
	trailer, err := readHeader(reader)
	if err != nil {
		return bodyContentBuffer, err
	}
	req.Trailer = trailer
	return bodyContentBuffer, nil
}

func parseBody(reader *bufio.Reader, req *http.Request) error {
	req.Host = req.Header.Get("Host")
	req.Header.Del("Host")

	req.TransferEncoding = req.Header.Values("Transfer-Encoding")

	contentLengthString := req.Header.Get("Content-Length")
	contentLength, err := strconv.ParseInt(contentLengthString, 10, 64)

	// Checking if chunked transfer-encoded
	// Chunked transfer encoding overwrites the content-length header
	chunked := false
	for _, encoding := range req.TransferEncoding {
		if strings.ToLower(encoding) == "chunked" {
			chunked = true
		}
	}

	if chunked && req.TransferEncoding[len(req.TransferEncoding)-1] != "chunked" {
		return ChunkEncodingError
	}

	if chunked {
		if contentLength != 0 {
			delete(req.Header, "Content-Length")
			req.ContentLength = 0
		}

		bodyContentBuffer, err := parseChunkedData(reader, req)
		if err != nil {
			return err
		}
		req.Body = io.NopCloser(strings.NewReader(bodyContentBuffer))
	} else if contentLengthString != "" {
		if err != nil {
			return fmt.Errorf("error parsing Content-Length: %v", err)
		}
		req.ContentLength = contentLength

		bodyContentBuffer, err := io.ReadAll(io.LimitReader(reader, contentLength))
		if err != nil {
			return fmt.Errorf("error reading chunked body content: %v", err)
		}

		req.Body = io.NopCloser(strings.NewReader(string(bodyContentBuffer)))
	}

	return nil
}

func Parser(reader *bufio.Reader) (*http.Request, error, bool) {
	r := http.Request{}

	err := parseStartLine(reader, &r)
	if err != nil {
		return nil, err, false
	}

	err = parseHeader(reader, &r)
	if err != nil {
		return nil, err, false
	}

	err = parseBody(reader, &r)
	if err != nil {
		return nil, err, false
	}

	if r.Header.Get("Connection") == "keep-alive" || (r.Header.Get("Connection") == "" && r.ProtoMinor == 1) {
		return &r, nil, true
	}
	return &r, nil, false
}
