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

// Three endpoints hand a client-supplied JSON value straight to a jsonb column:
// the session metadata becomes game_sessions.client_info, telemetry data becomes
// game_telemetry.data and an unlock's metadata becomes
// user_achievements.metadata. Every column is defined as an object with a '{}'
// default and is read back with object operators, so a non-object value has to
// be refused at the edge. What actually lands in the column is PostgreSQL's
// answer, not Go's, so these tests drive the real router against a real
// database. They are skipped when IGAME_TEST_DSN is unset; `make test-db
// DSN=...` runs them.

type jsonbFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	cookie string
	user   uuid.UUID
	game   uuid.UUID
	slug   string
	// code is a portal-wide achievement this fixture's client may unlock.
	code string
}

func newJSONBFixture(t *testing.T) jsonbFixture {
	t.Helper()
	pool := migratedPool(t)
	tag := uuid.NewString()[:8]
	f := jsonbFixture{pool: pool, slug: "jsonb-" + tag, code: "jsonb-ach-" + tag}

	f.user = insertTestUser(t, pool, "user")
	f.cookie = insertTestSession(t, pool, f.user)
	f.game = insertTestGame(t, pool, f.slug)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM achievements WHERE code=$1`, f.code)
	})
	if _, err := pool.Exec(context.Background(), `INSERT INTO achievements(game_id,code,name,criteria,xp,active) VALUES(NULL,$1,'Unlocked by the client','{"client_unlockable":true}'::jsonb,50,true)`, f.code); err != nil {
		t.Fatal(err)
	}

	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// do sends the body verbatim so the tests can post values Go's typed encoder
// would never produce for a map field.
func (f jsonbFixture) do(t *testing.T, method, path, body string) (int, map[string]any) {
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

func (f jsonbFixture) errorCode(out map[string]any) string {
	envelope, _ := out["error"].(map[string]any)
	code, _ := envelope["code"].(string)
	return code
}

// startSession opens a play session the way a game client does.
func (f jsonbFixture) startSession(t *testing.T) (id, token string) {
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

// storedTypes reports how many rows the query matches and the jsonb type of
// each one's object column. Every rejection test reads it before and after the
// request so a subtest is judged on what its own request stored, not on what an
// earlier one left behind.
func (f jsonbFixture) storedTypes(t *testing.T, query string, args ...any) (int, []string) {
	t.Helper()
	var rows int
	types := []string{}
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&rows, &types); err != nil {
		t.Fatal(err)
	}
	return rows, types
}

// nonObjects are the JSON values a jsonb object column must never accept.
var nonObjects = []struct{ name, value string }{
	{"null", `null`},
	{"array", `[1,2]`},
	{"number", `5`},
	{"string", `"x"`},
	{"boolean", `true`},
}

const sessionClientInfoQuery = `SELECT count(*),COALESCE(array_agg(jsonb_typeof(client_info)),'{}') FROM game_sessions WHERE user_id=$1 AND game_id=$2`

func TestJSONBSessionMetadataRejectsNonObject(t *testing.T) {
	f := newJSONBFixture(t)

	for _, tc := range nonObjects {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := f.storedTypes(t, sessionClientInfoQuery, f.user, f.game)
			status, out := f.do(t, http.MethodPost, "/api/v1/games/"+f.slug+"/sessions", `{"metadata":`+tc.value+`}`)
			// Read the table either way: when the request is wrongly
			// accepted, the stored row is the evidence of what it did.
			after, types := f.storedTypes(t, sessionClientInfoQuery, f.user, f.game)
			if status != http.StatusBadRequest {
				t.Fatalf("metadata %s: status %d, want 400; %d session row(s) now stored with client_info types %v", tc.value, status, after-before, types)
			}
			if code := f.errorCode(out); code != "invalid_metadata" {
				t.Fatalf("metadata %s: error %v, want code invalid_metadata", tc.value, out)
			}
			if after != before {
				t.Fatalf("metadata %s: %d session row(s) stored with client_info types %v, want none", tc.value, after-before, types)
			}
		})
	}
}

func TestJSONBSessionMetadataAcceptsObject(t *testing.T) {
	f := newJSONBFixture(t)

	for _, tc := range []struct{ name, body string }{
		{"omitted body", ``},
		{"empty body object", `{}`},
		{"empty metadata", `{"metadata":{}}`},
		{"populated metadata", `{"metadata":{"client":"web"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, out := f.do(t, http.MethodPost, "/api/v1/games/"+f.slug+"/sessions", tc.body)
			if status != http.StatusCreated {
				t.Fatalf("body %q: status %d %v, want 201", tc.body, status, out)
			}
			session, _ := out["session"].(map[string]any)
			id, _ := session["id"].(string)
			var clientInfoType string
			if err := f.pool.QueryRow(context.Background(), `SELECT jsonb_typeof(client_info) FROM game_sessions WHERE id=$1`, id).Scan(&clientInfoType); err != nil {
				t.Fatal(err)
			}
			if clientInfoType != "object" {
				t.Fatalf("body %q: client_info is %s, want object", tc.body, clientInfoType)
			}
		})
	}
}

