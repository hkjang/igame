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

var errDefenseStaleVersion = errors.New("defense draft checksum is stale")

func validDefenseSection(section string) bool { return slices.Contains(defenseSections, section) }

func (s *Server) defenseDraftVersion(ctx context.Context, slug, requested string) (defenseVersionRecord, error) {
	gameID, _, err := s.defenseGame(ctx, slug)
	if err != nil {
		return defenseVersionRecord{}, err
	}
	if requested != "" {
		id, err := uuid.Parse(requested)
		if err != nil {
			return defenseVersionRecord{}, fmt.Errorf("invalid version_id")
		}
		version, err := s.loadDefenseVersion(ctx, id)
		if err == nil && version.GameID != gameID {
			return version, pgx.ErrNoRows
		}
		return version, err
	}
	version, err := scanDefenseVersion(s.DB.QueryRow(ctx, `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE game_id=$1 AND status IN ('draft','testing','pending_approval','approved') ORDER BY version_no DESC LIMIT 1`, gameID))
	if err != nil {
		return version, err
	}
	return s.normalizeDefenseChecksum(ctx, version), nil
}

func (s *Server) getDefenseDraftSection(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	section := chi.URLParam(r, "section")
	if !validDefenseSection(section) {
		writeError(w, 404, "unknown_section", "unknown Defense Content Studio section")
		return
	}
	version, err := s.defenseDraftVersion(r.Context(), slug, r.URL.Query().Get("version_id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "draft_not_found", "create a Defense Series draft first")
		} else {
			writeError(w, 400, "invalid_version", err.Error())
		}
		return
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(version.RawContent, &document) != nil || len(document[section]) == 0 {
		s.serverError(w, r, 500, "invalid_content", "stored content is missing the requested section",
			fmt.Errorf("defense version %s has no %q section", version.ID, section))
		return
	}
	w.Header().Set("ETag", `"`+version.Checksum+`"`)
	writeJSON(w, 200, map[string]any{"version": defenseVersionJSON(version), "section": section, "data": document[section]})
}

func validateDefenseDraftSection(section string, raw json.RawMessage) error {
	if section == "balance" || section == "resource_rules" {
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil || value == nil {
			return fmt.Errorf("%s must be a JSON object", section)
		}
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return fmt.Errorf("%s must be a JSON array", section)
	}
	if len(items) > 10000 {
		return fmt.Errorf("%s exceeds the item limit", section)
	}
	for index, itemRaw := range items {
		var item map[string]any
		if json.Unmarshal(itemRaw, &item) != nil || item == nil {
			return fmt.Errorf("%s item %d must be an object", section, index)
		}
		id, _ := item["id"].(string)
		if !validRealmGuardIdentifier(id) {
			return fmt.Errorf("%s item %d requires a valid id", section, index)
		}
	}
	return nil
}

func (s *Server) putDefenseDraftSection(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	section := chi.URLParam(r, "section")
	if !validDefenseSection(section) {
		writeError(w, 404, "unknown_section", "unknown Defense Content Studio section")
		return
	}
	expected, ok := realmGuardExpectedChecksum(w, r)
	if !ok {
		return
	}
	data, ok := decodeRealmGuardSectionBody(w, r)
	if !ok {
		return
	}
	if err := validateDefenseDraftSection(section, data); err != nil {
		writeError(w, 400, "invalid_section", err.Error())
		return
	}
	version, err := s.mutateDefenseDraft(r.Context(), slug, r.URL.Query().Get("version_id"), section, expected, data)
	if err != nil {
		handleDefenseDesignerError(w, err)
		return
	}
	s.audit(r, "defense.designer.section.update", "defense_content_version", version.ID.String(), map[string]any{"game": slug, "section": section})
	var document map[string]json.RawMessage
	_ = json.Unmarshal(version.RawContent, &document)
	w.Header().Set("ETag", `"`+version.Checksum+`"`)
	writeJSON(w, 200, map[string]any{"version": defenseVersionJSON(version), "section": section, "data": document[section]})
}

func (s *Server) mutateDefenseDraft(ctx context.Context, slug, requested, section, expected string, data json.RawMessage) (defenseVersionRecord, error) {
	gameID, _, err := s.defenseGame(ctx, slug)
	if err != nil {
		return defenseVersionRecord{}, err
	}
	var id uuid.UUID
	if requested != "" {
		id, err = uuid.Parse(requested)
		if err != nil {
			return defenseVersionRecord{}, fmt.Errorf("invalid version_id")
		}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return defenseVersionRecord{}, err
	}
	defer tx.Rollback(ctx)
	version, err := scanDefenseVersion(tx.QueryRow(ctx, `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE game_id=$1 AND id=CASE WHEN $2='00000000-0000-0000-0000-000000000000'::uuid THEN (SELECT id FROM defense_content_versions WHERE game_id=$1 AND status IN ('draft','testing') ORDER BY version_no DESC LIMIT 1) ELSE $2 END FOR UPDATE`, gameID, id))
	if err != nil {
		return version, err
	}
	if version.Status != "draft" && version.Status != "testing" {
		return version, fmt.Errorf("only draft or testing versions can be edited")
	}
	if !strings.EqualFold(version.Checksum, expected) {
		return version, errDefenseStaleVersion
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(version.RawContent, &document) != nil {
		return version, fmt.Errorf("stored content is invalid")
	}
	document[section] = data
	raw, _ := json.Marshal(document)
	version.Status = "draft"
	version.ApprovedBy = nil
	version.RequestedAt = nil
	version.ApprovedAt = nil
	version.ReviewedAt = nil
	version.ReviewComment = ""
	base := strings.Split(version.ContentVersion, "-r")[0]
	version.ContentVersion = fmt.Sprintf("%s-r%d", base, version.VersionNo)
	// PostgreSQL jsonb has its own canonical representation. Hash the value
	// returned by PostgreSQL, rather than the pre-storage Go encoding, so the
	// PUT response, subsequent reads, and the next If-Match all share exactly
	// one checksum contract.
	err = tx.QueryRow(ctx, `UPDATE defense_content_versions SET status='draft',content=$2,content_version=$3,approved_by=NULL,approval_requested_at=NULL,approved_at=NULL,review_comment='',reviewed_at=NULL,tested_at=NULL,updated_at=now() WHERE id=$1 RETURNING content,updated_at`, version.ID, raw, version.ContentVersion).Scan(&version.RawContent, &version.UpdatedAt)
	if err != nil {
		return version, err
	}
	version.Checksum = defenseChecksum(version.RawContent)
	if _, err = tx.Exec(ctx, `UPDATE defense_content_versions SET checksum=$2 WHERE id=$1`, version.ID, version.Checksum); err != nil {
		return version, err
	}
	if err = tx.Commit(ctx); err != nil {
		return version, err
	}
	return version, nil
}

func handleDefenseDesignerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, 404, "not_found", "Defense draft was not found")
	case errors.Is(err, errDefenseStaleVersion):
		writeError(w, 409, "stale_version", "draft changed; reload it before saving")
	default:
		writeError(w, 400, "invalid_draft", err.Error())
	}
}

