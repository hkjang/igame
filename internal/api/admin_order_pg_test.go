package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The paged admin lists hand out rows with LIMIT/OFFSET, and a client that
// walks the offsets expects every matching row exactly once. OFFSET only
// delivers that when the ORDER BY settles on a single order. Sorting by
// created_at alone leaves rows that share a timestamp in whatever order the
// plan happens to produce, and nothing obliges PostgreSQL to produce the same
// one for the next page request — so a row can arrive on two pages while
// another arrives on none. On an audit trail that reads as "I paged to the end
// and the entry was not there".
//
// Rows share a created_at exactly when one statement writes them: now() is the
// transaction timestamp, so a bulk import, a migration seeding a catalogue, or
// one request leaving several audit entries stamps them identically. These
// tests build such a set, page through it against the real router and a real
// database, and count how often each id is handed out. Sorting on to a unique
// column is what makes the split deterministic; the ranking query already
// sorts that far (realmguard.go).
//
// Two ways of disturbing the split are used, because the lists differ in what
// reaches them. The administrator and game lists take an ordinary
// administrative write between two page requests: that rewrites the row at the
// end of the heap without touching created_at, which is all it takes for a plan
// with nothing further to sort on to place it on a page the walk has already
// passed, pushing a row it had not reached yet back past an offset already
// read. Nothing in production updates an audit entry, so the trail is walked a
// single row per request instead — each offset hands the planner a different
// bound to sort under, and under a tie it answers several offsets with the same
// entry while never answering with the ones behind it.
//
// They are skipped when IGAME_TEST_DSN is unset; `make test-db DSN=...` runs
// them.

// pageIDs reads one page of a paged admin list as the signed-in administrator
// and returns the id of each item in the order the page presented them.
//
// The ids arrive as raw JSON because the lists do not agree on their type —
// users and games carry a uuid string, audit entries a bigint — and the quotes
// are stripped so that both read as the plain identifier.
func (f adminListFixture) pageIDs(t *testing.T, path string) []string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d, want 200", path, response.StatusCode)
	}
	var out struct {
		Items []struct {
			ID json.RawMessage `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil && err != io.EOF {
		t.Fatalf("GET %s: decode response: %v", path, err)
	}
	ids := make([]string, 0, len(out.Items))
	for _, item := range out.Items {
		ids = append(ids, strings.Trim(string(item.ID), `"`))
	}
	return ids
}

// walkOneRowAtATime pages through count rows of a list a single row per request
// and reports how many times each id was handed out. Every offset is a separate
// query with its own bound, which is what a client stepping through the list
// does and what the planner is free to answer inconsistently under a tie.
func (f adminListFixture) walkOneRowAtATime(t *testing.T, path string, count int) map[string]int {
	t.Helper()
	seen := map[string]int{}
	for offset := 0; offset < count; offset++ {
		for _, id := range f.pageIDs(t, path+"&limit=1&offset="+strconv.Itoa(offset)) {
			seen[id]++
		}
	}
	return seen
}

// assertEachRowHandedOutOnce checks a completed walk: every row of the set
// appeared on exactly one page, and no page produced anything else.
func assertEachRowHandedOutOnce(t *testing.T, label string, want []string, seen map[string]int) {
	t.Helper()
	duplicated, missing, unexpected := []string{}, []string{}, []string{}
	wanted := map[string]bool{}
	for _, id := range want {
		wanted[id] = true
		switch seen[id] {
		case 0:
			missing = append(missing, id)
		case 1:
		default:
			duplicated = append(duplicated, id)
		}
	}
	for id := range seen {
		if !wanted[id] {
			unexpected = append(unexpected, id)
		}
	}
	sort.Strings(duplicated)
	sort.Strings(missing)
	sort.Strings(unexpected)
	if len(duplicated) > 0 || len(missing) > 0 {
		t.Errorf("%s: paging %d rows that share a created_at handed %d of them out on more than one page and never handed out %d; duplicated=%v missing=%v",
			label, len(want), len(duplicated), len(missing), duplicated, missing)
	}
	if len(unexpected) > 0 {
		t.Errorf("%s: the pages carried rows from outside the filtered set: %v", label, unexpected)
	}
}

