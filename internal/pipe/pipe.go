// Package pipe implements pipe mode — stdin/stdout JSON for scripting.
// Usage: echo '{"prompt":"fix the bug"}' | kov --mode pipe
//    or: cat prompts.txt | kov --mode pipe
package pipe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Request is the JSON input format for pipe mode.
type Request struct {
	Prompt  string            `json:"prompt"`
	Mode    string            `json:"mode,omitempty"`
	Model   string            `json:"model,omitempty"`
	Options map[string]string `json:"options,omitempty"`
}

// Response is the JSON output format for pipe mode.
type Response struct {
	Success    bool     `json:"success"`
	Content    string   `json:"content"`
	ToolCalls  int      `json:"tool_calls"`
	Cost       float64  `json:"cost"`
	Model      string   `json:"model"`
	Error      string   `json:"error,omitempty"`
	FilesEdited []string `json:"files_edited,omitempty"`
}

// ReadInput reads a prompt from stdin.
// Supports JSON format or plain text.
func ReadInput() (*Request, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("pipe mode requires stdin input")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}

	input := strings.TrimSpace(string(data))
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}

	// Try JSON first
	var req Request
	if json.Unmarshal([]byte(input), &req) == nil && req.Prompt != "" {
		return &req, nil
	}

	// Fall back to plain text
	return &Request{Prompt: input}, nil
}

// ReadMultipleInputs reads multiple prompts (one per line) from stdin.
func ReadMultipleInputs() ([]Request, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("pipe mode requires stdin input")
	}

	var reqs []Request
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req Request
		if json.Unmarshal([]byte(line), &req) == nil && req.Prompt != "" {
			reqs = append(reqs, req)
		} else {
			reqs = append(reqs, Request{Prompt: line})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return reqs, nil
}

// WriteResponse writes a JSON response to stdout.
func WriteResponse(resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// WriteError writes an error response to stdout.
func WriteError(errMsg string) error {
	return WriteResponse(&Response{
		Success: false,
		Error:   errMsg,
	})
}
