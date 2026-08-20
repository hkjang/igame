package api

import (
	"context"
	"crypto/sha256"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

var allowedAPIKeyPermissions = []string{
	"api:access", "mcp:access", "games:read", "sessions:write", "scores:write", "rankings:read", "profile:read", "profile:write", "ai:invoke", "workflow:write", "admin:*",
}

type apiKeyPolicy struct {
	AvailablePermissions []string            `json:"available_permissions"`
	RolePermissions      map[string][]string `json:"role_permissions"`
	MaxKeys              int                 `json:"max_keys"`
	MaxTTLDays           int                 `json:"max_ttl_days"`
}

func (s *Server) loadAPIKeyPolicy(r *http.Request) (apiKeyPolicy, error) {
	return s.loadAPIKeyPolicyContext(r.Context())
}

func (s *Server) loadAPIKeyPolicyContext(ctx context.Context) (apiKeyPolicy, error) {
	// Keep the package-level allowlist immutable: encoding/json may reuse the
	// backing array of a non-nil destination slice while decoding settings.
	p := apiKeyPolicy{AvailablePermissions: append([]string(nil), allowedAPIKeyPermissions...), MaxKeys: 10, MaxTTLDays: 365}
	if err := s.setting(ctx, "api_keys", &p); err != nil {
		return apiKeyPolicy{}, err
	}
	if p.MaxKeys < 1 {
		p.MaxKeys = 1
	}
	if p.MaxKeys > 100 {
		p.MaxKeys = 100
	}
	if p.MaxTTLDays < 1 {
		p.MaxTTLDays = 1
	}
	if p.MaxTTLDays > 3650 {
		p.MaxTTLDays = 3650
	}
	return p, nil
}

type apiKeyInput struct {
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

func validateKeyPermissions(p Principal, values []string, policy apiKeyPolicy) bool {
	if len(values) == 0 {
		return false
	}
	roleAllowed, roleConfigured := policy.RolePermissions[p.Role]
	if !roleConfigured {
		roleAllowed = policy.AvailablePermissions
	}
	for _, v := range values {
		if !slices.Contains(allowedAPIKeyPermissions, v) || !slices.Contains(policy.AvailablePermissions, v) || !slices.Contains(roleAllowed, v) || (v == "admin:*" && p.Role != "admin") {
			return false
		}
	}
	return true
}

func availableKeyPermissions(p Principal, policy apiKeyPolicy) []string {
	roleAllowed, roleConfigured := policy.RolePermissions[p.Role]
	if !roleConfigured {
		roleAllowed = policy.AvailablePermissions
	}
	available := make([]string, 0, len(policy.AvailablePermissions))
	for _, permission := range policy.AvailablePermissions {
		if slices.Contains(roleAllowed, permission) && (permission != "admin:*" || p.Role == "admin") {
			available = append(available, permission)
		}
	}
	return available
}

func effectiveKeyPermissions(p Principal, stored []string, policy apiKeyPolicy) []string {
	allowed := availableKeyPermissions(p, policy)
	effective := make([]string, 0, len(stored))
	for _, permission := range stored {
		if slices.Contains(allowed, permission) {
			effective = append(effective, permission)
		}
	}
	return effective
}

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	policy, err := s.loadAPIKeyPolicy(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "api_key_policy_unavailable", "API key policy is unavailable")
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,name,key_prefix,permissions,expires_at,last_used_at,revoked_at,created_at FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var name, prefix string
		var perms []string
		var expires, lastUsed, revoked *time.Time
		var created time.Time
		if err := rows.Scan(&id, &name, &prefix, &perms, &expires, &lastUsed, &revoked, &created); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "prefix": prefix, "permissions": perms, "expires_at": expires, "last_used_at": lastUsed, "revoked_at": revoked, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"items": items, "available_permissions": availableKeyPermissions(p, policy), "max_keys": policy.MaxKeys, "max_ttl_days": policy.MaxTTLDays})
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	policy, err := s.loadAPIKeyPolicy(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "api_key_policy_unavailable", "API key policy is unavailable")
		return
	}
	var in apiKeyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, 400, "invalid_name", "name is required and must be at most 100 characters")
		return
	}
	if !validateKeyPermissions(p, in.Permissions, policy) {
		writeError(w, 400, "invalid_permissions", "one or more permissions are invalid for this role")
		return
	}
	if in.ExpiresAt != nil && (in.ExpiresAt.Before(s.Now()) || in.ExpiresAt.After(s.Now().Add(time.Duration(policy.MaxTTLDays)*24*time.Hour))) {
		writeError(w, 400, "invalid_expiry", "expires_at exceeds the configured lifetime")
		return
	}
	var activeCount int
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM api_keys WHERE user_id=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now())`, p.UserID).Scan(&activeCount)
	if activeCount >= policy.MaxKeys {
		writeError(w, 409, "key_limit", "active API key limit reached")
		return
	}
	random, err := randomToken(32)
	if err != nil {
		dbError(w, err)
		return
	}
	raw := "igk_" + random
	hash := sha256.Sum256([]byte(raw))
	var id uuid.UUID
	var created time.Time
	err = s.DB.QueryRow(r.Context(), `INSERT INTO api_keys(user_id,name,key_prefix,key_hash,permissions,expires_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at`, p.UserID, in.Name, tokenPrefix(raw), hash[:], in.Permissions, in.ExpiresAt).Scan(&id, &created)
	if err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "api_key.create", "api_key", id.String(), map[string]any{"permissions": in.Permissions})
	writeJSON(w, 201, map[string]any{"api_key": map[string]any{"id": id, "name": in.Name, "prefix": tokenPrefix(raw), "permissions": in.Permissions, "expires_at": in.ExpiresAt, "created_at": created}, "secret": raw})
}

func (s *Server) updateAPIKey(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	policy, err := s.loadAPIKeyPolicy(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "api_key_policy_unavailable", "API key policy is unavailable")
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in apiKeyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || !validateKeyPermissions(p, in.Permissions, policy) {
		writeError(w, 400, "invalid_api_key", "name and valid permissions are required")
		return
	}
	if in.ExpiresAt != nil && (in.ExpiresAt.Before(s.Now()) || in.ExpiresAt.After(s.Now().Add(time.Duration(policy.MaxTTLDays)*24*time.Hour))) {
		writeError(w, 400, "invalid_expiry", "expires_at exceeds the configured lifetime")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE api_keys SET name=$3,permissions=$4,expires_at=$5 WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, id, p.UserID, in.Name, in.Permissions, in.ExpiresAt)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "API key not found")
		return
	}
	s.audit(r, "api_key.update", "api_key", id.String(), map[string]any{"permissions": in.Permissions})
	w.WriteHeader(204)
}

func (s *Server) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	oldID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var name string
	var perms []string
	var expires *time.Time
	err = tx.QueryRow(r.Context(), `UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL RETURNING name,permissions,expires_at`, oldID, p.UserID).Scan(&name, &perms, &expires)
	if err != nil {
		dbError(w, err)
		return
	}
	random, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "entropy_unavailable", "secure key generation failed")
		return
	}
	raw := "igk_" + random
	hash := sha256.Sum256([]byte(raw))
	var id uuid.UUID
	var created time.Time
	err = tx.QueryRow(r.Context(), `INSERT INTO api_keys(user_id,name,key_prefix,key_hash,permissions,expires_at,rotated_from) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,created_at`, p.UserID, name, tokenPrefix(raw), hash[:], perms, expires, oldID).Scan(&id, &created)
	if err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	s.audit(r, "api_key.rotate", "api_key", id.String(), map[string]any{"rotated_from": oldID})
	writeJSON(w, 201, map[string]any{"api_key": map[string]any{"id": id, "name": name, "prefix": tokenPrefix(raw), "permissions": perms, "expires_at": expires, "created_at": created}, "secret": raw})
}

func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE api_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, id, p.UserID)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "API key not found")
		return
	}
	s.audit(r, "api_key.revoke", "api_key", id.String(), nil)
	w.WriteHeader(204)
}
