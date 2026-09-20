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

// Whether a session may unlock an achievement is decided by one SQL join
// between the achievement and the session, so these tests run the real router
// against a real database. They are skipped when IGAME_TEST_DSN is unset;
// `make test-db DSN=...` runs them.

type achievementFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	cookie string
	tag    string
	// gameA and gameB are two games the viewer can play; bound is tied to
	// gameA, global to no game at all.
	gameA, gameB              string
	global, bound, serverOnly string
}

func newAchievementFixture(t *testing.T) achievementFixture {
	t.Helper()
	pool := migratedPool(t)
	ctx := context.Background()
	f := achievementFixture{pool: pool, tag: uuid.NewString()[:8]}
	f.gameA, f.gameB = "ach-a-"+f.tag, "ach-b-"+f.tag
	f.global, f.bound, f.serverOnly = "ach-global-"+f.tag, "ach-bound-"+f.tag, "ach-server-"+f.tag

	viewer := insertTestUser(t, pool, "user")
	f.cookie = insertTestSession(t, pool, viewer)
	gameA := insertTestGame(t, pool, f.gameA)
	insertTestGame(t, pool, f.gameB)

	// Deleting the achievements cascades to whatever the tests unlocked.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM achievements WHERE code LIKE $1`, "ach-%-"+f.tag)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO achievements(game_id,code,name,criteria,xp,active) VALUES
		(NULL,$1,'Portal-wide, unlocked by the client','{"client_unlockable":true}'::jsonb,50,true),
		($4,$2,'Bound to one game, unlocked by the client','{"client_unlockable":true}'::jsonb,50,true),
		(NULL,$3,'Portal-wide, awarded by the server','{"server_rule":"never"}'::jsonb,50,true)`,
		f.global, f.bound, f.serverOnly, gameA); err != nil {
		t.Fatal(err)
	}

	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

func (f achievementFixture) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(raw)
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

// startSession opens a play session for the slug the way a game client does
// and returns what the client would send back with an unlock.
func (f achievementFixture) startSession(t *testing.T, slug string) (id, token string) {
	t.Helper()
	status, out := f.do(t, http.MethodPost, "/api/v1/games/"+slug+"/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("start session for %s: status %d %v", slug, status, out)
	}
	session, _ := out["session"].(map[string]any)
	id, _ = session["id"].(string)
	token, _ = session["session_token"].(string)
	if id == "" || token == "" {
		t.Fatalf("start session for %s: no id/token in %v", slug, out)
	}
	return id, token
}

func (f achievementFixture) unlock(t *testing.T, code, sessionID, token string) int {
	t.Helper()
	status, _ := f.do(t, http.MethodPost, "/api/v1/me/achievements", map[string]any{"code": code, "session_id": sessionID, "session_token": token})
	return status
}

func (f achievementFixture) unlockedCodes(t *testing.T) map[string]bool {
	t.Helper()
	status, out := f.do(t, http.MethodGet, "/api/v1/me/achievements", nil)
	if status != http.StatusOK {
		t.Fatalf("list achievements: status %d %v", status, out)
	}
	codes := map[string]bool{}
	items, _ := out["items"].([]any)
	for _, item := range items {
		m, _ := item.(map[string]any)
		code, _ := m["code"].(string)
		codes[code] = true
	}
	return codes
}

func TestGlobalAchievementUnlocksFromAnyGameSession(t *testing.T) {
	f := newAchievementFixture(t)
	sessionID, token := f.startSession(t, f.gameB)

	if status := f.unlock(t, f.global, sessionID, token); status != http.StatusCreated {
		t.Fatalf("portal-wide achievement from a %s session: status %d, want 201", f.gameB, status)
	}
	// A second unlock is idempotent rather than a second row.
	if status := f.unlock(t, f.global, sessionID, token); status != http.StatusOK {
		t.Fatalf("repeated unlock: status %d, want 200", status)
	}
	if codes := f.unlockedCodes(t); !codes[f.global] {
		t.Fatalf("portal-wide achievement missing from /me/achievements: %v", codes)
	}
}

func TestGameBoundAchievementStillNeedsItsOwnGame(t *testing.T) {
	f := newAchievementFixture(t)
	sessionA, tokenA := f.startSession(t, f.gameA)
	sessionB, tokenB := f.startSession(t, f.gameB)

	if status := f.unlock(t, f.bound, sessionB, tokenB); status != http.StatusForbidden {
		t.Fatalf("%s achievement from a %s session: status %d, want 403", f.gameA, f.gameB, status)
	}
	if status := f.unlock(t, f.bound, sessionA, tokenA); status != http.StatusCreated {
		t.Fatalf("%s achievement from its own session: status %d, want 201", f.gameA, status)
	}
	codes := f.unlockedCodes(t)
	if !codes[f.bound] || codes[f.global] {
		t.Fatalf("unlocked set %v, want only %s", codes, f.bound)
	}
}

func TestGlobalAchievementStillNeedsClientUnlockableAndAValidSession(t *testing.T) {
	f := newAchievementFixture(t)
	sessionID, token := f.startSession(t, f.gameA)

	if status := f.unlock(t, f.serverOnly, sessionID, token); status != http.StatusForbidden {
		t.Fatalf("server-awarded achievement from the client: status %d, want 403", status)
	}
	if status := f.unlock(t, f.global, sessionID, token+"x"); status != http.StatusForbidden {
		t.Fatalf("wrong session token: status %d, want 403", status)
	}
	if status := f.unlock(t, f.global, uuid.NewString(), token); status != http.StatusForbidden {
		t.Fatalf("unknown session: status %d, want 403", status)
	}
	// Starting a session awards the seeded first-play achievement by itself;
	// nothing this test asked for may be there.
	if codes := f.unlockedCodes(t); codes[f.global] || codes[f.serverOnly] {
		t.Fatalf("nothing the client asked for should be unlocked, got %v", codes)
	}
}
