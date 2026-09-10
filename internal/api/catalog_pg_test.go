package api

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Whether the session a player is in right now counts towards the daily play
// limit is decided by a SQL sum against the database clock, so these tests need
// a real server. They are skipped when IGAME_TEST_DSN is unset; `make test-db
// DSN=...` runs them.

// openSessionAge is how long an open session has been running in these tests.
// The database clock decides how much an open session has accrued, so the
// instant it started has to be real rather than invented.
const openSessionAge = 5 * time.Minute

// insertTestGame creates a game the calling test owns and removes again.
func insertTestGame(t *testing.T, pool *pgxpool.Pool, slug string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `INSERT INTO games(slug,name,game_url,status) VALUES($1,'Play limit test','/games/limit','active') RETURNING id`, slug).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM games WHERE id=$1`, id) })
	return id
}

// insertTestPlaySession records one session for the pair. Status 'active' with
// no duration is the session a player has open right now.
func insertTestPlaySession(t *testing.T, pool *pgxpool.Pool, userID, gameID uuid.UUID, status string, startedAt time.Time, duration *int64) {
	t.Helper()
	hash := sha256.Sum256([]byte(uuid.NewString()))
	if _, err := pool.Exec(context.Background(), `INSERT INTO game_sessions(user_id,game_id,session_token_hash,status,started_at,duration_ms) VALUES($1,$2,$3,$4,$5,$6)`, userID, gameID, hash[:], status, startedAt, duration); err != nil {
		t.Fatal(err)
	}
}

// serviceDay reads the database clock and reports the start of the service day
// containing it. The policy below names Asia/Seoul, which is +09:00 the year
// round and is also what playAllowed falls back to when the zone cannot be
// loaded, so a fixed offset names the same boundary either way.
//
// A session cannot have been open since before the service day began, so a run
// that lands in the first minutes of a Seoul day has nowhere to put one and
// says so rather than asserting something weaker.
func serviceDay(t *testing.T, pool *pgxpool.Pool) (now, dayStart time.Time) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	local := now.In(time.FixedZone("KST", 9*60*60))
	dayStart = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	if local.Sub(dayStart) < openSessionAge {
		t.Skipf("the service day began %s ago, which leaves no room for a session open for %s", local.Sub(dayStart), openSessionAge)
	}
	return now, dayStart
}

// playLimitServer decides a daily limit for the slug against the real pool,
// with the server clock pinned to the database clock so the service day the
// policy measures is the one the sessions were placed in.
func playLimitServer(t *testing.T, pool *pgxpool.Pool, slug string, limitMinutes int, now time.Time) *Server {
	t.Helper()
	policy := `{"enabled":true,"windows":[],"daily_limits":{"` + slug + `":` + strconv.Itoa(limitMinutes) + `}}`
	s := &Server{DB: pool, Now: func() time.Time { return now }}
	s.storeSetting("play_policy", settingEntry{raw: []byte(policy), expires: time.Now().Add(time.Hour)})
	s.storeSetting("service", settingEntry{raw: []byte(`{"timezone":"Asia/Seoul"}`), expires: time.Now().Add(time.Hour)})
	return s
}

func askPlayLimit(t *testing.T, s *Server, userID, gameID uuid.UUID, slug string) (bool, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey, Principal{UserID: userID, Role: "user", AuthType: "session"}))
	allowed, denial, err := s.playAllowed(r, gameID, slug)
	if err != nil {
		t.Fatal(err)
	}
	return allowed, denial
}

// playLimitFixture sets up one user, one game and one server for a limit.
func playLimitFixture(t *testing.T, limitMinutes int) (*Server, *pgxpool.Pool, uuid.UUID, uuid.UUID, string, time.Time) {
	t.Helper()
	pool := migratedPool(t)
	now, dayStart := serviceDay(t, pool)
	userID := insertTestUser(t, pool, "user")
	slug := "play-limit-" + uuid.NewString()[:8]
	gameID := insertTestGame(t, pool, slug)
	return playLimitServer(t, pool, slug, limitMinutes, now), pool, userID, gameID, slug, dayStart
}

func TestPlayLimitCountsTheSessionStillOpen(t *testing.T) {
	// duration_ms is written only when a session ends, so summing it alone
	// counted the session open right now as no play at all. The limit is checked
	// as a session starts, and the session open at that moment is exactly the one
	// the sum dropped, so a player got one whole session past the limit each day.
	s, pool, userID, gameID, slug, _ := playLimitFixture(t, 1)

	insertTestPlaySession(t, pool, userID, gameID, "active", s.Now().Add(-openSessionAge), nil)

	allowed, denial := askPlayLimit(t, s, userID, gameID, slug)
	if allowed {
		t.Fatalf("a session open for %s did not count towards a one minute limit", openSessionAge)
	}
	if denial != "daily play limit reached" {
		t.Fatalf("denial = %q, want the daily limit", denial)
	}
}

func TestPlayLimitLeavesRoomBelowTheLimit(t *testing.T) {
	// Counting the open session must not close the door early: a player who is
	// still under the limit gets to start another session.
	s, pool, userID, gameID, slug, _ := playLimitFixture(t, 60)

	finished := int64(20 * 60000)
	insertTestPlaySession(t, pool, userID, gameID, "finished", s.Now().Add(-time.Hour), &finished)
	insertTestPlaySession(t, pool, userID, gameID, "active", s.Now().Add(-openSessionAge), nil)

	if allowed, denial := askPlayLimit(t, s, userID, gameID, slug); !allowed {
		t.Fatalf("well under an hour of a sixty minute limit was refused: %q", denial)
	}
}

func TestPlayLimitIgnoresSessionsFromBeforeTheServiceDay(t *testing.T) {
	// The limit is a daily one and the day is the service time zone's, so a
	// session left open yesterday must not spend today's allowance.
	s, pool, userID, gameID, slug, dayStart := playLimitFixture(t, 1)

	insertTestPlaySession(t, pool, userID, gameID, "active", dayStart.Add(-time.Hour), nil)

	if allowed, denial := askPlayLimit(t, s, userID, gameID, slug); !allowed {
		t.Fatalf("a session from before the service day was charged to today: %q", denial)
	}
}

func TestPlayLimitCountsAClosedSessionWithoutADurationAsNothing(t *testing.T) {
	// A session that is over stops accruing time even when no duration was ever
	// recorded for it, so the clause that measures open sessions must not reach
	// closed ones.
	s, pool, userID, gameID, slug, _ := playLimitFixture(t, 1)

	insertTestPlaySession(t, pool, userID, gameID, "abandoned", s.Now().Add(-openSessionAge), nil)

	if allowed, denial := askPlayLimit(t, s, userID, gameID, slug); !allowed {
		t.Fatalf("a closed session with no duration was billed for the time since it started: %q", denial)
	}
}
