package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) serviceLocation(ctx context.Context) *time.Location {
	var cfg struct {
		Timezone string `json:"timezone"`
	}
	_ = s.setting(ctx, "service", &cfg)
	if cfg.Timezone == "" {
		cfg.Timezone = "Asia/Seoul"
	}
	location, err := loadLocation(cfg.Timezone)
	if err != nil {
		return time.FixedZone("KST", 9*60*60)
	}
	return location
}
func (s *Server) parseAdminTime(ctx context.Context, value string, required bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			now := s.Now()
			return &now, nil
		}
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, s.serviceLocation(ctx)); err == nil {
			parsed = parsed.UTC()
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("invalid date/time %q", value)
}
func optionalUUID(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID")
	}
	return &id, nil
}
func validManagedURL(value string, required bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return !required
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

type tournamentInput struct {
	ID              string          `json:"id,omitempty"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	GameID          string          `json:"game_id"`
	GameName        string          `json:"game_name,omitempty"`
	Format          string          `json:"format"`
	MaxParticipants int             `json:"max_participants"`
	StartsAt        string          `json:"starts_at"`
	EndsAt          string          `json:"ends_at"`
	Status          string          `json:"status"`
	Rules           json.RawMessage `json:"rules"`
	CreatedAt       string          `json:"created_at,omitempty"`
	UpdatedAt       string          `json:"updated_at,omitempty"`
}

func (s *Server) normalizeTournament(ctx context.Context, in *tournamentInput) (*uuid.UUID, *time.Time, *time.Time, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, nil, nil, fmt.Errorf("name is required")
	}
	if in.Format == "" {
		in.Format = "score_attack"
	}
	if !slices.Contains([]string{"score_attack", "time_attack", "survival", "bracket", "team_battle"}, in.Format) {
		return nil, nil, nil, fmt.Errorf("invalid format")
	}
	if in.MaxParticipants == 0 {
		in.MaxParticipants = 128
	}
	if in.MaxParticipants < 1 || in.MaxParticipants > 100000 {
		return nil, nil, nil, fmt.Errorf("max_participants must be between 1 and 100000")
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if !slices.Contains([]string{"draft", "active", "closed", "cancelled"}, in.Status) {
		return nil, nil, nil, fmt.Errorf("invalid status")
	}
	gameID, err := optionalUUID(in.GameID)
	if err != nil {
		return nil, nil, nil, err
	}
	starts, err := s.parseAdminTime(ctx, in.StartsAt, true)
	if err != nil {
		return nil, nil, nil, err
	}
	ends, err := s.parseAdminTime(ctx, in.EndsAt, false)
	if err != nil {
		return nil, nil, nil, err
	}
	if ends != nil && !ends.After(*starts) {
		return nil, nil, nil, fmt.Errorf("ends_at must be after starts_at")
	}
	if len(in.Rules) == 0 {
		in.Rules = []byte("{}")
	}
	if !json.Valid(in.Rules) {
		return nil, nil, nil, fmt.Errorf("rules must be valid JSON")
	}
	return gameID, starts, ends, nil
}
func (s *Server) listTournaments(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT t.id,t.name,t.description,t.game_id,COALESCE(g.name,''),t.format,t.max_participants,t.starts_at,t.ends_at,t.status,t.rules,t.created_at,t.updated_at FROM tournaments t LEFT JOIN games g ON g.id=t.game_id ORDER BY t.starts_at DESC`)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, desc, gameName, format, status string
		var gameID *uuid.UUID
		var max int
		var starts, created, updated time.Time
		var ends *time.Time
		var rules json.RawMessage
		if err := rows.Scan(&id, &name, &desc, &gameID, &gameName, &format, &max, &starts, &ends, &status, &rules, &created, &updated); err != nil {
			s.dbError(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "description": desc, "game_id": gameID, "game_name": gameName, "format": format, "max_participants": max, "starts_at": starts, "ends_at": ends, "status": status, "rules": rules, "created_at": created, "updated_at": updated})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) createTournament(w http.ResponseWriter, r *http.Request) {
	var in tournamentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	gameID, starts, ends, err := s.normalizeTournament(r.Context(), &in)
	if err != nil {
		writeError(w, 400, "invalid_tournament", err.Error())
		return
	}
	var id uuid.UUID
	err = s.DB.QueryRow(r.Context(), `INSERT INTO tournaments(name,description,game_id,format,max_participants,starts_at,ends_at,status,rules) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, in.Name, in.Description, gameID, in.Format, in.MaxParticipants, starts, ends, in.Status, in.Rules).Scan(&id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(r, "tournament.create", "tournament", id.String(), nil)
	writeJSON(w, 201, map[string]any{"item": map[string]any{"id": id, "name": in.Name, "description": in.Description, "game_id": gameID, "format": in.Format, "max_participants": in.MaxParticipants, "starts_at": starts, "ends_at": ends, "status": in.Status, "rules": in.Rules}})
}
func (s *Server) updateTournament(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in tournamentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	gameID, starts, ends, err := s.normalizeTournament(r.Context(), &in)
	if err != nil {
		writeError(w, 400, "invalid_tournament", err.Error())
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE tournaments SET name=$2,description=$3,game_id=$4,format=$5,max_participants=$6,starts_at=$7,ends_at=$8,status=$9,rules=$10,updated_at=now() WHERE id=$1`, id, in.Name, in.Description, gameID, in.Format, in.MaxParticipants, starts, ends, in.Status, in.Rules)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "tournament not found")
		return
	}
	s.audit(r, "tournament.update", "tournament", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteTournament(w http.ResponseWriter, r *http.Request) {
	s.deleteExtended(w, r, "tournaments", "tournament")
}

type rewardInput struct {
	ID          string          `json:"id,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Type        string          `json:"type"`
	Metadata    json.RawMessage `json:"metadata"`
	Enabled     *bool           `json:"enabled"`
	CreatedAt   string          `json:"created_at,omitempty"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
}

func normalizeReward(in *rewardInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return fmt.Errorf("name is required")
	}
	if in.Type == "" {
		in.Type = "badge"
	}
	if !slices.Contains([]string{"badge", "title", "avatar_frame"}, in.Type) {
		return fmt.Errorf("invalid reward type")
	}
	if in.Enabled == nil {
		enabled := true
		in.Enabled = &enabled
	}
	if len(in.Metadata) == 0 {
		in.Metadata = []byte("{}")
	}
	if !json.Valid(in.Metadata) {
		return fmt.Errorf("metadata must be valid JSON")
	}
	return nil
}
func (s *Server) listRewards(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT id,name,description,reward_type,metadata,enabled,created_at,updated_at FROM rewards ORDER BY created_at DESC`)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, desc, typ string
		var metadata json.RawMessage
		var enabled bool
		var created, updated time.Time
		if err := rows.Scan(&id, &name, &desc, &typ, &metadata, &enabled, &created, &updated); err != nil {
			s.dbError(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "description": desc, "type": typ, "metadata": metadata, "enabled": enabled, "created_at": created, "updated_at": updated})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) createReward(w http.ResponseWriter, r *http.Request) {
	var in rewardInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := normalizeReward(&in); err != nil {
		writeError(w, 400, "invalid_reward", err.Error())
		return
	}
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `INSERT INTO rewards(name,description,reward_type,metadata,enabled) VALUES($1,$2,$3,$4,$5) RETURNING id`, in.Name, in.Description, in.Type, in.Metadata, *in.Enabled).Scan(&id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(r, "reward.create", "reward", id.String(), nil)
	writeJSON(w, 201, map[string]any{"item": map[string]any{"id": id, "name": in.Name, "description": in.Description, "type": in.Type, "metadata": in.Metadata, "enabled": *in.Enabled}})
}
func (s *Server) updateReward(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in rewardInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := normalizeReward(&in); err != nil {
		writeError(w, 400, "invalid_reward", err.Error())
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE rewards SET name=$2,description=$3,reward_type=$4,metadata=$5,enabled=$6,updated_at=now() WHERE id=$1`, id, in.Name, in.Description, in.Type, in.Metadata, *in.Enabled)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "reward not found")
		return
	}
	s.audit(r, "reward.update", "reward", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteReward(w http.ResponseWriter, r *http.Request) {
	s.deleteExtended(w, r, "rewards", "reward")
}

type noticeInput struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	Status      string `json:"status"`
	Pinned      bool   `json:"pinned"`
	PublishedAt string `json:"published_at"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func (s *Server) normalizeNotice(ctx context.Context, in *noticeInput) (*time.Time, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	if in.Title == "" || in.Content == "" {
		return nil, fmt.Errorf("title and content are required")
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if !slices.Contains([]string{"draft", "published"}, in.Status) {
		return nil, fmt.Errorf("invalid status")
	}
	published, err := s.parseAdminTime(ctx, in.PublishedAt, false)
	if err != nil {
		return nil, err
	}
	if in.Status == "published" && published == nil {
		now := s.Now()
		published = &now
	}
	if in.Status == "draft" {
		published = nil
	}
	return published, nil
}
func (s *Server) listAdminNotices(w http.ResponseWriter, r *http.Request) { s.queryNotices(w, r, true) }
func (s *Server) listPublicNotices(w http.ResponseWriter, r *http.Request) {
	s.queryNotices(w, r, false)
}
func (s *Server) queryNotices(w http.ResponseWriter, r *http.Request, admin bool) {
	where := "WHERE status='published' AND published_at<=now()"
	if admin {
		where = ""
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,title,content,status,pinned,published_at,created_at,updated_at FROM notices `+where+` ORDER BY pinned DESC,published_at DESC NULLS LAST,created_at DESC`)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var title, content, status string
		var pinned bool
		var published *time.Time
		var created, updated time.Time
		if err := rows.Scan(&id, &title, &content, &status, &pinned, &published, &created, &updated); err != nil {
			s.dbError(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "content": content, "status": status, "pinned": pinned, "published_at": published, "created_at": created, "updated_at": updated})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) createNotice(w http.ResponseWriter, r *http.Request) {
	var in noticeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	published, err := s.normalizeNotice(r.Context(), &in)
	if err != nil {
		writeError(w, 400, "invalid_notice", err.Error())
		return
	}
	var id uuid.UUID
	err = s.DB.QueryRow(r.Context(), `INSERT INTO notices(title,content,status,pinned,published_at) VALUES($1,$2,$3,$4,$5) RETURNING id`, in.Title, in.Content, in.Status, in.Pinned, published).Scan(&id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(r, "notice.create", "notice", id.String(), nil)
	writeJSON(w, 201, map[string]any{"item": map[string]any{"id": id, "title": in.Title, "content": in.Content, "status": in.Status, "pinned": in.Pinned, "published_at": published}})
}
func (s *Server) updateNotice(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in noticeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	published, err := s.normalizeNotice(r.Context(), &in)
	if err != nil {
		writeError(w, 400, "invalid_notice", err.Error())
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE notices SET title=$2,content=$3,status=$4,pinned=$5,published_at=$6,updated_at=now() WHERE id=$1`, id, in.Title, in.Content, in.Status, in.Pinned, published)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "notice not found")
		return
	}
	s.audit(r, "notice.update", "notice", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteNotice(w http.ResponseWriter, r *http.Request) {
	s.deleteExtended(w, r, "notices", "notice")
}

type bannerInput struct {
	ID        string `json:"id,omitempty"`
	Title     string `json:"title"`
	ImageURL  string `json:"image_url"`
	LinkURL   string `json:"link_url"`
	StartsAt  string `json:"starts_at"`
	EndsAt    string `json:"ends_at"`
	SortOrder int    `json:"sort_order"`
	Enabled   *bool  `json:"enabled"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (s *Server) normalizeBanner(ctx context.Context, in *bannerInput) (*time.Time, *time.Time, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || !validManagedURL(in.ImageURL, true) || !validManagedURL(in.LinkURL, false) {
		return nil, nil, fmt.Errorf("title and a safe image_url are required")
	}
	starts, err := s.parseAdminTime(ctx, in.StartsAt, true)
	if err != nil {
		return nil, nil, err
	}
	ends, err := s.parseAdminTime(ctx, in.EndsAt, false)
	if err != nil {
		return nil, nil, err
	}
	if ends != nil && !ends.After(*starts) {
		return nil, nil, fmt.Errorf("ends_at must be after starts_at")
	}
	if in.Enabled == nil {
		enabled := true
		in.Enabled = &enabled
	}
	return starts, ends, nil
}
func (s *Server) listAdminBanners(w http.ResponseWriter, r *http.Request) { s.queryBanners(w, r, true) }
func (s *Server) listPublicBanners(w http.ResponseWriter, r *http.Request) {
	s.queryBanners(w, r, false)
}
func (s *Server) queryBanners(w http.ResponseWriter, r *http.Request, admin bool) {
	where := "WHERE enabled AND starts_at<=now() AND (ends_at IS NULL OR ends_at>now())"
	if admin {
		where = ""
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,title,image_url,link_url,starts_at,ends_at,sort_order,enabled,created_at,updated_at FROM banners `+where+` ORDER BY sort_order,starts_at DESC`)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var title, image, link string
		var starts, created, updated time.Time
		var ends *time.Time
		var order int
		var enabled bool
		if err := rows.Scan(&id, &title, &image, &link, &starts, &ends, &order, &enabled, &created, &updated); err != nil {
			s.dbError(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "image_url": image, "link_url": link, "starts_at": starts, "ends_at": ends, "sort_order": order, "enabled": enabled, "created_at": created, "updated_at": updated})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) createBanner(w http.ResponseWriter, r *http.Request) {
	var in bannerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	starts, ends, err := s.normalizeBanner(r.Context(), &in)
	if err != nil {
		writeError(w, 400, "invalid_banner", err.Error())
		return
	}
	var id uuid.UUID
	err = s.DB.QueryRow(r.Context(), `INSERT INTO banners(title,image_url,link_url,starts_at,ends_at,sort_order,enabled) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, in.Title, in.ImageURL, in.LinkURL, starts, ends, in.SortOrder, *in.Enabled).Scan(&id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(r, "banner.create", "banner", id.String(), nil)
	writeJSON(w, 201, map[string]any{"item": map[string]any{"id": id, "title": in.Title, "image_url": in.ImageURL, "link_url": in.LinkURL, "starts_at": starts, "ends_at": ends, "sort_order": in.SortOrder, "enabled": *in.Enabled}})
}
func (s *Server) updateBanner(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in bannerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	starts, ends, err := s.normalizeBanner(r.Context(), &in)
	if err != nil {
		writeError(w, 400, "invalid_banner", err.Error())
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE banners SET title=$2,image_url=$3,link_url=$4,starts_at=$5,ends_at=$6,sort_order=$7,enabled=$8,updated_at=now() WHERE id=$1`, id, in.Title, in.ImageURL, in.LinkURL, starts, ends, in.SortOrder, *in.Enabled)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "banner not found")
		return
	}
	s.audit(r, "banner.update", "banner", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteBanner(w http.ResponseWriter, r *http.Request) {
	s.deleteExtended(w, r, "banners", "banner")
}

func (s *Server) deleteExtended(w http.ResponseWriter, r *http.Request, table, resource string) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	allowed := map[string]bool{"tournaments": true, "rewards": true, "notices": true, "banners": true}
	if !allowed[table] {
		writeError(w, 500, "internal_error", "invalid resource")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM `+table+` WHERE id=$1`, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", resource+" not found")
		return
	}
	s.audit(r, resource+".delete", resource, id.String(), nil)
	w.WriteHeader(204)
}

func (s *Server) adminRankings(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	status := r.URL.Query().Get("status")
	game := r.URL.Query().Get("game_id")
	rows, err := s.DB.Query(r.Context(), `SELECT s.id,s.session_id,s.user_id,u.username,u.display_name,u.department,u.team,s.game_id,g.name,s.score,s.verified,s.moderation_status,s.rejection_reason,s.metadata,s.created_at FROM scores s JOIN users u ON u.id=s.user_id JOIN games g ON g.id=s.game_id WHERE ($1='' OR s.moderation_status=$1) AND ($2='' OR s.game_id::text=$2 OR g.slug=$2) ORDER BY s.created_at DESC LIMIT $3 OFFSET $4`, status, game, limit, offset)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sessionID, userID, gameID uuid.UUID
		var username, display, dept, team, gameName, status, reason string
		var score int64
		var verified bool
		var metadata json.RawMessage
		var created time.Time
		if err := rows.Scan(&id, &sessionID, &userID, &username, &display, &dept, &team, &gameID, &gameName, &score, &verified, &status, &reason, &metadata, &created); err != nil {
			s.dbError(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "session_id": sessionID, "user_id": userID, "username": username, "display_name": display, "department": dept, "team": team, "game_id": gameID, "game_name": gameName, "score": score, "verified": verified, "status": status, "rejection_reason": reason, "metadata": metadata, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

type moderationInput struct {
	ID              string          `json:"id,omitempty"`
	SessionID       string          `json:"session_id,omitempty"`
	UserID          string          `json:"user_id,omitempty"`
	Username        string          `json:"username,omitempty"`
	DisplayName     string          `json:"display_name,omitempty"`
	Department      string          `json:"department,omitempty"`
	Team            string          `json:"team,omitempty"`
	GameID          string          `json:"game_id,omitempty"`
	GameName        string          `json:"game_name,omitempty"`
	Score           *int64          `json:"score,omitempty"`
	Verified        *bool           `json:"verified,omitempty"`
	Status          string          `json:"status"`
	RejectionReason string          `json:"rejection_reason,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	CreatedAt       string          `json:"created_at,omitempty"`
}

func (s *Server) moderateRanking(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in moderationInput
	if !decodeJSON(w, r, &in) {
		return
	}
	status := in.Status
	if status == "" && in.Verified != nil {
		if *in.Verified {
			status = "valid"
		} else {
			status = "excluded"
		}
	}
	if !slices.Contains([]string{"valid", "flagged", "excluded"}, status) {
		writeError(w, 400, "invalid_status", "status must be valid, flagged, or excluded")
		return
	}
	verified := status == "valid"
	reason := strings.TrimSpace(in.RejectionReason)
	if !verified && reason == "" {
		reason = "moderated_" + status
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE scores SET moderation_status=$2,verified=$3,rejection_reason=$4 WHERE id=$1`, id, status, verified, reason)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "ranking record not found")
		return
	}
	s.audit(r, "ranking.moderate", "score", id.String(), map[string]any{"status": status, "reason": reason})
	writeJSON(w, 200, map[string]any{"item": map[string]any{"id": id, "status": status, "verified": verified, "rejection_reason": reason}})
}
func (s *Server) excludeRanking(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE scores SET moderation_status='excluded',verified=false,rejection_reason='moderated_excluded' WHERE id=$1`, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "ranking record not found")
		return
	}
	s.audit(r, "ranking.exclude", "score", id.String(), nil)
	w.WriteHeader(204)
}

func (s *Server) analyticsData(ctx context.Context) (map[string]any, error) {
	location := s.serviceLocation(ctx)
	now := s.Now().In(location)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).UTC()
	weekStart := now.AddDate(0, 0, -7).UTC()
	monthStart := now.AddDate(0, 0, -30).UTC()
	var dau, wau, mau, launches, pending int64
	var avgSession, completion, rankingParticipation, eventParticipation, retention float64
	err := s.DB.QueryRow(ctx, `SELECT
(SELECT count(DISTINCT user_id) FROM game_sessions WHERE started_at >= $1),
(SELECT count(DISTINCT user_id) FROM game_sessions WHERE started_at >= $2),
(SELECT count(DISTINCT user_id) FROM game_sessions WHERE started_at >= $3),
(SELECT count(*) FROM game_sessions WHERE started_at >= $1),
(SELECT COALESCE(avg(duration_ms)/1000.0,0) FROM game_sessions WHERE duration_ms IS NOT NULL AND started_at >= $3),
(SELECT COALESCE(100.0*count(*) FILTER(WHERE status='finished')/NULLIF(count(*),0),0) FROM game_sessions WHERE started_at >= $3),
(SELECT COALESCE(100.0*count(DISTINCT user_id)/NULLIF((SELECT count(DISTINCT user_id) FROM game_sessions WHERE started_at >= $3),0),0) FROM scores WHERE created_at >= $3 AND verified AND moderation_status='valid'),
(SELECT COALESCE(100.0*count(DISTINCT user_id)/NULLIF((SELECT count(*) FROM users WHERE status='active'),0),0) FROM event_participants WHERE joined_at >= $3),
(WITH previous AS (SELECT DISTINCT user_id FROM game_sessions WHERE started_at >= $4 AND started_at < $2), current_users AS (SELECT DISTINCT user_id FROM game_sessions WHERE started_at >= $2) SELECT COALESCE(100.0*count(*) FILTER(WHERE c.user_id IS NOT NULL)/NULLIF(count(*),0),0) FROM previous p LEFT JOIN current_users c USING(user_id)),
(SELECT count(*) FROM workflow_requests WHERE status='pending')`, dayStart, weekStart, monthStart, now.AddDate(0, 0, -14).UTC()).Scan(&dau, &wau, &mau, &launches, &avgSession, &completion, &rankingParticipation, &eventParticipation, &retention, &pending)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT g.name,count(*) launches FROM game_sessions gs JOIN games g ON g.id=gs.game_id WHERE gs.started_at >= $1 GROUP BY g.id,g.name ORDER BY launches DESC,g.name LIMIT 5`, monthStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	popular := []map[string]any{}
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		popular = append(popular, map[string]any{"name": name, "launches": count})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"dau": dau, "wau": wau, "mau": mau, "game_launches": launches, "avg_session_seconds": avgSession, "completion_rate": completion, "ranking_participation": rankingParticipation, "event_participation": eventParticipation, "retention": retention, "pending_approvals": pending, "popular_games": popular}, nil
}
func (s *Server) adminAnalytics(w http.ResponseWriter, r *http.Request) {
	data, err := s.analyticsData(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, data)
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var name, desc, typ, gameName, status string
	var gameID *uuid.UUID
	var starts, ends, created time.Time
	var rules json.RawMessage
	var joined bool
	var participants int64
	err := s.DB.QueryRow(r.Context(), `SELECT e.name,e.description,e.event_type,e.game_id,COALESCE(g.name,''),e.starts_at,e.ends_at,e.status,e.rules,e.created_at,EXISTS(SELECT 1 FROM event_participants ep WHERE ep.event_id=e.id AND ep.user_id=$2),(SELECT count(*) FROM event_participants ep WHERE ep.event_id=e.id) FROM events e LEFT JOIN games g ON g.id=e.game_id WHERE e.id=$1 AND e.status<>'draft'`, id, p.UserID).Scan(&name, &desc, &typ, &gameID, &gameName, &starts, &ends, &status, &rules, &created, &joined, &participants)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"event": map[string]any{"id": id, "name": name, "description": desc, "event_type": typ, "game_id": gameID, "game_name": gameName, "starts_at": starts, "ends_at": ends, "status": status, "rules": rules, "joined": joined, "participant_count": participants, "created_at": created}})
}
