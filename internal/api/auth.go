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

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/hkjang/igame/internal/version"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

type oidcSetting struct {
	Enabled          bool     `json:"enabled"`
	Issuer           string   `json:"issuer"`
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret"`
	Scopes           []string `json:"scopes,omitempty"`
	UsernameClaim    string   `json:"username_claim,omitempty"`
	DisplayNameClaim string   `json:"display_name_claim,omitempty"`
	EmailClaim       string   `json:"email_claim,omitempty"`
	GroupsClaim      string   `json:"groups_claim,omitempty"`
	DepartmentClaim  string   `json:"department_claim,omitempty"`
	TeamClaim        string   `json:"team_claim,omitempty"`
	AdminGroups      []string `json:"admin_groups,omitempty"`
	ManagerGroups    []string `json:"manager_groups,omitempty"`
	OperatorGroups   []string `json:"operator_groups,omitempty"`
}

func (o *oidcSetting) defaults() {
	if len(o.Scopes) == 0 {
		o.Scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	if o.UsernameClaim == "" {
		o.UsernameClaim = "preferred_username"
	}
	if o.DisplayNameClaim == "" {
		o.DisplayNameClaim = "name"
	}
	if o.EmailClaim == "" {
		o.EmailClaim = "email"
	}
	if o.GroupsClaim == "" {
		o.GroupsClaim = "groups"
	}
	if o.TeamClaim == "" {
		o.TeamClaim = "team"
	}
	if o.DepartmentClaim == "" {
		o.DepartmentClaim = "department"
	}
}

func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	var oidcCfg oidcSetting
	_ = s.setting(r.Context(), "oidc", &oidcCfg)
	var service map[string]any
	_ = s.setting(r.Context(), "service", &service)
	var aiCfg aiSetting
	_ = s.setting(r.Context(), "ai", &aiCfg)
	var approvalCfg approvalSetting
	_ = s.setting(r.Context(), "approval", &approvalCfg)
	bootstrapEnabled := true
	if configured, ok := service["bootstrap_login_enabled"].(bool); ok {
		bootstrapEnabled = configured
	}
	writeJSON(w, 200, map[string]any{
		"name": serviceName, "display_name": firstString(service["display_name"], "iGame"), "version": versionString(),
		"oidc_enabled": oidcCfg.Enabled, "oidc_login_url": "/api/v1/auth/oidc/login", "ai_enabled": aiCfg.Enabled, "approval_enabled": approvalCfg.Enabled, "bootstrap_login_enabled": bootstrapEnabled,
	})
}

func versionString() string { return version.Version }

func firstString(value any, fallback string) string {
	if v, ok := value.(string); ok && v != "" {
		return v
	}
	return fallback
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var service struct {
		BootstrapLoginEnabled *bool `json:"bootstrap_login_enabled"`
	}
	_ = s.setting(r.Context(), "service", &service)
	if service.BootstrapLoginEnabled != nil && !*service.BootstrapLoginEnabled {
		writeError(w, http.StatusForbidden, "local_login_disabled", "local bootstrap login is disabled")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	var p Principal
	var hash string
	err := s.DB.QueryRow(r.Context(), `SELECT id,username,display_name,email,department,team,role,password_hash FROM users WHERE lower(username)=lower($1) AND status='active'`, in.Username).Scan(
		&p.UserID, &p.Username, &p.DisplayName, &p.Email, &p.Department, &p.Team, &p.Role, &hash)
	if err != nil || hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		writeError(w, 401, "invalid_credentials", "invalid username or password")
		return
	}
	if err := s.issueSession(w, r, p.UserID); err != nil {
		dbError(w, err)
		return
	}
	_, _ = s.DB.Exec(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, p.UserID)
	s.audit(r, "auth.login", "user", p.UserID.String(), map[string]any{"method": "local", "username": p.Username})
	writeJSON(w, 200, map[string]any{"user": p})
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID uuid.UUID) error {
	raw, err := randomToken(32)
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(raw))
	expires := s.Now().Add(12 * time.Hour)
	_, err = s.DB.Exec(r.Context(), `INSERT INTO auth_sessions(user_id,token_hash,expires_at,user_agent,remote_addr) VALUES($1,$2,$3,$4,$5)`, userID, hash[:], expires, r.UserAgent(), s.clientIP(r))
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: raw, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.requestBaseURL(r), "https://"), SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		hash := sha256.Sum256([]byte(cookie.Value))
		_, _ = s.DB.Exec(r.Context(), `DELETE FROM auth_sessions WHERE token_hash=$1`, hash[:])
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, Expires: time.Unix(1, 0), SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loadOIDCSetting(ctx context.Context) (oidcSetting, error) {
	var cfg oidcSetting
	if err := s.setting(ctx, "oidc", &cfg); err != nil {
		return cfg, err
	}
	cfg.defaults()
	if cfg.ClientSecret != "" {
		plain, err := s.Secrets.Open(cfg.ClientSecret)
		if err != nil {
			return cfg, err
		}
		cfg.ClientSecret = plain
	}
	return cfg, nil
}

