package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The export answers 200 before it can know whether the trail will come out
// whole, so when the database went away mid-stream the file used to simply
// end — and a trail that ends looks complete. Whether the connection drop
// actually reaches the client is net/http's decision and whether the cursor
// fails is PostgreSQL's, so this drives the real router over TCP and kills
// the exporting backend from a second connection while the client is reading.
func TestAuditExportDoesNotPassOffATruncatedTrailAsComplete(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()
	admin := insertTestUser(t, pool, "admin")
	cookie := insertTestSession(t, pool, admin)

	// Enough bytes that PostgreSQL cannot have handed the whole result over
	// before the client stops reading: the server blocks on the client socket,
	// the backend blocks on the server, and the trail is still open to be cut.
	marker := "audit-export-pg-" + uuid.NewString()
	agent := strings.Repeat("x", 8000)
	if _, err := pool.Exec(ctx, `INSERT INTO audit_logs(action,resource_type,resource_id,remote_addr,user_agent,detail) SELECT 'export.filler','test',$1,'',$2,'{}' FROM generate_series(1,4000)`, marker, agent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE resource_id=$1 OR actor_id=$2`, marker, admin)
	})

	server := httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(server.Close)
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/admin/audit?format=csv&q="+marker, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200 before the trail is cut", response.StatusCode)
	}

	// Once the first bytes arrive the export is under way; the client then
	// stops reading and the backend serving it is terminated.
	var body bytes.Buffer
	if _, err := io.CopyN(&body, response.Body, 64); err != nil {
		t.Fatalf("reading the start of the export: %v", err)
	}
	terminated := false
	for attempt := 0; attempt < 50 && !terminated; attempt++ {
		var killed int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND state='active' AND query LIKE '%FROM audit_logs a LEFT JOIN users u%') k`).Scan(&killed); err != nil {
			t.Fatal(err)
		}
		terminated = killed > 0
		if !terminated {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !terminated {
		t.Fatal("could not find the exporting backend to terminate")
	}

	// The rest of the body must not read as a clean end: the connection drops
	// before the final chunk, so a browser would report a failed download.
	_, err = io.Copy(&body, response.Body)
	if err == nil {
		t.Fatalf("the download completed as if the trail were whole (%d bytes)", body.Len())
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("download ended with %v, want an unexpected EOF", err)
	}

	// What did arrive ends in the truncation record, for a client that keeps
	// partial downloads.
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(body.Bytes(), []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	if err != nil {
		t.Fatalf("partial export is not well-formed CSV: %v", err)
	}
	if len(records) < 2 || len(records) > 4000 {
		t.Fatalf("got %d records, want a header, some rows and the marker but not the whole trail", len(records))
	}
	last := records[len(records)-1]
	if last[4] != auditTruncatedAction {
		t.Fatalf("last record is %v, want %s", last, auditTruncatedAction)
	}
	if !strings.Contains(last[9], `"rows":`+strconv.Itoa(len(records)-2)) {
		t.Fatalf("marker detail %q does not count the %d rows before it", last[9], len(records)-2)
	}

	// The audit trail records that this export did not get all of it.
	var truncated bool
	var rows int
	if err := pool.QueryRow(ctx, `SELECT COALESCE((detail->>'truncated')::boolean,false),(detail->>'rows')::int FROM audit_logs WHERE action='audit.export' AND actor_id=$1 ORDER BY id DESC LIMIT 1`, admin).Scan(&truncated, &rows); err != nil {
		t.Fatalf("no audit.export entry was recorded: %v", err)
	}
	if !truncated || rows != len(records)-2 {
		t.Fatalf("audit.export recorded truncated=%v rows=%d, want true and %d", truncated, rows, len(records)-2)
	}
}