func (s *Server) listDefenseVersions(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	gameID, _, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE game_id=$1 ORDER BY version_no DESC`, gameID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		version, err := scanDefenseVersion(rows)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		version = s.normalizeDefenseChecksum(r.Context(), version)
		item := defenseVersionJSON(version)
		item["game_slug"] = slug
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

type createDefenseVersionInput struct {
	Label           string     `json:"label,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	AssetVersion    string     `json:"asset_version,omitempty"`
	PolicyVersion   string     `json:"policy_version,omitempty"`
	SourceVersionID *uuid.UUID `json:"source_version_id,omitempty"`
}

func defenseLifecycleMajorMinor(value string) (string, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value = strings.SplitN(value, "-", 2)[0]
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return "", false
	}
	components := make([]int, len(parts))
	for index, part := range parts {
		component, err := strconv.Atoi(part)
		if err != nil || component < 0 || strconv.Itoa(component) != part {
			return "", false
		}
		components[index] = component
	}
	return fmt.Sprintf("%d.%d", components[0], components[1]), true
}

func defenseDraftLifecycle(source defenseVersionRecord) string {
	var document struct {
		SchemaVersion string `json:"schema_version"`
	}
	if json.Unmarshal(source.RawContent, &document) == nil {
		if lifecycle, ok := defenseLifecycleMajorMinor(document.SchemaVersion); ok {
			return lifecycle
		}
	}
	if lifecycle, ok := defenseLifecycleMajorMinor(source.ContentVersion); ok {
		return lifecycle
	}
	// Pre-schema custom packs were created in the original 0.3 lifecycle.
	return "0.3"
}

