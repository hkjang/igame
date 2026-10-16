package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Whether OFFSET pages a result set into disjoint windows is decided by
// PostgreSQL, and only when the ORDER BY is a total order: rows that tie on the
// sort column may land in a different relative position on every page request,
// so one row can be handed out twice and another never. These tests run the
// real router against a real database for that reason. They are skipped when
// IGAME_TEST_DSN is unset; `make test-db DSN=...` runs them.

type historyOrderFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	cookie string
	user   uuid.UUID
	tied   []string
	newer  string
	older  string
}

// historyTiedAt is the one instant every tied session starts at. A fixed value
// keeps the tie exact rather than relying on the clock's resolution.
var historyTiedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// insertHistorySession writes one session with an explicit ID so the test can
// assert the order by ID. The shared insertTestPlaySession helper does not
// return the ID it generated, so these tests write their own row.
func insertHistorySession(t *testing.T, pool *pgxpool.Pool, id string, userID, gameID uuid.UUID, startedAt time.Time) {
	t.Helper()
	hash := sha256.Sum256([]byte(uuid.NewString()))
	if _, err := pool.Exec(context.Background(), `INSERT INTO game_sessions(id,user_id,game_id,session_token_hash,status,started_at,duration_ms)
		VALUES($1,$2,$3,$4,'finished',$5,60000)`, id, userID, gameID, hash[:], startedAt); err != nil {
		t.Fatal(err)
	}
}

func newHistoryOrderFixture(t *testing.T) historyOrderFixture {
	t.Helper()
	pool := migratedPool(t)
	f := historyOrderFixture{pool: pool}
	f.user = insertTestUser(t, pool, "user")
	f.cookie = insertTestSession(t, pool, f.user)
	game := insertTestGame(t, pool, "history-order-"+uuid.NewString()[:8])

	// The IDs ascend with insertion order, so the expected order below (ID
	// descending) is the API contract and never the heap or index order.
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("30000000-0000-0000-0000-%012d", i)
		insertHistorySession(t, pool, id, f.user, game, historyTiedAt)
		f.tied = append(f.tied, id)
	}
	slices.Reverse(f.tied)

	// Controls with distinct started_at values: time still orders first, and the
	// ID tiebreak only decides between rows that tie.
	f.newer = "30000000-0000-0000-0000-000000000009"
	f.older = "30000000-0000-0000-0000-000000000000"
	insertHistorySession(t, pool, f.newer, f.user, game, historyTiedAt.Add(time.Hour))
	insertHistorySession(t, pool, f.older, f.user, game, historyTiedAt.Add(-time.Hour))

	// Another player's session must never appear in this player's history.
	other := insertTestUser(t, pool, "user")
	insertHistorySession(t, pool, "30000000-0000-0000-0000-00000000000a", other, game, historyTiedAt)

	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// page fetches one window of the caller's own history.
func (f historyOrderFixture) page(t *testing.T, limit, offset int) []map[string]any {
	t.Helper()
	path := fmt.Sprintf("/api/v1/me/history?limit=%d&offset=%d", limit, offset)
	req, err := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d, want 200", path, res.StatusCode)
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Items == nil || len(out.Items) > limit {
		t.Fatalf("GET %s: invalid page envelope: %+v", path, out)
	}
	return out.Items
}

func assertHistoryOrder(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("history started_at/id order: got %v, want %v", got, want)
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] != 1 {
			t.Errorf("history ID %s appeared %d times, want exactly once", id, seen[id])
		}
	}
}

func TestHistoryOrdersTiedStartTimesByID(t *testing.T) {
	f := newHistoryOrderFixture(t)
	want := append([]string{f.newer}, f.tied...)
	want = append(want, f.older)

	var got []string
	for offset := 0; offset < len(want); offset += 2 {
		items := f.page(t, 2, offset)
		if len(items) != 2 {
			t.Fatalf("offset %d: got %d items, want 2", offset, len(items))
		}
		for _, item := range items {
			got = append(got, item["id"].(string))
		}
	}
	assertHistoryOrder(t, got, want)
	if items := f.page(t, 2, len(want)); len(items) != 0 {
		t.Errorf("page past the end returned %d items", len(items))
	}
}

func TestHistoryPagesTiedStartTimesWithoutGapsOrDuplicates(t *testing.T) {
	f := newHistoryOrderFixture(t)
	// Walking only the tied block proves the window boundaries fall inside a run
	// of equal started_at values and still hand out every row exactly once.
	var got []string
	for _, offset := range []int{1, 3, 5} {
		items := f.page(t, 2, offset)
		if len(items) != 2 {
			t.Fatalf("offset %d: got %d items, want 2", offset, len(items))
		}
		for _, item := range items {
			got = append(got, item["id"].(string))
		}
	}
	assertHistoryOrder(t, got, f.tied)
}

func TestHistoryKeepsOtherPlayersSessionsOut(t *testing.T) {
	f := newHistoryOrderFixture(t)
	items := f.page(t, 200, 0)
	if len(items) != len(f.tied)+2 {
		t.Fatalf("history returned %d items, want %d", len(items), len(f.tied)+2)
	}
	for _, item := range items {
		var owner uuid.UUID
		if err := f.pool.QueryRow(context.Background(), `SELECT user_id FROM game_sessions WHERE id=$1`, item["id"]).Scan(&owner); err != nil {
			t.Fatal(err)
		}
		if owner != f.user {
			t.Errorf("history returned session %s owned by %s", item["id"], owner)
		}
	}
}
