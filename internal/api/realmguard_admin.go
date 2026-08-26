package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errRealmGuardStaleVersion = errors.New("realmguard draft checksum is stale")

func validRealmGuardSection(section string) bool {
	return slices.Contains(realmGuardSections, section)
}

func (s *Server) realmGuardDraftVersion(ctx context.Context, requested string) (realmGuardVersionRecord, error) {
	if requested != "" {
		id, err := uuid.Parse(requested)
		if err != nil {
			return realmGuardVersionRecord{}, fmt.Errorf("invalid version_id")
		}
		return s.loadRealmGuardVersion(ctx, id)
	}
	version, err := scanRealmGuardVersion(s.DB.QueryRow(ctx, `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions WHERE status IN ('draft','testing','pending_approval','approved') ORDER BY version_no DESC LIMIT 1`))
	if err != nil {
		return version, err
	}
	return s.normalizeRealmGuardChecksum(ctx, version), nil
}

func (s *Server) getRealmGuardDraftSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if !validRealmGuardSection(section) {
		writeError(w, 404, "unknown_section", "unknown RealmGuard designer section")
		return
	}
	version, err := s.realmGuardDraftVersion(r.Context(), r.URL.Query().Get("version_id"))
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, 404, "draft_not_found", "create a RealmGuard draft version first")
			return
		}
		writeError(w, 400, "invalid_version", err.Error())
		return
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(version.RawContent, &document); err != nil {
		writeError(w, 500, "invalid_content", err.Error())
		return
	}
	if len(document[section]) == 0 {
		writeError(w, 500, "invalid_content", "stored content is missing the requested section")
		return
	}
	w.Header().Set("ETag", `"`+version.Checksum+`"`)
	writeJSON(w, 200, map[string]any{"version": realmGuardVersionJSON(version), "section": section, "data": document[section]})
}

func realmGuardExpectedChecksum(w http.ResponseWriter, r *http.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if value == "" {
		writeError(w, http.StatusPreconditionRequired, "precondition_required", "If-Match with the draft version checksum is required")
		return "", false
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "W/"))
	value = strings.Trim(value, `"`)
	if len(value) != 64 {
		writeError(w, 400, "invalid_precondition", "If-Match must contain one SHA-256 draft checksum")
		return "", false
	}
	for _, ch := range value {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F') {
			writeError(w, 400, "invalid_precondition", "If-Match must contain one SHA-256 draft checksum")
			return "", false
		}
	}
	return strings.ToLower(value), true
}

func decodeRealmGuardSectionBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	var raw json.RawMessage
	if !decodeJSON(w, r, &raw) {
		return nil, false
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &envelope) == nil && len(envelope.Data) > 0 {
		raw = envelope.Data
	}
	if len(raw) == 0 || !json.Valid(raw) {
		writeError(w, 400, "invalid_section", "section data must be valid JSON")
		return nil, false
	}
	return raw, true
}

func validateRealmGuardDraftSection(section string, raw json.RawMessage) error {
	if section == "balance" {
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil || value == nil {
			return fmt.Errorf("balance section must be a JSON object")
		}
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return fmt.Errorf("%s section must be a JSON array", section)
	}
	for index, itemRaw := range items {
		var item map[string]any
		if json.Unmarshal(itemRaw, &item) != nil || item == nil {
			return fmt.Errorf("%s item %d must be a JSON object", section, index)
		}
		id, _ := item["id"].(string)
		if strings.TrimSpace(id) == "" || len(id) > 100 {
			return fmt.Errorf("%s item %d requires an id of at most 100 characters", section, index)
		}
		if section == "waves" {
			stageID, _ := item["stage_id"].(string)
			number, _ := item["number"].(float64)
			if strings.TrimSpace(stageID) == "" || number < 1 || number != float64(int(number)) {
				return fmt.Errorf("wave %s requires stage_id and a positive integer number", id)
			}
		}
	}
	return nil
}

func (s *Server) putRealmGuardDraftSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if !validRealmGuardSection(section) {
		writeError(w, 404, "unknown_section", "unknown RealmGuard designer section")
		return
	}
	expectedChecksum, ok := realmGuardExpectedChecksum(w, r)
	if !ok {
		return
	}
	data, ok := decodeRealmGuardSectionBody(w, r)
	if !ok {
		return
	}
	if err := validateRealmGuardDraftSection(section, data); err != nil {
		writeError(w, 400, "invalid_section", err.Error())
		return
	}
	version, err := s.mutateRealmGuardDraft(r.Context(), r.URL.Query().Get("version_id"), section, expectedChecksum, func(_ json.RawMessage) (json.RawMessage, error) { return data, nil })
	if err != nil {
		handleRealmGuardDesignerError(w, err)
		return
	}
	s.audit(r, "realmguard.designer.section.update", "realmguard_content_version", version.ID.String(), map[string]any{"section": section})
	var document map[string]json.RawMessage
	_ = json.Unmarshal(version.RawContent, &document)
	w.Header().Set("ETag", `"`+version.Checksum+`"`)
	writeJSON(w, 200, map[string]any{"version": realmGuardVersionJSON(version), "section": section, "data": document[section]})
}

