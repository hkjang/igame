package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
)

// MCP over SSO — the personal key stays, and a Keycloak access token opens
// /mcp as well.
//
// The MCP authorization specification (2025-06-18 and later) is OAuth 2.1: the
// MCP server is a resource server that publishes where its authorization
// server is (RFC 9728, /.well-known/oauth-protected-resource), and a client
// refused with 401 reads that document, sends the person through Keycloak
// with PKCE and comes back with an access token whose audience is this
// server. Nothing about issuing tokens happens here — Keycloak does that —
// and this file only answers two questions: where is the authorization
// server, and is this token one it issued for us.
//
// A token from SSO is a second door into the same room a personal key opens.
// It authenticates an account that already exists, carries the scopes the
// administrator chose, and is held to the same policy a key is. It never
// creates an account: signing in to the web once is what registers one, and
// a program presenting a token is not the moment to decide who somebody is.
// It is accepted on /mcp only; REST, the admin API and everything else keep
// taking keys and sessions exactly as before.

// mcpSetting is the `mcp` system setting. Its fields are addressed the way the
// standard names them — mcp.oauth.enabled, mcp.oauth.resource,
// mcp.oauth.audience, mcp.oauth.scopes — as nested JSON, the shape every other
// setting in this service has.
type mcpSetting struct {
	OAuth mcpOAuthSetting `json:"oauth"`
}

type mcpOAuthSetting struct {
	// Enabled is off by default: a fresh installation must behave exactly as
	// before.
	Enabled bool `json:"enabled"`
	// Resource is the identifier this server claims (RFC 8707): the public
	// address clients actually connect to plus the MCP path. Empty means it is
	// derived from the service public URL.
	Resource string `json:"resource,omitempty"`
	// Audience lists accepted aud or azp values beside the resource itself. A
	// real Keycloak 26 puts only "account" in aud and the client id in azp,
	// so naming the MCP client id here is the path that needs no mapper.
	Audience []string `json:"audience,omitempty"`
	// Scopes are what a valid token may do. Tokens do not carry this
	// service's permission vocabulary unless somebody teaches Keycloak it,
	// so the administrator states the ceiling here instead.
	Scopes []string `json:"scopes,omitempty"`
}

// defaultMCPOAuthScopes is the read-only set a personal key for MCP usually
// carries: enough to browse the catalog, rankings and one's own profile.
var defaultMCPOAuthScopes = []string{"mcp:access", "games:read", "rankings:read", "profile:read"}

func (m *mcpOAuthSetting) defaults() {
	m.Resource = strings.TrimSpace(m.Resource)
	if len(m.Scopes) == 0 {
		m.Scopes = slices.Clone(defaultMCPOAuthScopes)
	}
}

// mcpOAuthConfig is the resolved picture for one request: the stored setting
// joined with the OIDC setting it reuses and the resource identifier derived
// for this deployment.
type mcpOAuthConfig struct {
	mcpOAuthSetting
	Issuer        string
	ClientID      string
	UsernameClaim string
	// Resource is always filled in, from the setting or the public URL.
	Resource string
	// Inactive names why tokens are not accepted even though the switch may be
	// on; empty when everything needed is present.
	Inactive string
}

func (c mcpOAuthConfig) Active() bool { return c.Inactive == "" }

// MetadataURL is where a refused client is sent to learn the above. It sits
// beside the resource identifier, which is the one address that is known to
// reach this server from outside.
func (c mcpOAuthConfig) MetadataURL(base string) string {
	if strings.HasSuffix(c.Resource, "/mcp") {
		base = strings.TrimSuffix(c.Resource, "/mcp")
	}
	return base + "/.well-known/oauth-protected-resource/mcp"
}

// AcceptedAudiences is every value a token may be bound to and still be for
// this server: the resource identifier (the Audience mapper path), what the
// administrator listed, and the web sign-in client — Keycloak issues tokens to
// that client with its id in azp without any mapper.
func (c mcpOAuthConfig) AcceptedAudiences() []string {
	accepted := append([]string{c.Resource}, c.Audience...)
	if c.ClientID != "" {
		accepted = append(accepted, c.ClientID)
	}
	return accepted
}

// mcpOAuthConfig reads the settings that decide whether SSO tokens are
// accepted. A setting that was never stored is the default (off); a setting
// that cannot be read is an error, and the caller decides whether to answer
// "unavailable" or to refuse.
func (s *Server) mcpOAuthConfig(r *http.Request) (mcpOAuthConfig, error) {
	var stored mcpSetting
	if err := s.setting(r.Context(), "mcp", &stored); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return mcpOAuthConfig{}, err
	}
	stored.OAuth.defaults()
	var oidcCfg oidcSetting
	if err := s.setting(r.Context(), "oidc", &oidcCfg); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return mcpOAuthConfig{}, err
	}
	oidcCfg.defaults()
	cfg := mcpOAuthConfig{
		mcpOAuthSetting: stored.OAuth,
		Issuer:          strings.TrimRight(strings.TrimSpace(oidcCfg.Issuer), "/"),
		ClientID:        strings.TrimSpace(oidcCfg.ClientID),
		UsernameClaim:   oidcCfg.UsernameClaim,
		Resource:        stored.OAuth.Resource,
	}
	if cfg.Resource == "" {
		cfg.Resource = s.requestBaseURL(r) + "/mcp"
	}
	switch {
	case !stored.OAuth.Enabled:
		cfg.Inactive = "mcp.oauth.enabled is off"
	case cfg.Issuer == "":
		// Enabled without an issuer is refused at save time; this is the
		// second instance, or the OIDC setting emptied afterwards.
		cfg.Inactive = "oidc.issuer_url is empty"
		if s.Log != nil {
			s.Log.Warn("mcp oauth is enabled but inactive", "reason", cfg.Inactive)
		}
	}
	return cfg, nil
}

