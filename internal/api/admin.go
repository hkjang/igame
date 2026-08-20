package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var editableSettings = map[string]bool{"service": true, "approval": true, "privacy": true, "play_policy": true, "api_keys": true}

func (s *Server) listSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT key,value,updated_at FROM system_settings ORDER BY key`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := map[string]any{}
	updated := map[string]time.Time{}
	for rows.Next() {
		var key string
		var raw json.RawMessage
		var at time.Time
		if err := rows.Scan(&key, &raw, &at); err != nil {
			dbError(w, err)
			return
		}
		var value any
		_ = json.Unmarshal(raw, &value)
		if m, ok := value.(map[string]any); ok {
			if _, exists := m["client_secret"]; exists {
				m["client_secret"] = ""
				m["client_secret_configured"] = secretConfigured(raw, "client_secret")
			}
			if _, exists := m["api_key"]; exists {
				m["api_key"] = ""
				m["api_key_configured"] = secretConfigured(raw, "api_key")
			}
		}
		items[key] = value
		updated[key] = at
	}
	writeJSON(w, 200, map[string]any{"settings": items, "updated_at": updated})
}

func secretConfigured(raw []byte, key string) bool {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	v, _ := m[key].(string)
	return v != ""
}

func (s *Server) getSetting(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chiURLParam(r, "key"))
	if key == "oidc" {
		s.getOIDCSetting(w, r)
		return
	}
	if key == "ai" {
		s.getAISetting(w, r)
		return
	}
	var raw json.RawMessage
	var at time.Time
	err := s.DB.QueryRow(r.Context(), `SELECT value,updated_at FROM system_settings WHERE key=$1`, key).Scan(&raw, &at)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"key": key, "value": raw, "updated_at": at})
}

func (s *Server) putSetting(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chiURLParam(r, "key"))
	if key == "oidc" {
		s.putOIDCSetting(w, r)
		return
	}
	if key == "ai" {
		s.putAISetting(w, r)
		return
	}
	if !editableSettings[key] {
		writeError(w, 400, "setting_not_editable", "unknown or protected setting")
		return
	}
	var wrapper struct {
		Value json.RawMessage `json:"value"`
	}
	if !decodeJSON(w, r, &wrapper) {
		return
	}
	if len(wrapper.Value) == 0 || !json.Valid(wrapper.Value) {
		writeError(w, 400, "invalid_setting", "value must be valid JSON")
		return
	}
	if err := validateSetting(key, wrapper.Value); err != "" {
		writeError(w, 400, "invalid_setting", err)
		return
	}
	p, _ := principalFrom(r)
	_, err := s.DB.Exec(r.Context(), `INSERT INTO system_settings(key,value,updated_by) VALUES($1,$2,$3) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_by=excluded.updated_by,updated_at=now()`, key, wrapper.Value, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "setting.update", "setting", key, nil)
	writeJSON(w, 200, map[string]any{"key": key, "value": wrapper.Value})
}

func validateSetting(key string, raw []byte) string {
	switch key {
	case "service":
		var v struct {
			DisplayName           string   `json:"display_name"`
			Timezone              string   `json:"timezone"`
			PublicURL             string   `json:"public_url"`
			TrustProxy            bool     `json:"trust_proxy"`
			AllowedFrameOrigins   []string `json:"allowed_frame_origins"`
			AllowedConnectOrigins []string `json:"allowed_connect_origins"`
			BootstrapLoginEnabled *bool    `json:"bootstrap_login_enabled,omitempty"`
		}
		if json.Unmarshal(raw, &v) != nil {
			return "invalid service setting"
		}
		if v.PublicURL != "" {
			u, e := url.Parse(v.PublicURL)
			if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
				return "public_url must be an absolute HTTP(S) URL"
			}
		}
		if v.Timezone != "" {
			if _, e := time.LoadLocation(v.Timezone); e != nil {
				return "timezone is invalid"
			}
		}
	case "approval":
		var v struct {
			Enabled            bool  `json:"enabled"`
			ManagerRequired    bool  `json:"manager_required"`
			SeparationOfDuties *bool `json:"separation_of_duties,omitempty"`
		}
		if json.Unmarshal(raw, &v) != nil {
			return "invalid approval setting"
		}
	case "privacy":
		var v struct {
			RankingName    string `json:"ranking_name"`
			ShowDepartment bool   `json:"show_department"`
			RankingOptOut  bool   `json:"ranking_opt_out"`
		}
		if json.Unmarshal(raw, &v) != nil || !slices.Contains([]string{"nickname", "real_name"}, v.RankingName) {
			return "ranking_name must be nickname or real_name"
		}
	case "play_policy":
		var v struct {
			Enabled bool `json:"enabled"`
			Windows []struct {
				Days  []int  `json:"days"`
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"windows"`
			DailyLimits map[string]int `json:"daily_limits"`
		}
		if json.Unmarshal(raw, &v) != nil {
			return "invalid play policy"
		}
		for _, window := range v.Windows {
			if _, err := clockMinutes(window.Start, false); err != nil {
				return "invalid play window start: " + err.Error()
			}
			if _, err := clockMinutes(window.End, true); err != nil {
				return "invalid play window end: " + err.Error()
			}
			for _, day := range window.Days {
				if day < 0 || day > 6 {
					return "play window days must be between 0 and 6"
				}
			}
		}
		for slug, minutes := range v.DailyLimits {
			if strings.TrimSpace(slug) == "" || minutes < 0 || minutes > 1440 {
				return "daily limit for " + slug + " must be between 0 and 1440 minutes"
			}
		}
	case "api_keys":
		var v apiKeyPolicy
		if json.Unmarshal(raw, &v) != nil {
			return "invalid API key policy"
		}
		if v.MaxKeys < 1 || v.MaxKeys > 100 || v.MaxTTLDays < 1 || v.MaxTTLDays > 3650 {
			return "max_keys or max_ttl_days is outside the allowed range"
		}
		for _, scope := range v.AvailablePermissions {
			if !slices.Contains(allowedAPIKeyPermissions, scope) {
				return "unknown API key permission: " + scope
			}
		}
		for _, scopes := range v.RolePermissions {
			for _, scope := range scopes {
				if !slices.Contains(v.AvailablePermissions, scope) {
					return "role permission is not available: " + scope
				}
			}
		}
	}
	return ""
}

