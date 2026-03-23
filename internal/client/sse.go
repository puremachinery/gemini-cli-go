package client

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxSSELineBytes  = 64 * 1024
	maxSSEEventBytes = 1 * 1024 * 1024
)

// sseDecoder parses Server-Sent Events data lines into single events.
type sseDecoder struct {
	reader *bufio.Reader
	buffer []string
}

func newSSEDecoder(r io.Reader) *sseDecoder {
	return &sseDecoder{reader: bufio.NewReaderSize(r, maxSSELineBytes+1)}
}

// Next reads the next SSE event payload.
func (d *sseDecoder) Next() ([]byte, error) {
	eventBytes := 0
	for {
		line, err := d.readLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		eof := errors.Is(err, io.EOF)
		if eof && line == "" {
			if len(d.buffer) > 0 {
				data := strings.Join(d.buffer, "\n")
				d.buffer = nil
				return []byte(data), nil
			}
			return nil, io.EOF
		}
		if line == "" {
			if len(d.buffer) == 0 {
				if eof {
					return nil, io.EOF
				}
				continue
			}
			data := strings.Join(d.buffer, "\n")
			d.buffer = nil
			return []byte(data), nil
		}
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimPrefix(line, "data:")
			payload = strings.TrimPrefix(payload, " ")
			eventBytes += len(payload)
			if eventBytes > maxSSEEventBytes {
				return nil, fmt.Errorf("sse event exceeds %d bytes", maxSSEEventBytes)
			}
			d.buffer = append(d.buffer, payload)
		}
		if eof {
			if len(d.buffer) > 0 {
				data := strings.Join(d.buffer, "\n")
				d.buffer = nil
				return []byte(data), nil
			}
			return nil, io.EOF
		}
	}
}

// readLine reads a single line and enforces the maximum line size.
func (d *sseDecoder) readLine() (string, error) {
	line, err := d.reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return "", fmt.Errorf("sse line exceeds %d bytes", maxSSELineBytes)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if len(line) > maxSSELineBytes {
		return "", fmt.Errorf("sse line exceeds %d bytes", maxSSELineBytes)
	}
	return strings.TrimRight(string(line), "\r\n"), err
}
