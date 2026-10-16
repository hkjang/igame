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
// PostgreSQL, and only when the ORDER BY is a total order: scores that tie on
// created_at may land in a different relative position on every page request,
// so one score can be handed out twice and another never. A score that never
// appears in the moderation list is a score nobody reviews, so these tests run
// the real router against a real database. They are skipped when
// IGAME_TEST_DSN is unset; `make test-db DSN=...` runs them.

type rankingsOrderFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	cookie string
	game   uuid.UUID
	tied   []string
	newer  string
	older  string
}

// rankingsTiedAt is the one instant every tied score is created at. A fixed
// value keeps the tie exact rather than relying on the clock's resolution.
var rankingsTiedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// insertModeratedScore writes one score with an explicit ID so the test can
// assert the order by ID. scores.session_id is NOT NULL UNIQUE, so every score
// needs a session of its own; the shared insertTestPlaySession helper does not
// return the ID it generated, so this writes its own session row too.
func insertModeratedScore(t *testing.T, pool *pgxpool.Pool, id string, userID, gameID uuid.UUID, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	hash := sha256.Sum256([]byte(uuid.NewString()))
	var session uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO game_sessions(user_id,game_id,session_token_hash,status,started_at,duration_ms)
		VALUES($1,$2,$3,'finished',$4,60000) RETURNING id`, userID, gameID, hash[:], createdAt).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO scores(id,user_id,game_id,session_id,score,created_at)
		VALUES($1,$2,$3,$4,1000,$5)`, id, userID, gameID, session, createdAt); err != nil {
		t.Fatal(err)
	}
}

func newRankingsOrderFixture(t *testing.T) rankingsOrderFixture {
	t.Helper()
	pool := migratedPool(t)
	f := rankingsOrderFixture{pool: pool}
	admin := insertTestUser(t, pool, "admin")
	f.cookie = insertTestSession(t, pool, admin)
	f.game = insertTestGame(t, pool, "rankings-order-"+uuid.NewString()[:8])

	// The IDs ascend with insertion order, so the expected order below (ID
	// descending) is the API contract and never the heap or index order.
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("40000000-0000-0000-0000-%012d", i)
		insertModeratedScore(t, pool, id, admin, f.game, rankingsTiedAt)
		f.tied = append(f.tied, id)
	}
	slices.Reverse(f.tied)

	// Controls with distinct created_at values: time still orders first, and the
	// ID tiebreak only decides between rows that tie.
	f.newer = "40000000-0000-0000-0000-000000000009"
	f.older = "40000000-0000-0000-0000-000000000000"
	insertModeratedScore(t, pool, f.newer, admin, f.game, rankingsTiedAt.Add(time.Hour))
	insertModeratedScore(t, pool, f.older, admin, f.game, rankingsTiedAt.Add(-time.Hour))

	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// page fetches one window of the moderation list, scoped to this fixture's game
// so no other score can take part in the ordering.
func (f rankingsOrderFixture) page(t *testing.T, limit, offset int, query string) []map[string]any {
	t.Helper()
	path := fmt.Sprintf("/api/v1/admin/rankings?game_id=%s&limit=%d&offset=%d%s", f.game, limit, offset, query)
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

func (f rankingsOrderFixture) pageIDs(t *testing.T, limit, offset int) []string {
	t.Helper()
	var ids []string
	for _, item := range f.page(t, limit, offset, "") {
		ids = append(ids, item["id"].(string))
	}
	return ids
}

func assertRankingsOrder(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("rankings created_at/id order: got %v, want %v", got, want)
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] != 1 {
			t.Errorf("rankings ID %s appeared %d times, want exactly once", id, seen[id])
		}
	}
}

func TestAdminRankingsOrdersTiedCreationTimesByID(t *testing.T) {
	f := newRankingsOrderFixture(t)
	want := append([]string{f.newer}, f.tied...)
	want = append(want, f.older)

	var got []string
	for offset := 0; offset < len(want); offset += 2 {
		ids := f.pageIDs(t, 2, offset)
		if len(ids) != 2 {
			t.Fatalf("offset %d: got %d items, want 2", offset, len(ids))
		}
		got = append(got, ids...)
	}
	assertRankingsOrder(t, got, want)
	if items := f.page(t, 2, len(want), ""); len(items) != 0 {
		t.Errorf("page past the end returned %d items", len(items))
	}
}

func TestAdminRankingsPagesTiedCreationTimesWithoutGapsOrDuplicates(t *testing.T) {
	f := newRankingsOrderFixture(t)
	// Walking only the tied block proves the window boundaries fall inside a run
	// of equal created_at values and still hand out every row exactly once.
	var got []string
	for _, offset := range []int{1, 3, 5} {
		ids := f.pageIDs(t, 2, offset)
		if len(ids) != 2 {
			t.Fatalf("offset %d: got %d items, want 2", offset, len(ids))
		}
		got = append(got, ids...)
	}
	assertRankingsOrder(t, got, f.tied)
}

func TestAdminRankingsKeepsFiltersAndFieldsWhileOrdering(t *testing.T) {
	f := newRankingsOrderFixture(t)
	// The status filter and the item shape must survive the ordering change.
	valid := f.page(t, 200, 0, "&status=valid")
	if len(valid) != len(f.tied)+2 {
		t.Fatalf("status=valid returned %d items, want %d", len(valid), len(f.tied)+2)
	}
	for _, key := range []string{"id", "session_id", "user_id", "username", "display_name", "department", "team", "game_id", "game_name", "score", "verified", "status", "rejection_reason", "metadata", "created_at"} {
		if _, ok := valid[0][key]; !ok {
			t.Errorf("item is missing key %q", key)
		}
	}
	if n := len(valid[0]); n != 15 {
		t.Errorf("item has %d keys, want 15: %+v", n, valid[0])
	}
	if got := valid[0]["game_id"]; got != f.game.String() {
		t.Errorf("game_id filter returned game_id %v, want %s", got, f.game)
	}
	if items := f.page(t, 200, 0, "&status=excluded"); len(items) != 0 {
		t.Errorf("status=excluded returned %d items, want 0", len(items))
	}
}