func (s *Server) createRealmGuardDraftItem(w http.ResponseWriter, r *http.Request) {
	s.mutateRealmGuardDraftItem(w, r, "create")
}

func (s *Server) updateRealmGuardDraftItem(w http.ResponseWriter, r *http.Request) {
	s.mutateRealmGuardDraftItem(w, r, "update")
}

func (s *Server) deleteRealmGuardDraftItem(w http.ResponseWriter, r *http.Request) {
	s.mutateRealmGuardDraftItem(w, r, "delete")
}

func (s *Server) mutateRealmGuardDraftItem(w http.ResponseWriter, r *http.Request, action string) {
	section := chi.URLParam(r, "section")
	if !validRealmGuardSection(section) || section == "balance" {
		writeError(w, 400, "invalid_section", "item CRUD is available for array sections only")
		return
	}
	expectedChecksum, ok := realmGuardExpectedChecksum(w, r)
	if !ok {
		return
	}
	itemID := strings.TrimSpace(chi.URLParam(r, "itemID"))
	var input json.RawMessage
	if action != "delete" {
		if !decodeJSON(w, r, &input) {
			return
		}
		var item map[string]any
		if json.Unmarshal(input, &item) != nil {
			writeError(w, 400, "invalid_item", "item must be a JSON object")
			return
		}
		bodyID, _ := item["id"].(string)
		if action == "create" {
			itemID = bodyID
		} else if bodyID != "" && bodyID != itemID {
			writeError(w, 409, "item_id_mismatch", "body id must match the route item ID")
			return
		}
	}
	if itemID == "" || len(itemID) > 100 {
		writeError(w, 400, "invalid_item", "item id is required and must be at most 100 characters")
		return
	}
	version, err := s.mutateRealmGuardDraft(r.Context(), r.URL.Query().Get("version_id"), section, expectedChecksum, func(current json.RawMessage) (json.RawMessage, error) {
		var items []json.RawMessage
		if json.Unmarshal(current, &items) != nil {
			return nil, fmt.Errorf("section is not an array")
		}
		found := -1
		for index, raw := range items {
			var item struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(raw, &item)
			if item.ID == itemID {
				found = index
				break
			}
		}
		switch action {
		case "create":
			if found >= 0 {
				return nil, fmt.Errorf("item already exists")
			}
			items = append(items, input)
		case "update":
			if found < 0 {
				return nil, pgx.ErrNoRows
			}
			items[found] = input
		case "delete":
			if found < 0 {
				return nil, pgx.ErrNoRows
			}
			items = append(items[:found], items[found+1:]...)
		}
		return json.Marshal(items)
	})
	if err != nil {
		handleRealmGuardDesignerError(w, err)
		return
	}
	s.audit(r, "realmguard.designer.item."+action, "realmguard_content_version", version.ID.String(), map[string]any{"section": section, "item_id": itemID})
	status := http.StatusOK
	if action == "create" {
		status = http.StatusCreated
	}
	if action == "delete" {
		w.Header().Set("ETag", `"`+version.Checksum+`"`)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("ETag", `"`+version.Checksum+`"`)
	writeJSON(w, status, map[string]any{"version": realmGuardVersionJSON(version), "section": section, "item_id": itemID})
}

func (s *Server) mutateRealmGuardDraft(ctx context.Context, requested, section, expectedChecksum string, mutate func(json.RawMessage) (json.RawMessage, error)) (realmGuardVersionRecord, error) {
	var versionID uuid.UUID
	if requested != "" {
		parsed, err := uuid.Parse(requested)
		if err != nil {
			return realmGuardVersionRecord{}, fmt.Errorf("invalid version_id")
		}
		versionID = parsed
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return realmGuardVersionRecord{}, err
	}
	defer tx.Rollback(ctx)
	query := `SELECT ` + realmGuardVersionColumns + ` FROM realmguard_content_versions WHERE id=CASE WHEN $1='00000000-0000-0000-0000-000000000000'::uuid THEN (SELECT id FROM realmguard_content_versions WHERE status IN ('draft','testing') ORDER BY version_no DESC LIMIT 1) ELSE $1 END FOR UPDATE`
	version, err := scanRealmGuardVersion(tx.QueryRow(ctx, query, versionID))
	if err != nil {
		return version, err
	}
	if version.Status != "draft" && version.Status != "testing" {
		return version, fmt.Errorf("only draft or testing versions can be edited")
	}
	if !strings.EqualFold(version.Checksum, expectedChecksum) {
		return version, errRealmGuardStaleVersion
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(version.RawContent, &document) != nil {
		return version, fmt.Errorf("stored content is invalid")
	}
	updated, err := mutate(document[section])
	if err != nil {
		return version, err
	}
	document[section] = updated
	baseRevision := func(value string) string {
		if index := strings.Index(value, "-r"); index >= 0 {
			return value[:index]
		}
		return value
	}
	if section == "stages" || section == "waves" {
		var stages []map[string]any
		if err := json.Unmarshal(document["stages"], &stages); err != nil {
			return version, fmt.Errorf("stages section must be an array")
		}
		for _, stage := range stages {
			current, _ := stage["version"].(string)
			stage["version"] = fmt.Sprintf("%s-r%d", baseRevision(current), version.VersionNo)
		}
		document["stages"], _ = json.Marshal(stages)
	}
	raw, _ := json.Marshal(document)
	version.RawContent = raw
	version.Checksum = realmGuardChecksum(raw)
	version.Status = "draft"
	version.ApprovedBy, version.RequestedAt, version.ApprovedAt, version.ReviewedAt = nil, nil, nil, nil
	version.ReviewComment = ""
	switch section {
	case "stages", "waves":
		version.StageVersion = fmt.Sprintf("%s-r%d", baseRevision(version.StageVersion), version.VersionNo)
	case "balance":
		version.BalanceVersion = fmt.Sprintf("%s-r%d", baseRevision(version.BalanceVersion), version.VersionNo)
	default:
		version.ContentVersion = fmt.Sprintf("%s-r%d", baseRevision(version.ContentVersion), version.VersionNo)
	}
	err = tx.QueryRow(ctx, `UPDATE realmguard_content_versions SET status='draft',content=$2,content_version=$3,stage_version=$4,balance_version=$5,checksum=$6,approved_by=NULL,approval_requested_at=NULL,approved_at=NULL,review_comment='',reviewed_at=NULL,tested_at=NULL,updated_at=now() WHERE id=$1 RETURNING updated_at`, version.ID, raw, version.ContentVersion, version.StageVersion, version.BalanceVersion, version.Checksum).Scan(&version.UpdatedAt)
	if err != nil {
		return version, err
	}
	if err := tx.Commit(ctx); err != nil {
		return version, err
	}
	return version, nil
}

func handleRealmGuardDesignerError(w http.ResponseWriter, err error) {
	switch {
	case err == pgx.ErrNoRows:
		writeError(w, 404, "not_found", "RealmGuard draft or item was not found")
	case errors.Is(err, errRealmGuardStaleVersion):
		writeError(w, 409, "stale_version", "the draft changed; reload it and retry with the latest checksum")
	case strings.Contains(err.Error(), "only draft"):
		writeError(w, 409, "version_not_editable", err.Error())
	case strings.Contains(err.Error(), "already exists"):
		writeError(w, 409, "duplicate_item", err.Error())
	default:
		writeError(w, 400, "invalid_content", err.Error())
	}
}

func (s *Server) listRealmGuardVersions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions ORDER BY version_no DESC`)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	versions, err := collectRealmGuardVersions(rows)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	items := []map[string]any{}
	for _, version := range versions {
		items = append(items, realmGuardVersionJSON(s.normalizeRealmGuardChecksum(r.Context(), version)))
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// collectRealmGuardVersions drains the cursor before the caller issues any
// follow-up query. Querying while rows are open borrows a second pooled
// connection per row and can deadlock the pool under load.
func collectRealmGuardVersions(rows pgx.Rows) ([]realmGuardVersionRecord, error) {
	defer rows.Close()
	var versions []realmGuardVersionRecord
	for rows.Next() {
		version, err := scanRealmGuardVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Server) previewRealmGuardVersion(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := realmGuardIDParam(w, r)
	if !ok {
		return
	}
	version, err := s.loadRealmGuardVersion(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if !slices.Contains([]string{"draft", "testing", "pending_approval", "approved", "published"}, version.Status) {
		writeError(w, 409, "preview_unavailable", "this content version cannot be previewed")
		return
	}
	if p.Role == "manager" {
		creatorTeam := ""
		if version.CreatedBy != nil {
			if err := s.DB.QueryRow(r.Context(), `SELECT team FROM users WHERE id=$1`, *version.CreatedBy).Scan(&creatorTeam); err != nil {
				s.dbError(w, r, err)
				return
			}
		}
		if code, message := realmGuardManagerReviewTeamError(p.Team, creatorTeam); code != "" {
			writeError(w, 403, code, message)
			return
		}
	}
	if err := validateRealmGuardContent(version.RawContent); err != nil {
		writeError(w, 422, "content_validation_failed", err.Error())
		return
	}
	payload, err := realmGuardConfigPayload(version)
	if err != nil {
		writeError(w, 422, "invalid_content", err.Error())
		return
	}
	payload["preview"] = true
	payload["practice_only"] = true
	writeJSON(w, 200, payload)
}

type createRealmGuardVersionInput struct {
	Notes        string `json:"notes"`
	Label        string `json:"label,omitempty"`
	AssetVersion string `json:"asset_version,omitempty"`
}


// realmGuardContentVersionBase strips the `-r<n>` suffix a saved draft carries.
func realmGuardContentVersionBase(value string) string {
	if index := strings.Index(value, "-r"); index >= 0 {
		return value[:index]
	}
	return value
}

// realmGuardNextContentVersion is the content version a new draft should carry.
//
// A draft's stage, balance and asset versions are all taken from the published
// row it is cut from. This one was invented from the table's row count as
// `0.2.<n>` instead, so a draft cut from published 0.3.1 was labelled 0.2.5 —
// and publishing it moved the live content version backwards, past two releases
// that had already shipped.
//
// Every existing version is considered, not just the published one, so the
// sequence advances even while several drafts are open at once.
func realmGuardNextContentVersion(existing []string) string {
	best := []int(nil)
	for _, candidate := range existing {
		parts := strings.Split(realmGuardContentVersionBase(strings.TrimSpace(candidate)), ".")
		if len(parts) < 2 {
			continue
		}
		numbers := make([]int, 0, len(parts))
		for _, part := range parts {
			number, err := strconv.Atoi(part)
			if err != nil || number < 0 {
				numbers = nil
				break
			}
			numbers = append(numbers, number)
		}
		if numbers == nil {
			continue
		}
		if best == nil || realmGuardVersionLess(best, numbers) {
			best = numbers
		}
	}
	if best == nil {
		// Nothing on file parses as a version. Leaving it alone is better than
		// inventing a number that claims an ordering the content does not have.
		for _, candidate := range existing {
			if strings.TrimSpace(candidate) != "" {
				return realmGuardContentVersionBase(candidate)
			}
		}
		return "0.0.1"
	}
	best[len(best)-1]++
	parts := make([]string, len(best))
	for index, number := range best {
		parts[index] = strconv.Itoa(number)
	}
	return strings.Join(parts, ".")
}

func realmGuardVersionLess(left, right []int) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return len(left) < len(right)
}

func (s *Server) createRealmGuardVersion(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in createRealmGuardVersionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.Notes) > 2000 || len(in.Label) > 100 || len(in.AssetVersion) > 100 {
		writeError(w, 400, "invalid_version", "notes, label, or asset_version is too long")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('realmguard_content_versions'))`); err != nil {
		s.dbError(w, r, err)
		return
	}
	published, err := scanRealmGuardVersion(tx.QueryRow(r.Context(), `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions WHERE status='published' FOR SHARE`))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	var versionNo int
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(max(version_no),0)+1 FROM realmguard_content_versions`).Scan(&versionNo); err != nil {
		s.dbError(w, r, err)
		return
	}
	existingVersions := []string{published.ContentVersion}
	rows, err := tx.Query(r.Context(), `SELECT content_version FROM realmguard_content_versions`)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			s.dbError(w, r, err)
			return
		}
		existingVersions = append(existingVersions, value)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	contentVersion := realmGuardNextContentVersion(existingVersions)
	if strings.TrimSpace(in.Label) == "" {
		// The label column is unique, so a second draft opened against the same
		// published version needs something to tell it apart. Its version number
		// is the only thing guaranteed to be its own.
		in.Label = "v" + contentVersion
		var taken bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM realmguard_content_versions WHERE label=$1)`, in.Label).Scan(&taken); err != nil {
			s.dbError(w, r, err)
			return
		}
		if taken {
			in.Label = fmt.Sprintf("%s-%d", in.Label, versionNo)
		}
	}
	if strings.TrimSpace(in.AssetVersion) == "" {
		in.AssetVersion = published.AssetVersion
	}
	checksum := realmGuardChecksum(published.RawContent)
	var id uuid.UUID
	var created, updated time.Time
	err = tx.QueryRow(r.Context(), `INSERT INTO realmguard_content_versions(version_no,label,status,content_version,stage_version,balance_version,asset_version,checksum,notes,content,created_by)
		VALUES($1,$2,'draft',$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id,created_at,updated_at`, versionNo, in.Label, contentVersion, published.StageVersion, published.BalanceVersion, in.AssetVersion, checksum, in.Notes, published.RawContent, p.UserID).Scan(&id, &created, &updated)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	version := realmGuardVersionRecord{ID: id, VersionNo: versionNo, Label: in.Label, Status: "draft", ContentVersion: contentVersion, StageVersion: published.StageVersion, BalanceVersion: published.BalanceVersion, AssetVersion: in.AssetVersion, Checksum: checksum, Notes: in.Notes, RawContent: published.RawContent, CreatedBy: &p.UserID, CreatedAt: created, UpdatedAt: updated}
	s.audit(r, "realmguard.version.create", "realmguard_content_version", id.String(), map[string]any{"version_no": versionNo, "source": published.ID})
	writeJSON(w, 201, map[string]any{"version": realmGuardVersionJSON(version)})
}

