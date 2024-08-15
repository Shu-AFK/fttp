package httpRequest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

/****** HTTP1.1 ******/
func parseStartLine(scanner *bufio.Scanner, req *http.Request) error {
	var startLine string
	if scanner.Scan() {
		startLine = scanner.Text()
	} else {
		return fmt.Errorf("can't scan start-line: %v", scanner.Err())
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

	version := splitStartLine[2]

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

func parseHeader(scanner *bufio.Scanner, req *http.Request) error {
	header := make(http.Header)

	for {
		if !scanner.Scan() {
			return fmt.Errorf("can't scan header: %v", scanner.Err())
		}

		if scanner.Text() == "" {
			break
		}

		// Get key value pairs
		key, value, found := strings.Cut(scanner.Text(), ":")
		if !found {
			return fmt.Errorf("header seperator not found: %v", scanner.Text())
		}

		// Split up after each comma for header insertion
		values := strings.Split(value, ",")

		for _, v := range values {
			header.Add(key, strings.TrimSpace(v))
		}
	}

	req.Header = header
	return nil
}

func parseBody(scanner *bufio.Scanner, req *http.Request) error {
	req.Host = req.Header.Get("Host")
	req.Header.Del("Host")

	req.TransferEncoding = req.Header.Values("Transfer-Encoding")

	// Checking for empty body
	contentLengthString := req.Header.Get("Content-Length")
	if contentLengthString == "" {
		return nil
	}

	contentLength, err := strconv.ParseInt(contentLengthString, 10, 64)
	if err != nil {
		return fmt.Errorf("error parsing Content-Length: %v", err)
	}
	req.ContentLength = contentLength

	var bodyContentBuffer bytes.Buffer
	contentWrote := 0
	for contentWrote < int(contentLength) {
		scanner.Scan()
		wrote, err := bodyContentBuffer.WriteString(scanner.Text())
		if err != nil {
			return fmt.Errorf("error parsing body content: %v", err)
		}
		contentWrote += wrote
	}

	req.Body = io.NopCloser(io.LimitReader(strings.NewReader(bodyContentBuffer.String()), contentLength))

	return nil
}

/****** end of HTTP1.1 ******/

func Parser(reader io.Reader) (*http.Request, error) {
	r := http.Request{}
	parsingBody := false
	scanner := bufio.NewScanner(reader)

	// The split function gets change in between parseHeader and parseBody because when parsing the Body, the whole
	// content should get returned by the scanner instead of each line separately
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if !parsingBody {
			return bufio.ScanLines(data, atEOF)
		}
		return len(data), data, nil
	})

	/****** HTTP1.1 ******/
	err := parseStartLine(scanner, &r)
	if err != nil {
		return nil, err
	}

	err = parseHeader(scanner, &r)
	if err != nil {
		return nil, err
	}

	parsingBody = true
	err = parseBody(scanner, &r)
	if err != nil {
		return nil, err
	}
	/****** end of HTTP1.1 ******/

	return &r, nil
}