func (s *Server) createDefenseVersion(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	var in createDefenseVersionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.Label) > 100 || len(in.Notes) > 2000 || len(in.AssetVersion) > 100 || len(in.PolicyVersion) > 100 {
		writeError(w, 400, "invalid_version", "version metadata is too long")
		return
	}
	gameID, _, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))`, gameID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	var source defenseVersionRecord
	if in.SourceVersionID == nil {
		source, err = scanDefenseVersion(tx.QueryRow(r.Context(), `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE game_id=$1 AND status='published' FOR SHARE`, gameID))
	} else {
		source, err = scanDefenseVersion(tx.QueryRow(r.Context(), `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE game_id=$1 AND id=$2 FOR SHARE`, gameID, *in.SourceVersionID))
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "source_version_not_found", "source version must belong to the same Defense game")
		} else {
			s.dbError(w, r, err)
		}
		return
	}
	var number int
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(max(version_no),0)+1 FROM defense_content_versions WHERE game_id=$1`, gameID).Scan(&number); err != nil {
		s.dbError(w, r, err)
		return
	}
	lifecycle := defenseDraftLifecycle(source)
	contentVersion := fmt.Sprintf("%s.%d", lifecycle, number-1)
	if strings.TrimSpace(in.Label) == "" {
		in.Label = "v" + contentVersion
	}
	if in.AssetVersion == "" {
		in.AssetVersion = source.AssetVersion
	}
	if in.PolicyVersion == "" {
		in.PolicyVersion = source.PolicyVersion
	}
	metadata := defenseVersionRecord{Label: in.Label, ContentVersion: contentVersion, PolicyVersion: in.PolicyVersion, AssetVersion: in.AssetVersion, Notes: in.Notes}
	if err = validateDefenseVersionMetadata(metadata); err != nil {
		writeError(w, 400, "invalid_version", err.Error())
		return
	}
	checksum := defenseChecksum(source.RawContent)
	var id uuid.UUID
	var created, updated time.Time
	err = tx.QueryRow(r.Context(), `INSERT INTO defense_content_versions(game_id,version_no,label,status,content_version,policy_version,asset_version,checksum,notes,content,source_version_id,created_by) VALUES($1,$2,$3,'draft',$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id,created_at,updated_at`, gameID, number, in.Label, contentVersion, in.PolicyVersion, in.AssetVersion, checksum, in.Notes, source.RawContent, source.ID, p.UserID).Scan(&id, &created, &updated)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	version := defenseVersionRecord{ID: id, GameID: gameID, VersionNo: number, Label: in.Label, Status: "draft", ContentVersion: contentVersion, PolicyVersion: in.PolicyVersion, AssetVersion: in.AssetVersion, Checksum: checksum, Notes: in.Notes, RawContent: source.RawContent, SourceVersionID: &source.ID, CreatedBy: &p.UserID, CreatedAt: created, UpdatedAt: updated}
	s.audit(r, "defense.version.create", "defense_content_version", id.String(), map[string]any{"game": slug, "source": source.ID})
	writeJSON(w, 201, map[string]any{"version": defenseVersionJSON(version)})
}

func defenseVersionIDForSlug(w http.ResponseWriter, r *http.Request, s *Server, slug string) (defenseVersionRecord, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_id", "invalid version identifier")
		return defenseVersionRecord{}, false
	}
	version, err := s.loadDefenseVersion(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return version, false
	}
	gameID, _, err := s.defenseGame(r.Context(), slug)
	if err != nil || version.GameID != gameID {
		writeError(w, 404, "not_found", "Defense version was not found")
		return version, false
	}
	return version, true
}