// requireSharedTimestamps fails unless some group of rows really does tie on
// created_at. Without it these tests could pass by never reaching the condition
// they are about.
func (f adminListFixture) requireSharedTimestamps(t *testing.T, what, query string, args ...any) {
	t.Helper()
	var largest int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&largest); err != nil {
		t.Fatal(err)
	}
	if largest < 2 {
		t.Fatalf("no %s share a created_at (largest group is %d row); the tie these tests need is absent", what, largest)
	}
}

// insertUsersSharingOneTimestamp writes count accounts in a single statement so
// that every row takes the same created_at. One statement is what guarantees
// the tie: calling insertTestUser in a loop would stamp each row differently.
//
// Every username carries the caller's prefix, so the list's q filter can narrow
// a page to exactly these rows and nothing else the shared pool holds has to be
// accounted for.
func (f adminListFixture) insertUsersSharingOneTimestamp(t *testing.T, prefix string, count int) []string {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM users WHERE username LIKE $1||'-%'`, prefix)
	})
	rows, err := f.pool.Query(ctx, `INSERT INTO users(username,display_name,role,status,password_hash)
		SELECT $1||'-'||i,'Tiebreak '||i,'user','active','old-hash' FROM generate_series(1,$2) AS g(i) RETURNING id`, prefix, count)
	if err != nil {
		t.Fatalf("insert users sharing one timestamp: %v", err)
	}
	ids := []string{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id.String())
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != count {
		t.Fatalf("inserted %d users, want %d", len(ids), count)
	}
	f.requireSharedTimestamps(t, "inserted users",
		`SELECT coalesce(max(n),0) FROM (SELECT count(*) n FROM users WHERE username LIKE $1||'-%' GROUP BY created_at) g`, prefix)
	return ids
}

// A page split has to survive a write landing between two page requests, which
// on a list an operator is working through is the ordinary case.
func TestAdminUserListHandsOutEachRowOnceWhenTimestampsTie(t *testing.T) {
	f := newAdminListFixture(t)
	prefix := "tiebreak-" + uuid.NewString()
	created := f.insertUsersSharingOneTimestamp(t, prefix, 6)

	path := "/api/v1/admin/users?q=" + prefix + "&limit=2&offset="
	seen := map[string]int{}
	patched := false
	for _, offset := range []string{"0", "2", "4"} {
		for _, id := range f.pageIDs(t, path+offset) {
			seen[id]++
		}
		if patched {
			continue
		}
		// An audited administrative write on a row the walk has already been
		// shown. PATCH leaves created_at alone and moves the row to the end of
		// the heap, so under a tie it reappears on a later page and pushes a
		// row the walk had not reached yet back past an offset already read.
		for _, id := range created {
			if seen[id] > 0 {
				f.patchUser(t, uuid.MustParse(id), "moved mid-walk")
				patched = true
				break
			}
		}
	}
	if !patched {
		t.Fatal("no row was rewritten during the walk; the test did not exercise what it is about")
	}
	assertEachRowHandedOutOnce(t, "GET /api/v1/admin/users", created, seen)
}

// The audit trail is the list where a row that never surfaces matters most, and
// one request can leave several entries, which then tie exactly. The entries
// are written directly because no production path writes more than one per
// transaction; the read under test is the real list endpoint.
func TestAdminAuditListHandsOutEachRowOnceWhenTimestampsTie(t *testing.T) {
	f := newAdminListFixture(t)
	const count = 8
	token := "tiebreak-" + uuid.NewString()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO audit_logs(actor_id,action,resource_type,resource_id,detail)
		SELECT $1,'user.update','user',$2,'{}'::jsonb FROM generate_series(1,$3) AS g(i)`, f.admin, token, count); err != nil {
		t.Fatalf("insert audit entries sharing one timestamp: %v", err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE resource_id=$1`, token) })
	f.requireSharedTimestamps(t, "inserted audit entries",
		`SELECT coalesce(max(n),0) FROM (SELECT count(*) n FROM audit_logs WHERE resource_id=$1 GROUP BY created_at) g`, token)

	want := []string{}
	rows, err := f.pool.Query(ctx, `SELECT id FROM audit_logs WHERE resource_id=$1`, token)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		want = append(want, strconv.FormatInt(id, 10))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(want) != count {
		t.Fatalf("inserted %d audit entries, want %d", len(want), count)
	}

	// q narrows the trail to exactly these entries, so the walk is unaffected
	// by whatever else the shared pool has audited.
	seen := f.walkOneRowAtATime(t, "/api/v1/admin/audit?q="+token, count)
	assertEachRowHandedOutOnce(t, "GET /api/v1/admin/audit", want, seen)
}

// insertGamesSharingOneTimestamp writes count catalogue entries in a single
// statement so that every row takes the same created_at, and returns their ids
// in slug order. The slugs carry the caller's prefix: the game list takes no
// search filter, so the walk relies on these rows being the newest in the
// table, and the prefix is what lets the update during the walk address one of
// them by the slug it has to send back unchanged.
func (f adminListFixture) insertGamesSharingOneTimestamp(t *testing.T, prefix string, count int) ([]string, []string) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM games WHERE slug LIKE $1||'-%'`, prefix)
	})
	rows, err := f.pool.Query(ctx, `INSERT INTO games(slug,name,game_url,game_type,status)
		SELECT $1||'-'||i,'Tiebreak '||i,'/games/tiebreak/','iframe','active' FROM generate_series(1,$2) AS g(i) RETURNING id,slug`, prefix, count)
	if err != nil {
		t.Fatalf("insert games sharing one timestamp: %v", err)
	}
	ids, slugs := []string{}, []string{}
	for rows.Next() {
		var id uuid.UUID
		var slug string
		if err := rows.Scan(&id, &slug); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id.String())
		slugs = append(slugs, slug)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != count {
		t.Fatalf("inserted %d games, want %d", len(ids), count)
	}
	f.requireSharedTimestamps(t, "inserted games",
		`SELECT coalesce(max(n),0) FROM (SELECT count(*) n FROM games WHERE slug LIKE $1||'-%' GROUP BY created_at) g`, prefix)
	return ids, slugs
}

