package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// These tests use PostgreSQL and the production router/session middleware via
// newAdminListFixture. Run with IGAME_TEST_DSN set; without it they skip.
//
// What they pin down is the meaning of GET /api/v1/games?q=... when the term
// carries a character ILIKE reads as syntax. searchPattern escapes \ % and _
// so the filter stays a plain substring search, and the tag arm of the same
// WHERE clause deliberately compares the unescaped term for an exact match.
// Both halves were only covered by comparing the helper's output, which cannot
// show that the endpoint reaches the helper at all, nor that the escaped
// pattern stays out of the tag comparison.
type catalogSearchFixture struct {
	adminListFixture
	prefix string
	// labels maps a game id to the short name used in assertions, so a
	// mismatch names the row instead of a UUID.
	labels map[string]string
}

// catalogSearchRows are the fixture rows. Each one carries a character the
// search term may contain, plus the controls that must stay out of a literal
// match. "{p}" stands in for the fixture prefix; a format verb would have to
// be escaped here, and getting that wrong plants a stray % in a control row.
var catalogSearchRows = []struct {
	id          string
	label       string
	name        string
	description string
	tags        []string
}{
	// No special character anywhere: only the prefix control may return it.
	{"30000000-0000-0000-0000-000000000001", "plain", "{p} plain", "an ordinary row", nil},
	// A literal percent sign in the name.
	{"30000000-0000-0000-0000-000000000002", "pct", "{p} 50% off", "", nil},
	// A literal underscore, and next to it the row a one-character wildcard
	// would also sweep up if the underscore were not escaped.
	{"30000000-0000-0000-0000-000000000003", "und", "{p} user_id", "", nil},
	{"30000000-0000-0000-0000-000000000004", "undwild", "{p} userXid", "", nil},
	// Backslashes, in the description rather than the name, so the second
	// substring arm is exercised too.
	{"30000000-0000-0000-0000-000000000005", "bs", "{p} backslash", `C:\Users\{p}`, nil},
	// A tag that appears in neither the name nor the description, so only the
	// tag arm can return it.
	{"30000000-0000-0000-0000-000000000006", "tag", "{p} tagged", "", []string{"{p}-tagonly"}},
	// A tag containing a percent sign. Tags are an exact match, so q=% must
	// not return this row, and the full tag must.
	{"30000000-0000-0000-0000-000000000007", "pcttag", "{p} pct tagged", "", []string{"{p}-50%-tag"}},
}

func newCatalogSearchFixture(t *testing.T) catalogSearchFixture {
	t.Helper()
	f := catalogSearchFixture{adminListFixture: newAdminListFixture(t), prefix: "catalog-search-" + uuid.NewString()}
	f.labels = map[string]string{}
	ctx := context.Background()
	expand := func(s string) string { return strings.ReplaceAll(s, "{p}", f.prefix) }
	for _, row := range catalogSearchRows {
		name := expand(row.name)
		description := expand(row.description)
		tags := make([]string, 0, len(row.tags))
		for _, tag := range row.tags {
			tags = append(tags, expand(tag))
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO games(id,slug,name,description,tags,game_url,status)
			VALUES($1,$2,$3,$4,$5,'/games/search/','active')`,
			row.id, f.prefix+"-"+row.label, name, description, tags); err != nil {
			t.Fatal(err)
		}
		f.labels[row.id] = row.label
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM games WHERE slug LIKE $1||'-%'`, f.prefix)
	})
	return f
}

// search runs one catalog query and returns the labels of the fixture rows it
// returned, sorted. Rows seeded by the migrations share the schema, so the
// comparison is narrowed to the rows this fixture owns.
func (f catalogSearchFixture) search(t *testing.T, q string) []string {
	t.Helper()
	query := url.Values{"q": {q}, "limit": {"200"}}.Encode()
	path := "/api/v1/games?" + query
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
		t.Fatalf("GET %s (q=%q): status %d, want 200", path, q, res.StatusCode)
	}
	var out struct {
		Items  []map[string]any `json:"items"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Items == nil || out.Limit != 200 || out.Offset != 0 {
		t.Fatalf("GET %s (q=%q): invalid page envelope: %+v", path, q, out)
	}
	// The term has to survive query encoding, or the handler filters on
	// something else and the assertions below pass for the wrong reason.
	if got := res.Request.URL.Query().Get("q"); got != q {
		t.Fatalf("q round-tripped as %q, want %q", got, q)
	}
	labels := []string{}
	for _, item := range out.Items {
		id, _ := item["id"].(string)
		if label, ok := f.labels[id]; ok {
			labels = append(labels, label)
		}
	}
	slices.Sort(labels)
	return labels
}

func (f catalogSearchFixture) assertSearch(t *testing.T, q string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := f.search(t, q); !slices.Equal(got, want) {
		t.Errorf("q=%q returned %v, want %v", q, got, want)
	}
}

// TestCatalogSearchLiteralSpecials fixes that % _ and \ in q are matched as
// themselves rather than as ILIKE syntax.
func TestCatalogSearchLiteralSpecials(t *testing.T) {
	f := newCatalogSearchFixture(t)

	// The control first: without it, an empty result everywhere would look
	// like a passing escape contract.
	t.Run("prefix returns every fixture row", func(t *testing.T) {
		want := make([]string, 0, len(catalogSearchRows))
		for _, row := range catalogSearchRows {
			want = append(want, row.label)
		}
		f.assertSearch(t, f.prefix, want...)
	})

	t.Run("percent matches only a literal percent", func(t *testing.T) {
		f.assertSearch(t, "%", "pct")
	})

	t.Run("underscore matches only a literal underscore", func(t *testing.T) {
		f.assertSearch(t, "_", "und")
	})

	t.Run("underscore in a term is not a single-character wildcard", func(t *testing.T) {
		f.assertSearch(t, "user_id", "und")
	})

	t.Run("backslash matches a literal windows path", func(t *testing.T) {
		f.assertSearch(t, `C:\Users`, "bs")
	})

	t.Run("a bare backslash is neither an error nor an escape character", func(t *testing.T) {
		f.assertSearch(t, `\`, "bs")
		// `\%` is the two literal characters, which no fixture row carries --
		// not "a backslash followed by anything", which would return pct.
		f.assertSearch(t, `\%`)
	})
}

// TestCatalogSearchTagExactMatch fixes that the tag arm of the same handler
// compares the unescaped term for equality: a whole tag matches, a prefix of
// one does not, and the escaped substring pattern never reaches it.
func TestCatalogSearchTagExactMatch(t *testing.T) {
	f := newCatalogSearchFixture(t)

	t.Run("a whole tag returns its row", func(t *testing.T) {
		f.assertSearch(t, f.prefix+"-tagonly", "tag")
	})

	t.Run("a prefix of a tag returns nothing", func(t *testing.T) {
		f.assertSearch(t, strings.TrimSuffix(f.prefix+"-tagonly", "y"))
	})

	t.Run("a tag containing a percent sign matches only in full", func(t *testing.T) {
		f.assertSearch(t, f.prefix+"-50%-tag", "pcttag")
		// The escaped pattern would never equal the stored tag, so this is
		// also where $3 leaking into the tag comparison would show.
		f.assertSearch(t, "%", "pct")
	})
}
