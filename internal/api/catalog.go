package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type gameInput struct {
	Slug               string          `json:"slug"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	CategoryID         *uuid.UUID      `json:"category_id"`
	Tags               []string        `json:"tags"`
	ThumbnailURL       string          `json:"thumbnail_url"`
	BannerURL          string          `json:"banner_url"`
	GameURL            string          `json:"game_url"`
	GameType           string          `json:"game_type"`
	Multiplayer        bool            `json:"multiplayer"`
	RankingEnabled     bool            `json:"ranking_enabled"`
	AchievementEnabled bool            `json:"achievement_enabled"`
	SeasonEnabled      bool            `json:"season_enabled"`
	MinPlayers         int             `json:"min_players"`
	MaxPlayers         int             `json:"max_players"`
	Status             string          `json:"status"`
	Version            string          `json:"version"`
	Developer          string          `json:"developer"`
	ScoreOrder         string          `json:"score_order"`
	ScoreRules         json.RawMessage `json:"score_rules"`
}

func (g *gameInput) normalize() error {
	g.Slug = strings.ToLower(strings.TrimSpace(g.Slug))
	g.Name = strings.TrimSpace(g.Name)
	g.GameURL = strings.TrimSpace(g.GameURL)
	if g.Slug == "" || g.Name == "" || g.GameURL == "" {
		return fmt.Errorf("slug, name and game_url are required")
	}
	if len(g.Slug) > 100 || len(g.Name) > 200 {
		return fmt.Errorf("slug or name is too long")
	}
	for _, r := range g.Slug {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return fmt.Errorf("slug may contain lowercase letters, digits and hyphens")
		}
	}
	if g.GameType == "" {
		g.GameType = "iframe"
	}
	if !slices.Contains([]string{"iframe", "embedded", "external"}, g.GameType) {
		return fmt.Errorf("game_type must be iframe, embedded, or external")
	}
	if !validManagedURL(g.GameURL, true) {
		return fmt.Errorf("game_url must be a relative path or absolute HTTP(S) URL")
	}
	if g.Tags == nil {
		g.Tags = []string{}
	}
	if g.Status == "" {
		g.Status = "draft"
	}
	if !slices.Contains([]string{"draft", "active", "maintenance", "disabled"}, g.Status) {
		return fmt.Errorf("invalid game status")
	}
	if g.Version == "" {
		g.Version = "1.0.0"
	}
	if g.ScoreOrder == "" {
		g.ScoreOrder = "desc"
	}
	if g.ScoreOrder != "asc" && g.ScoreOrder != "desc" {
		return fmt.Errorf("score_order must be asc or desc")
	}
	if g.MinPlayers < 1 {
		g.MinPlayers = 1
	}
	if g.MaxPlayers < g.MinPlayers {
		g.MaxPlayers = g.MinPlayers
	}
	if g.MinPlayers > 1000 || g.MaxPlayers > 1000 {
		return fmt.Errorf("player limits cannot exceed 1000")
	}
	if len(g.ScoreRules) == 0 {
		g.ScoreRules = []byte("{}")
	}
	if !json.Valid(g.ScoreRules) {
		return fmt.Errorf("score_rules must be JSON")
	}
	return nil
}

func scanGame(row pgx.Row) (map[string]any, error) {
	var id uuid.UUID
	var slug, name, description, categoryName string
	var categoryID *uuid.UUID
	var tags []string
	var thumb, banner, gameURL, gameType, status, ver, developer, scoreOrder string
	var multiplayer, ranking, achievement, season bool
	var minP, maxP int
	var rules json.RawMessage
	var created, updated time.Time
	var favorite bool
	err := row.Scan(&id, &slug, &name, &description, &categoryID, &categoryName, &tags, &thumb, &banner, &gameURL, &gameType, &multiplayer, &ranking, &achievement, &season, &minP, &maxP, &status, &ver, &developer, &scoreOrder, &rules, &created, &updated, &favorite)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "slug": slug, "name": name, "description": description, "category_id": categoryID, "category_name": categoryName, "tags": tags, "thumbnail_url": thumb, "banner_url": banner, "game_url": gameURL, "game_type": gameType, "multiplayer": multiplayer, "ranking_enabled": ranking, "achievement_enabled": achievement, "season_enabled": season, "min_players": minP, "max_players": maxP, "status": status, "version": ver, "developer": developer, "score_order": scoreOrder, "score_rules": rules, "created_at": created, "updated_at": updated, "favorite": favorite}, nil
}

const gameSelect = `SELECT g.id,g.slug,g.name,g.description,g.category_id,COALESCE(c.name,''),g.tags,g.thumbnail_url,g.banner_url,g.game_url,g.game_type,g.multiplayer,g.ranking_enabled,g.achievement_enabled,g.season_enabled,g.min_players,g.max_players,g.status,g.version,g.developer,g.score_order,g.score_rules,g.created_at,g.updated_at,EXISTS(SELECT 1 FROM favorites f WHERE f.game_id=g.id AND f.user_id=$1) FROM games g LEFT JOIN categories c ON c.id=g.category_id`

func (s *Server) listGames(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	limit, offset := pageParams(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	favorite := r.URL.Query().Get("favorite") == "true"
	rows, err := s.DB.Query(r.Context(), gameSelect+` WHERE g.status='active' AND ($2='' OR g.name ILIKE '%'||$2||'%' OR g.description ILIKE '%'||$2||'%' OR $2=ANY(g.tags)) AND ($3='' OR c.slug=$3) AND (NOT $4 OR EXISTS(SELECT 1 FROM favorites ff WHERE ff.game_id=g.id AND ff.user_id=$1)) ORDER BY g.name LIMIT $5 OFFSET $6`, p.UserID, q, category, favorite, limit, offset)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		item, err := scanGame(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (s *Server) getGame(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	identifier := chi.URLParam(r, "id")
	item, err := scanGame(s.DB.QueryRow(r.Context(), gameSelect+` WHERE g.status='active' AND (g.id::text=$2 OR g.slug=$2)`, p.UserID, identifier))
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"game": item})
}

func (s *Server) addFavorite(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, err := s.parseGameIdentifier(r)
	if err != nil {
		dbError(w, err)
		return
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO favorites(user_id,game_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, p.UserID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) removeFavorite(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, err := s.parseGameIdentifier(r)
	if err != nil {
		dbError(w, err)
		return
	}
	_, err = s.DB.Exec(r.Context(), `DELETE FROM favorites WHERE user_id=$1 AND game_id=$2`, p.UserID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) parseGameIdentifier(r *http.Request) (uuid.UUID, error) {
	v := chi.URLParam(r, "id")
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `SELECT id FROM games WHERE id::text=$1 OR slug=$1`, v).Scan(&id)
	return id, err
}

// parseGameID accepts both UUID and stable slug so the Game SDK remains readable.
func (s *Server) startGameSession(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in struct {
		Metadata json.RawMessage `json:"metadata"`
	}
	if !decodeOptionalJSON(w, r, &in) {
		return
	}
	if len(in.Metadata) == 0 {
		in.Metadata = []byte("{}")
	} else {
		var object map[string]any
		if json.Unmarshal(in.Metadata, &object) != nil {
			writeError(w, 400, "invalid_metadata", "metadata must be a JSON object")
			return
		}
	}
	gameID, err := s.parseGameIdentifier(r)
	if err != nil {
		dbError(w, err)
		return
	}
	if ok, msg := s.playAllowed(r, gameID); !ok {
		writeError(w, 403, "play_policy_denied", msg)
		return
	}
	var recentStarts int
	if err := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM game_sessions WHERE user_id=$1 AND started_at>=now()-interval '1 minute'`, p.UserID).Scan(&recentStarts); err != nil {
		dbError(w, err)
		return
	}
	if recentStarts >= 30 {
		writeError(w, 429, "session_rate_limited", "too many game sessions started; retry shortly")
		return
	}
	rawRandom, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "entropy_unavailable", "secure game session creation failed")
		return
	}
	raw := "igs_" + rawRandom
	hash := sha256.Sum256([]byte(raw))
	var id uuid.UUID
	var started time.Time
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize replacement of the single active session for this user/game pair.
	_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1::text||':'||$2::text,0))`, p.UserID, gameID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE game_sessions SET status='abandoned',ended_at=now(),duration_ms=GREATEST(0,extract(epoch FROM(now()-started_at))*1000)::bigint WHERE user_id=$1 AND game_id=$2 AND status='active'`, p.UserID, gameID)
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO game_sessions(user_id,game_id,season_id,session_token_hash,client_info)
			SELECT $1,g.id,(SELECT id FROM seasons WHERE status='active' AND now() BETWEEN starts_at AND ends_at ORDER BY starts_at DESC LIMIT 1),$2,$4 FROM games g WHERE g.id=$3 AND g.status='active' RETURNING id,started_at`, p.UserID, hash[:], gameID, in.Metadata).Scan(&id, &started)
	}
	if err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	_, _ = s.DB.Exec(r.Context(), `INSERT INTO user_achievements(user_id,achievement_id) SELECT $1,id FROM achievements WHERE code='first-play' ON CONFLICT DO NOTHING`, p.UserID)
	_, _ = s.DB.Exec(r.Context(), `INSERT INTO user_achievements(user_id,achievement_id) SELECT $1,a.id FROM achievements a WHERE a.code='explorer' AND (SELECT count(DISTINCT game_id) FROM game_sessions WHERE user_id=$1)>=5 ON CONFLICT DO NOTHING`, p.UserID)
	writeJSON(w, 201, map[string]any{"session": map[string]any{"id": id, "game_id": gameID, "status": "active", "started_at": started, "session_token": raw}, "user": p})
}

func (s *Server) playAllowed(r *http.Request, gameID uuid.UUID) (bool, string) {
	var cfg struct {
		Enabled bool `json:"enabled"`
		Windows []struct {
			Days  []int  `json:"days"`
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"windows"`
		DailyLimits map[string]int `json:"daily_limits"`
	}
	if s.setting(r.Context(), "play_policy", &cfg) != nil || !cfg.Enabled {
		return true, ""
	}
	var service struct {
		Timezone string `json:"timezone"`
	}
	_ = s.setting(r.Context(), "service", &service)
	if service.Timezone == "" {
		service.Timezone = "Asia/Seoul"
	}
	location, err := time.LoadLocation(service.Timezone)
	if err != nil {
		location = time.FixedZone("KST", 9*60*60)
	}
	now := s.Now().In(location)
	inside := len(cfg.Windows) == 0
	for _, window := range cfg.Windows {
		start, startErr := clockMinutes(window.Start, false)
		end, endErr := clockMinutes(window.End, true)
		if startErr != nil || endErr != nil {
			continue
		}
		minutes := now.Hour()*60 + now.Minute()
		dayAllowed := func(day time.Weekday) bool {
			if len(window.Days) == 0 {
				return true
			}
			return slices.Contains(window.Days, int(day))
		}
		if start == end && dayAllowed(now.Weekday()) {
			inside = true
		} else if start < end && dayAllowed(now.Weekday()) && minutes >= start && minutes <= end {
			inside = true
		} else if start > end && ((dayAllowed(now.Weekday()) && minutes >= start) || (dayAllowed((now.Weekday()+6)%7) && minutes <= end)) {
			inside = true
		}
	}
	if !inside {
		return false, "game play is outside the allowed time window"
	}
	var slug string
	_ = s.DB.QueryRow(r.Context(), `SELECT slug FROM games WHERE id=$1`, gameID).Scan(&slug)
	limit := cfg.DailyLimits[slug]
	if limit <= 0 {
		return true, ""
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).UTC()
	var used int64
	_ = s.DB.QueryRow(r.Context(), `SELECT COALESCE(sum(duration_ms),0) FROM game_sessions WHERE user_id=$1 AND game_id=$2 AND started_at>=$3`, mustPrincipal(r).UserID, gameID, dayStart).Scan(&used)
	if used >= int64(limit)*60000 {
		return false, "daily play limit reached"
	}
	return true, ""
}
func mustPrincipal(r *http.Request) Principal { p, _ := principalFrom(r); return p }

