package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// auditCSVHeader names the columns in the exported audit trail.
var auditCSVHeader = []string{"id", "created_at", "actor_username", "actor_id", "action", "resource_type", "resource_id", "remote_addr", "user_agent", "detail"}

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
		WHERE $1='' OR u.username ILIKE '%'||$1||'%' OR a.action ILIKE '%'||$1||'%' OR a.resource_type ILIKE '%'||$1||'%' OR a.resource_id ILIKE '%'||$1||'%' OR a.remote_addr ILIKE '%'||$1||'%'
		ORDER BY a.created_at DESC`, q)
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

	writer := csv.NewWriter(w)
	if err := writer.Write(auditCSVHeader); err != nil {
		return
	}
	exported := 0
	for rows.Next() {
		var id int64
		var actor *uuid.UUID
		var username, action, typ, rid, remote, agent string
		var detail json.RawMessage
		var created time.Time
		if err := rows.Scan(&id, &actor, &username, &action, &typ, &rid, &remote, &agent, &detail, &created); err != nil {
			// The response is already committed, so the truncated file is
			// flushed and the fault recorded for the operator to find.
			s.logRequestError(r, fmt.Errorf("audit export scan: %w", err))
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
			return
		}
		exported++
		// Flushing periodically keeps a long export moving instead of buffering.
		if exported%500 == 0 {
			writer.Flush()
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
	if err := rows.Err(); err != nil {
		s.logRequestError(r, fmt.Errorf("audit export: %w", err))
	}
	writer.Flush()
	// Reading the whole audit trail is itself worth recording.
	s.audit(r, "audit.export", "audit_log", "csv", map[string]any{"query": q, "rows": exported})
}
