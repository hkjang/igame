package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

type seasonInput struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Status      string    `json:"status"`
}

func (s *Server) listSeasons(w http.ResponseWriter, r *http.Request)      { s.querySeasons(w, r, false) }
func (s *Server) adminListSeasons(w http.ResponseWriter, r *http.Request) { s.querySeasons(w, r, true) }
func (s *Server) querySeasons(w http.ResponseWriter, r *http.Request, admin bool) {
	where := "WHERE status<>'draft'"
	if admin {
		where = ""
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,name,description,starts_at,ends_at,status,created_at FROM seasons `+where+` ORDER BY starts_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, desc, status string
		var start, end, created time.Time
		if err := rows.Scan(&id, &name, &desc, &start, &end, &status, &created); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "description": desc, "starts_at": start, "ends_at": end, "status": status, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func validateSeason(in seasonInput) string {
	if strings.TrimSpace(in.Name) == "" {
		return "name is required"
	}
	if !in.EndsAt.After(in.StartsAt) {
		return "ends_at must be after starts_at"
	}
	if !slices.Contains([]string{"draft", "active", "closed"}, in.Status) {
		return "invalid status"
	}
	return ""
}
func (s *Server) createSeason(w http.ResponseWriter, r *http.Request) {
	var in seasonInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validateSeason(in); msg != "" {
		writeError(w, 400, "invalid_season", msg)
		return
	}
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `INSERT INTO seasons(name,description,starts_at,ends_at,status) VALUES($1,$2,$3,$4,$5) RETURNING id`, in.Name, in.Description, in.StartsAt, in.EndsAt, in.Status).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "season.create", "season", id.String(), nil)
	writeJSON(w, 201, map[string]any{"season": map[string]any{"id": id, "name": in.Name, "description": in.Description, "starts_at": in.StartsAt, "ends_at": in.EndsAt, "status": in.Status}})
}
func (s *Server) updateSeason(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in seasonInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validateSeason(in); msg != "" {
		writeError(w, 400, "invalid_season", msg)
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE seasons SET name=$2,description=$3,starts_at=$4,ends_at=$5,status=$6 WHERE id=$1`, id, in.Name, in.Description, in.StartsAt, in.EndsAt, in.Status)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "season not found")
		return
	}
	s.audit(r, "season.update", "season", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteSeason(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE seasons SET status='closed' WHERE id=$1`, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "season not found")
		return
	}
	s.audit(r, "season.close", "season", id.String(), nil)
	w.WriteHeader(204)
}

type eventInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	EventType   string          `json:"event_type"`
	GameID      *uuid.UUID      `json:"game_id"`
	StartsAt    time.Time       `json:"starts_at"`
	EndsAt      time.Time       `json:"ends_at"`
	Status      string          `json:"status"`
	Rules       json.RawMessage `json:"rules"`
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request)      { s.queryEvents(w, r, false) }
func (s *Server) adminListEvents(w http.ResponseWriter, r *http.Request) { s.queryEvents(w, r, true) }
func (s *Server) queryEvents(w http.ResponseWriter, r *http.Request, admin bool) {
	p, _ := principalFrom(r)
	where := "WHERE e.status<>'draft'"
	if admin {
		where = ""
	}
	rows, err := s.DB.Query(r.Context(), `SELECT e.id,e.name,e.description,e.event_type,e.game_id,COALESCE(g.name,''),e.starts_at,e.ends_at,e.status,e.rules,e.created_at,EXISTS(SELECT 1 FROM event_participants ep WHERE ep.event_id=e.id AND ep.user_id=$1),(SELECT count(*) FROM event_participants ep WHERE ep.event_id=e.id) FROM events e LEFT JOIN games g ON g.id=e.game_id `+where+` ORDER BY e.starts_at DESC`, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, desc, typ, gameName, status string
		var gameID *uuid.UUID
		var start, end, created time.Time
		var rules json.RawMessage
		var joined bool
		var participants int64
		if err := rows.Scan(&id, &name, &desc, &typ, &gameID, &gameName, &start, &end, &status, &rules, &created, &joined, &participants); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "description": desc, "event_type": typ, "game_id": gameID, "game_name": gameName, "starts_at": start, "ends_at": end, "status": status, "rules": rules, "joined": joined, "participant_count": participants, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func validateEvent(in *eventInput) string {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "name is required"
	}
	if in.EventType == "" {
		in.EventType = "score_attack"
	}
	if !in.EndsAt.After(in.StartsAt) {
		return "ends_at must be after starts_at"
	}
	if !slices.Contains([]string{"draft", "active", "closed", "cancelled"}, in.Status) {
		return "invalid status"
	}
	if len(in.Rules) == 0 {
		in.Rules = []byte("{}")
	}
	if !json.Valid(in.Rules) {
		return "rules must be valid JSON"
	}
	return ""
}
func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var in eventInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validateEvent(&in); msg != "" {
		writeError(w, 400, "invalid_event", msg)
		return
	}
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `INSERT INTO events(name,description,event_type,game_id,starts_at,ends_at,status,rules) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, in.Name, in.Description, in.EventType, in.GameID, in.StartsAt, in.EndsAt, in.Status, in.Rules).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "event.create", "event", id.String(), nil)
	writeJSON(w, 201, map[string]any{"event": map[string]any{"id": id, "name": in.Name, "status": in.Status}})
}
func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in eventInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validateEvent(&in); msg != "" {
		writeError(w, 400, "invalid_event", msg)
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE events SET name=$2,description=$3,event_type=$4,game_id=$5,starts_at=$6,ends_at=$7,status=$8,rules=$9 WHERE id=$1`, id, in.Name, in.Description, in.EventType, in.GameID, in.StartsAt, in.EndsAt, in.Status, in.Rules)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "event not found")
		return
	}
	s.audit(r, "event.update", "event", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE events SET status='cancelled' WHERE id=$1`, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "event not found")
		return
	}
	s.audit(r, "event.cancel", "event", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) joinEvent(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `INSERT INTO event_participants(event_id,user_id) SELECT id,$2 FROM events WHERE id=$1 AND status='active' AND now() BETWEEN starts_at AND ends_at ON CONFLICT DO NOTHING`, id, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM event_participants WHERE event_id=$1 AND user_id=$2)`, id, p.UserID).Scan(&exists)
		if !exists {
			writeError(w, 409, "event_unavailable", "event is not available")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"joined": true, "event_id": id})
}

