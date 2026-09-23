package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// game_sessions.result is jsonb and every writer merges into it with `||`.
// PostgreSQL's `||` promotes a non-array operand to a one-element array before
// concatenating, so a non-object `result` from the client turns the column into
// an array and every later `result||jsonb_build_object(...)` appends an element
// instead of setting a key. These tests run the real router against a real
// database because that promotion is PostgreSQL's behaviour, not Go's. They are
// skipped when IGAME_TEST_DSN is unset; `make test-db DSN=...` runs them.

type finishFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	cookie string
	slug   string
}

func newFinishFixture(t *testing.T) finishFixture {
	t.Helper()
	pool := migratedPool(t)
	f := finishFixture{pool: pool, slug: "finish-" + uuid.NewString()[:8]}

	user := insertTestUser(t, pool, "user")
	f.cookie = insertTestSession(t, pool, user)
	insertTestGame(t, pool, f.slug)

	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// do sends raw JSON so the tests can post `result` values Go's typed encoder
// would never produce for a map field.
func (f finishFixture) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	var payload io.Reader
	if body != "" {
		payload = bytes.NewReader([]byte(body))
	}
	request, err := http.NewRequest(method, f.server.URL+path, payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil && err != io.EOF {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response.StatusCode, out
}

// startSession opens a play session the way a game client does.
func (f finishFixture) startSession(t *testing.T) (id, token string) {
	t.Helper()
	status, out := f.do(t, http.MethodPost, "/api/v1/games/"+f.slug+"/sessions", "")
	if status != http.StatusCreated {
		t.Fatalf("start session: status %d %v", status, out)
	}
	session, _ := out["session"].(map[string]any)
	id, _ = session["id"].(string)
	token, _ = session["session_token"].(string)
	if id == "" || token == "" {
		t.Fatalf("start session: no id/token in %v", out)
	}
	return id, token
}

// sessionRow reports what the column actually holds, which is the thing the
// promotion rule corrupts.
func (f finishFixture) sessionRow(t *testing.T, id string) (status, resultType, result string) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(), `SELECT status,jsonb_typeof(result),result::text FROM game_sessions WHERE id=$1`, id).Scan(&status, &resultType, &result); err != nil {
		t.Fatal(err)
	}
	return status, resultType, result
}

func TestFinishSessionRejectsNonObjectResult(t *testing.T) {
	f := newFinishFixture(t)

	for _, tc := range []struct{ name, result string }{
		{"array", `[1,2]`},
		{"number", `5`},
		{"string", `"x"`},
		{"null", `null`},
		{"boolean", `true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, token := f.startSession(t)
			body := `{"session_token":` + strconvQuote(token) + `,"result":` + tc.result + `}`

			status, out := f.do(t, http.MethodPost, "/api/v1/sessions/"+id+"/finish", body)
			// Read the row either way: when the request is wrongly
			// accepted, the row is the evidence of what it did.
			gotStatus, gotType, gotResult := f.sessionRow(t, id)
			if status != http.StatusBadRequest {
				t.Fatalf("result %s: status %d, want 400; row is now status=%q result=%s (%s)", tc.result, status, gotStatus, gotResult, gotType)
			}
			envelope, _ := out["error"].(map[string]any)
			if code, _ := envelope["code"].(string); code != "invalid_result" {
				t.Fatalf("result %s: error %v, want code invalid_result", tc.result, out)
			}
			// A rejected request must not have finished the session or
			// touched the column on its way to the error.
			if gotStatus != "active" || gotType != "object" || gotResult != "{}" {
				t.Fatalf("result %s: row is status=%q result=%s (%s), want an untouched active session with {}", tc.result, gotStatus, gotResult, gotType)
			}
		})
	}
}

func TestFinishSessionAcceptsObjectResultAndKeepsScoreReadable(t *testing.T) {
	f := newFinishFixture(t)

	t.Run("omitted result still finishes", func(t *testing.T) {
		id, token := f.startSession(t)
		status, out := f.do(t, http.MethodPost, "/api/v1/sessions/"+id+"/finish", `{"session_token":`+strconvQuote(token)+`}`)
		if status != http.StatusOK {
			t.Fatalf("status %d %v, want 200", status, out)
		}
		session, _ := out["session"].(map[string]any)
		if session["status"] != "finished" {
			t.Fatalf("session %v, want status finished", session)
		}
		if gotStatus, gotType, _ := f.sessionRow(t, id); gotStatus != "finished" || gotType != "object" {
			t.Fatalf("row status=%q result type=%q, want finished/object", gotStatus, gotType)
		}
	})

	t.Run("empty object still finishes", func(t *testing.T) {
		id, token := f.startSession(t)
		status, _ := f.do(t, http.MethodPost, "/api/v1/sessions/"+id+"/finish", `{"session_token":`+strconvQuote(token)+`,"result":{}}`)
		if status != http.StatusOK {
			t.Fatalf("status %d, want 200", status)
		}
		if gotStatus, gotType, _ := f.sessionRow(t, id); gotStatus != "finished" || gotType != "object" {
			t.Fatalf("row status=%q result type=%q, want finished/object", gotStatus, gotType)
		}
	})

	// The reason the object contract matters: submitScore sets a key on the
	// same column, and a key can only be read back off an object.
	t.Run("object result merges and leaves score readable", func(t *testing.T) {
		id, token := f.startSession(t)
		status, _ := f.do(t, http.MethodPost, "/api/v1/sessions/"+id+"/finish", `{"session_token":`+strconvQuote(token)+`,"result":{"level":3}}`)
		if status != http.StatusOK {
			t.Fatalf("finish: status %d, want 200", status)
		}
		scoreStatus, out := f.do(t, http.MethodPost, "/api/v1/scores", `{"session_id":`+strconvQuote(id)+`,"session_token":`+strconvQuote(token)+`,"score":42}`)
		if scoreStatus != http.StatusCreated {
			t.Fatalf("submit score: status %d %v, want 201", scoreStatus, out)
		}

		var resultType, level, score string
		if err := f.pool.QueryRow(context.Background(), `SELECT jsonb_typeof(result),COALESCE(result->>'level',''),COALESCE(result->>'score','') FROM game_sessions WHERE id=$1`, id).Scan(&resultType, &level, &score); err != nil {
			t.Fatal(err)
		}
		if resultType != "object" || level != "3" || score != "42" {
			t.Fatalf("result is %s with level=%q score=%q, want an object carrying both the client key and the score", resultType, level, score)
		}
	})
}

// strconvQuote keeps the raw-JSON bodies above readable.
func strconvQuote(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}