func clockMinutes(value string, allow24 bool) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("time must use HH:MM")
	}
	hour, e1 := strconv.Atoi(parts[0])
	minute, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || minute < 0 || minute > 59 || hour < 0 || hour > 23 {
		if allow24 && hour == 24 && minute == 0 && e1 == nil && e2 == nil {
			return 1440, nil
		}
		return 0, fmt.Errorf("time must use 00:00 through %s", map[bool]string{true: "24:00", false: "23:59"}[allow24])
	}
	return hour*60 + minute, nil
}

func (s *Server) getOIDCSetting(w http.ResponseWriter, r *http.Request) {
	var cfg oidcSetting
	if err := s.setting(r.Context(), "oidc", &cfg); err != nil {
		dbError(w, err)
		return
	}
	configured := cfg.ClientSecret != ""
	cfg.ClientSecret = ""
	cfg.defaults()
	writeJSON(w, 200, map[string]any{"setting": cfg, "client_secret_configured": configured, "redirect_uri": s.requestBaseURL(r) + "/api/v1/auth/oidc/callback"})
}

func (s *Server) putOIDCSetting(w http.ResponseWriter, r *http.Request) {
	var in oidcSetting
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Issuer = strings.TrimRight(strings.TrimSpace(in.Issuer), "/")
	in.ClientID = strings.TrimSpace(in.ClientID)
	in.defaults()
	if in.Enabled && (in.Issuer == "" || in.ClientID == "") {
		writeError(w, 400, "invalid_oidc", "issuer and client_id are required when enabled")
		return
	}
	if in.Issuer != "" {
		u, err := url.Parse(in.Issuer)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			writeError(w, 400, "invalid_oidc", "issuer must be an absolute HTTP(S) URL")
			return
		}
	}
	var old oidcSetting
	_ = s.setting(r.Context(), "oidc", &old)
	if in.ClientSecret == "" || in.ClientSecret == "********" {
		in.ClientSecret = old.ClientSecret
	} else {
		sealed, err := s.Secrets.Seal(in.ClientSecret)
		if err != nil {
			dbError(w, err)
			return
		}
		in.ClientSecret = sealed
	}
	configured := in.ClientSecret != ""
	raw, _ := encodeSetting(in)
	p, _ := principalFrom(r)
	_, err := s.DB.Exec(r.Context(), `UPDATE system_settings SET value=$1,secret=true,updated_by=$2,updated_at=now() WHERE key='oidc'`, raw, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "oidc.update", "setting", "oidc", map[string]any{"enabled": in.Enabled, "issuer": in.Issuer, "client_id": in.ClientID})
	in.ClientSecret = ""
	writeJSON(w, 200, map[string]any{"setting": in, "client_secret_configured": configured, "redirect_uri": s.requestBaseURL(r) + "/api/v1/auth/oidc/callback"})
}