func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadOIDCSetting(r.Context())
	if err != nil || !cfg.Enabled || cfg.Issuer == "" || cfg.ClientID == "" {
		writeError(w, 404, "oidc_disabled", "OIDC login is not configured")
		return
	}
	providerCtx := oidc.ClientContext(r.Context(), s.HTTP)
	provider, err := oidc.NewProvider(providerCtx, cfg.Issuer)
	if err != nil {
		writeError(w, 502, "oidc_discovery_failed", "OIDC provider discovery failed")
		return
	}
	state, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "entropy_unavailable", "secure login initialization failed")
		return
	}
	nonce, err := randomToken(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "entropy_unavailable", "secure login initialization failed")
		return
	}
	verifier := oauth2.GenerateVerifier()
	stateHash := sha256.Sum256([]byte(state))
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	_, err = s.DB.Exec(r.Context(), `INSERT INTO oidc_flows(state_hash,nonce,code_verifier,return_to,expires_at) VALUES($1,$2,$3,$4,now()+interval '10 minutes')`, stateHash[:], nonce, verifier, returnTo)
	if err != nil {
		dbError(w, err)
		return
	}
	oauthCfg := oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: s.requestBaseURL(r) + "/api/v1/auth/oidc/callback", Scopes: cfg.Scopes}
	http.Redirect(w, r, oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		writeError(w, 401, "oidc_error", providerError)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeError(w, 400, "invalid_callback", "missing code or state")
		return
	}
	stateHash := sha256.Sum256([]byte(state))
	var nonce, verifier, returnTo string
	err := s.DB.QueryRow(r.Context(), `DELETE FROM oidc_flows WHERE state_hash=$1 AND expires_at>now() RETURNING nonce,code_verifier,return_to`, stateHash[:]).Scan(&nonce, &verifier, &returnTo)
	if err != nil {
		writeError(w, 400, "invalid_state", "expired or invalid OIDC state")
		return
	}
	cfg, err := s.loadOIDCSetting(r.Context())
	if err != nil || !cfg.Enabled {
		writeError(w, 400, "oidc_disabled", "OIDC login is not configured")
		return
	}
	providerCtx := oidc.ClientContext(r.Context(), s.HTTP)
	provider, err := oidc.NewProvider(providerCtx, cfg.Issuer)
	if err != nil {
		writeError(w, 502, "oidc_discovery_failed", "OIDC provider discovery failed")
		return
	}
	oauthCfg := oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: s.requestBaseURL(r) + "/api/v1/auth/oidc/callback", Scopes: cfg.Scopes}
	token, err := oauthCfg.Exchange(providerCtx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		writeError(w, 401, "oidc_exchange_failed", "OIDC code exchange failed")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		writeError(w, 401, "oidc_token_missing", "provider did not return an ID token")
		return
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(providerCtx, rawIDToken)
	if err != nil {
		writeError(w, 401, "oidc_token_invalid", "ID token verification failed")
		return
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		writeError(w, 401, "oidc_claims_invalid", "invalid ID token claims")
		return
	}
	if claimString(claims, "nonce") != nonce {
		writeError(w, 401, "oidc_nonce_invalid", "invalid OIDC nonce")
		return
	}
	sub := claimString(claims, "sub")
	if sub == "" {
		writeError(w, 401, "oidc_subject_missing", "ID token subject is missing")
		return
	}
	username := claimString(claims, cfg.UsernameClaim)
	if username == "" {
		username = sub
	}
	display := claimString(claims, cfg.DisplayNameClaim)
	if display == "" {
		display = username
	}
	email := claimString(claims, cfg.EmailClaim)
	department := claimString(claims, cfg.DepartmentClaim)
	team := claimString(claims, cfg.TeamClaim)
	groups := claimStrings(claims, cfg.GroupsClaim)
	role := "user"
	if anyIn(groups, cfg.ManagerGroups) {
		role = "manager"
	}
	if anyIn(groups, cfg.OperatorGroups) {
		role = "operator"
	}
	if anyIn(groups, cfg.AdminGroups) {
		role = "admin"
	}
	var userID uuid.UUID
	err = s.DB.QueryRow(r.Context(), `INSERT INTO users(username,display_name,email,department,team,role,status,oidc_subject,last_login_at)
		VALUES($1,$2,$3,$4,$5,$6,'active',$7,now())
		ON CONFLICT(oidc_subject) DO UPDATE SET display_name=excluded.display_name,email=excluded.email,department=excluded.department,team=excluded.team,role=excluded.role,last_login_at=now(),updated_at=now()
		RETURNING id`, username, display, email, department, team, role, sub).Scan(&userID)
	if err != nil {
		// A local account may already own the username; preserve it and create a stable OIDC alias.
		username = username + "-" + hexHash(sub)[:8]
		err = s.DB.QueryRow(r.Context(), `INSERT INTO users(username,display_name,email,department,team,role,status,oidc_subject,last_login_at) VALUES($1,$2,$3,$4,$5,$6,'active',$7,now()) RETURNING id`, username, display, email, department, team, role, sub).Scan(&userID)
	}
	if err != nil {
		dbError(w, err)
		return
	}
	if err = s.issueSession(w, r, userID); err != nil {
		dbError(w, err)
		return
	}
	http.Redirect(w, r, safeReturnTo(returnTo), http.StatusFound)
}

