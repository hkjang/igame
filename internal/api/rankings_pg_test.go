package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The order of a ranking page is whatever PostgreSQL hands back for the
// query, so which of several tied players is on the podium, and whether that
// answer holds from one request to the next, can only be checked against a
// real database. These tests are skipped when IGAME_TEST_DSN is unset;
// `make test-db DSN=...` runs them.

// tiedPlayers is how many players share the top score. It is large enough
// that the planner aggregates the players in a hash table, as it does for a
// production-sized game, which is where the order of ties comes undone.
const tiedPlayers = 3000

const tiedScore = 7_000_000

// rankingFixture is a set of players who all posted the same score in every
// ranked game, each in a department, team and hero of their own so grouped
// rankings tie in the same way individual ones do.
type rankingFixture struct {
	pool                              *pgxpool.Pool
	server                            *httptest.Server
	cookie, tag, slugDesc, slugAsc    string
	gameDesc, gameAsc, realm, defense uuid.UUID
	realmVersion, defenseVersion      uuid.UUID
}

func newRankingFixture(t *testing.T) rankingFixture {
	t.Helper()
	pool := migratedPool(t)
	ctx := context.Background()
	f := rankingFixture{pool: pool, tag: uuid.NewString()[:8]}
	f.slugDesc, f.slugAsc = "rank-desc-"+f.tag, "rank-asc-"+f.tag

	viewer := insertTestUser(t, pool, "user")
	f.cookie = insertTestSession(t, pool, viewer)

	f.gameDesc = insertTestGame(t, pool, f.slugDesc)
	f.gameAsc = insertTestGame(t, pool, f.slugAsc)
	if _, err := pool.Exec(ctx, `UPDATE games SET score_order='asc' WHERE id=$1`, f.gameAsc); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM games WHERE slug=$1`, realmGuardSlug).Scan(&f.realm); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM realmguard_content_versions WHERE status='published'`).Scan(&f.realmVersion); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM games WHERE slug='office-guardians'`).Scan(&f.defense); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM defense_content_versions WHERE game_id=$1 AND status='published'`, f.defense).Scan(&f.defenseVersion); err != nil {
		t.Fatal(err)
	}

	// Every player this fixture creates carries the tag in their username;
	// deleting them cascades to their sessions, scores and results.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE username LIKE $1`, f.userPattern())
	})
	if _, err := pool.Exec(ctx, `INSERT INTO users(username,display_name,department,team,role,status,password_hash)
		SELECT 'rank-pg-'||$1||'-'||lpad(g::text,4,'0'),'Rank test','dept-'||$1||'-'||lpad(g::text,4,'0'),'team-'||$1||'-'||lpad(g::text,4,'0'),'user','active','' FROM generate_series(1,$2::int) g`, f.tag, tiedPlayers); err != nil {
		t.Fatal(err)
	}
	f.post(t, f.userPattern(), tiedScore, tiedScore, 3)

	// Planner statistics are what decide how the players are aggregated, and
	// a production table has them.
	if _, err := pool.Exec(ctx, `ANALYZE users; ANALYZE game_sessions; ANALYZE scores; ANALYZE realmguard_results; ANALYZE defense_results`); err != nil {
		t.Fatal(err)
	}

	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

func (f rankingFixture) userPattern() string { return "rank-pg-" + f.tag + "-%" }

// post records one finished session with the score, and the matching
// RealmGuard and Defense results, for every user matching the pattern. The
// game ranked ascending gets its own score, since a low one is a win there.
// All rows share one created_at so the RealmGuard individual ranking, which
// already breaks ties on it, still faces a full tie.
func (f rankingFixture) post(t *testing.T, pattern string, score, ascScore int64, stars int) {
	t.Helper()
	ctx := context.Background()
	for _, game := range []uuid.UUID{f.gameDesc, f.gameAsc, f.realm, f.defense} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO game_sessions(user_id,game_id,session_token_hash,status,started_at,ended_at,duration_ms)
			SELECT u.id,$2::uuid,sha256((u.id::text||$2::uuid::text)::bytea),'finished','2026-01-01T00:00:00Z','2026-01-01T00:00:01Z',1000 FROM users u WHERE u.username LIKE $1`, pattern, game); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO scores(user_id,game_id,session_id,score,created_at)
		SELECT gs.user_id,gs.game_id,gs.id,CASE WHEN gs.game_id=$4 THEN $6::bigint ELSE $2::bigint END,'2026-01-01T00:00:00Z' FROM game_sessions gs JOIN users u ON u.id=gs.user_id WHERE u.username LIKE $1 AND gs.game_id IN ($3,$4,$5)`, pattern, score, f.gameDesc, f.gameAsc, f.realm, ascScore); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO realmguard_results(session_id,user_id,content_version_id,stage_id,mode,difficulty,duration_ms,remaining_lives,remaining_gold,earned_gold,spent_gold,sold_gold,kills,escaped,spawned,waves_completed,hero_id,hero_level,score,stars,created_at)
		SELECT gs.id,gs.user_id,$3,'rank-stage','campaign','normal',1000,1,0,0,0,0,0,0,0,1,replace(u.username,'rank-pg-','hero-'),1,$4,$5,'2026-01-01T00:00:00Z' FROM game_sessions gs JOIN users u ON u.id=gs.user_id WHERE u.username LIKE $1 AND gs.game_id=$2`, pattern, f.realm, f.realmVersion, score, stars); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO defense_results(session_id,user_id,game_id,content_version_id,stage_id,difficulty,duration_ms,remaining_health,remaining_resource,kills,escaped,spawned,waves_completed,victory,score,stars,policy_version,request_hash,created_at)
		SELECT gs.id,gs.user_id,$2,$3,'rank-stage','normal',1000,1,0,0,0,0,1,true,$4,$5,'test',gen_random_uuid()::text,'2026-01-01T00:00:00Z' FROM game_sessions gs JOIN users u ON u.id=gs.user_id WHERE u.username LIKE $1 AND gs.game_id=$2`, pattern, f.defense, f.defenseVersion, score, stars); err != nil {
		t.Fatal(err)
	}
}