func (s *Server) testDefenseVersion(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	version, ok := defenseVersionIDForSlug(w, r, s, slug)
	if !ok {
		return
	}
	if version.Status != "draft" && version.Status != "testing" {
		writeError(w, 409, "invalid_transition", "only draft or testing versions can be tested")
		return
	}
	if err := validateDefenseVersionMetadata(version); err != nil {
		writeError(w, 422, "version_metadata_invalid", err.Error())
		return
	}
	if err := validateDefenseContent(slug, version.RawContent); err != nil {
		writeError(w, 422, "content_validation_failed", err.Error())
		return
	}
	checksum := defenseChecksum(version.RawContent)
	var tested, updated time.Time
	err := s.DB.QueryRow(r.Context(), `UPDATE defense_content_versions SET status='testing',tested_at=now(),checksum=$2,updated_at=now() WHERE id=$1 AND status IN ('draft','testing') RETURNING tested_at,updated_at`, version.ID, checksum).Scan(&tested, &updated)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version.Status = "testing"
	version.Checksum = checksum
	version.TestedAt = &tested
	version.UpdatedAt = updated
	content, _ := decodeDefenseContent(version.RawContent)
	s.audit(r, "defense.version.test", "defense_content_version", version.ID.String(), map[string]any{"game": slug, "checksum": checksum})
	writeJSON(w, 200, map[string]any{"version": defenseVersionJSON(version), "validation": map[string]any{"valid": true, "stages": len(content.Stages), "waves": len(content.Waves), "towers": len(content.Towers), "enemies": len(content.Enemies), "bosses": len(content.Bosses), "heroes": len(content.Heroes), "events": len(content.Events), "education": len(content.Education), "model_profiles": len(content.ModelProfiles)}})
}

func (s *Server) previewDefenseVersion(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	version, ok := defenseVersionIDForSlug(w, r, s, slug)
	if !ok {
		return
	}
	p, _ := principalFrom(r)
	if p.Role == "manager" {
		if version.Status != "pending_approval" {
			writeError(w, 403, "review_preview_forbidden", "managers may preview only content submitted to their pending review queue")
			return
		}
		if version.CreatedBy == nil {
			writeError(w, 403, "team_required", "preview requires an accountable creator team")
			return
		}
		var creatorTeam string
		if err := s.DB.QueryRow(r.Context(), `SELECT team FROM users WHERE id=$1`, *version.CreatedBy).Scan(&creatorTeam); err != nil {
			s.dbError(w, r, err)
			return
		}
		if code, message := realmGuardManagerReviewTeamError(p.Team, creatorTeam); code != "" {
			writeError(w, 403, code, message)
			return
		}
	}
	if err := validateDefenseContent(slug, version.RawContent); err != nil {
		writeError(w, 422, "content_validation_failed", err.Error())
		return
	}
	decoded, err := decodeDefenseContent(version.RawContent)
	if err != nil {
		s.serverError(w, r, 500, "invalid_content", err.Error(), err)
		return
	}
	content, err := sanitizeDefenseContent(version.RawContent)
	if err != nil {
		s.serverError(w, r, 500, "invalid_content", err.Error(), err)
		return
	}
	_, name, _ := s.defenseGame(r.Context(), slug)
	writeJSON(w, 200, map[string]any{"game": map[string]any{"slug": slug, "name": name, "education_enabled": defenseEducationEnabled(decoded)}, "version": defenseVersionJSON(version), "content": content, "preview": true, "practice_only": true})
}

