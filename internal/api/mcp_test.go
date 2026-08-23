package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMCPStreamStaysOpenUntilTheClientLeaves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/mcp", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); (&Server{}).mcpGet(response, request) }()

	select {
	case <-done:
		t.Fatal("the MCP stream ended while the client was still connected, which makes every client reconnect on the retry interval")
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the MCP stream did not end after the client disconnected")
	}

	if got := response.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if body := response.Body.String(); !strings.Contains(body, "igame MCP stream ready") {
		t.Fatalf("stream preamble missing from %q", body)
	}
}

func TestMCPToolRequestsKeepTheRealCallerIdentity(t *testing.T) {
	parent := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	parent.RemoteAddr = "10.20.30.40:51515"
	parent.Host = "games.example.internal"
	parent.Header.Set("User-Agent", "claude-mcp/1.0")
	parent.Header.Set("X-Forwarded-For", "10.20.30.40")

	var seen *http.Request
	_, err := (&Server{}).captureHandler(parent, http.MethodGet, "/api/v1/games", func(w http.ResponseWriter, r *http.Request) {
		seen = r
		writeJSON(w, 200, map[string]any{"items": []any{}})
	}, "", "")
	if err != nil {
		t.Fatalf("capture handler: %v", err)
	}
	if seen.RemoteAddr != parent.RemoteAddr {
		t.Fatalf("RemoteAddr = %q, want %q", seen.RemoteAddr, parent.RemoteAddr)
	}
	if seen.Host != parent.Host {
		t.Fatalf("Host = %q, want %q", seen.Host, parent.Host)
	}
	for _, header := range []string{"User-Agent", "X-Forwarded-For"} {
		if seen.Header.Get(header) != parent.Header.Get(header) {
			t.Fatalf("%s = %q, want %q", header, seen.Header.Get(header), parent.Header.Get(header))
		}
	}
}

func TestMCPStreamEndsWhenTheServiceDrains(t *testing.T) {
	service := New(nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); service.mcpGet(response, request) }()

	select {
	case <-done:
		t.Fatal("the stream ended before the service began draining")
	case <-time.After(100 * time.Millisecond):
	}

	service.Drain()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("draining did not release the stream, so a graceful shutdown would wait for its timeout")
	}
	service.Drain() // draining twice must not panic on an already closed channel
}
