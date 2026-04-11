package provider

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// SSEParser reads Server-Sent Events from an io.Reader.
// Zero-allocation on the hot path using bufio.Scanner with a reusable buffer.
type SSEParser struct {
	scanner *bufio.Scanner
}

// SSEEvent represents a single server-sent event.
type SSEEvent struct {
	Event string // event type (if "event:" field present)
	Data  string // event data
}

// NewSSEParser creates a new parser for reading SSE streams.
func NewSSEParser(r io.Reader) *SSEParser {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 512*1024) // 64KB default, 512KB max
	return &SSEParser{scanner: scanner}
}

// Next reads the next SSE event from the stream.
// Returns io.EOF when the stream ends.
func (p *SSEParser) Next() (*SSEEvent, error) {
	event := &SSEEvent{}
	var dataLines []string

	for p.scanner.Scan() {
		line := p.scanner.Text()

		// Empty line = end of event
		if line == "" {
			if len(dataLines) > 0 {
				event.Data = strings.Join(dataLines, "\n")
				return event, nil
			}
			continue
		}

		// SSE comment (starts with :)
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field
		if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimPrefix(data, " ") // optional single space after colon
			dataLines = append(dataLines, data)
		}
		// Ignore id:, retry:, and unknown fields
	}

	if err := p.scanner.Err(); err != nil {
		return nil, err
	}

	// If we have accumulated data but hit EOF, return the event
	if len(dataLines) > 0 {
		event.Data = strings.Join(dataLines, "\n")
		return event, nil
	}

	return nil, io.EOF
}

// ParseSSELine parses a single data: line value.
// This is a fast path for single-line events (most LLM SSE responses).
func ParseSSELine(line []byte) (eventType string, data []byte, ok bool) {
	// Fast path: data: prefix
	if bytes.HasPrefix(line, []byte("data: ")) {
		return "", line[6:], true
	}
	if bytes.HasPrefix(line, []byte("data:")) {
		return "", line[5:], true
	}
	// event: prefix
	if bytes.HasPrefix(line, []byte("event: ")) {
		return string(line[7:]), nil, true
	}
	if bytes.HasPrefix(line, []byte("event:")) {
		return string(line[6:]), nil, true
	}
	return "", nil, false
}