func (s *Server) listPendingDefenseVersions(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	if p.Role == "manager" && strings.TrimSpace(p.Team) == "" {
		writeError(w, 403, "team_required", "a manager must belong to a team")
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT v.id,v.game_id,v.version_no,v.label,v.status,v.content_version,v.policy_version,v.asset_version,v.checksum,v.notes,v.content,v.source_version_id,v.created_by,v.approved_by,v.created_at,v.tested_at,v.approval_requested_at,v.approved_at,v.review_comment,v.reviewed_at,v.published_at,v.updated_at,g.slug,g.name,COALESCE(creator.username,''),COALESCE(creator.display_name,''),COALESCE(creator.team,''),COALESCE((SELECT published.content FROM defense_content_versions published WHERE published.game_id=v.game_id AND published.status='published' LIMIT 1),'{}'::jsonb) FROM defense_content_versions v JOIN games g ON g.id=v.game_id LEFT JOIN users creator ON creator.id=v.created_by WHERE v.status='pending_approval' AND ($1::text<>'manager' OR (creator.team<>'' AND creator.team=$2)) ORDER BY v.approval_requested_at,v.version_no`, p.Role, p.Team)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var version defenseVersionRecord
		var slug, name, creatorUsername, creatorDisplayName, creatorTeam string
		var publishedContent json.RawMessage
		if err := rows.Scan(&version.ID, &version.GameID, &version.VersionNo, &version.Label, &version.Status, &version.ContentVersion, &version.PolicyVersion, &version.AssetVersion, &version.Checksum, &version.Notes, &version.RawContent, &version.SourceVersionID, &version.CreatedBy, &version.ApprovedBy, &version.CreatedAt, &version.TestedAt, &version.RequestedAt, &version.ApprovedAt, &version.ReviewComment, &version.ReviewedAt, &version.PublishedAt, &version.UpdatedAt, &slug, &name, &creatorUsername, &creatorDisplayName, &creatorTeam, &publishedContent); err != nil {
			s.dbError(w, r, err)
			return
		}
		item := defenseVersionJSON(version)
		item["game_slug"] = slug
		item["game_name"] = name
		item["creator"] = map[string]any{"username": creatorUsername, "display_name": creatorDisplayName, "team": creatorTeam}
		item["changed_sections"] = defenseChangedSections(version.RawContent, publishedContent)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func defenseChangedSections(candidate, published json.RawMessage) []string {
	var candidateSections, publishedSections map[string]json.RawMessage
	if json.Unmarshal(candidate, &candidateSections) != nil || json.Unmarshal(published, &publishedSections) != nil {
		return append([]string(nil), defenseSections...)
	}
	changed := []string{}
	for _, section := range defenseSections {
		if defenseChecksum(candidateSections[section]) != defenseChecksum(publishedSections[section]) {
			changed = append(changed, section)
		}
	}
	return changed
}

type reviewDefenseInput struct {
	Decision string `json:"decision,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

func (s *Server) reviewDefenseVersion(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in reviewDefenseInput
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
		s.serverError(w, r, 503, "approval_setting_unavailable", "approval policy is unavailable", err)
		return
	}
	if !approval.Enabled {
		writeError(w, 409, "approval_not_enabled", "Defense content approval is disabled")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	version, err := scanDefenseVersion(tx.QueryRow(r.Context(), `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE id=$1 FOR UPDATE`, id))
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
			writeError(w, 403, "team_required", "manager review requires an accountable creator team")
			return
		}
		var creatorTeam string
		if err = tx.QueryRow(r.Context(), `SELECT team FROM users WHERE id=$1`, *version.CreatedBy).Scan(&creatorTeam); err != nil {
			s.dbError(w, r, err)
			return
		}
		if code, message := realmGuardManagerReviewTeamError(p.Team, creatorTeam); code != "" {
			writeError(w, 403, code, message)
			return
		}
	}
	if in.Decision == "approved" {
		_, err = tx.Exec(r.Context(), `UPDATE defense_content_versions SET status='approved',approved_by=$2,approved_at=now(),review_comment=$3,reviewed_at=now(),updated_at=now() WHERE id=$1`, id, p.UserID, in.Comment)
	} else {
		_, err = tx.Exec(r.Context(), `UPDATE defense_content_versions SET status='draft',approved_by=NULL,approved_at=NULL,approval_requested_at=NULL,tested_at=NULL,review_comment=$2,reviewed_at=now(),updated_at=now() WHERE id=$1`, id, in.Comment)
	}
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	version.Status = map[bool]string{true: "approved", false: "draft"}[in.Decision == "approved"]
	action := "defense.version.approve"
	if in.Decision == "rejected" {
		action = "defense.version.reject"
	}
	s.audit(r, action, "defense_content_version", id.String(), map[string]any{"decision": in.Decision, "comment": in.Comment})
	writeJSON(w, 200, map[string]any{"version": defenseVersionJSON(version), "decision": in.Decision, "approved": in.Decision == "approved", "rejected": in.Decision == "rejected"})
}

