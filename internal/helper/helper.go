package helper

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
)

func ReadUntil(r *bytes.Reader, c byte) ([]byte, error) {
	var rBytes []byte
	rByte, err := r.ReadByte()

	for err != io.EOF && rByte != c {
		if err != nil {
			return rBytes, err
		}

		rBytes = append(rBytes, rByte)
		rByte, err = r.ReadByte()
	}

	return rBytes, nil
}

func CheckForNewLines(r *bytes.Reader) error {
	newLine, err := r.ReadByte()
	if err != nil {
		return err
	}
	if newLine != '\r' {
		return errors.New("httpParser: expected '\\r', got " + strconv.Itoa(int(newLine)) + " instead")
	}
	newLine, err = r.ReadByte()
	if err != nil {
		return err
	}
	if newLine != '\n' {
		return errors.New("httpParser: expected '\\n', got " + strconv.Itoa(int(newLine)) + " instead")
	}

	return nil
}

func PeekForNewLines(r *bytes.Reader) error {
	reader := bufio.NewReader(r)

	newLine, err := reader.Peek(2)
	if err != nil {
		return err
	}
	if newLine[0] != '\r' {
		return errors.New("httpHeaderParser: expected '\\r', got " + strconv.Itoa(int(newLine[0])) + " instead")
	}
	if newLine[1] != '\n' {
		return errors.New("httpHeaderParser: expected '\\n', got " + strconv.Itoa(int(newLine[0])) + " instead")
	}

	return nil
}
