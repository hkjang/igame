package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

const mcpProtocolVersion = "2025-11-25"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (s *Server) requireMCPAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, s.requestBaseURL(r)) {
			writeRPC(w, http.StatusForbidden, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32001, Message: "origin not allowed"}}, false)
			return
		}
		p, err := s.authenticate(r)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="igame-mcp", scope="mcp:access"`)
			writeRPC(w, 401, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32000, Message: "authentication required"}}, false)
			return
		}
		if p.AuthType == "api_key" && !p.Can("mcp:access") {
			writeRPC(w, 403, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32002, Message: "API key requires mcp:access"}}, false)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

func sameOrigin(origin, base string) bool {
	o, e1 := url.Parse(origin)
	b, e2 := url.Parse(base)
	return e1 == nil && e2 == nil && strings.EqualFold(o.Scheme, b.Scheme) && strings.EqualFold(o.Host, b.Host)
}

func (s *Server) mcpGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	_, _ = w.Write([]byte("retry: 3000\n: igame MCP stream ready\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) mcpPost(w http.ResponseWriter, r *http.Request) {
	if version := r.Header.Get("MCP-Protocol-Version"); version != "" && version != "2025-11-25" && version != "2025-06-18" && version != "2024-11-05" {
		writeRPC(w, 400, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32600, Message: "unsupported MCP-Protocol-Version"}}, false)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeRPC(w, 400, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}, false)
		return
	}
	trim := bytes.TrimSpace(raw)
	sse := strings.Contains(r.Header.Get("Accept"), "text/event-stream") && !strings.Contains(r.Header.Get("Accept"), "application/json")
	if len(trim) > 0 && trim[0] == '[' {
		writeRPC(w, 400, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32600, Message: "JSON-RPC batch requests are not supported by MCP Streamable HTTP"}}, sse)
		return
	}
	var request rpcRequest
	if json.Unmarshal(trim, &request) != nil {
		writeRPC(w, 400, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}, sse)
		return
	}
	response, ok := s.handleRPC(r, request)
	if !ok {
		w.WriteHeader(202)
		return
	}
	writeRPC(w, 200, response, sse)
}

func (s *Server) handleRPC(r *http.Request, request rpcRequest) (rpcResponse, bool) {
	response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
	if request.JSONRPC != "2.0" || request.Method == "" {
		response.Error = &rpcError{Code: -32600, Message: "invalid request"}
		return response, true
	}
	notification := len(request.ID) == 0 || string(request.ID) == "null"
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &params)
		negotiated := mcpProtocolVersion
		if params.ProtocolVersion == "2025-06-18" || params.ProtocolVersion == "2024-11-05" {
			negotiated = params.ProtocolVersion
		}
		response.Result = map[string]any{"protocolVersion": negotiated, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]any{"name": "igame", "version": versionString()}, "instructions": "Browse the company game catalog, inspect rankings and profile data, and start authenticated game sessions."}
	case "notifications/initialized":
		return response, false
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
			response.Error = &rpcError{Code: -32602, Message: "invalid tool arguments"}
			break
		}
		result, err := s.callMCPTool(r, params.Name, params.Arguments)
		if err != nil {
			response.Result = map[string]any{"content": []map[string]any{{"type": "text", "text": err.Error()}}, "isError": true}
		} else {
			encoded, _ := json.Marshal(result)
			response.Result = map[string]any{"content": []map[string]any{{"type": "text", "text": string(encoded)}}, "structuredContent": result, "isError": false}
		}
	default:
		response.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	if notification {
		return response, false
	}
	return response, true
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "games_list", "description": "List active games in the iGame catalog.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}, "category": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 200}}, "additionalProperties": false}},
		{"name": "game_get", "description": "Get one game by UUID or slug.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"}, "additionalProperties": false}},
		{"name": "leaderboard_get", "description": "Get an individual, department, or team leaderboard.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"game_id": map[string]any{"type": "string"}, "period": map[string]any{"type": "string", "enum": []string{"daily", "weekly", "monthly", "season", "all_time"}}, "group": map[string]any{"type": "string", "enum": []string{"individual", "department", "team"}}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 200}}, "required": []string{"game_id"}, "additionalProperties": false}},
		{"name": "profile_get", "description": "Get the authenticated user's iGame profile.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}},
		{"name": "events_list", "description": "List available company game events.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}},
		{"name": "game_session_start", "description": "Start a signed game session. RealmGuard requires realmguard_version_id from its published config. Requires sessions:write for API keys.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"game_id": map[string]any{"type": "string"}, "realmguard_version_id": map[string]any{"type": "string", "format": "uuid", "description": "Published RealmGuard config version UUID; required when game_id resolves to RealmGuard."}}, "required": []string{"game_id"}, "additionalProperties": false}},
		{"name": "score_submit", "description": "Submit a score for a signed game session. Requires scores:write for API keys.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"session_id": map[string]any{"type": "string"}, "session_token": map[string]any{"type": "string"}, "game_id": map[string]any{"type": "string"}, "score": map[string]any{"type": "integer"}, "duration_ms": map[string]any{"type": "integer", "minimum": 0}, "metadata": map[string]any{"type": "object"}}, "required": []string{"session_id", "session_token", "score"}, "additionalProperties": false}},
	}
}

func (s *Server) callMCPTool(r *http.Request, name string, args map[string]any) (any, error) {
	p, _ := principalFrom(r)
	permission := "games:read"
	switch name {
	case "leaderboard_get":
		permission = "rankings:read"
	case "profile_get":
		permission = "profile:read"
	case "game_session_start":
		permission = "sessions:write"
	case "score_submit":
		permission = "scores:write"
	}
	if p.AuthType == "api_key" && !p.Can(permission) {
		return nil, fmt.Errorf("API key requires %s", permission)
	}
	switch name {
	case "profile_get":
		return map[string]any{"user": p}, nil
	case "games_list":
		query := url.Values{}
		if v, ok := args["query"].(string); ok {
			query.Set("q", v)
		}
		if v, ok := args["category"].(string); ok {
			query.Set("category", v)
		}
		if v, ok := numberArg(args["limit"]); ok {
			query.Set("limit", fmt.Sprint(v))
		}
		return s.captureHandler(r, http.MethodGet, "/api/v1/games?"+query.Encode(), s.listGames, "", "")
	case "game_get":
		id, _ := args["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		return s.captureHandler(r, http.MethodGet, "/api/v1/games/"+url.PathEscape(id), s.getGame, "id", id)
	case "leaderboard_get":
		game, _ := args["game_id"].(string)
		if game == "" {
			return nil, fmt.Errorf("game_id is required")
		}
		query := url.Values{"game_id": []string{game}}
		for _, key := range []string{"period", "group"} {
			if v, ok := args[key].(string); ok && v != "" {
				query.Set(key, v)
			}
		}
		if v, ok := numberArg(args["limit"]); ok {
			query.Set("limit", fmt.Sprint(v))
		}
		return s.captureHandler(r, http.MethodGet, "/api/v1/rankings?"+query.Encode(), s.rankings, "", "")
	case "events_list":
		return s.captureHandler(r, http.MethodGet, "/api/v1/events", s.listEvents, "", "")
	case "game_session_start":
		game, _ := args["game_id"].(string)
		if game == "" {
			return nil, fmt.Errorf("game_id is required")
		}
		metadata := map[string]any{}
		if versionID, exists := args["realmguard_version_id"]; exists {
			metadata["realmguard_version_id"] = versionID
		}
		body, err := json.Marshal(map[string]any{"metadata": metadata})
		if err != nil {
			return nil, fmt.Errorf("invalid session arguments")
		}
		return s.captureHandlerRouteBody(r, http.MethodPost, "/api/v1/games/"+url.PathEscape(game)+"/sessions", s.startGameSession, body, "id", game)
	case "score_submit":
		body, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("invalid score arguments")
		}
		return s.captureHandlerBody(r, http.MethodPost, "/api/v1/scores", s.submitScore, body)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func (s *Server) captureHandlerBody(parent *http.Request, method, target string, handler http.HandlerFunc, body []byte) (any, error) {
	return s.captureHandlerRouteBody(parent, method, target, handler, body, "", "")
}

func (s *Server) captureHandlerRouteBody(parent *http.Request, method, target string, handler http.HandlerFunc, body []byte, param, value string) (any, error) {
	request := httptest.NewRequest(method, target, bytes.NewReader(body)).WithContext(parent.Context())
	request.Header.Set("Content-Type", "application/json")
	if param != "" {
		route := chi.NewRouteContext()
		route.URLParams.Add(param, value)
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code >= 400 {
		var envelope map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
		return nil, fmt.Errorf("tool request failed: %v", envelope["error"])
	}
	var result any
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func numberArg(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), n == float64(int(n))
	case json.Number:
		i, e := n.Int64()
		return int(i), e == nil
	case int:
		return n, true
	}
	return 0, false
}
func (s *Server) captureHandler(parent *http.Request, method, target string, handler http.HandlerFunc, param, value string) (any, error) {
	request := httptest.NewRequest(method, target, nil).WithContext(parent.Context())
	if param != "" {
		route := chi.NewRouteContext()
		route.URLParams.Add(param, value)
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code >= 400 {
		var envelope map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
		return nil, fmt.Errorf("tool request failed: %v", envelope["error"])
	}
	var result any
	if recorder.Body.Len() == 0 {
		return map[string]any{"status": recorder.Code}, nil
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func writeRPC(w http.ResponseWriter, status int, value any, sse bool) {
	if sse {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(status)
		data, _ := json.Marshal(value)
		_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	writeJSON(w, status, value)
}