func (s *Server) publishDefenseVersion(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	version, ok := defenseVersionIDForSlug(w, r, s, slug)
	if !ok {
		return
	}
	if err := validateDefenseVersionMetadata(version); err != nil {
		writeError(w, 422, "version_metadata_invalid", err.Error())
		return
	}
	if err := validateDefenseContent(slug, version.RawContent); err != nil {
		writeError(w, 422, "content_validation_failed", err.Error())
		return
	}
	var approval approvalSetting
	if err := s.setting(r.Context(), "approval", &approval); err != nil {
		s.serverError(w, r, 503, "approval_setting_unavailable", "approval policy is unavailable", err)
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1::text,1))`, version.GameID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err = scanDefenseVersion(tx.QueryRow(r.Context(), `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE id=$1 FOR UPDATE`, version.ID))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if version.Status == "published" {
		writeJSON(w, 200, map[string]any{"version": defenseVersionJSON(version), "published": true})
		return
	}
	if approval.Enabled && version.Status != "approved" {
		if version.Status != "testing" && version.Status != "pending_approval" {
			writeError(w, 409, "invalid_transition", "test the version before requesting publication")
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE defense_content_versions SET status='pending_approval',approval_requested_at=COALESCE(approval_requested_at,now()),updated_at=now() WHERE id=$1`, version.ID)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			s.dbError(w, r, err)
			return
		}
		version.Status = "pending_approval"
		s.audit(r, "defense.version.publish_request", "defense_content_version", version.ID.String(), map[string]any{"game": slug})
		writeJSON(w, 202, map[string]any{"version": defenseVersionJSON(version), "published": false, "approval_required": true})
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
	checksum := defenseChecksum(version.RawContent)
	_, err = tx.Exec(r.Context(), `UPDATE defense_content_versions SET status='archived',updated_at=now() WHERE game_id=$1 AND status='published' AND id<>$2`, version.GameID, version.ID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE defense_content_versions SET status='published',checksum=$2,published_at=now(),updated_at=now() WHERE id=$1`, version.ID, checksum)
	}
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	version.Status = "published"
	version.Checksum = checksum
	s.audit(r, "defense.version.publish", "defense_content_version", version.ID.String(), map[string]any{"game": slug, "checksum": checksum})
	writeJSON(w, 200, map[string]any{"version": defenseVersionJSON(version), "published": true, "approval_required": approval.Enabled})
}

func (s *Server) defenseTelemetryReport(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		if _, err := fmt.Sscan(raw, &days); err != nil || days < 1 || days > 365 {
			writeError(w, 400, "invalid_days", "days must be between 1 and 365")
			return
		}
	}
	gameID, _, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err := s.loadDefensePublished(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	since := s.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var runs, users, victories int64
	var avgScore, avgDuration, avgLearning float64
	err = s.DB.QueryRow(r.Context(), `SELECT count(*),count(DISTINCT user_id),count(*) FILTER(WHERE victory),COALESCE(avg(score),0),COALESCE(avg(duration_ms),0),COALESCE(avg(learning_score),0) FROM defense_results WHERE game_id=$1 AND content_version_id=$2 AND verified AND created_at>=$3`, gameID, version.ID, since).Scan(&runs, &users, &victories, &avgScore, &avgDuration, &avgLearning)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	completionRate := float64(0)
	if runs > 0 {
		completionRate = float64(victories) * 100 / float64(runs)
	}
	var privacy struct {
		ShowDepartment bool `json:"show_department"`
	}
	if err = s.setting(r.Context(), "privacy", &privacy); err != nil {
		s.serverError(w, r, 503, "privacy_setting_unavailable", "privacy policy is unavailable", err)
		return
	}
	departmentCount := int64(0)
	if privacy.ShowDepartment {
		if err = s.DB.QueryRow(r.Context(), `SELECT count(DISTINCT u.department) FROM defense_results r JOIN users u ON u.id=r.user_id WHERE r.game_id=$1 AND r.content_version_id=$2 AND r.verified AND r.created_at>=$3 AND u.department<>''`, gameID, version.ID, since).Scan(&departmentCount); err != nil {
			s.dbError(w, r, err)
			return
		}
	}
	// Refused authoritative results are the signal operators watch for forged
	// submissions, the same way the RealmGuard report surfaces them.
	var rejected int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM audit_logs WHERE action='defense.result.reject' AND created_at>=$1 AND detail->>'game'=$2`, since, slug).Scan(&rejected); err != nil {
		s.dbError(w, r, err)
		return
	}
	summary := map[string]any{"participants": users, "plays": runs, "completion_rate": completionRate, "average_score": avgScore, "average_game_score": avgScore, "average_play_time_ms": avgDuration, "average_learning_score": avgLearning, "department_count": departmentCount, "rejected_results": rejected}
	writeJSON(w, 200, map[string]any{"game": slug, "days": days, "version": defenseVersionJSON(version), "summary": summary, "rejected_results": rejected, "participants": users, "plays": runs, "runs": runs, "unique_users": users, "completion_rate": completionRate, "average_score": avgScore, "average_game_score": avgScore, "average_play_time_ms": avgDuration, "average_duration_ms": avgDuration, "average_learning_score": avgLearning, "department_count": departmentCount, "department_visible": privacy.ShowDepartment, "verification_method": defenseVerificationMethod})
}