const telemetryDataQuery = `SELECT count(*),COALESCE(array_agg(jsonb_typeof(data)),'{}') FROM game_telemetry WHERE session_id=$1`

func TestJSONBTelemetryDataRejectsNonObject(t *testing.T) {
	f := newJSONBFixture(t)
	sessionID, token := f.startSession(t)

	for _, tc := range nonObjects {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"session_id":` + strconvQuote(sessionID) + `,"session_token":` + strconvQuote(token) + `,"event":"level.up","data":` + tc.value + `}`
			before, _ := f.storedTypes(t, telemetryDataQuery, sessionID)
			status, out := f.do(t, http.MethodPost, "/api/v1/telemetry", body)
			after, types := f.storedTypes(t, telemetryDataQuery, sessionID)
			if status != http.StatusBadRequest {
				t.Fatalf("data %s: status %d, want 400; telemetry data types are now %v", tc.value, status, types)
			}
			if code := f.errorCode(out); code != "invalid_telemetry" {
				t.Fatalf("data %s: error %v, want code invalid_telemetry", tc.value, out)
			}
			if after != before {
				t.Fatalf("data %s: %d telemetry row(s) stored, data types are now %v, want none added", tc.value, after-before, types)
			}
		})
	}
}

func TestJSONBTelemetryDataAcceptsObject(t *testing.T) {
	f := newJSONBFixture(t)

	for _, tc := range []struct{ name, data string }{
		{"omitted", ``},
		{"empty object", `{}`},
		{"populated", `{"level":3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessionID, token := f.startSession(t)
			body := `{"session_id":` + strconvQuote(sessionID) + `,"session_token":` + strconvQuote(token) + `,"event":"level.up"`
			if tc.data != "" {
				body += `,"data":` + tc.data
			}
			body += `}`
			status, out := f.do(t, http.MethodPost, "/api/v1/telemetry", body)
			if status != http.StatusAccepted {
				t.Fatalf("data %q: status %d %v, want 202", tc.data, status, out)
			}
			var rows int
			var dataType string
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*),COALESCE(min(jsonb_typeof(data)),'') FROM game_telemetry WHERE session_id=$1`, sessionID).Scan(&rows, &dataType); err != nil {
				t.Fatal(err)
			}
			if rows != 1 || dataType != "object" {
				t.Fatalf("data %q: %d row(s) with data type %q, want one object", tc.data, rows, dataType)
			}
		})
	}
}

const unlockMetadataQuery = `SELECT count(*),COALESCE(array_agg(jsonb_typeof(ua.metadata)),'{}') FROM user_achievements ua JOIN achievements a ON a.id=ua.achievement_id WHERE ua.user_id=$1 AND a.code=$2`

func TestJSONBAchievementMetadataRejectsNonObject(t *testing.T) {
	f := newJSONBFixture(t)
	sessionID, token := f.startSession(t)

	for _, tc := range nonObjects {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"code":` + strconvQuote(f.code) + `,"session_id":` + strconvQuote(sessionID) + `,"session_token":` + strconvQuote(token) + `,"metadata":` + tc.value + `}`
			before, _ := f.storedTypes(t, unlockMetadataQuery, f.user, f.code)
			status, out := f.do(t, http.MethodPost, "/api/v1/me/achievements", body)
			after, types := f.storedTypes(t, unlockMetadataQuery, f.user, f.code)
			if status != http.StatusBadRequest {
				t.Fatalf("metadata %s: status %d, want 400; unlock metadata types are now %v", tc.value, status, types)
			}
			if code := f.errorCode(out); code != "invalid_achievement_unlock" {
				t.Fatalf("metadata %s: error %v, want code invalid_achievement_unlock", tc.value, out)
			}
			if after != before {
				t.Fatalf("metadata %s: %d unlock row(s) stored, metadata types are now %v, want none added", tc.value, after-before, types)
			}
		})
	}
}

func TestJSONBAchievementMetadataAcceptsObject(t *testing.T) {
	f := newJSONBFixture(t)
	sessionID, token := f.startSession(t)

	body := `{"code":` + strconvQuote(f.code) + `,"session_id":` + strconvQuote(sessionID) + `,"session_token":` + strconvQuote(token) + `,"metadata":{"source":"test"}}`
	status, out := f.do(t, http.MethodPost, "/api/v1/me/achievements", body)
	if status != http.StatusCreated {
		t.Fatalf("object metadata: status %d %v, want 201", status, out)
	}
	var metadataType, source string
	if err := f.pool.QueryRow(context.Background(), `SELECT jsonb_typeof(ua.metadata),COALESCE(ua.metadata->>'source','') FROM user_achievements ua JOIN achievements a ON a.id=ua.achievement_id WHERE ua.user_id=$1 AND a.code=$2`, f.user, f.code).Scan(&metadataType, &source); err != nil {
		t.Fatal(err)
	}
	if metadataType != "object" || source != "test" {
		t.Fatalf("stored metadata is %s with source=%q, want an object carrying the client key", metadataType, source)
	}
}
