package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) aiChatCompletions(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadAISetting(r.Context())
	if err != nil || !cfg.Enabled {
		s.serverError(w, r, 503, "ai_disabled", "AI service is not configured", err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		writeError(w, 400, "invalid_json", "invalid AI request body")
		return
	}
	if _, ok := payload["messages"].([]any); !ok {
		writeError(w, 400, "invalid_ai_request", "messages array is required")
		return
	}
	if model, _ := payload["model"].(string); strings.TrimSpace(model) == "" {
		payload["model"] = cfg.DefaultModel
	}
	maxTokens := 0
	if value, ok := payload["max_tokens"]; ok {
		parsed, ok := jsonInt(value)
		if !ok || parsed < 1 {
			writeError(w, 400, "invalid_max_tokens", "max_tokens must be a positive integer")
			return
		}
		maxTokens = parsed
	}
	if value, ok := payload["max_completion_tokens"]; ok {
		parsed, ok := jsonInt(value)
		if !ok || parsed < 1 {
			writeError(w, 400, "invalid_max_tokens", "max_completion_tokens must be a positive integer")
			return
		}
		if parsed > maxTokens {
			maxTokens = parsed
		}
	}
	if maxTokens == 0 {
		maxTokens = cfg.MaxTokens
		payload["max_tokens"] = maxTokens
	}
	if maxTokens > cfg.MaxTokens || maxTokens > 262144 {
		writeError(w, 400, "max_tokens_exceeded", "requested token limit exceeds the configured maximum")
		return
	}
	stream := true
	if value, ok := payload["stream"].(bool); ok {
		stream = value
	} else {
		payload["stream"] = true
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, 400, "invalid_ai_request", "AI request cannot be encoded")
		return
	}
	endpoint, err := chatCompletionsURL(cfg.BaseURL)
	if err != nil {
		s.serverError(w, r, 500, "invalid_ai_configuration", "AI base URL is invalid", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		s.serverError(w, r, 500, "ai_request_failed", "AI request cannot be created", err)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Accept", "application/json")
	if stream {
		upstream.Header.Set("Accept", "text/event-stream")
	}
	if cfg.APIKey != "" {
		upstream.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Transport: s.HTTP.Transport}
	response, err := client.Do(upstream)
	if err != nil {
		s.serverError(w, r, 502, "ai_upstream_unavailable", "AI upstream is unavailable", err)
		return
	}
	defer response.Body.Close()
	// The invocation is audited even when the browser disconnects part way
	// through the stream, which returns early from the copy loop below.
	defer s.audit(r, "ai.invoke", "ai", "chat.completions", map[string]any{"model": payload["model"], "stream": stream, "max_tokens": maxTokens, "upstream_status": response.StatusCode})
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		if stream {
			contentType = "text/event-stream"
		} else {
			contentType = "application/json"
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(response.StatusCode)
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReaderSize(response.Body, 32*1024)
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				s.Log.Warn("AI stream ended", "error", readErr)
			}
			break
		}
	}
}

func jsonInt(value any) (int, bool) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil && n <= int64(^uint(0)>>1)
	case float64:
		return int(v), v == float64(int(v))
	case int:
		return v, true
	}
	return 0, false
}
func chatCompletionsURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Host == "" {
		return "", err
	}
	if strings.HasSuffix(u.Path, "/chat/completions") {
		return u.String(), nil
	}
	if strings.HasSuffix(u.Path, "/v1") {
		u.Path += "/chat/completions"
	} else {
		u.Path += "/v1/chat/completions"
	}
	return u.String(), nil
}
