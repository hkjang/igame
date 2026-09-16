package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// auditCSVHeader names the columns in the exported audit trail.
var auditCSVHeader = []string{"id", "created_at", "actor_username", "actor_id", "action", "resource_type", "resource_id", "remote_addr", "user_agent", "detail"}

// auditTruncatedAction is the action of the closing record an export writes
// when it stops before the trail ends, so the file cannot pass for complete.
const auditTruncatedAction = "export.truncated"

// auditRows is the slice of pgx.Rows the export reads. Naming it lets a test
// feed the writer a cursor that fails part-way without a database behind it.
type auditRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// csvSafeCell defuses a value a spreadsheet would otherwise run as a formula.
//
// Audit rows carry attacker-influenced text — user agents and resource ids —
// and Excel treats a leading =, +, - or @ as the start of a formula. Prefixing
// an apostrophe keeps the cell literal without changing what it says.
func csvSafeCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

// exportAuditLogs streams the whole filtered audit trail as CSV.
//
// It deliberately ignores limit/offset: a partial export of an audit log that
// looks complete is worse than no export. Rows are written as they arrive from
// PostgreSQL so memory stays flat regardless of how much history exists.
func (s *Server) exportAuditLogs(w http.ResponseWriter, r *http.Request, q string) {
	rows, err := s.DB.Query(r.Context(), `SELECT a.id,a.actor_id,COALESCE(u.username,''),a.action,a.resource_type,a.resource_id,a.remote_addr,a.user_agent,a.detail,a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id
		WHERE $1='' OR u.username ILIKE $1 OR a.action ILIKE $1 OR a.resource_type ILIKE $1 OR a.resource_id ILIKE $1 OR a.remote_addr ILIKE $1
		ORDER BY a.created_at DESC`, searchPattern(q))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()

	location := s.serviceLocation(r.Context())
	filename := "igame-audit-" + s.Now().In(location).Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	// Excel only reads UTF-8 CSV correctly when the byte order mark is present,
	// and these exports contain Korean action descriptions.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	exported, err := writeAuditCSV(w, rows, location, s.Now(), middleware.GetReqID(r.Context()))
	if err != nil {
		s.logRequestError(r, fmt.Errorf("audit export: %w", err))
		// Reading the whole audit trail is itself worth recording, and so is
		// the fact that this read did not get the whole of it.
		s.audit(r, "audit.export", "audit_log", "csv", map[string]any{"query": q, "rows": exported, "truncated": true})
		// The status line already promised a file, so the one signal left
		// that the client did not get one is to drop the connection before
		// the final chunk: a browser then reports a failed download instead
		// of filing a partial trail away as the whole of it. Non-browser
		// clients keep what arrived, which ends in the truncation record.
		panic(http.ErrAbortHandler)
	}
	s.audit(r, "audit.export", "audit_log", "csv", map[string]any{"query": q, "rows": exported})
}

// writeAuditCSV writes the header and one record per row, flushing as it goes,
// and reports how many rows were written.
//
// When the cursor fails part-way the file is closed with an export.truncated
// record naming how many rows preceded it and the request to look up in the
// server log, and the failure is returned for the caller to record.
func writeAuditCSV(w io.Writer, rows auditRows, location *time.Location, now time.Time, requestID string) (int, error) {
	writer := csv.NewWriter(w)
	if err := writer.Write(auditCSVHeader); err != nil {
		return 0, err
	}
	flush := func() {
		writer.Flush()
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	exported := 0
	var failure error
	for rows.Next() {
		var id int64
		var actor *uuid.UUID
		var username, action, typ, rid, remote, agent string
		var detail json.RawMessage
		var created time.Time
		if err := rows.Scan(&id, &actor, &username, &action, &typ, &rid, &remote, &agent, &detail, &created); err != nil {
			failure = fmt.Errorf("scan: %w", err)
			break
		}
		actorID := ""
		if actor != nil {
			actorID = actor.String()
		}
		record := []string{
			fmt.Sprint(id),
			created.In(location).Format(time.RFC3339),
			csvSafeCell(username),
			actorID,
			csvSafeCell(action),
			csvSafeCell(typ),
			csvSafeCell(rid),
			csvSafeCell(remote),
			csvSafeCell(agent),
			csvSafeCell(strings.TrimSpace(string(detail))),
		}
		if err := writer.Write(record); err != nil {
			return exported, err
		}
		exported++
		// Flushing periodically keeps a long export moving instead of buffering.
		if exported%500 == 0 {
			flush()
		}
	}
	if failure == nil {
		if err := rows.Err(); err != nil {
			failure = err
		}
	}
	if failure != nil {
		detail, _ := json.Marshal(map[string]any{"rows": exported, "request_id": requestID, "reason": "the database stopped answering before the export completed; see the server log"})
		_ = writer.Write([]string{"", now.In(location).Format(time.RFC3339), "", "", auditTruncatedAction, "audit_log", "csv", "", "", string(detail)})
	}
	flush()
	if failure == nil {
		failure = writer.Error()
	}
	return exported, failure
}