func (s *Server) defenseLearningReport(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	gameID, _, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err := s.loadDefensePublished(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	content, err := decodeDefenseContent(version.RawContent)
	if err != nil {
		s.serverError(w, r, 500, "invalid_published_content", err.Error(), err)
		return
	}
	var privacy struct {
		ShowDepartment bool `json:"show_department"`
	}
	if err = s.setting(r.Context(), "privacy", &privacy); err != nil {
		s.serverError(w, r, 503, "privacy_setting_unavailable", "privacy policy is unavailable", err)
		return
	}
	var participants, plays, battleClears int64
	var averageGameScore, averageLearningScore, averagePlayTime, retryRate, improvement float64
	err = s.DB.QueryRow(r.Context(), `SELECT count(DISTINCT user_id),count(*),count(*) FILTER(WHERE victory),COALESCE(avg(score),0),COALESCE(avg(learning_score),0),COALESCE(avg(duration_ms),0) FROM defense_results WHERE game_id=$1 AND content_version_id=$2 AND verified`, gameID, version.ID).Scan(&participants, &plays, &battleClears, &averageGameScore, &averageLearningScore, &averagePlayTime)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	err = s.DB.QueryRow(r.Context(), `SELECT COALESCE(100.0*count(*) FILTER(WHERE attempts>1)/NULLIF(count(*),0),0),COALESCE(avg(last_score-first_score) FILTER(WHERE attempts>1),0) FROM (SELECT user_id,stage_id,difficulty,count(*) attempts,(array_agg(learning_score ORDER BY created_at,id))[1] first_score,(array_agg(learning_score ORDER BY created_at DESC,id DESC))[1] last_score FROM defense_results WHERE game_id=$1 AND content_version_id=$2 AND verified GROUP BY user_id,stage_id,difficulty) retries`, gameID, version.ID).Scan(&retryRate, &improvement)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	var campaignCompletedUsers int64
	campaignCount := len(content.Campaigns)
	if campaignCount > 0 {
		err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM (SELECT user_id FROM defense_campaign_progress WHERE game_id=$1 AND content_version_id=$2 GROUP BY user_id HAVING count(*) FILTER(WHERE completed)=$3) completed_users`, gameID, version.ID, campaignCount).Scan(&campaignCompletedUsers)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
	}
	rows, err := s.DB.Query(r.Context(), `SELECT topic,count(*) FILTER(WHERE correct),count(*),round(100.0*count(*) FILTER(WHERE correct)/NULLIF(count(*),0))::int FROM defense_event_answers WHERE game_id=$1 AND content_version_id=$2 GROUP BY topic ORDER BY topic`, gameID, version.ID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	topics := []map[string]any{}
	for rows.Next() {
		var topic string
		var correct, total, score int
		if err = rows.Scan(&topic, &correct, &total, &score); err != nil {
			rows.Close()
			s.dbError(w, r, err)
			return
		}
		topics = append(topics, map[string]any{"type": "topic", "topic": topic, "correct": correct, "total": total, "score": score, "accuracy": score})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	rows.Close()
	questionText := map[string]string{}
	for _, question := range content.Education {
		questionText[question.ID] = question.Question
	}
	rows, err = s.DB.Query(r.Context(), `SELECT question_id,min(topic),count(*) FILTER(WHERE correct),count(*),round(100.0*count(*) FILTER(WHERE correct)/NULLIF(count(*),0))::int FROM defense_event_answers WHERE game_id=$1 AND content_version_id=$2 GROUP BY question_id ORDER BY question_id`, gameID, version.ID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	questions := []map[string]any{}
	for rows.Next() {
		var questionID, topic string
		var correct, total, accuracy int
		if err = rows.Scan(&questionID, &topic, &correct, &total, &accuracy); err != nil {
			rows.Close()
			s.dbError(w, r, err)
			return
		}
		questions = append(questions, map[string]any{"type": "question", "question_id": questionID, "question": questionText[questionID], "topic": topic, "correct": correct, "total": total, "accuracy": accuracy})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	rows.Close()
	weakTopics := []map[string]any{}
	for _, topic := range topics {
		if score, _ := topic["score"].(int); score < 70 {
			copy := map[string]any{"type": "weak_topic"}
			for key, value := range topic {
				copy[key] = value
			}
			weakTopics = append(weakTopics, copy)
		}
	}
	departments := []map[string]any{}
	if privacy.ShowDepartment {
		rows, err = s.DB.Query(r.Context(), `SELECT u.department,count(DISTINCT r.user_id),count(*),count(*) FILTER(WHERE r.victory),COALESCE(round(avg(r.score)),0)::bigint,COALESCE(round(avg(r.learning_score)),0)::int,COALESCE(round(avg(r.duration_ms)),0)::bigint FROM defense_results r JOIN users u ON u.id=r.user_id WHERE r.game_id=$1 AND r.content_version_id=$2 AND r.verified AND u.department<>'' GROUP BY u.department ORDER BY u.department`, gameID, version.ID)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var department string
			var users, attempts, completed, learning int
			var score, playTime int64
			if err = rows.Scan(&department, &users, &attempts, &completed, &score, &learning, &playTime); err != nil {
				s.dbError(w, r, err)
				return
			}
			departments = append(departments, map[string]any{"type": "department", "department": department, "participants": users, "attempts": attempts, "completed": completed, "score": score, "learning_score": learning, "average_play_time_ms": playTime})
		}
		if err := rows.Err(); err != nil {
			s.dbError(w, r, err)
			return
		}
	}
	battleClearRate := float64(0)
	if plays > 0 {
		battleClearRate = float64(battleClears) * 100 / float64(plays)
	}
	campaignCompletionRate := float64(0)
	if participants > 0 {
		campaignCompletionRate = float64(campaignCompletedUsers) * 100 / float64(participants)
	}
	summary := map[string]any{"participants": participants, "plays": plays, "completion_rate": campaignCompletionRate, "campaign_completion_rate": campaignCompletionRate, "campaign_completed_users": campaignCompletedUsers, "battle_clear_rate": battleClearRate, "battle_clears": battleClears, "average_score": averageLearningScore, "average_game_score": averageGameScore, "retry_rate": retryRate, "improvement": improvement, "average_play_time_ms": averagePlayTime, "department_count": len(departments)}
	writeJSON(w, 200, map[string]any{"game": slug, "version": defenseVersionJSON(version), "policy_version": version.PolicyVersion, "education_enabled": defenseEducationEnabled(content), "summary": summary, "participants": participants, "plays": plays, "completion_rate": campaignCompletionRate, "campaign_completion_rate": campaignCompletionRate, "campaign_completed_users": campaignCompletedUsers, "battle_clear_rate": battleClearRate, "battle_clears": battleClears, "average_score": averageLearningScore, "average_game_score": averageGameScore, "retry_rate": retryRate, "improvement": improvement, "average_play_time_ms": averagePlayTime, "topics": topics, "weak_topics": weakTopics, "questions": questions, "departments": departments, "department_visible": privacy.ShowDepartment})
}