type achievementInput struct {
	GameID      *uuid.UUID      `json:"game_id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	IconURL     string          `json:"icon_url"`
	Criteria    json.RawMessage `json:"criteria"`
	XP          int             `json:"xp"`
	Active      bool            `json:"active"`
}

func (s *Server) listAchievements(w http.ResponseWriter, r *http.Request) {
	s.queryAchievements(w, r, false)
}
func (s *Server) adminListAchievements(w http.ResponseWriter, r *http.Request) {
	s.queryAchievements(w, r, true)
}
func (s *Server) queryAchievements(w http.ResponseWriter, r *http.Request, admin bool) {
	where := "WHERE a.active"
	if admin {
		where = ""
	}
	rows, err := s.DB.Query(r.Context(), `SELECT a.id,a.game_id,a.code,a.name,a.description,a.icon_url,a.criteria,a.xp,a.active,a.created_at FROM achievements a `+where+` ORDER BY a.name`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var gid *uuid.UUID
		var code, name, desc, icon string
		var criteria json.RawMessage
		var xp int
		var active bool
		var created time.Time
		if err := rows.Scan(&id, &gid, &code, &name, &desc, &icon, &criteria, &xp, &active, &created); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "game_id": gid, "code": code, "name": name, "description": desc, "icon_url": icon, "criteria": criteria, "xp": xp, "active": active, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func validateAchievement(in *achievementInput) string {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" || in.Name == "" {
		return "code and name are required"
	}
	if len(in.Criteria) == 0 {
		in.Criteria = []byte("{}")
	}
	if !json.Valid(in.Criteria) {
		return "criteria must be valid JSON"
	}
	if in.XP < 0 {
		return "xp cannot be negative"
	}
	return ""
}
func (s *Server) createAchievement(w http.ResponseWriter, r *http.Request) {
	var in achievementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validateAchievement(&in); msg != "" {
		writeError(w, 400, "invalid_achievement", msg)
		return
	}
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `INSERT INTO achievements(game_id,code,name,description,icon_url,criteria,xp,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, in.GameID, in.Code, in.Name, in.Description, in.IconURL, in.Criteria, in.XP, in.Active).Scan(&id)
	if err != nil {
		writeError(w, 409, "achievement_conflict", "achievement code already exists or data is invalid")
		return
	}
	s.audit(r, "achievement.create", "achievement", id.String(), nil)
	writeJSON(w, 201, map[string]any{"achievement": map[string]any{"id": id, "code": in.Code, "name": in.Name}})
}
func (s *Server) updateAchievement(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in achievementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validateAchievement(&in); msg != "" {
		writeError(w, 400, "invalid_achievement", msg)
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE achievements SET game_id=$2,code=$3,name=$4,description=$5,icon_url=$6,criteria=$7,xp=$8,active=$9 WHERE id=$1`, id, in.GameID, in.Code, in.Name, in.Description, in.IconURL, in.Criteria, in.XP, in.Active)
	if err != nil {
		writeError(w, 409, "achievement_conflict", "achievement code already exists or data is invalid")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "achievement not found")
		return
	}
	s.audit(r, "achievement.update", "achievement", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteAchievement(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE achievements SET active=false WHERE id=$1`, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "achievement not found")
		return
	}
	s.audit(r, "achievement.disable", "achievement", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) myAchievements(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	rows, err := s.DB.Query(r.Context(), `SELECT a.id,a.game_id,a.code,a.name,a.description,a.icon_url,a.xp,ua.unlocked_at,ua.metadata FROM user_achievements ua JOIN achievements a ON a.id=ua.achievement_id WHERE ua.user_id=$1 ORDER BY ua.unlocked_at DESC`, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	xp := 0
	for rows.Next() {
		var id uuid.UUID
		var gid *uuid.UUID
		var code, name, desc, icon string
		var points int
		var unlocked time.Time
		var metadata json.RawMessage
		if err := rows.Scan(&id, &gid, &code, &name, &desc, &icon, &points, &unlocked, &metadata); err != nil {
			dbError(w, err)
			return
		}
		xp += points
		items = append(items, map[string]any{"id": id, "game_id": gid, "code": code, "name": name, "description": desc, "icon_url": icon, "xp": points, "unlocked_at": unlocked, "metadata": metadata})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total_xp": xp, "level": levelForXP(xp)})
}
func levelForXP(xp int) int {
	level := 1
	threshold := 100
	for xp >= threshold {
		level++
		threshold = threshold*2 + 100
	}
	return level
}

func (s *Server) unlockAchievement(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in struct {
		Code         string          `json:"code"`
		SessionID    uuid.UUID       `json:"session_id"`
		SessionToken string          `json:"session_token"`
		Metadata     json.RawMessage `json:"metadata"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Code = strings.TrimSpace(in.Code)
	if in.Code == "" || in.SessionID == uuid.Nil || in.SessionToken == "" {
		writeError(w, 400, "invalid_achievement_unlock", "code, session_id and session_token are required")
		return
	}
	if len(in.Metadata) == 0 {
		in.Metadata = []byte("{}")
	}
	hash := sha256.Sum256([]byte(in.SessionToken))
	var achievementID uuid.UUID
	err := s.DB.QueryRow(r.Context(), `SELECT a.id FROM achievements a JOIN game_sessions gs ON gs.game_id=a.game_id WHERE a.code=$1 AND a.active AND COALESCE((a.criteria->>'client_unlockable')::boolean,false) AND gs.id=$2 AND gs.user_id=$3 AND gs.session_token_hash=$4 AND gs.status IN ('active','finished')`, in.Code, in.SessionID, p.UserID, hash[:]).Scan(&achievementID)
	if err != nil {
		writeError(w, 403, "achievement_not_unlockable", "achievement cannot be unlocked by this session")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `INSERT INTO user_achievements(user_id,achievement_id,metadata) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, p.UserID, achievementID, in.Metadata)
	if err != nil {
		dbError(w, err)
		return
	}
	status := http.StatusCreated
	if tag.RowsAffected() == 0 {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"achievement_id": achievementID, "unlocked": true})
}

type workflowInput struct {
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *uuid.UUID      `json:"resource_id"`
	Payload      json.RawMessage `json:"payload"`
}
type approvalSetting struct {
	Enabled            bool  `json:"enabled"`
	ManagerRequired    bool  `json:"manager_required"`
	SeparationOfDuties *bool `json:"separation_of_duties,omitempty"`
}

func (a approvalSetting) separatesDuties() bool {
	return a.SeparationOfDuties == nil || *a.SeparationOfDuties
}

func (s *Server) createWorkflowRequest(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in workflowInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !allowedWorkflowAction(in.Action, in.ResourceType) {
		writeError(w, 400, "invalid_workflow_action", "unsupported workflow action")
		return
	}
	if in.Action == "update" {
		if in.ResourceID == nil {
			writeError(w, 400, "resource_required", "resource_id is required for update")
			return
		}
		if !s.canModifyGame(r.Context(), p, *in.ResourceID) {
			writeError(w, 403, "forbidden", "only the game owner or an operator can request this update")
			return
		}
	}
	if len(in.Payload) == 0 {
		in.Payload = []byte("{}")
	}
	if !json.Valid(in.Payload) {
		writeError(w, 400, "invalid_payload", "payload must be valid JSON")
		return
	}
	var cfg approvalSetting
	if err := s.setting(r.Context(), "approval", &cfg); err != nil {
		writeError(w, http.StatusServiceUnavailable, "approval_setting_unavailable", "approval policy is unavailable")
		return
	}
	if !cfg.Enabled {
		id, err := s.applyWorkflow(r.Context(), p, in)
		if err != nil {
			writeError(w, 400, "apply_failed", err.Error())
			return
		}
		s.audit(r, "workflow.direct_apply", in.ResourceType, id.String(), map[string]any{"action": in.Action})
		writeJSON(w, 200, map[string]any{"status": "applied", "resource_id": id, "approval_required": false})
		return
	}
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `INSERT INTO workflow_requests(requester_id,action,resource_type,resource_id,payload) VALUES($1,$2,$3,$4,$5) RETURNING id`, p.UserID, in.Action, in.ResourceType, in.ResourceID, in.Payload).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "workflow.submit", "workflow_request", id.String(), map[string]any{"action": in.Action})
	writeJSON(w, 202, map[string]any{"request": map[string]any{"id": id, "status": "pending", "approval_required": true}})
}
func allowedWorkflowAction(action, typ string) bool {
	return (typ == "game" && (action == "create" || action == "update"))
}
func (s *Server) canModifyGame(ctx context.Context, p Principal, id uuid.UUID) bool {
	if p.Role == "operator" || p.Role == "admin" {
		return true
	}
	var owner *uuid.UUID
	if err := s.DB.QueryRow(ctx, `SELECT created_by FROM games WHERE id=$1`, id).Scan(&owner); err != nil || owner == nil {
		return false
	}
	return *owner == p.UserID
}
func (s *Server) applyWorkflow(ctx context.Context, p Principal, in workflowInput) (uuid.UUID, error) {
	var game gameInput
	if err := json.Unmarshal(in.Payload, &game); err != nil {
		return uuid.Nil, fmt.Errorf("invalid game payload")
	}
	if err := game.normalize(); err != nil {
		return uuid.Nil, err
	}
	if in.Action == "create" {
		var id uuid.UUID
		err := s.DB.QueryRow(ctx, `INSERT INTO games(slug,name,description,category_id,tags,thumbnail_url,banner_url,game_url,game_type,multiplayer,ranking_enabled,achievement_enabled,season_enabled,min_players,max_players,status,version,developer,score_order,score_rules,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) RETURNING id`, game.Slug, game.Name, game.Description, game.CategoryID, game.Tags, game.ThumbnailURL, game.BannerURL, game.GameURL, game.GameType, game.Multiplayer, game.RankingEnabled, game.AchievementEnabled, game.SeasonEnabled, game.MinPlayers, game.MaxPlayers, game.Status, game.Version, game.Developer, game.ScoreOrder, game.ScoreRules, p.UserID).Scan(&id)
		return id, err
	}
	if in.ResourceID == nil {
		return uuid.Nil, fmt.Errorf("resource_id is required")
	}
	if !s.canModifyGame(ctx, p, *in.ResourceID) {
		return uuid.Nil, fmt.Errorf("only the game owner or an operator can update this game")
	}
	var currentSlug string
	if err := s.DB.QueryRow(ctx, `SELECT slug FROM games WHERE id=$1`, *in.ResourceID).Scan(&currentSlug); err != nil {
		return uuid.Nil, fmt.Errorf("load game identity: %w", err)
	}
	if (currentSlug == realmGuardSlug) != (game.Slug == realmGuardSlug) {
		return uuid.Nil, fmt.Errorf("protected game identity: the RealmGuard slug is reserved for its built-in authoritative runtime")
	}
	tag, err := s.DB.Exec(ctx, `UPDATE games SET slug=$2,name=$3,description=$4,category_id=$5,tags=$6,thumbnail_url=$7,banner_url=$8,game_url=$9,game_type=$10,multiplayer=$11,ranking_enabled=$12,achievement_enabled=$13,season_enabled=$14,min_players=$15,max_players=$16,status=$17,version=$18,developer=$19,score_order=$20,score_rules=$21,updated_at=now() WHERE id=$1`, *in.ResourceID, game.Slug, game.Name, game.Description, game.CategoryID, game.Tags, game.ThumbnailURL, game.BannerURL, game.GameURL, game.GameType, game.Multiplayer, game.RankingEnabled, game.AchievementEnabled, game.SeasonEnabled, game.MinPlayers, game.MaxPlayers, game.Status, game.Version, game.Developer, game.ScoreOrder, game.ScoreRules)
	if err != nil {
		return uuid.Nil, err
	}
	if tag.RowsAffected() == 0 {
		return uuid.Nil, fmt.Errorf("game not found")
	}
	return *in.ResourceID, nil
}
func (s *Server) listMyWorkflowRequests(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	s.queryWorkflow(w, r, &p.UserID, nil, "all")
}
func (s *Server) adminListWorkflowRequests(w http.ResponseWriter, r *http.Request) {
	s.queryWorkflow(w, r, nil, nil, "all")
}
func (s *Server) listWorkflowReviews(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	if p.Role != "manager" && p.Role != "operator" && p.Role != "admin" {
		writeError(w, 403, "forbidden", "reviewer role required")
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	if !slices.Contains([]string{"all", "pending", "approved", "rejected", "applied", "cancelled"}, status) {
		writeError(w, 400, "invalid_status", "invalid review status")
		return
	}
	var reviewerTeam *string
	if p.Role == "manager" {
		team := strings.TrimSpace(p.Team)
		if team == "" {
			writeError(w, 403, "team_required", "managers require a team assignment to view review requests")
			return
		}
		reviewerTeam = &team
	}
	s.queryWorkflow(w, r, nil, reviewerTeam, status)
}
func (s *Server) queryWorkflow(w http.ResponseWriter, r *http.Request, requester *uuid.UUID, reviewerTeam *string, status string) {
	rows, err := s.DB.Query(r.Context(), `SELECT wr.id,wr.requester_id,COALESCE(u.username,''),COALESCE(u.display_name,''),COALESCE(u.department,''),COALESCE(u.team,''),wr.reviewer_id,COALESCE(reviewer.username,''),wr.action,wr.resource_type,wr.resource_id,wr.payload,wr.status,wr.comment,wr.created_at,wr.reviewed_at,wr.applied_at FROM workflow_requests wr LEFT JOIN users u ON u.id=wr.requester_id LEFT JOIN users reviewer ON reviewer.id=wr.reviewer_id WHERE ($1::uuid IS NULL OR wr.requester_id=$1) AND ($2::text IS NULL OR (u.team=$2 AND u.team<>'')) AND ($3='all' OR wr.status=$3) ORDER BY wr.created_at DESC`, requester, reviewerTeam, status)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, requester uuid.UUID
		var username, displayName, department, team, reviewerUsername, action, typ, itemStatus, comment string
		var reviewer, resource *uuid.UUID
		var payload json.RawMessage
		var created time.Time
		var reviewed, applied *time.Time
		if err := rows.Scan(&id, &requester, &username, &displayName, &department, &team, &reviewer, &reviewerUsername, &action, &typ, &resource, &payload, &itemStatus, &comment, &created, &reviewed, &applied); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "requester_id": requester, "requester_username": username, "requester_display_name": displayName, "requester_department": department, "requester_team": team, "reviewer_id": reviewer, "reviewer_username": reviewerUsername, "action": action, "resource_type": typ, "resource_id": resource, "payload": payload, "status": itemStatus, "comment": comment, "created_at": created, "reviewed_at": reviewed, "applied_at": applied})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) reviewWorkflowRequest(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	if p.Role != "manager" && p.Role != "operator" && p.Role != "admin" {
		writeError(w, 403, "forbidden", "manager role required")
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Decision != "approved" && in.Decision != "rejected" {
		writeError(w, 400, "invalid_decision", "decision must be approved or rejected")
		return
	}
	if in.Decision == "rejected" && strings.TrimSpace(in.Comment) == "" {
		writeError(w, 400, "comment_required", "a rejection comment is required")
		return
	}
	var cfg approvalSetting
	if err := s.setting(r.Context(), "approval", &cfg); err != nil {
		writeError(w, http.StatusServiceUnavailable, "approval_setting_unavailable", "approval policy is unavailable")
		return
	}
	if cfg.ManagerRequired && p.Role != "manager" && p.Role != "admin" {
		writeError(w, 403, "manager_required", "a manager must review this request")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var input workflowInput
	input.ResourceID = nil
	var resource *uuid.UUID
	var requesterID uuid.UUID
	var requesterTeam, requesterRole string
	err = tx.QueryRow(r.Context(), `SELECT wr.requester_id,COALESCE(u.team,''),u.role,wr.action,wr.resource_type,wr.resource_id,wr.payload FROM workflow_requests wr JOIN users u ON u.id=wr.requester_id WHERE wr.id=$1 AND wr.status='pending' FOR UPDATE`, id).Scan(&requesterID, &requesterTeam, &requesterRole, &input.Action, &input.ResourceType, &resource, &input.Payload)
	if err != nil {
		writeError(w, 409, "not_pending", "workflow request is not pending")
		return
	}
	if cfg.separatesDuties() && requesterID == p.UserID {
		writeError(w, 403, "self_approval_forbidden", "requesters cannot review their own requests")
		return
	}
	if p.Role == "manager" {
		managerTeam, creatorTeam := strings.TrimSpace(p.Team), strings.TrimSpace(requesterTeam)
		if managerTeam == "" || creatorTeam == "" {
			writeError(w, 403, "team_required", "manager and requester require team assignments for review")
			return
		}
		if managerTeam != creatorTeam {
			writeError(w, 403, "different_team", "managers can only review requests from their team")
			return
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE workflow_requests SET status=$2,reviewer_id=$3,comment=$4,reviewed_at=now() WHERE id=$1`, id, in.Decision, p.UserID, in.Comment)
	if err != nil {
		dbError(w, err)
		return
	}
	input.ResourceID = resource
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	status := in.Decision
	if in.Decision == "approved" {
		requesterPrincipal := p
		requesterPrincipal.UserID = requesterID
		requesterPrincipal.Role = requesterRole
		resourceID, applyErr := s.applyWorkflow(r.Context(), requesterPrincipal, input)
		if applyErr != nil {
			_, _ = s.DB.Exec(r.Context(), `UPDATE workflow_requests SET status='pending',reviewer_id=NULL,reviewed_at=NULL,comment=comment||$2 WHERE id=$1`, id, "\nApply failed: "+applyErr.Error())
			writeError(w, 409, "apply_failed", applyErr.Error())
			return
		}
		_, _ = s.DB.Exec(r.Context(), `UPDATE workflow_requests SET status='applied',resource_id=$2,applied_at=now() WHERE id=$1`, id, resourceID)
		status = "applied"
	}
	s.audit(r, "workflow.review", "workflow_request", id.String(), map[string]any{"decision": in.Decision})
	writeJSON(w, 200, map[string]any{"id": id, "status": status})
}
