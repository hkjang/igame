package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hkjang/igame/internal/database"
)

// migratedPool opens the disposable database named by IGAME_TEST_DSN and brings
// it up to the current schema.
//
// Whether a password reset actually closes the sessions opened with the old
// password is decided by PostgreSQL, not by Go, so these tests need a real
// server. They are skipped when no DSN is set; `make test-db DSN=...` runs them.
func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("IGAME_TEST_DSN")
	if dsn == "" {
		t.Skip("IGAME_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// insertTestUser creates an account the calling test owns and removes again.
func insertTestUser(t *testing.T, pool *pgxpool.Pool, role string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	username := "admin-pg-" + uuid.NewString()
	if err := pool.QueryRow(ctx, `INSERT INTO users(username,display_name,role,status,password_hash) VALUES($1,'Admin session test',$2,'active','old-hash') RETURNING id`, username, role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
	return id
}

// insertTestSession opens a session for a user and returns its raw cookie value.
func insertTestSession(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) string {
	t.Helper()
	token := uuid.NewString()
	hash := sha256.Sum256([]byte(token))
	if _, err := pool.Exec(context.Background(), `INSERT INTO auth_sessions(user_id,token_hash,expires_at) VALUES($1,$2,now()+interval '12 hours')`, userID, hash[:]); err != nil {
		t.Fatal(err)
	}
	return token
}

func countSessions(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM auth_sessions WHERE user_id=$1`, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// callUpdateUser runs the handler as actor against target with the given body.
// cookie, when set, is the actor's own session token.
func callUpdateUser(t *testing.T, pool *pgxpool.Pool, actor, target uuid.UUID, cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+target.String(), strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	route := chi.NewRouteContext()
	route.URLParams.Add("id", target.String())
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
	request = request.WithContext(context.WithValue(request.Context(), principalKey, Principal{UserID: actor, Role: "admin", AuthType: "session"}))
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	response := httptest.NewRecorder()
	New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))).updateUser(response, request)
	return response
}

// lastUserUpdateDetail reads back what the audit trail recorded for the update.
func lastUserUpdateDetail(t *testing.T, pool *pgxpool.Pool, target uuid.UUID) map[string]any {
	t.Helper()
	var raw json.RawMessage
	if err := pool.QueryRow(context.Background(), `SELECT detail FROM audit_logs WHERE action='user.update' AND resource_id=$1 ORDER BY id DESC LIMIT 1`, target.String()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	detail := map[string]any{}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	return detail
}

// A reset is how an operator takes an account back from whoever got into it. The
// password used to change while every session opened with the old one stayed
// valid for another twelve hours, so the stolen cookie kept working.
func TestPasswordResetClosesTheTargetsSessions(t *testing.T) {
	pool := migratedPool(t)
	actor := insertTestUser(t, pool, "admin")
	target := insertTestUser(t, pool, "user")
	stolen := insertTestSession(t, pool, target)
	insertTestSession(t, pool, target)
	survivor := insertTestUser(t, pool, "user")
	insertTestSession(t, pool, survivor)

	response := callUpdateUser(t, pool, actor, target, "", `{"password":"correct horse battery"}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("reset status = %d, want 204: %s", response.Code, response.Body.String())
	}
	if n := countSessions(t, pool, target); n != 0 {
		t.Fatalf("%d sessions survived the reset, want 0", n)
	}
	if n := countSessions(t, pool, survivor); n != 1 {
		t.Fatalf("another account lost %d sessions, want 0", 1-n)
	}

	// The stolen cookie must no longer authenticate, not merely be gone from a
	// count the handler wrote itself.
	probe := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	probe.AddCookie(&http.Cookie{Name: sessionCookie, Value: stolen})
	if _, err := New(pool, nil, slog.Default()).authenticate(probe); err == nil {
		t.Fatal("the session cookie from before the reset still authenticates")
	}

	detail := lastUserUpdateDetail(t, pool, target)
	if detail["password_reset"] != true {
		t.Fatalf("audit detail did not record the reset: %v", detail)
	}
	if got, ok := detail["sessions_revoked"].(float64); !ok || got != 2 {
		t.Fatalf("audit detail says sessions_revoked=%v, want 2", detail["sessions_revoked"])
	}
}

// Operators are also people with an account, and being signed out mid-change
// protects nobody — the self-service path keeps the caller's session too.
func TestPasswordResetKeepsTheOperatorsOwnSession(t *testing.T) {
	pool := migratedPool(t)
	actor := insertTestUser(t, pool, "admin")
	own := insertTestSession(t, pool, actor)
	insertTestSession(t, pool, actor)

	response := callUpdateUser(t, pool, actor, actor, own, `{"password":"correct horse battery"}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("self reset status = %d, want 204: %s", response.Code, response.Body.String())
	}
	if n := countSessions(t, pool, actor); n != 1 {
		t.Fatalf("%d sessions remain after a self reset, want only the caller's", n)
	}
	probe := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	probe.AddCookie(&http.Cookie{Name: sessionCookie, Value: own})
	if _, err := New(pool, nil, slog.Default()).authenticate(probe); err != nil {
		t.Fatalf("the operator was signed out of their own request: %v", err)
	}
	if got, ok := lastUserUpdateDetail(t, pool, actor)["sessions_revoked"].(float64); !ok || got != 1 {
		t.Fatalf("audit detail says sessions_revoked=%v, want 1", got)
	}
}

// Nothing else this endpoint writes needs a sweep: authenticate reads role and
// status per request, so an edit that leaves the password alone must not sign
// anyone out.
func TestProfileEditLeavesTheSessionsAlone(t *testing.T) {
	pool := migratedPool(t)
	actor := insertTestUser(t, pool, "admin")
	target := insertTestUser(t, pool, "user")
	token := insertTestSession(t, pool, target)

	response := callUpdateUser(t, pool, actor, target, "", `{"display_name":"Renamed","status":"disabled"}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("edit status = %d, want 204: %s", response.Code, response.Body.String())
	}
	if n := countSessions(t, pool, target); n != 1 {
		t.Fatalf("%d sessions remain after a profile edit, want 1", n)
	}
	// Disabling the account still shuts the session out, through the status the
	// authentication query checks rather than through a delete.
	probe := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	probe.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	if _, err := New(pool, nil, slog.Default()).authenticate(probe); err == nil {
		t.Fatal("a disabled account still authenticates")
	}
	if detail := lastUserUpdateDetail(t, pool, target); detail["password_reset"] != nil || detail["sessions_revoked"] != nil {
		t.Fatalf("audit detail claims a reset that did not happen: %v", detail)
	}
}

// The 404 path must not become a way to close another account's sessions.
func TestUpdatingAMissingUserStillReportsNotFound(t *testing.T) {
	pool := migratedPool(t)
	actor := insertTestUser(t, pool, "admin")
	insertTestSession(t, pool, actor)
	missing := uuid.New()

	response := callUpdateUser(t, pool, actor, missing, "", `{"password":"correct horse battery"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", response.Code, response.Body.String())
	}
	if n := countSessions(t, pool, actor); n != 1 {
		t.Fatalf("the caller has %d sessions after a request that found nothing, want 1", n)
	}
}