// putGame performs one administrative game update, sending the slug back
// unchanged so that the write is an edit rather than a rename.
func (f adminListFixture) putGame(t *testing.T, id, slug, name string) {
	t.Helper()
	body := `{"slug":"` + slug + `","name":"` + name + `","game_url":"/games/tiebreak/","game_type":"iframe","status":"active"}`
	request, err := http.NewRequest(http.MethodPut, f.server.URL+"/api/v1/admin/games/"+id, strings.NewReader(body))
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
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("PUT game %s: status %d, want 204; body %s", id, response.StatusCode, raw)
	}
}

// The game list is disturbed the same way the administrator list is, by an
// ordinary administrative write between two page requests. It takes no search
// filter, so the walk leans on the inserted rows being the newest in the table
// and therefore the ones the first pages carry.
func TestAdminGameListHandsOutEachRowOnceWhenTimestampsTie(t *testing.T) {
	f := newAdminListFixture(t)
	prefix := "tiebreak-" + uuid.NewString()
	created, slugs := f.insertGamesSharingOneTimestamp(t, prefix, 6)
	bySlug := map[string]string{}
	for i, id := range created {
		bySlug[id] = slugs[i]
	}

	seen := map[string]int{}
	updated := false
	for _, offset := range []string{"0", "2", "4"} {
		for _, id := range f.pageIDs(t, "/api/v1/admin/games?limit=2&offset="+offset) {
			seen[id]++
		}
		if updated {
			continue
		}
		for _, id := range created {
			if seen[id] > 0 {
				f.putGame(t, id, bySlug[id], "Moved mid-walk")
				updated = true
				break
			}
		}
	}
	if !updated {
		t.Fatal("no row was rewritten during the walk; the test did not exercise what it is about")
	}
	assertEachRowHandedOutOnce(t, "GET /api/v1/admin/games", created, seen)
}