type aiSetting struct {
	Enabled        bool   `json:"enabled"`
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	DefaultModel   string `json:"default_model"`
	MaxTokens      int    `json:"max_tokens"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

func (a *aiSetting) defaults() {
	a.BaseURL = strings.TrimRight(strings.TrimSpace(a.BaseURL), "/")
	if a.MaxTokens == 0 {
		a.MaxTokens = 262144
	}
	if a.TimeoutSeconds == 0 {
		a.TimeoutSeconds = 600
	}
}
func (s *Server) loadAISetting(ctx context.Context) (aiSetting, error) {
	var cfg aiSetting
	if err := s.setting(ctx, "ai", &cfg); err != nil {
		return cfg, err
	}
	cfg.defaults()
	if cfg.APIKey != "" {
		plain, err := s.Secrets.Open(cfg.APIKey)
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = plain
	}
	return cfg, nil
}
func (s *Server) getAISetting(w http.ResponseWriter, r *http.Request) {
	var cfg aiSetting
	if err := s.setting(r.Context(), "ai", &cfg); err != nil {
		dbError(w, err)
		return
	}
	configured := cfg.APIKey != ""
	cfg.APIKey = ""
	cfg.defaults()
	writeJSON(w, 200, map[string]any{"setting": cfg, "api_key_configured": configured})
}
func (s *Server) putAISetting(w http.ResponseWriter, r *http.Request) {
	var in aiSetting
	if !decodeJSON(w, r, &in) {
		return
	}
	in.defaults()
	if in.Enabled && (in.BaseURL == "" || in.DefaultModel == "") {
		writeError(w, 400, "invalid_ai", "base_url and default_model are required when enabled")
		return
	}
	if in.MaxTokens < 1 || in.MaxTokens > 262144 {
		writeError(w, 400, "invalid_ai", "max_tokens must be between 1 and 262144")
		return
	}
	if in.TimeoutSeconds < 5 || in.TimeoutSeconds > 3600 {
		writeError(w, 400, "invalid_ai", "timeout_seconds must be between 5 and 3600")
		return
	}
	if in.BaseURL != "" {
		u, e := url.Parse(in.BaseURL)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			writeError(w, 400, "invalid_ai", "base_url must be an absolute HTTP(S) URL")
			return
		}
	}
	var old aiSetting
	_ = s.setting(r.Context(), "ai", &old)
	if in.APIKey == "" || in.APIKey == "********" {
		in.APIKey = old.APIKey
	} else {
		sealed, e := s.Secrets.Seal(in.APIKey)
		if e != nil {
			dbError(w, e)
			return
		}
		in.APIKey = sealed
	}
	raw, _ := encodeSetting(in)
	p, _ := principalFrom(r)
	_, err := s.DB.Exec(r.Context(), `UPDATE system_settings SET value=$1,secret=true,updated_by=$2,updated_at=now() WHERE key='ai'`, raw, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	configured := in.APIKey != ""
	s.audit(r, "ai.update", "setting", "ai", map[string]any{"enabled": in.Enabled, "base_url": in.BaseURL, "model": in.DefaultModel})
	in.APIKey = ""
	writeJSON(w, 200, map[string]any{"setting": in, "api_key_configured": configured})
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	data, err := s.analyticsData(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	location := s.serviceLocation(r.Context())
	now := s.Now().In(location)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).UTC()
	var users, games, scoresToday int64
	err = s.DB.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM users WHERE status='active'),(SELECT count(*) FROM games WHERE status='active'),(SELECT count(*) FROM scores WHERE created_at >= $1)`, dayStart).Scan(&users, &games, &scoresToday)
	if err != nil {
		dbError(w, err)
		return
	}
	data["users"] = users
	data["active_games"] = games
	data["sessions_today"] = data["game_launches"]
	data["scores_today"] = scoresToday
	writeJSON(w, 200, data)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	q := r.URL.Query().Get("q")
	rows, err := s.DB.Query(r.Context(), `SELECT id,username,display_name,email,department,team,role,status,created_at,last_login_at FROM users WHERE $1='' OR username ILIKE '%'||$1||'%' OR display_name ILIKE '%'||$1||'%' OR email ILIKE '%'||$1||'%' OR department ILIKE '%'||$1||'%' OR team ILIKE '%'||$1||'%' ORDER BY created_at DESC LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var username, display, email, department, team, role, status string
		var created time.Time
		var last *time.Time
		if err := rows.Scan(&id, &username, &display, &email, &department, &team, &role, &status, &created, &last); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "username": username, "display_name": display, "email": email, "department": department, "team": team, "role": role, "status": status, "created_at": created, "last_login_at": last})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		DisplayName *string `json:"display_name"`
		Department  *string `json:"department"`
		Team        *string `json:"team"`
		Role        *string `json:"role"`
		Status      *string `json:"status"`
		Password    *string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Role != nil && !slices.Contains([]string{"user", "manager", "operator", "admin"}, *in.Role) {
		writeError(w, 400, "invalid_role", "invalid role")
		return
	}
	if in.Status != nil && !slices.Contains([]string{"active", "disabled"}, *in.Status) {
		writeError(w, 400, "invalid_status", "invalid status")
		return
	}
	var hash *string
	if in.Password != nil {
		if len(*in.Password) < 12 {
			writeError(w, 400, "weak_password", "password must be at least 12 characters")
			return
		}
		b, _ := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		h := string(b)
		hash = &h
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE users SET display_name=COALESCE($2,display_name),department=COALESCE($3,department),team=COALESCE($4,team),role=COALESCE($5,role),status=COALESCE($6,status),password_hash=COALESCE($7,password_hash),updated_at=now() WHERE id=$1`, id, in.DisplayName, in.Department, in.Team, in.Role, in.Status, hash)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "user not found")
		return
	}
	s.audit(r, "user.update", "user", id.String(), map[string]any{"role": in.Role, "status": in.Status})
	w.WriteHeader(204)
}