// rankedPage fetches one page of a ranking through the real router.
func (f rankingFixture) rankedPage(t *testing.T, path string) []map[string]any {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	response, err := f.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, response.StatusCode)
	}
	return body.Items
}

// rankingCase names one ranking page and the field whose ascending order is
// the tiebreaker the query promises for it.
type rankingCase struct {
	name, path, tiebreaker, measure string
}

func rankingCases(f rankingFixture) []rankingCase {
	const page = "&limit=5"
	return []rankingCase{
		{"catalog individual", "/api/v1/rankings?game_id=" + f.slugDesc + page, "user_id", "score"},
		{"catalog individual ascending", "/api/v1/rankings?game_id=" + f.slugAsc + page, "user_id", "score"},
		{"catalog department", "/api/v1/rankings?game_id=" + f.slugDesc + "&group=department" + page, "name", "score"},
		{"catalog team", "/api/v1/rankings?game_id=" + f.slugDesc + "&group=team" + page, "name", "score"},
		{"defense individual", "/api/v1/defense/office-guardians/rankings?" + page[1:], "user_id", "score"},
		{"defense department", "/api/v1/defense/office-guardians/rankings?group=department" + page, "name", "score"},
		{"defense team", "/api/v1/defense/office-guardians/rankings?group=team" + page, "name", "score"},
		{"realmguard individual", "/api/v1/realmguard/rankings?" + page[1:], "user_id", "score"},
		{"realmguard department", "/api/v1/realmguard/rankings?group=department" + page, "name", "score"},
		{"realmguard department stars", "/api/v1/realmguard/rankings?group=department&metric=stars" + page, "name", "stars"},
		{"realmguard hero", "/api/v1/realmguard/rankings?group=hero" + page, "name", "score"},
	}
}

func tiebreakers(t *testing.T, tc rankingCase, items []map[string]any) []string {
	t.Helper()
	if len(items) != 5 {
		t.Fatalf("got %d rows, want a page of 5 tied players", len(items))
	}
	keys := make([]string, 0, len(items))
	for i, item := range items {
		if rank := item["rank"].(float64); int(rank) != i+1 {
			t.Fatalf("row %d carries rank %v: %v", i+1, rank, ranksOf(items))
		}
		if tc.measure == "stars" {
			if stars := item["stars"].(float64); stars != 3 {
				t.Fatalf("row %d is not one of the tied players: stars %v", i+1, stars)
			}
		} else if score := item["score"].(float64); score != tiedScore {
			t.Fatalf("row %d is not one of the tied players: score %v", i+1, score)
		}
		keys = append(keys, item[tc.tiebreaker].(string))
	}
	return keys
}

func TestRankingsBreakTiesOnTheRowsOwnIdentifier(t *testing.T) {
	// Among equal scores the page is in ascending order of the row's own
	// identifier, and the rank column counts the rows in the order they are
	// returned. Nothing but a tiebreaker in the query defines either.
	f := newRankingFixture(t)
	for _, tc := range rankingCases(f) {
		t.Run(tc.name, func(t *testing.T) {
			keys := tiebreakers(t, tc, f.rankedPage(t, tc.path))
			for i := 1; i < len(keys); i++ {
				if keys[i-1] >= keys[i] {
					t.Fatalf("tie is not broken on ascending %s: %v", tc.tiebreaker, keys)
				}
			}
		})
	}
}

func TestRankingsKeepATiedPodiumWhenAnUnrelatedScoreLands(t *testing.T) {
	// Without a tiebreaker the players come out of the aggregate in hash
	// order, and that order moves whenever the table does: a player who had
	// nothing to do with the tie posts a low score and a different one of the
	// tied players is first. Nobody's score changed, so the page must not.
	f := newRankingFixture(t)
	before := map[string][]string{}
	for _, tc := range rankingCases(f) {
		before[tc.name] = tiebreakers(t, tc, f.rankedPage(t, tc.path))
	}

	if _, err := f.pool.Exec(context.Background(), `INSERT INTO users(username,display_name,department,team,role,status,password_hash) VALUES('rank-pg-'||$1||'-late','Rank test','dept-'||$1||'-late','team-'||$1||'-late','user','active','')`, f.tag); err != nil {
		t.Fatal(err)
	}
	f.post(t, "rank-pg-"+f.tag+"-late", 1, tiedScore*2, 1)

	for _, tc := range rankingCases(f) {
		t.Run(tc.name, func(t *testing.T) {
			after := tiebreakers(t, tc, f.rankedPage(t, tc.path))
			for i := range after {
				if after[i] != before[tc.name][i] {
					t.Fatalf("row %d changed after an unrelated score: %v then %v", i+1, before[tc.name], after)
				}
			}
		})
	}
}

func ranksOf(items []map[string]any) string {
	ranks := make([]string, 0, len(items))
	for _, item := range items {
		ranks = append(ranks, fmt.Sprint(item["rank"]))
	}
	return strings.Join(ranks, ",")
}