func claimString(claims map[string]any, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}
func claimStrings(claims map[string]any, key string) []string {
	switch v := claims[key].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	case string:
		return []string{v}
	}
	return nil
}
func anyIn(values, wanted []string) bool {
	for _, v := range values {
		if slices.Contains(wanted, v) {
			return true
		}
	}
	return false
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var nickname, avatarURL string
	var rankingOptOut bool
	var xp int
	err := s.DB.QueryRow(r.Context(), `SELECT u.nickname,u.avatar_url,u.ranking_opt_out,
		COALESCE((SELECT sum(a.xp) FROM user_achievements ua JOIN achievements a ON a.id=ua.achievement_id WHERE ua.user_id=u.id),0)
		FROM users u WHERE u.id=$1`, p.UserID).Scan(&nickname, &avatarURL, &rankingOptOut, &xp)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": map[string]any{
		"id": p.UserID, "username": p.Username, "display_name": p.DisplayName,
		"email": p.Email, "department": p.Department, "team": p.Team, "role": p.Role,
		"nickname": nickname, "avatar_url": avatarURL, "ranking_opt_out": rankingOptOut,
		"xp": xp, "level": levelForXP(xp),
	}})
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in struct {
		DisplayName   *string `json:"display_name"`
		Nickname      *string `json:"nickname"`
		AvatarURL     *string `json:"avatar_url"`
		RankingOptOut *bool   `json:"ranking_opt_out"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	var out struct {
		ID            uuid.UUID `json:"id"`
		Username      string    `json:"username"`
		DisplayName   string    `json:"display_name"`
		Email         string    `json:"email"`
		Department    string    `json:"department"`
		Team          string    `json:"team"`
		Role          string    `json:"role"`
		Nickname      string    `json:"nickname"`
		AvatarURL     string    `json:"avatar_url"`
		RankingOptOut bool      `json:"ranking_opt_out"`
	}
	err := s.DB.QueryRow(r.Context(), `UPDATE users SET display_name=COALESCE($2,display_name),nickname=COALESCE($3,nickname),avatar_url=COALESCE($4,avatar_url),ranking_opt_out=COALESCE($5,ranking_opt_out),updated_at=now() WHERE id=$1 RETURNING id,username,display_name,email,department,team,role,nickname,avatar_url,ranking_opt_out`, p.UserID, in.DisplayName, in.Nickname, in.AvatarURL, in.RankingOptOut).Scan(&out.ID, &out.Username, &out.DisplayName, &out.Email, &out.Department, &out.Team, &out.Role, &out.Nickname, &out.AvatarURL, &out.RankingOptOut)
	if err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "profile.update", "user", p.UserID.String(), nil)
	writeJSON(w, 200, map[string]any{"user": out})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	if p.AuthType != "session" {
		writeError(w, 403, "session_required", "password changes require an interactive session")
		return
	}
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.NewPassword) < 12 {
		writeError(w, 400, "weak_password", "new_password must be at least 12 characters")
		return
	}
	if in.NewPassword == in.CurrentPassword {
		writeError(w, 400, "password_unchanged", "new password must differ from the current password")
		return
	}
	var currentHash string
	if err := s.DB.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1 AND status='active'`, p.UserID).Scan(&currentHash); err != nil || currentHash == "" || bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(in.CurrentPassword)) != nil {
		writeError(w, 403, "invalid_current_password", "current password is incorrect")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		dbError(w, err)
		return
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		writeError(w, 401, "session_required", "active session cookie is missing")
		return
	}
	tokenHash := sha256.Sum256([]byte(cookie.Value))
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `UPDATE users SET password_hash=$2,updated_at=now() WHERE id=$1`, p.UserID, string(newHash)); err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM auth_sessions WHERE user_id=$1 AND token_hash<>$2`, p.UserID, tokenHash[:])
	}
	if err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "password.change", "user", p.UserID.String(), map[string]any{"other_sessions_revoked": true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var raw json.RawMessage
	err := s.DB.QueryRow(r.Context(), `SELECT value FROM user_preferences WHERE user_id=$1`, p.UserID).Scan(&raw)
	if err == pgx.ErrNoRows {
		raw = []byte("{}")
	} else if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"preferences": raw})
}

func (s *Server) putPreferences(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var value map[string]any
	if !decodeJSON(w, r, &value) {
		return
	}
	raw, _ := json.Marshal(value)
	_, err := s.DB.Exec(r.Context(), `INSERT INTO user_preferences(user_id,value) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET value=excluded.value,updated_at=now()`, p.UserID, raw)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"preferences": value})
}

// marshal validation ensures settings are valid JSON before encryption/storage.
func encodeSetting(value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode setting: %w", err)
	}
	return b, nil
}