func (s *Server) finishGameSession(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Result       json.RawMessage `json:"result"`
		SessionToken string          `json:"session_token"`
		Score        *int64          `json:"score,omitempty"`
		DurationMS   *int64          `json:"duration_ms,omitempty"`
		GameID       string          `json:"game_id,omitempty"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.Result) == 0 {
		in.Result = []byte("{}")
	}
	hash := sha256.Sum256([]byte(in.SessionToken))
	var status string
	err := s.DB.QueryRow(r.Context(), `UPDATE game_sessions SET status='finished',ended_at=COALESCE(ended_at,now()),duration_ms=COALESCE(duration_ms,GREATEST(0,extract(epoch FROM(now()-started_at))*1000)::bigint),result=result||$4 WHERE id=$1 AND user_id=$2 AND status IN ('active','finished') AND session_token_hash=$3 RETURNING status`, id, p.UserID, hash[:], in.Result).Scan(&status)
	if err != nil {
		writeError(w, 409, "invalid_session", "session or token not found")
		return
	}
	writeJSON(w, 200, map[string]any{"session": map[string]any{"id": id, "status": status}})
}

func (s *Server) submitScore(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in struct {
		SessionID    uuid.UUID       `json:"session_id"`
		SessionToken string          `json:"session_token"`
		GameID       string          `json:"game_id,omitempty"`
		Proof        string          `json:"proof,omitempty"`
		DurationMS   *int64          `json:"duration_ms,omitempty"`
		Score        int64           `json:"score"`
		Metadata     json.RawMessage `json:"metadata"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.SessionID == uuid.Nil || in.SessionToken == "" {
		writeError(w, 400, "invalid_score", "session_id and session_token are required")
		return
	}
	if len(in.Metadata) == 0 {
		in.Metadata = []byte("{}")
	}
	hash := sha256.Sum256([]byte(in.SessionToken))
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var gameID uuid.UUID
	var seasonID *uuid.UUID
	var started time.Time
	var status string
	var rules json.RawMessage
	err = tx.QueryRow(r.Context(), `SELECT gs.game_id,gs.season_id,gs.started_at,gs.status,g.score_rules FROM game_sessions gs JOIN games g ON g.id=gs.game_id WHERE gs.id=$1 AND gs.user_id=$2 AND gs.session_token_hash=$3 FOR UPDATE`, in.SessionID, p.UserID, hash[:]).Scan(&gameID, &seasonID, &started, &status, &rules)
	if err != nil {
		writeError(w, 409, "invalid_session", "session or token is invalid")
		return
	}
	if status != "active" && status != "finished" {
		writeError(w, 409, "invalid_session", "session cannot accept a score")
		return
	}
	if in.GameID != "" {
		var claimed uuid.UUID
		if err := tx.QueryRow(r.Context(), `SELECT id FROM games WHERE id::text=$1 OR slug=$1`, in.GameID).Scan(&claimed); err != nil || claimed != gameID {
			writeError(w, 409, "game_mismatch", "game_id does not match the session")
			return
		}
	}
	var rule struct {
		MinScore      *int64 `json:"min_score"`
		MaxScore      *int64 `json:"max_score"`
		MinDurationMS int64  `json:"min_duration_ms"`
		MaxDurationMS int64  `json:"max_duration_ms"`
	}
	_ = json.Unmarshal(rules, &rule)
	duration := s.Now().Sub(started).Milliseconds()
	if in.DurationMS != nil && *in.DurationMS > duration+5000 {
		writeError(w, 422, "invalid_duration", "client duration exceeds server duration")
		return
	}
	reason := ""
	if rule.MinScore != nil && in.Score < *rule.MinScore {
		reason = "score_below_minimum"
	}
	if rule.MaxScore != nil && in.Score > *rule.MaxScore {
		reason = "score_above_maximum"
	}
	if rule.MinDurationMS > 0 && duration < rule.MinDurationMS {
		reason = "duration_too_short"
	}
	if rule.MaxDurationMS > 0 && duration > rule.MaxDurationMS {
		reason = "duration_too_long"
	}
	verified := reason == ""
	var scoreID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO scores(user_id,game_id,session_id,season_id,score,metadata,verified,rejection_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, p.UserID, gameID, in.SessionID, seasonID, in.Score, in.Metadata, verified, reason).Scan(&scoreID)
	if err != nil {
		writeError(w, 409, "duplicate_score", "a score was already submitted for this session")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE game_sessions SET status='finished',ended_at=COALESCE(ended_at,now()),duration_ms=COALESCE(duration_ms,$2),result=result||jsonb_build_object('score',$3::bigint) WHERE id=$1`, in.SessionID, duration, in.Score)
	if err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	statusCode := 201
	if !verified {
		statusCode = 422
	}
	writeJSON(w, statusCode, map[string]any{"score": map[string]any{"id": scoreID, "game_id": gameID, "score": in.Score, "verified": verified, "rejection_reason": reason}})
}

func (s *Server) submitTelemetry(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in struct {
		GameID       string          `json:"game_id"`
		SessionID    uuid.UUID       `json:"session_id"`
		SessionToken string          `json:"session_token"`
		Event        string          `json:"event"`
		Data         json.RawMessage `json:"data"`
		OccurredAt   *time.Time      `json:"occurred_at"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Event = strings.TrimSpace(in.Event)
	if in.SessionID == uuid.Nil || in.SessionToken == "" || in.Event == "" || len(in.Event) > 100 {
		writeError(w, 400, "invalid_telemetry", "session_id, session_token and an event of at most 100 characters are required")
		return
	}
	for _, ch := range in.Event {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-') {
			writeError(w, 400, "invalid_telemetry", "event contains unsupported characters")
			return
		}
	}
	if len(in.Data) == 0 {
		in.Data = []byte("{}")
	}
	occurred := s.Now()
	if in.OccurredAt != nil {
		occurred = *in.OccurredAt
		if occurred.Before(s.Now().Add(-24*time.Hour)) || occurred.After(s.Now().Add(5*time.Minute)) {
			writeError(w, 400, "invalid_telemetry_time", "occurred_at is outside the allowed window")
			return
		}
	}
	hash := sha256.Sum256([]byte(in.SessionToken))
	var gameID uuid.UUID
	err := s.DB.QueryRow(r.Context(), `SELECT game_id FROM game_sessions WHERE id=$1 AND user_id=$2 AND session_token_hash=$3 AND status IN ('active','finished')`, in.SessionID, p.UserID, hash[:]).Scan(&gameID)
	if err != nil {
		writeError(w, 403, "invalid_session", "session or token is invalid")
		return
	}
	if in.GameID != "" {
		var claimed uuid.UUID
		if err := s.DB.QueryRow(r.Context(), `SELECT id FROM games WHERE id::text=$1 OR slug=$1`, in.GameID).Scan(&claimed); err != nil || claimed != gameID {
			writeError(w, 409, "game_mismatch", "game_id does not match the session")
			return
		}
	}
	var count int
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM game_telemetry WHERE session_id=$1`, in.SessionID).Scan(&count)
	if count >= 5000 {
		writeError(w, 429, "telemetry_limit", "session telemetry limit reached")
		return
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO game_telemetry(session_id,user_id,game_id,event,data,occurred_at) VALUES($1,$2,$3,$4,$5,$6)`, in.SessionID, p.UserID, gameID, in.Event, in.Data, occurred)
	if err != nil {
		dbError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) rankings(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameID")
	if gameID == "" {
		gameID = r.URL.Query().Get("game_id")
	}
	if gameID == "" {
		writeError(w, 400, "game_required", "game_id is required")
		return
	}
	var id uuid.UUID
	var order, gameName string
	err := s.DB.QueryRow(r.Context(), `SELECT id,score_order,name FROM games WHERE id::text=$1 OR slug=$1`, gameID).Scan(&id, &order, &gameName)
	if err != nil {
		dbError(w, err)
		return
	}
	period := r.URL.Query().Get("period")
	since := time.Time{}
	now := s.Now().In(s.serviceLocation(r.Context()))
	switch period {
	case "daily":
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC()
	case "weekly":
		offset := (int(now.Weekday()) + 6) % 7
		since = time.Date(now.Year(), now.Month(), now.Day()-offset, 0, 0, 0, 0, now.Location()).UTC()
	case "monthly":
		since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).UTC()
	case "", "all_time":
		period = "all_time"
	case "season":
	default:
		writeError(w, 400, "invalid_period", "period must be daily, weekly, monthly, season, or all_time")
		return
	}
	direction := "DESC"
	aggregate := "MAX"
	if order == "asc" {
		direction = "ASC"
		aggregate = "MIN"
	}
	group := r.URL.Query().Get("group")
	limit, _ := pageParams(r)
	if group == "department" || group == "team" {
		column := "department"
		if group == "team" {
			column = "team"
		}
		query := fmt.Sprintf(`WITH user_best AS (SELECT u.id,u.%s group_name,%s(s.score) score FROM scores s JOIN users u ON u.id=s.user_id WHERE s.game_id=$1 AND s.verified AND s.moderation_status='valid' AND NOT u.ranking_opt_out AND ($2::timestamptz='0001-01-01' OR s.created_at >= $2) AND ($3<>'season' OR s.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1)) AND u.%s<>'' GROUP BY u.id,u.%s), group_totals AS (SELECT group_name,SUM(score) score,COUNT(*) members FROM user_best GROUP BY group_name) SELECT row_number() OVER(ORDER BY score %s),group_name,score,members FROM group_totals ORDER BY score %s LIMIT $4`, column, aggregate, column, column, direction, direction)
		rows, err := s.DB.Query(r.Context(), query, id, since, period, limit)
		if err != nil {
			dbError(w, err)
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var rank int64
			var groupName string
			var score int64
			var members int
			if err := rows.Scan(&rank, &groupName, &score, &members); err != nil {
				dbError(w, err)
				return
			}
			item := map[string]any{"rank": rank, "name": groupName, "display_name": groupName, "score": score, "members": members, "game_name": gameName}
			item[group] = groupName
			items = append(items, item)
		}
		writeJSON(w, 200, map[string]any{"items": items, "period": period, "group": group})
		return
	}
	if group != "" && group != "individual" {
		writeError(w, 400, "invalid_group", "group must be individual, department, or team")
		return
	}
	query := fmt.Sprintf(`WITH best AS (SELECT u.id,u.username,u.display_name,u.nickname,u.department,u.team,%s(s.score) score FROM scores s JOIN users u ON u.id=s.user_id WHERE s.game_id=$1 AND s.verified AND s.moderation_status='valid' AND NOT u.ranking_opt_out AND ($2::timestamptz='0001-01-01' OR s.created_at >= $2) AND ($3<>'season' OR s.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1)) GROUP BY u.id) SELECT row_number() OVER(ORDER BY score %s),id,username,display_name,nickname,department,team,score FROM best ORDER BY score %s LIMIT $4`, aggregate, direction, direction)
	rows, err := s.DB.Query(r.Context(), query, id, since, period, limit)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	var privacy struct {
		RankingName    string `json:"ranking_name"`
		ShowDepartment bool   `json:"show_department"`
	}
	_ = s.setting(r.Context(), "privacy", &privacy)
	items := []map[string]any{}
	for rows.Next() {
		var rank int64
		var uid uuid.UUID
		var username, display, nickname, dept, team string
		var score int64
		if err := rows.Scan(&rank, &uid, &username, &display, &nickname, &dept, &team, &score); err != nil {
			dbError(w, err)
			return
		}
		name := nickname
		if name == "" {
			name = username
		}
		if privacy.RankingName == "real_name" && display != "" {
			name = display
		}
		item := map[string]any{"rank": rank, "user_id": uid, "name": name, "display_name": name, "score": score, "team": team, "game_name": gameName}
		if privacy.ShowDepartment {
			item["department"] = dept
		}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items, "period": period, "group": "individual"})
}

func (s *Server) playHistory(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	limit, offset := pageParams(r)
	rows, err := s.DB.Query(r.Context(), `SELECT gs.id,gs.game_id,g.name,g.slug,gs.status,gs.started_at,gs.ended_at,gs.duration_ms,s.score,s.verified FROM game_sessions gs JOIN games g ON g.id=gs.game_id LEFT JOIN scores s ON s.session_id=gs.id WHERE gs.user_id=$1 ORDER BY gs.started_at DESC LIMIT $2 OFFSET $3`, p.UserID, limit, offset)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, gid uuid.UUID
		var name, slug, status string
		var started time.Time
		var ended *time.Time
		var duration, score *int64
		var verified *bool
		if err := rows.Scan(&id, &gid, &name, &slug, &status, &started, &ended, &duration, &score, &verified); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "game_id": gid, "game_name": name, "game_slug": slug, "status": status, "started_at": started, "ended_at": ended, "duration_ms": duration, "score": score, "verified": verified})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
