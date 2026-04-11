package mock

import (
	"net/http"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer()
	defer s.Close()

	if s.URL() == "" {
		t.Error("expected non-empty URL")
	}
}

func TestServer_DefaultResponse(t *testing.T) {
	s := NewServer()
	defer s.Close()

	resp, err := http.Get(s.URL() + "/v1/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServer_429RateLimit(t *testing.T) {
	s := NewServer()
	defer s.Close()
	s.SetError(Error429RateLimit)

	resp, err := http.Get(s.URL() + "/v1/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 429 {
		t.Errorf("expected 429, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestServer_401Auth(t *testing.T) {
	s := NewServer()
	defer s.Close()
	s.SetError(Error401Auth)

	resp, err := http.Get(s.URL() + "/v1/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestServer_500Server(t *testing.T) {
	s := NewServer()
	defer s.Close()
	s.SetError(Error500Server)

	resp, err := http.Get(s.URL() + "/v1/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestServer_RecordsCalls(t *testing.T) {
	s := NewServer()
	defer s.Close()

	http.Get(s.URL() + "/v1/messages")
	http.Get(s.URL() + "/v1/chat/completions")

	calls := s.Calls()
	if len(calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Path != "/v1/messages" {
		t.Errorf("expected /v1/messages, got %s", calls[0].Path)
	}
}

func TestServer_CustomResponse(t *testing.T) {
	s := NewServer()
	defer s.Close()

	s.SetResponse(Response{
		Content: "Custom response content",
		Tokens:  TokenUsage{Input: 200, Output: 100},
	})

	resp, err := http.Get(s.URL() + "/v1/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServer_Delay(t *testing.T) {
	s := NewServer()
	defer s.Close()
	s.SetDelay(100 * time.Millisecond)

	start := time.Now()
	http.Get(s.URL() + "/v1/messages")
	elapsed := time.Since(start)

	if elapsed < 90*time.Millisecond {
		t.Errorf("expected delay >= 100ms, got %v", elapsed)
	}
}

func TestServer_Reset(t *testing.T) {
	s := NewServer()
	defer s.Close()

	s.SetError(Error429RateLimit)
	s.SetDelay(1 * time.Second)
	http.Get(s.URL() + "/test")

	s.Reset()

	if len(s.Calls()) != 0 {
		t.Error("expected calls to be cleared after reset")
	}
}
