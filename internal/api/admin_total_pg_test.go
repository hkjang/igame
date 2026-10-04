package api

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The paged admin lists answer with the count of everything the filter matched
// next to the page itself, so that a client can work out how many pages there
// are (docs/api.md). That total rides along as count(*) OVER() on each returned
// row, which means a page with no rows carries no count and the handler reports
// zero matches.
//
// Nothing in the response separates that zero from a filter that genuinely
// matched nothing, so an offset past the end reads as an empty result set. The
// bundled console walks itself back a page when it sees one
// (web/src/pages/admin/AdminResourcePage.tsx), but while it does it shows the
// count as zero and disables the audit CSV export, which it gates on the total;
// a script or SDK caller paging by the reported total has nothing to walk back
// with and stops at the first page it overshoots.
//
// These tests drive the real router against a real database so the total they
// read is the one a client reads, and they check the audit list and the CSV
// export of the same filter against each other — the two select the same rows
// through separate SQL and must agree on which rows those are. They are skipped
// when IGAME_TEST_DSN is unset; `make test-db DSN=...` runs them.

type adminListFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	admin  uuid.UUID
	cookie string
}

func newAdminListFixture(t *testing.T) adminListFixture {
	t.Helper()
	pool := migratedPool(t)
	admin := insertTestUser(t, pool, "admin")
	f := adminListFixture{pool: pool, admin: admin, cookie: insertTestSession(t, pool, admin)}
	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// list calls a paged admin endpoint as the signed-in administrator.
func (f adminListFixture) list(t *testing.T, path string) (int, int64, int) {
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
	var out struct {
		Items []json.RawMessage `json:"items"`
		Total int64             `json:"total"`
	}
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil && err != io.EOF {
		t.Fatalf("GET %s: decode response: %v", path, err)
	}
	return response.StatusCode, out.Total, len(out.Items)
}

// patchUser performs one audited administrative write.
func (f adminListFixture) patchUser(t *testing.T, target uuid.UUID, displayName string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPatch, f.server.URL+"/api/v1/admin/users/"+target.String(), strings.NewReader(`{"display_name":"`+displayName+`"}`))
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
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH user %s: status %d, want 204", target, response.StatusCode)
	}
}

// exportedAuditRows counts the data rows of the streamed CSV export, leaving
// out the header and the truncation record a failed read appends.
func (f adminListFixture) exportedAuditRows(t *testing.T, query string) int {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, f.server.URL+"/api/v1/admin/audit?format=csv&q="+query, nil)
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
		t.Fatalf("GET audit CSV: status %d, want 200", response.StatusCode)
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read audit CSV: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"))).ReadAll()
	if err != nil {
		t.Fatalf("parse audit CSV: %v", err)
	}
	if len(records) == 0 || records[0][0] != auditCSVHeader[0] {
		t.Fatalf("audit CSV has no header row: %q", string(raw))
	}
	rows := 0
	for _, record := range records[1:] {
		if record[4] == auditTruncatedAction {
			t.Fatalf("audit CSV export was truncated: %v", record)
		}
		rows++
	}
	return rows
}

func TestAdminUserListReportsTheTotalOnAPagePastTheEnd(t *testing.T) {
	f := newAdminListFixture(t)
	for i := 0; i < 3; i++ {
		insertTestUser(t, f.pool, "user")
	}

	status, total, items := f.list(t, "/api/v1/admin/users?limit=2&offset=0")
	if status != http.StatusOK || total != 4 || items != 2 {
		t.Fatalf("first page: status %d total %d items %d, want 200 total 4 items 2", status, total, items)
	}

	status, total, items = f.list(t, "/api/v1/admin/users?limit=2&offset=10")
	if status != http.StatusOK || items != 0 {
		t.Fatalf("page past the end: status %d items %d, want 200 with no items", status, items)
	}
	if total != 4 {
		t.Errorf("page past the end: total %d, want 4; a client reads this as a filter that matched nothing", total)
	}
}

func TestAdminAuditListReportsTheTotalOnAPagePastTheEnd(t *testing.T) {
	f := newAdminListFixture(t)
	target := insertTestUser(t, f.pool, "user")
	for _, name := range []string{"one", "two", "three"} {
		f.patchUser(t, target, name)
	}

	query := target.String()
	status, total, items := f.list(t, "/api/v1/admin/audit?limit=2&offset=0&q="+query)
	if status != http.StatusOK || total != 3 || items != 2 {
		t.Fatalf("first page: status %d total %d items %d, want 200 total 3 items 2", status, total, items)
	}

	status, total, items = f.list(t, "/api/v1/admin/audit?limit=2&offset=10&q="+query)
	if status != http.StatusOK || items != 0 {
		t.Fatalf("page past the end: status %d items %d, want 200 with no items", status, items)
	}
	if total != 3 {
		t.Errorf("page past the end: total %d, want 3; a client reads this as a filter that matched nothing, and the console disables the CSV export on it", total)
	}
}

// The audit list and the audit CSV export answer the same q through separate
// SQL. A filter that selected different rows in each would hand an operator an
// export that does not hold what the screen showed.
func TestAdminAuditListAndExportSelectTheSameRows(t *testing.T) {
	f := newAdminListFixture(t)
	target := insertTestUser(t, f.pool, "user")
	other := insertTestUser(t, f.pool, "user")
	for _, name := range []string{"one", "two", "three"} {
		f.patchUser(t, target, name)
	}
	f.patchUser(t, other, "unrelated")

	for _, query := range []string{target.String(), "user.update", ""} {
		status, total, _ := f.list(t, "/api/v1/admin/audit?limit=200&offset=0&q="+query)
		if status != http.StatusOK {
			t.Fatalf("q=%q: list status %d, want 200", query, status)
		}
		if exported := f.exportedAuditRows(t, query); int64(exported) != total {
			t.Errorf("q=%q: the list reports %d rows and the export writes %d", query, total, exported)
		}
	}
}