func (s *Server) testRealmGuardVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := realmGuardIDParam(w, r)
	if !ok {
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	version, err := scanRealmGuardVersion(tx.QueryRow(r.Context(), `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if version.Status != "draft" && version.Status != "testing" {
		writeError(w, 409, "invalid_transition", "only draft or testing versions can be tested")
		return
	}
	if err := validateRealmGuardContent(version.RawContent); err != nil {
		writeError(w, 422, "content_validation_failed", err.Error())
		return
	}
	content, _ := decodeRealmGuardContent(version.RawContent)
	campaigns, endless, branches := 0, 0, 0
	for _, stage := range content.Stages {
		if stage.Mode == "campaign" {
			campaigns++
		} else if stage.Mode == "endless" {
			endless++
		}
	}
	for _, tower := range content.Towers {
		branches += len(tower.Branches)
	}
	version.Checksum = realmGuardChecksum(version.RawContent)
	var tested, updated time.Time
	err = tx.QueryRow(r.Context(), `UPDATE realmguard_content_versions SET status='testing',tested_at=now(),checksum=$2,updated_at=now() WHERE id=$1 RETURNING tested_at,updated_at`, id, version.Checksum).Scan(&tested, &updated)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	version.Status, version.TestedAt, version.UpdatedAt = "testing", &tested, updated
	s.audit(r, "realmguard.version.test", "realmguard_content_version", id.String(), map[string]any{"checksum": version.Checksum})
	writeJSON(w, 200, map[string]any{"version": realmGuardVersionJSON(version), "validation": map[string]any{"valid": true, "campaign_stages": campaigns, "endless_stages": endless, "waves": len(content.Waves), "base_towers": len(content.Towers), "advanced_towers": branches, "enemies": len(content.Enemies), "bosses": len(content.Bosses), "heroes": len(content.Heroes), "skills": len(content.Skills)}})
}

type approveRealmGuardInput struct {
	Decision string `json:"decision,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

func (s *Server) listPendingRealmGuardVersions(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	if p.Role == "manager" && strings.TrimSpace(p.Team) == "" {
		writeError(w, 403, "team_required", "a manager must belong to a team to review RealmGuard content")
		return
	}
	published, publishedErr := s.loadRealmGuardPublished(r.Context())
	rows, err := s.DB.Query(r.Context(), `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions v WHERE status='pending_approval' AND ($1::text<>'manager' OR EXISTS(SELECT 1 FROM users creator WHERE creator.id=v.created_by AND creator.team<>'' AND creator.team=$2)) ORDER BY approval_requested_at,version_no`, p.Role, p.Team)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	versions, err := collectRealmGuardVersions(rows)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	creators, err := s.realmGuardCreators(r.Context(), versions)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	items := []map[string]any{}
	for _, version := range versions {
		version = s.normalizeRealmGuardChecksum(r.Context(), version)
		item := realmGuardVersionJSON(version)
		if version.CreatedBy != nil {
			if creator, ok := creators[*version.CreatedBy]; ok {
				item["creator"] = creator
			}
		}
		if publishedErr == nil {
			item["changed_sections"] = realmGuardChangedSections(published.RawContent, version.RawContent)
			item["published_version"] = realmGuardVersionJSON(published)
		}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// realmGuardCreators resolves every author in one query instead of one per
// pending version.
func (s *Server) realmGuardCreators(ctx context.Context, versions []realmGuardVersionRecord) (map[uuid.UUID]map[string]any, error) {
	ids := make([]uuid.UUID, 0, len(versions))
	for _, version := range versions {
		if version.CreatedBy != nil && !slices.Contains(ids, *version.CreatedBy) {
			ids = append(ids, *version.CreatedBy)
		}
	}
	creators := map[uuid.UUID]map[string]any{}
	if len(ids) == 0 {
		return creators, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT id,username,display_name,team FROM users WHERE id=ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var username, displayName, team string
		if err := rows.Scan(&id, &username, &displayName, &team); err != nil {
			return nil, err
		}
		creators[id] = map[string]any{"id": id, "username": username, "display_name": displayName, "team": team}
	}
	return creators, rows.Err()
}

func realmGuardChangedSections(before, after json.RawMessage) []string {
	var beforeSections, afterSections map[string]json.RawMessage
	if json.Unmarshal(before, &beforeSections) != nil || json.Unmarshal(after, &afterSections) != nil {
		return []string{}
	}
	changed := []string{}
	for _, section := range realmGuardSections {
		if realmGuardChecksum(beforeSections[section]) != realmGuardChecksum(afterSections[section]) {
			changed = append(changed, section)
		}
	}
	return changed
}

func realmGuardManagerReviewTeamError(managerTeam, creatorTeam string) (string, string) {
	managerTeam = strings.TrimSpace(managerTeam)
	creatorTeam = strings.TrimSpace(creatorTeam)
	if managerTeam == "" || creatorTeam == "" {
		return "team_required", "manager and content creator must both belong to a team"
	}
	if managerTeam != creatorTeam {
		return "different_team", "managers can only approve content from their team"
	}
	return "", ""
}

func (s *Server) approveRealmGuardVersion(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := realmGuardIDParam(w, r)
	if !ok {
		return
	}
	var in approveRealmGuardInput
	if !decodeOptionalJSON(w, r, &in) {
		return
	}
	in.Decision = strings.ToLower(strings.TrimSpace(in.Decision))
	if in.Decision == "" {
		in.Decision = "approved"
	}
	in.Comment = strings.TrimSpace(in.Comment)
	if !slices.Contains([]string{"approved", "rejected"}, in.Decision) {
		writeError(w, 400, "invalid_decision", "decision must be approved or rejected")
		return
	}
	if in.Decision == "rejected" && in.Comment == "" {
		writeError(w, 400, "comment_required", "a rejection comment is required")
		return
	}
	if len(in.Comment) > 2000 {
		writeError(w, 400, "invalid_comment", "comment is too long")
		return
	}
	var approval approvalSetting
	if err := s.setting(r.Context(), "approval", &approval); err != nil {
		writeError(w, http.StatusServiceUnavailable, "approval_setting_unavailable", "approval policy is unavailable")
		return
	}
	if !approval.Enabled {
		writeError(w, 409, "approval_not_enabled", "RealmGuard content approval is disabled")
		return
	}
	if p.Role != "manager" && p.Role != "admin" {
		writeError(w, 403, "forbidden", "this role cannot approve RealmGuard content")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	version, err := scanRealmGuardVersion(tx.QueryRow(r.Context(), `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if version.Status != "pending_approval" {
		writeError(w, 409, "invalid_transition", "only pending versions can be reviewed")
		return
	}
	if approval.separatesDuties() && version.CreatedBy != nil && *version.CreatedBy == p.UserID {
		writeError(w, 403, "self_approval_forbidden", "content creators cannot approve their own version")
		return
	}
	if p.Role == "manager" {
		if version.CreatedBy == nil {
			writeError(w, 403, "team_required", "a manager cannot review content without an accountable creator team")
			return
		}
		var creatorTeam string
		if err := tx.QueryRow(r.Context(), `SELECT team FROM users WHERE id=$1`, *version.CreatedBy).Scan(&creatorTeam); err != nil {
			s.dbError(w, r, err)
			return
		}
		if code, message := realmGuardManagerReviewTeamError(p.Team, creatorTeam); code != "" {
			writeError(w, 403, code, message)
			return
		}
	}
	reviewedAt := s.Now()
	if in.Decision == "approved" {
		_, err = tx.Exec(r.Context(), `UPDATE realmguard_content_versions SET status='approved',approved_by=$2,approved_at=now(),review_comment=$3,reviewed_at=now(),updated_at=now() WHERE id=$1`, id, p.UserID, in.Comment)
	} else {
		_, err = tx.Exec(r.Context(), `UPDATE realmguard_content_versions SET status='draft',approved_by=NULL,approved_at=NULL,approval_requested_at=NULL,tested_at=NULL,review_comment=$2,reviewed_at=now(),updated_at=now() WHERE id=$1`, id, in.Comment)
	}
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	version.ReviewComment, version.ReviewedAt = in.Comment, &reviewedAt
	if in.Decision == "approved" {
		version.Status, version.ApprovedBy, version.ApprovedAt = "approved", &p.UserID, &reviewedAt
	} else {
		version.Status, version.ApprovedBy, version.ApprovedAt, version.RequestedAt, version.TestedAt = "draft", nil, nil, nil, nil
	}
	auditAction := "realmguard.version.approve"
	if in.Decision == "rejected" {
		auditAction = "realmguard.version.reject"
	}
	s.audit(r, auditAction, "realmguard_content_version", id.String(), map[string]any{"decision": in.Decision, "comment": in.Comment})
	writeJSON(w, 200, map[string]any{"version": realmGuardVersionJSON(version), "decision": in.Decision, "approved": in.Decision == "approved", "rejected": in.Decision == "rejected"})
}

func (s *Server) publishRealmGuardVersion(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := realmGuardIDParam(w, r)
	if !ok {
		return
	}
	var optional struct {
		Notes *string `json:"notes,omitempty"`
	}
	if !decodeOptionalJSON(w, r, &optional) {
		return
	}
	if optional.Notes != nil && len(*optional.Notes) > 2000 {
		writeError(w, 400, "invalid_notes", "notes must be at most 2000 characters")
		return
	}
	var approval approvalSetting
	if err := s.setting(r.Context(), "approval", &approval); err != nil {
		writeError(w, http.StatusServiceUnavailable, "approval_setting_unavailable", "approval policy is unavailable")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('realmguard_publish'))`); err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err := scanRealmGuardVersion(tx.QueryRow(r.Context(), `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err := validateRealmGuardContent(version.RawContent); err != nil {
		writeError(w, 422, "content_validation_failed", err.Error())
		return
	}
	if version.Status == "published" {
		writeJSON(w, 200, map[string]any{"version": realmGuardVersionJSON(version), "published": true})
		return
	}
	if approval.Enabled && version.Status != "approved" {
		if version.Status != "testing" && version.Status != "pending_approval" {
			writeError(w, 409, "invalid_transition", "test the version before requesting publication")
			return
		}
		requestedAt := s.Now()
		_, err = tx.Exec(r.Context(), `UPDATE realmguard_content_versions SET status='pending_approval',approval_requested_at=COALESCE(approval_requested_at,now()),notes=CASE WHEN $2::text IS NULL THEN notes ELSE $2 END,updated_at=now() WHERE id=$1`, id, optional.Notes)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			s.dbError(w, r, err)
			return
		}
		version.Status, version.RequestedAt = "pending_approval", &requestedAt
		s.audit(r, "realmguard.version.publish_request", "realmguard_content_version", id.String(), nil)
		writeJSON(w, 202, map[string]any{"version": realmGuardVersionJSON(version), "published": false, "approval_required": true})
		return
	}
	if approval.Enabled && p.Role != "admin" {
		writeError(w, 403, "admin_publish_required", "an administrator must publish an approved version")
		return
	}
	if !approval.Enabled && p.Role != "admin" && p.Role != "operator" {
		writeError(w, 403, "forbidden", "operator or administrator role required")
		return
	}
	if !approval.Enabled && !slices.Contains([]string{"testing", "pending_approval", "approved"}, version.Status) {
		writeError(w, 409, "invalid_transition", "test the version before publication")
		return
	}
	if optional.Notes != nil {
		version.Notes = *optional.Notes
	}
	checksum := realmGuardChecksum(version.RawContent)
	if _, err = tx.Exec(r.Context(), `UPDATE realmguard_content_versions SET status='archived',updated_at=now() WHERE status='published' AND id<>$1`, id); err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE realmguard_content_versions SET status='published',checksum=$2,notes=CASE WHEN $3::text IS NULL THEN notes ELSE $3 END,published_at=now(),updated_at=now() WHERE id=$1`, id, checksum, optional.Notes)
	}
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	publishedAt := s.Now()
	version.Status, version.Checksum, version.PublishedAt = "published", checksum, &publishedAt
	s.audit(r, "realmguard.version.publish", "realmguard_content_version", id.String(), map[string]any{"checksum": checksum, "approval_enabled": approval.Enabled})
	writeJSON(w, 200, map[string]any{"version": realmGuardVersionJSON(version), "published": true, "approval_required": approval.Enabled})
}

func (s *Server) realmGuardTelemetry(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		if _, err := fmt.Sscan(raw, &days); err != nil || days < 1 || days > 365 {
			writeError(w, 400, "invalid_days", "days must be between 1 and 365")
			return
		}
	}
	since := s.Now().Add(-time.Duration(days) * 24 * time.Hour)
	version, err := s.loadRealmGuardPublished(r.Context())
	if requested := r.URL.Query().Get("version_id"); requested != "" {
		id, parseErr := uuid.Parse(requested)
		if parseErr != nil {
			writeError(w, 400, "invalid_version", "version_id must be a UUID")
			return
		}
		version, err = s.loadRealmGuardVersion(r.Context(), id)
	}
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	var runs, users, campaignRuns, victories int64
	var avgScore, avgDuration float64
	var totalPlaytime, highestWave int64
	err = s.DB.QueryRow(r.Context(), `SELECT count(*),count(DISTINCT user_id),COALESCE(avg(score),0),COALESCE(avg(duration_ms),0),COALESCE(sum(duration_ms),0),COALESCE(max(waves_completed),0),count(*) FILTER(WHERE mode='campaign'),count(*) FILTER(WHERE mode='campaign' AND stars>0) FROM realmguard_results WHERE verified AND content_version_id=$2 AND created_at>=$1`, since, version.ID).Scan(&runs, &users, &avgScore, &avgDuration, &totalPlaytime, &highestWave, &campaignRuns, &victories)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	queryBreakdown := func(column string) ([]map[string]any, error) {
		rows, err := s.DB.Query(r.Context(), `SELECT `+column+`,count(*),count(DISTINCT user_id),COALESCE(avg(score),0),COALESCE(avg(duration_ms),0) FROM realmguard_results WHERE verified AND content_version_id=$2 AND created_at>=$1 GROUP BY `+column+` ORDER BY count(*) DESC`, since, version.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var key string
			var count, unique int64
			var score, duration float64
			if err := rows.Scan(&key, &count, &unique, &score, &duration); err != nil {
				return nil, err
			}
			items = append(items, map[string]any{"key": key, "runs": count, "unique_users": unique, "avg_score": score, "avg_duration_ms": duration})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return items, nil
	}
	stages, err := queryBreakdown("stage_id")
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	heroes, err := queryBreakdown("hero_id")
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	difficulties, err := queryBreakdown("difficulty")
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	queryUsage := func(event, key string) ([]map[string]any, error) {
		rows, err := s.DB.Query(r.Context(), `SELECT data->>$3,count(*) FROM game_telemetry gt JOIN game_sessions gs ON gs.id=gt.session_id WHERE gt.event=$1 AND gt.received_at>=$2 AND gs.realmguard_content_version_id=$4 AND COALESCE(data->>$3,'')<>'' GROUP BY data->>$3 ORDER BY count(*) DESC`, event, since, key, version.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id string
			var count int64
			if err := rows.Scan(&id, &count); err != nil {
				return nil, err
			}
			items = append(items, map[string]any{"id": id, "count": count})
		}
		return items, rows.Err()
	}
	towerUsage, err := queryUsage("realmguard.tower.build", "tower")
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	skillUsage, err := queryUsage("realmguard.skill.cast", "skill")
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	failedRows, err := s.DB.Query(r.Context(), `SELECT stage_id,waves_completed,count(*) FROM realmguard_results WHERE content_version_id=$1 AND verified AND mode='campaign' AND stars=0 AND created_at>=$2 GROUP BY stage_id,waves_completed ORDER BY stage_id,waves_completed`, version.ID, since)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	failedWaves := []map[string]any{}
	for failedRows.Next() {
		var stageID string
		var wave int
		var count int64
		if err := failedRows.Scan(&stageID, &wave, &count); err != nil {
			failedRows.Close()
			s.dbError(w, r, err)
			return
		}
		failedWaves = append(failedWaves, map[string]any{"stage_id": stageID, "wave": wave, "runs": count})
	}
	if err := failedRows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	failedRows.Close()
	var rejected int64
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM audit_logs WHERE action='realmguard.result.reject' AND created_at>=$1`, since).Scan(&rejected)
	completionRate := float64(0)
	if campaignRuns > 0 {
		completionRate = float64(victories) / float64(campaignRuns)
	}
	writeJSON(w, 200, map[string]any{"period_days": days, "since": since, "version": realmGuardVersionJSON(version), "summary": map[string]any{"runs": runs, "unique_users": users, "avg_score": avgScore, "avg_duration_ms": avgDuration, "total_playtime_ms": totalPlaytime, "highest_wave": highestWave, "campaign_runs": campaignRuns, "campaign_victories": victories, "campaign_completion_rate": completionRate, "rejected_results": rejected}, "stages": stages, "heroes": heroes, "difficulties": difficulties, "tower_usage": towerUsage, "skill_usage": skillUsage, "failed_waves": failedWaves})
}