func (s *Server) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	rows, err := s.DB.Query(r.Context(), `SELECT a.id,a.actor_id,COALESCE(u.username,''),a.action,a.resource_type,a.resource_id,a.remote_addr,a.user_agent,a.detail,a.created_at FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id ORDER BY a.created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var actor *uuid.UUID
		var username, action, typ, rid, remote, agent string
		var detail json.RawMessage
		var created time.Time
		if err := rows.Scan(&id, &actor, &username, &action, &typ, &rid, &remote, &agent, &detail, &created); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "actor_id": actor, "actor_username": username, "action": action, "resource_type": typ, "resource_id": rid, "remote_addr": remote, "user_agent": agent, "detail": detail, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func chiURLParam(r *http.Request, key string) string { return chi.URLParam(r, key) }

func (s *Server) adminListGames(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	limit, offset := pageParams(r)
	rows, err := s.DB.Query(r.Context(), gameSelect+` ORDER BY g.created_at DESC LIMIT $2 OFFSET $3`, p.UserID, limit, offset)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		item, err := scanGame(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) createGame(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in gameInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := in.normalize(); err != nil {
		writeError(w, 400, "invalid_game", err.Error())
		return
	}
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `INSERT INTO games(slug,name,description,category_id,tags,thumbnail_url,banner_url,game_url,game_type,multiplayer,ranking_enabled,achievement_enabled,season_enabled,min_players,max_players,status,version,developer,score_order,score_rules,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) RETURNING id`, in.Slug, in.Name, in.Description, in.CategoryID, in.Tags, in.ThumbnailURL, in.BannerURL, in.GameURL, in.GameType, in.Multiplayer, in.RankingEnabled, in.AchievementEnabled, in.SeasonEnabled, in.MinPlayers, in.MaxPlayers, in.Status, in.Version, in.Developer, in.ScoreOrder, in.ScoreRules, p.UserID).Scan(&id)
	if err != nil {
		writeError(w, 409, "game_conflict", "game slug already exists or data is invalid")
		return
	}
	s.audit(r, "game.create", "game", id.String(), map[string]any{"slug": in.Slug})
	item, err := scanGame(s.DB.QueryRow(r.Context(), gameSelect+` WHERE g.id=$2`, p.UserID, id))
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"game": item})
}

func (s *Server) updateGame(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in gameInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := in.normalize(); err != nil {
		writeError(w, 400, "invalid_game", err.Error())
		return
	}
	var currentSlug string
	if err := s.DB.QueryRow(r.Context(), `SELECT slug FROM games WHERE id=$1`, id).Scan(&currentSlug); err != nil {
		dbError(w, err)
		return
	}
	if (currentSlug == realmGuardSlug) != (in.Slug == realmGuardSlug) {
		writeError(w, 409, "protected_game_identity", "the RealmGuard slug is reserved for its built-in authoritative runtime")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE games SET slug=$2,name=$3,description=$4,category_id=$5,tags=$6,thumbnail_url=$7,banner_url=$8,game_url=$9,game_type=$10,multiplayer=$11,ranking_enabled=$12,achievement_enabled=$13,season_enabled=$14,min_players=$15,max_players=$16,status=$17,version=$18,developer=$19,score_order=$20,score_rules=$21,updated_at=now() WHERE id=$1`, id, in.Slug, in.Name, in.Description, in.CategoryID, in.Tags, in.ThumbnailURL, in.BannerURL, in.GameURL, in.GameType, in.Multiplayer, in.RankingEnabled, in.AchievementEnabled, in.SeasonEnabled, in.MinPlayers, in.MaxPlayers, in.Status, in.Version, in.Developer, in.ScoreOrder, in.ScoreRules)
	if err != nil {
		writeError(w, 409, "game_conflict", "game slug already exists or data is invalid")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "game not found")
		return
	}
	s.audit(r, "game.update", "game", id.String(), nil)
	item, err := scanGame(s.DB.QueryRow(r.Context(), gameSelect+` WHERE g.id=$2`, p.UserID, id))
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"game": item})
}