// validateMCPSetting is the save-time half of the rule above: turning the
// switch on needs the issuer it will verify tokens against, and the message
// says which setting to fill in first.
func (s *Server) validateMCPSetting(ctx context.Context, stored mcpSetting) string {
	if !stored.OAuth.Enabled {
		return ""
	}
	var oidcCfg oidcSetting
	if err := s.setting(ctx, "oidc", &oidcCfg); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "current OIDC setting is unavailable; try again"
	}
	if strings.TrimSpace(oidcCfg.Issuer) == "" {
		return "mcp.oauth.enabled requires the Keycloak issuer in the OIDC setting"
	}
	return ""
}

// mcpOAuthSigningAlgs is the asymmetric family only. Keycloak advertises HS*
// as well, and go-oidc would accept whatever the discovery document lists;
// naming the list here is what keeps a token signed with a shared secret out.
var mcpOAuthSigningAlgs = []string{oidc.RS256, oidc.RS384, oidc.RS512, oidc.ES256, oidc.ES384, oidc.ES512, oidc.PS256, oidc.PS384, oidc.PS512}

// mcpRefusal is why a presented token was not accepted, in words an operator
// can act on. The cause stays in the log; the message goes to the client.
type mcpRefusal struct {
	Message string
	Cause   error
}

func (e *mcpRefusal) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *mcpRefusal) Unwrap() error { return e.Cause }

// looksLikeJWT is the cheap shape test that separates "not a key" from "a
// token we might verify": three non-empty dot-separated parts.
func looksLikeJWT(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
}

// bearerToken returns the value of an Authorization: Bearer header.
func bearerToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")), true
}

// authenticateMCP is authenticate with the second door. The same
// Authorization: Bearer header carries either a personal key, recognised by
// its igk_ prefix, or a JWT. A key and a session go exactly where they went
// before. A JWT is verified as an SSO access token only while the feature is
// active; otherwise it is refused the way an unknown bearer always was, so an
// installation that never turned this on says nothing new.
func (s *Server) authenticateMCP(r *http.Request, cfg mcpOAuthConfig) (Principal, error) {
	token, ok := bearerToken(r)
	if !ok || strings.HasPrefix(token, "igk_") || !looksLikeJWT(token) || !cfg.Active() {
		return s.authenticate(r)
	}
	return s.authenticateOAuth(r, cfg, token)
}

