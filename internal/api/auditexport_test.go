package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCSVSafeCellNeutralisesSpreadsheetFormulas(t *testing.T) {
	// A user agent or resource id is attacker-influenced text that lands in a
	// cell an operator opens in Excel.
	dangerous := map[string]string{
		"=1+1":                     "'=1+1",
		"=HYPERLINK(\"http://x\")": "'=HYPERLINK(\"http://x\")",
		"+44 000":                  "'+44 000",
		"-2+3":                     "'-2+3",
		"@SUM(A1)":                 "'@SUM(A1)",
		"\tlead":                   "'\tlead",
		"\rlead":                   "'\rlead",
	}
	for input, want := range dangerous {
		if got := csvSafeCell(input); got != want {
			t.Fatalf("csvSafeCell(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestCSVSafeCellLeavesOrdinaryValuesAlone(t *testing.T) {
	for _, value := range []string{"", "auth.login", "10.0.0.9", "관리자 설정 변경", "Mozilla/5.0", "a=b", "0-day"} {
		if got := csvSafeCell(value); got != value {
			t.Fatalf("csvSafeCell(%q)=%q, want it unchanged", value, got)
		}
	}
}

func TestAuditCSVHeaderCoversEveryExportedField(t *testing.T) {
	want := []string{"id", "created_at", "actor_username", "actor_id", "action", "resource_type", "resource_id", "remote_addr", "user_agent", "detail"}
	if len(auditCSVHeader) != len(want) {
		t.Fatalf("header has %d columns, want %d", len(auditCSVHeader), len(want))
	}
	for i, column := range want {
		if auditCSVHeader[i] != column {
			t.Fatalf("column %d is %q, want %q", i, auditCSVHeader[i], column)
		}
	}
}

// fakeAuditRows plays the cursor exportAuditLogs reads, failing where told.
type fakeAuditRows struct {
	actions    []string
	failScanAt int   // 1-based row whose Scan fails; 0 never
	errAfter   error // what Err reports once the rows run out
	pos        int
}

func (f *fakeAuditRows) Next() bool {
	if f.pos >= len(f.actions) {
		return false
	}
	f.pos++
	return true
}

func (f *fakeAuditRows) Scan(dest ...any) error {
	if f.pos == f.failScanAt {
		return errors.New("unexpected EOF")
	}
	*dest[0].(*int64) = int64(f.pos)
	*dest[3].(*string) = f.actions[f.pos-1]
	*dest[8].(*json.RawMessage) = json.RawMessage(`{}`)
	*dest[9].(*time.Time) = time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	return nil
}

func (f *fakeAuditRows) Err() error {
	if f.pos >= len(f.actions) {
		return f.errAfter
	}
	return nil
}

func exportCSV(t *testing.T, rows *fakeAuditRows) ([][]string, int, error) {
	t.Helper()
	var buf bytes.Buffer
	exported, err := writeAuditCSV(&buf, rows, time.UTC, time.Date(2026, 9, 17, 9, 30, 0, 0, time.UTC), "req-1")
	records, parseErr := csv.NewReader(&buf).ReadAll()
	if parseErr != nil {
		t.Fatalf("export is not well-formed CSV: %v\n%s", parseErr, buf.String())
	}
	return records, exported, err
}

func TestWriteAuditCSVExportsEveryRow(t *testing.T) {
	records, exported, err := exportCSV(t, &fakeAuditRows{actions: []string{"auth.login", "user.update"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exported != 2 || len(records) != 3 {
		t.Fatalf("exported %d rows in %d records, want 2 in 3", exported, len(records))
	}
	for _, record := range records[1:] {
		if record[4] == auditTruncatedAction {
			t.Fatalf("a complete export must not carry a truncation record: %v", record)
		}
	}
}

func TestWriteAuditCSVMarksAScanFailure(t *testing.T) {
	// The response is already committed when the cursor fails, so the file
	// itself has to say it stopped short - a trail that merely ends looks whole.
	records, exported, err := exportCSV(t, &fakeAuditRows{actions: []string{"auth.login", "user.update", "auth.logout"}, failScanAt: 2})
	if err == nil {
		t.Fatal("expected the scan failure to be returned")
	}
	if exported != 1 {
		t.Fatalf("exported %d rows before the failure, want 1", exported)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want header + 1 row + truncation marker:\n%v", len(records), records)
	}
	assertTruncationRecord(t, records[2], 1)
}

func TestWriteAuditCSVMarksACursorError(t *testing.T) {
	records, exported, err := exportCSV(t, &fakeAuditRows{actions: []string{"auth.login", "user.update"}, errAfter: errors.New("connection reset")})
	if err == nil {
		t.Fatal("expected the cursor error to be returned")
	}
	if exported != 2 || len(records) != 4 {
		t.Fatalf("exported %d rows in %d records, want 2 rows + header + marker", exported, len(records))
	}
	assertTruncationRecord(t, records[3], 2)
}

func assertTruncationRecord(t *testing.T, record []string, rows int) {
	t.Helper()
	if len(record) != len(auditCSVHeader) {
		t.Fatalf("marker has %d columns, want %d", len(record), len(auditCSVHeader))
	}
	if record[0] != "" || record[4] != auditTruncatedAction || record[5] != "audit_log" || record[6] != "csv" {
		t.Fatalf("marker does not read as export.truncated: %v", record)
	}
	if record[1] != "2026-09-17T09:30:00Z" {
		t.Fatalf("marker created_at=%q, want the export time", record[1])
	}
	var detail struct {
		Rows      int    `json:"rows"`
		RequestID string `json:"request_id"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(record[9]), &detail); err != nil {
		t.Fatalf("marker detail is not JSON: %v (%q)", err, record[9])
	}
	if detail.Rows != rows || detail.RequestID != "req-1" || detail.Reason == "" {
		t.Fatalf("marker detail=%+v, want rows=%d request_id=req-1 and a reason", detail, rows)
	}
}
