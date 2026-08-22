package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckHealth(t *testing.T) {
	t.Parallel()

	t.Run("healthy response", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		if err := checkHealth(context.Background(), server.URL); err != nil {
			t.Fatalf("checkHealth() error = %v", err)
		}
	})

	t.Run("unhealthy response", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
		}))
		defer server.Close()

		err := checkHealth(context.Background(), server.URL)
		if err == nil || !strings.Contains(err.Error(), "503 Service Unavailable") {
			t.Fatalf("checkHealth() error = %v, want status failure", err)
		}
	})

	t.Run("connection failure", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		endpoint := server.URL
		server.Close()

		err := checkHealth(context.Background(), endpoint)
		if err == nil || !strings.Contains(err.Error(), "request health endpoint") {
			t.Fatalf("checkHealth() error = %v, want request failure", err)
		}
	})
}