// authenticateOAuth turns a bearer access token into a principal, or says
// exactly why it will not.
func (s *Server) authenticateOAuth(r *http.Request, cfg mcpOAuthConfig, token string) (Principal, error) {
	// Discovery must outlive this request: the provider keeps the context for
	// later key fetches.
	providerCtx := oidc.ClientContext(context.WithoutCancel(r.Context()), s.HTTP)
	provider, err := s.oidcProvider(providerCtx, cfg.Issuer)
	if err != nil {
		s.logRequestError(r, fmt.Errorf("mcp oauth discovery: %w", err))
		return Principal{}, &mcpRefusal{Message: "the identity provider could not be reached to verify the SSO token; try again later or tell an administrator"}
	}
	// Signature, issuer, expiry and not-before. The audience is checked below
	// by hand because more than one value is acceptable and the library
	// compares aud alone — it knows nothing of azp.
	verified, err := provider.Verifier(&oidc.Config{SkipClientIDCheck: true, SupportedSigningAlgs: mcpOAuthSigningAlgs, Now: s.Now}).Verify(oidc.ClientContext(r.Context(), s.HTTP), token)
	if err != nil {
		return Principal{}, &mcpRefusal{Message: "SSO access token is not valid (signature, issuer, expiry or not-before); sign in again from the client", Cause: err}
	}
	var claims map[string]any
	if err := verified.Claims(&claims); err != nil {
		return Principal{}, &mcpRefusal{Message: "SSO token claims could not be read", Cause: err}
	}
	// An ID token proves a login; it is not an API credential, and Keycloak
	// marks it as such in its payload.
	if strings.EqualFold(claimString(claims, "typ"), "ID") {
		return Principal{}, &mcpRefusal{Message: "an ID token was presented; send the access token instead"}
	}
	// A cnf claim binds the token to a key this server cannot check (DPoP,
	// mTLS). Accepting it as a plain bearer would drop that binding.
	if _, bound := claims["cnf"]; bound {
		return Principal{}, &mcpRefusal{Message: "the token carries a proof-of-possession binding (cnf) this server cannot verify"}
	}
	if verified.Subject == "" {
		return Principal{}, &mcpRefusal{Message: "SSO token has no subject"}
	}
	// Whom the token was minted for. A token from another application in the
	// same realm verifies just as well, and this is the check that keeps it
	// out. The message names what was seen and what to write down, which is
	// all an operator needs to finish the setup.
	azp := claimString(claims, "azp")
	accepted := cfg.AcceptedAudiences()
	bound := append(slices.Clone(verified.Audience), azp)
	if !slices.ContainsFunc(bound, func(value string) bool { return value != "" && slices.Contains(accepted, value) }) {
		return Principal{}, &mcpRefusal{Message: fmt.Sprintf("token was not issued for this server (aud=%v, azp=%q): add %q to mcp.oauth.audience, or give the Keycloak client an Audience mapper for %q", verified.Audience, azp, azp, cfg.Resource)}
	}
	// The same lookup the web sign-in ends in, without the provisioning half:
	// only an account that signed in through the web already, and only while
	// it is active. Accounts are keyed by subject there, and a local account
	// that happens to share the username is a different person.
	var p Principal
	err = s.DB.QueryRow(r.Context(), `SELECT id,username,display_name,email,department,team,role FROM users WHERE oidc_subject=$1 AND status='active'`, verified.Subject).Scan(
		&p.UserID, &p.Username, &p.DisplayName, &p.Email, &p.Department, &p.Team, &p.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, &mcpRefusal{Message: "no active igame account is linked to this SSO identity; sign in to the web portal once first"}
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up SSO account: %w", err)
	}
	// What the token may do is the administrator's list, narrowed by the
	// token's own scope when it speaks this vocabulary, and then held to the
	// same global and role policy a personal key is.
	policy, err := s.loadAPIKeyPolicyContext(r.Context())
	if err != nil {
		return Principal{}, fmt.Errorf("load API key policy: %w", err)
	}
	p.Permissions = effectiveKeyPermissions(p, tokenScopes(cfg.Scopes, claimString(claims, "scope")), policy)
	p.AuthType = "oauth"
	return p, nil
}

// tokenScopes narrows the configured scopes by the token's when the token
// carries any of this service's permission words. A Keycloak token normally
// says "openid profile email", which names none of them, and then the
// administrator's list stands as it is.
func tokenScopes(configured []string, scope string) []string {
	var spoken []string
	for _, word := range strings.Fields(scope) {
		if slices.Contains(allowedAPIKeyPermissions, word) {
			spoken = append(spoken, word)
		}
	}
	if len(spoken) == 0 {
		return configured
	}
	narrowed := make([]string, 0, len(configured))
	for _, permission := range configured {
		if slices.Contains(spoken, permission) {
			narrowed = append(narrowed, permission)
		}
	}
	return narrowed
}

// mcpChallenge is the WWW-Authenticate header a refusal on /mcp carries. While
// SSO is active it points at the metadata document, which is what turns the
// 401 into an invitation: the client reads it and starts the OAuth flow from
// there. The error parameter is added only when a token was presented and
// refused, and the header is what it always was while SSO is off.
func mcpChallenge(cfg mcpOAuthConfig, base string, refused bool) string {
	if !cfg.Active() {
		return `Bearer realm="igame-mcp", scope="mcp:access"`
	}
	challenge := fmt.Sprintf(`Bearer realm="igame-mcp", resource_metadata=%q`, cfg.MetadataURL(base))
	if refused {
		challenge += `, error="invalid_token"`
	}
	return challenge
}

// protectedResourceMetadata is RFC 9728: the document a refused MCP client
// reads to find the authorization server. Public by design — it says where to
// sign in, not who is signed in — and served bare rather than in the API's
// error envelope, because its reader is an OAuth client library.
func (s *Server) protectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.mcpOAuthConfig(r)
	if err != nil {
		s.serverError(w, r, 503, "setting_unavailable", "MCP settings are unavailable", err)
		return
	}
	if !cfg.Active() {
		writeError(w, 404, "mcp_oauth_disabled", "this server's MCP does not accept SSO tokens; use a personal API key")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, 200, map[string]any{
		"resource":                 cfg.Resource,
		"authorization_servers":    []string{cfg.Issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         cfg.Scopes,
		"resource_name":            "igame MCP",
	})
}

// validMCPSetting checks the shape of the `mcp` setting on save, the way the
// other settings are checked in validateSetting.
func validMCPSetting(stored mcpSetting) string {
	if resource := strings.TrimSpace(stored.OAuth.Resource); resource != "" {
		u, err := url.Parse(resource)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return "mcp.oauth.resource must be an absolute HTTP(S) URL"
		}
	}
	for _, audience := range stored.OAuth.Audience {
		if strings.TrimSpace(audience) == "" || strings.ContainsAny(audience, " \t\r\n") {
			return "mcp.oauth.audience entries must be single words"
		}
	}
	for _, scope := range stored.OAuth.Scopes {
		if scope == "admin:*" || !slices.Contains(allowedAPIKeyPermissions, scope) {
			return "unknown MCP OAuth scope: " + scope
		}
	}
	return ""
}
