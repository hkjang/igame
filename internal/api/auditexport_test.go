package api

import "testing"

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