func (s *Server) deleteGame(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE games SET status='disabled',updated_at=now() WHERE id=$1`, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "game not found")
		return
	}
	s.audit(r, "game.disable", "game", id.String(), nil)
	w.WriteHeader(204)
}

type categoryInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT id,slug,name,description,sort_order,created_at FROM categories ORDER BY sort_order,name`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var slug, name, desc string
		var order int
		var created time.Time
		if err := rows.Scan(&id, &slug, &name, &desc, &order, &created); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "slug": slug, "name": name, "description": desc, "sort_order": order, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	var in categoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	if in.Slug == "" || in.Name == "" {
		writeError(w, 400, "invalid_category", "slug and name are required")
		return
	}
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `INSERT INTO categories(slug,name,description,sort_order) VALUES($1,$2,$3,$4) RETURNING id`, in.Slug, in.Name, in.Description, in.SortOrder).Scan(&id)
	if err != nil {
		writeError(w, 409, "category_conflict", "category slug already exists")
		return
	}
	s.audit(r, "category.create", "category", id.String(), nil)
	writeJSON(w, 201, map[string]any{"category": map[string]any{"id": id, "slug": in.Slug, "name": in.Name, "description": in.Description, "sort_order": in.SortOrder}})
}
func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in categoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	if in.Slug == "" || in.Name == "" {
		writeError(w, 400, "invalid_category", "slug and name are required")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE categories SET slug=$2,name=$3,description=$4,sort_order=$5 WHERE id=$1`, id, in.Slug, in.Name, in.Description, in.SortOrder)
	if err != nil {
		writeError(w, 409, "category_conflict", "category slug already exists")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "category not found")
		return
	}
	s.audit(r, "category.update", "category", id.String(), nil)
	w.WriteHeader(204)
}
func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM categories WHERE id=$1`, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "category not found")
		return
	}
	s.audit(r, "category.delete", "category", id.String(), nil)
	w.WriteHeader(204)
}
