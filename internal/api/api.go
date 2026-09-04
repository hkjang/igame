package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/hkjang/igame/internal/secretbox"
	"github.com/hkjang/igame/internal/version"
	"github.com/hkjang/igame/internal/web"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	serviceName   = "igame"
	sessionCookie = "igame_session"
	maxJSONBody   = 2 << 20
)

type Server struct {
	DB      *pgxpool.Pool
	Secrets *secretbox.Box
	Log     *slog.Logger
	HTTP    *http.Client
	Now     func() time.Time

	settingsMu    sync.RWMutex
	settingsCache map[string]settingEntry
	providerMu    sync.RWMutex
	providerCache map[string]providerEntry
	logins        loginThrottle
	draining      chan struct{}
	drainOnce     sync.Once
}

// Drain releases responses that would otherwise stay connected indefinitely, so
// a graceful shutdown is not held open by idle stream clients. Requests that are
// actually doing work are left alone and finish normally.
func (s *Server) Drain() {
	s.drainOnce.Do(func() {
		if s.draining != nil {
			close(s.draining)
		}
	})
}

func New(db *pgxpool.Pool, secrets *secretbox.Box, log *slog.Logger) *Server {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // offline installations do not inherit ambient proxy variables
	return &Server{DB: db, Secrets: secrets, Log: log, HTTP: &http.Client{Transport: transport, Timeout: 30 * time.Second}, Now: func() time.Time { return time.Now().UTC() }, draining: make(chan struct{})}
}

type Principal struct {
	UserID      uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Department  string    `json:"department"`
	Team        string    `json:"team"`
	Role        string    `json:"role"`
	Permissions []string  `json:"-"`
	AuthType    string    `json:"-"`
}

type contextKey int

const principalKey contextKey = 1

func principalFrom(r *http.Request) (Principal, bool) {
	p, ok := r.Context().Value(principalKey).(Principal)
	return p, ok
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(s.securityHeaders)
	r.Use(s.csrfProtection)
	r.Get("/healthz", s.live)
	r.Get("/readyz", s.ready)
	r.Get("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, version.Current()) })
	r.Get("/api/v1/public/config", s.publicConfig)
	r.Post("/api/v1/auth/login", s.login)
	r.Post("/api/v1/auth/bootstrap/login", s.login)
	r.Post("/api/v1/auth/logout", s.logout)
	r.Get("/api/v1/auth/oidc/login", s.oidcLogin)
	r.Get("/api/v1/auth/oidc/start", s.oidcLogin)
	r.Get("/api/v1/auth/oidc/callback", s.oidcCallback)

	r.Group(func(a chi.Router) {
		a.Use(s.requireAuth)
		a.Use(s.enforceAPIKeyPermissions)
		a.Get("/api/v1/me", s.me)
		a.Patch("/api/v1/me", s.updateMe)
		a.Put("/api/v1/me/password", s.changePassword)
		a.Get("/api/v1/me/preferences", s.getPreferences)
		a.Put("/api/v1/me/preferences", s.putPreferences)
		a.Get("/api/v1/me/history", s.playHistory)
		a.Get("/api/v1/me/achievements", s.myAchievements)
		a.Get("/api/v1/me/api-keys", s.listAPIKeys)
		a.Post("/api/v1/me/api-keys", s.createAPIKey)
		a.Patch("/api/v1/me/api-keys/{id}", s.updateAPIKey)
		a.Post("/api/v1/me/api-keys/{id}/rotate", s.rotateAPIKey)
		a.Delete("/api/v1/me/api-keys/{id}", s.revokeAPIKey)

		a.Get("/api/v1/games", s.listGames)
		a.Get("/api/v1/games/{id}", s.getGame)
		a.Post("/api/v1/games/{id}/favorite", s.addFavorite)
		a.Delete("/api/v1/games/{id}/favorite", s.removeFavorite)
		a.Post("/api/v1/games/{id}/sessions", s.startGameSession)
		a.Post("/api/v1/sessions/{id}/finish", s.finishGameSession)
		a.Post("/api/v1/scores", s.submitScore)
		a.Post("/api/v1/telemetry", s.submitTelemetry)
		a.Get("/api/v1/rankings", s.rankings)
		a.Get("/api/v1/rankings/{gameID}", s.rankings)
		a.Get("/api/v1/achievements", s.listAchievements)
		a.Post("/api/v1/me/achievements", s.unlockAchievement)
		a.Get("/api/v1/seasons", s.listSeasons)
		a.Get("/api/v1/events", s.listEvents)
		a.Get("/api/v1/events/{id}", s.getEvent)
		a.Post("/api/v1/events/{id}/join", s.joinEvent)
		a.Get("/api/v1/notices", s.listPublicNotices)
		a.Get("/api/v1/banners", s.listPublicBanners)
		a.Get("/api/v1/realmguard/config", s.realmGuardConfig)
		a.Get("/api/v1/realmguard/version", s.realmGuardVersion)
		a.Get("/api/v1/realmguard/progress", s.realmGuardProgress)
		a.Put("/api/v1/realmguard/progress", s.putRealmGuardProgress)
		a.Post("/api/v1/realmguard/results", s.submitRealmGuardResult)
		a.Get("/api/v1/realmguard/rankings", s.realmGuardRankings)
		a.Get("/api/v1/defense/{slug}/config", s.defenseConfig)
		a.Get("/api/v1/defense/{slug}/version", s.defenseVersion)
		a.Get("/api/v1/defense/{slug}/progress", s.defenseProgress)
		a.Post("/api/v1/defense/{slug}/results", s.submitDefenseResult)
		a.Get("/api/v1/defense/{slug}/rankings", s.defenseRankings)
		a.Get("/api/v1/defense/{slug}/learning", s.defenseLearning)
		a.Post("/api/v1/defense/{slug}/education/events/{eventID}/answer", s.answerDefenseEducationEvent)
		a.With(s.requireRole("manager", "operator", "admin")).Get("/api/v1/defense/{slug}/versions/{id}/preview", s.previewDefenseVersion)
		a.With(s.requireRole("manager", "admin")).Get("/api/v1/defense/versions/pending", s.listPendingDefenseVersions)
		a.With(s.requireRole("manager", "admin")).Post("/api/v1/defense/versions/{id}/review", s.reviewDefenseVersion)
		a.With(s.requireRole("manager", "admin")).Get("/api/v1/realmguard/versions/pending", s.listPendingRealmGuardVersions)
		a.With(s.requireRole("manager", "admin")).Post("/api/v1/realmguard/versions/{id}/approve", s.approveRealmGuardVersion)
		a.With(s.requireRole("manager", "admin")).Post("/api/v1/realmguard/versions/{id}/review", s.approveRealmGuardVersion)
		a.With(s.requireRole("manager", "operator", "admin")).Get("/api/v1/realmguard/versions/{id}/preview", s.previewRealmGuardVersion)
		a.Post("/api/v1/workflow/requests", s.createWorkflowRequest)
		a.Get("/api/v1/workflow/requests", s.listMyWorkflowRequests)
		a.Post("/api/v1/workflow/requests/{id}/review", s.reviewWorkflowRequest)
		a.Get("/api/v1/workflow/reviews", s.listWorkflowReviews)
		a.Post("/api/v1/workflow/reviews/{id}", s.reviewWorkflowRequest)
		a.Post("/api/v1/ai/chat/completions", s.aiChatCompletions)

		a.Route("/api/v1/admin", func(admin chi.Router) {
			admin.Use(s.requireRole("admin", "operator"))
			admin.Get("/dashboard", s.adminDashboard)
			admin.Get("/status", s.adminStatus)
			admin.Get("/analytics", s.adminAnalytics)
			admin.With(s.requireRole("admin")).Get("/settings", s.listSettings)
			admin.With(s.requireRole("admin")).Get("/settings/{key}", s.getSetting)
			admin.With(s.requireRole("admin")).Put("/settings/{key}", s.putSetting)
			admin.With(s.requireRole("admin")).Get("/oidc", s.getOIDCSetting)
			admin.With(s.requireRole("admin")).Put("/oidc", s.putOIDCSetting)
			admin.With(s.requireRole("admin")).Get("/ai", s.getAISetting)
			admin.With(s.requireRole("admin")).Put("/ai", s.putAISetting)
			admin.Get("/games", s.adminListGames)
			admin.Post("/games", s.createGame)
			admin.Put("/games/{id}", s.updateGame)
			admin.Delete("/games/{id}", s.deleteGame)
			admin.Get("/categories", s.listCategories)
			admin.Post("/categories", s.createCategory)
			admin.Put("/categories/{id}", s.updateCategory)
			admin.Delete("/categories/{id}", s.deleteCategory)
			admin.With(s.requireRole("admin")).Get("/users", s.listUsers)
			admin.With(s.requireRole("admin")).Patch("/users/{id}", s.updateUser)
			admin.Get("/seasons", s.adminListSeasons)
			admin.Post("/seasons", s.createSeason)
			admin.Put("/seasons/{id}", s.updateSeason)
			admin.Delete("/seasons/{id}", s.deleteSeason)
			admin.Get("/events", s.adminListEvents)
			admin.Post("/events", s.createEvent)
			admin.Put("/events/{id}", s.updateEvent)
			admin.Delete("/events/{id}", s.deleteEvent)
			admin.Get("/tournaments", s.listTournaments)
			admin.Post("/tournaments", s.createTournament)
			admin.Put("/tournaments/{id}", s.updateTournament)
			admin.Delete("/tournaments/{id}", s.deleteTournament)
			admin.Get("/achievements", s.adminListAchievements)
			admin.Post("/achievements", s.createAchievement)
			admin.Put("/achievements/{id}", s.updateAchievement)
			admin.Delete("/achievements/{id}", s.deleteAchievement)
			admin.Get("/rewards", s.listRewards)
			admin.Post("/rewards", s.createReward)
			admin.Put("/rewards/{id}", s.updateReward)
			admin.Delete("/rewards/{id}", s.deleteReward)
			admin.Get("/notices", s.listAdminNotices)
			admin.Post("/notices", s.createNotice)
			admin.Put("/notices/{id}", s.updateNotice)
			admin.Delete("/notices/{id}", s.deleteNotice)
			admin.Get("/banners", s.listAdminBanners)
			admin.Post("/banners", s.createBanner)
			admin.Put("/banners/{id}", s.updateBanner)
			admin.Delete("/banners/{id}", s.deleteBanner)
			admin.Get("/rankings", s.adminRankings)
			admin.Put("/rankings/{id}", s.moderateRanking)
			admin.Delete("/rankings/{id}", s.excludeRanking)
			admin.Get("/workflow/requests", s.adminListWorkflowRequests)
			admin.Post("/workflow/requests/{id}/review", s.reviewWorkflowRequest)
			admin.Get("/realmguard/drafts/{section}", s.getRealmGuardDraftSection)
			admin.Put("/realmguard/drafts/{section}", s.putRealmGuardDraftSection)
			admin.Post("/realmguard/drafts/{section}/items", s.createRealmGuardDraftItem)
			admin.Put("/realmguard/drafts/{section}/items/{itemID}", s.updateRealmGuardDraftItem)
			admin.Delete("/realmguard/drafts/{section}/items/{itemID}", s.deleteRealmGuardDraftItem)
			admin.Get("/realmguard/versions", s.listRealmGuardVersions)
			admin.Post("/realmguard/versions", s.createRealmGuardVersion)
			admin.Post("/realmguard/versions/{id}/test", s.testRealmGuardVersion)
			admin.Post("/realmguard/versions/{id}/approve", s.approveRealmGuardVersion)
			admin.Post("/realmguard/versions/{id}/review", s.approveRealmGuardVersion)
			admin.Post("/realmguard/versions/{id}/publish", s.publishRealmGuardVersion)
			admin.Get("/realmguard/telemetry", s.realmGuardTelemetry)
			admin.Get("/defense/{slug}/drafts/{section}", s.getDefenseDraftSection)
			admin.Put("/defense/{slug}/drafts/{section}", s.putDefenseDraftSection)
			admin.Get("/defense/{slug}/versions", s.listDefenseVersions)
			admin.Post("/defense/{slug}/versions", s.createDefenseVersion)
			admin.Post("/defense/{slug}/versions/{id}/test", s.testDefenseVersion)
			admin.Post("/defense/{slug}/versions/{id}/publish", s.publishDefenseVersion)
			admin.Get("/defense/{slug}/telemetry", s.defenseTelemetryReport)
			admin.Get("/defense/{slug}/learning-report", s.defenseLearningReport)
			admin.With(s.requireRole("admin")).Get("/audit", s.listAuditLogs)
		})
	})

	// Streamable HTTP MCP shares the same auth implementation, but produces
	// JSON-RPC authentication errors rather than REST errors.
	r.With(s.requireMCPAuth).Get("/mcp", s.mcpGet)
	r.With(s.requireMCPAuth).Post("/mcp", s.mcpPost)
	r.Mount("/", web.Handler())
	return r
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The identifier the failure logs carry, handed back on every response so
		// a colleague reporting "저장이 안 됩니다" can quote something an operator
		// can grep for. It identifies the request, not the person.
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		var policy struct {
			AllowedFrameOrigins   []string `json:"allowed_frame_origins"`
			AllowedConnectOrigins []string `json:"allowed_connect_origins"`
		}
		// The health endpoints answer without consulting service configuration so
		// they stay usable while the database is degraded.
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			_ = s.setting(r.Context(), "service", &policy)
			// Only the configured public URL or a trusted proxy may declare HTTPS;
			// an untrusted forwarded header must not be able to pin HSTS on a
			// deployment that is actually served over plaintext.
			if strings.HasPrefix(s.requestBaseURL(r), "https://") {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000")
			}
		}
		frames := []string{"'self'"}
		connect := []string{"'self'"}
		for _, candidate := range policy.AllowedFrameOrigins {
			if origin, ok := cspOrigin(candidate); ok {
				frames = append(frames, origin)
			}
		}
		for _, candidate := range policy.AllowedConnectOrigins {
			if origin, ok := cspOrigin(candidate); ok {
				connect = append(connect, origin)
			}
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src "+strings.Join(connect, " ")+"; frame-src "+strings.Join(frames, " ")+"; object-src 'none'; base-uri 'self'; frame-ancestors 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func cspOrigin(candidate string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(candidate))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

func (s *Server) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "service": serviceName, "version": version.Version})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		s.serverError(w, r, 503, "database_unavailable", "database is unavailable", err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "service": serviceName, "version": version.Version})
}

func (s *Server) csrfProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		} // non-browser clients
		if !s.originAllowed(r, origin) {
			// A rejection here is almost always a configuration mismatch rather
			// than an attack, and these values are exactly what has to be
			// reconciled. Without them an operator has only an opaque 403.
			if s.Log != nil {
				s.Log.Warn("request origin rejected",
					"origin", origin, "accepted", s.browserOrigins(r),
					"method", r.Method, "path", r.URL.Path)
			}
			writeError(w, 403, "csrf_rejected", "request origin does not match this service address or its configured public URL")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

func (s *Server) authenticate(r *http.Request) (Principal, error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		key := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if strings.HasPrefix(key, "igk_") {
			return s.authenticateAPIKey(r.Context(), key)
		}
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return Principal{}, errors.New("no credentials")
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	var p Principal
	// The last_seen_at refresh rides along in a data-modifying CTE: PostgreSQL
	// runs it exactly once per statement, which keeps authentication at a single
	// round trip on every request.
	err = s.DB.QueryRow(r.Context(), `WITH touched AS (
			UPDATE auth_sessions SET last_seen_at=now()
			WHERE token_hash=$1 AND expires_at>now() AND last_seen_at<now()-interval '5 minutes')
		SELECT u.id,u.username,u.display_name,u.email,u.department,u.team,u.role
		FROM auth_sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.expires_at>now() AND u.status='active'`, hash[:]).Scan(
		&p.UserID, &p.Username, &p.DisplayName, &p.Email, &p.Department, &p.Team, &p.Role)
	if err != nil {
		return Principal{}, err
	}
	p.AuthType = "session"
	return p, nil
}

func (s *Server) authenticateAPIKey(ctx context.Context, raw string) (Principal, error) {
	hash := sha256.Sum256([]byte(raw))
	var p Principal
	err := s.DB.QueryRow(ctx, `WITH touched AS (
			UPDATE api_keys SET last_used_at=now()
			WHERE key_hash=$1 AND revoked_at IS NULL AND (last_used_at IS NULL OR last_used_at<now()-interval '5 minutes'))
		SELECT u.id,u.username,u.display_name,u.email,u.department,u.team,u.role,k.permissions
		FROM api_keys k JOIN users u ON u.id=k.user_id
		WHERE k.key_hash=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()) AND u.status='active'`, hash[:]).Scan(
		&p.UserID, &p.Username, &p.DisplayName, &p.Email, &p.Department, &p.Team, &p.Role, &p.Permissions)
	if err != nil {
		return Principal{}, err
	}
	// Apply the current global and role policy on every request. This makes
	// permission removal and role changes effective immediately for existing
	// keys without exposing or rewriting their original secret.
	policy, err := s.loadAPIKeyPolicyContext(ctx)
	if err != nil {
		return Principal{}, fmt.Errorf("load API key policy: %w", err)
	}
	p.Permissions = effectiveKeyPermissions(p, p.Permissions, policy)
	p.AuthType = "api_key"
	return p, nil
}

func (s *Server) requireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := principalFrom(r)
			if !allowed[p.Role] || (p.AuthType == "api_key" && !p.Can("admin:*")) {
				s.recordAccessDenied(r, roles)
				writeError(w, 403, "forbidden", "insufficient role or API key scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// recordAccessDenied writes down an attempt on something the caller may not
// have.
//
// The audit log recorded what succeeded and nothing about what was refused, so
// an identified account could probe every admin endpoint — user management, the
// audit log itself, the OIDC configuration — and leave no row and no log line.
// A refusal is the entry an auditor most wants to find.
func (s *Server) recordAccessDenied(r *http.Request, required []string) {
	p, _ := principalFrom(r)
	detail := accessDeniedDetail(r.Method, r.URL.Path, p.Role, p.AuthType, required)
	if s.Log != nil {
		s.Log.Warn("access denied", "path", r.URL.Path, "method", r.Method,
			"role", p.Role, "required", required, "request_id", middleware.GetReqID(r.Context()))
	}
	s.audit(r, "access.denied", "endpoint", r.URL.Path, detail)
}

// accessDeniedDetail is what the audit row says about a refusal: what was
// attempted, by an account holding what, against what was needed.
func accessDeniedDetail(method, path, role, authType string, required []string) map[string]any {
	return map[string]any{
		"method":         method,
		"path":           path,
		"role":           role,
		"auth_type":      authType,
		"required_roles": required,
	}
}

func (s *Server) enforceAPIKeyPermissions(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r)
		if p.AuthType != "api_key" {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		var required string
		switch {
		case strings.HasPrefix(path, "/api/v1/admin/"):
			required = "admin:*"
		case strings.HasPrefix(path, "/api/v1/realmguard/versions/"):
			required = "admin:*"
		case strings.HasPrefix(path, "/api/v1/defense/versions/") || strings.Contains(path, "/versions/") && strings.HasPrefix(path, "/api/v1/defense/"):
			required = "admin:*"
		case strings.HasPrefix(path, "/api/v1/defense/") && strings.HasSuffix(path, "/results"):
			required = "scores:write"
		case strings.HasPrefix(path, "/api/v1/defense/") && strings.Contains(path, "/education/events/"):
			required = "scores:write"
		case strings.HasPrefix(path, "/api/v1/defense/") && strings.HasSuffix(path, "/progress") || strings.HasPrefix(path, "/api/v1/defense/") && strings.HasSuffix(path, "/learning"):
			required = "profile:read"
		case strings.HasPrefix(path, "/api/v1/defense/") && strings.HasSuffix(path, "/rankings"):
			required = "rankings:read"
		case strings.HasPrefix(path, "/api/v1/defense/"):
			required = "games:read"
		case path == "/api/v1/realmguard/results":
			required = "scores:write"
		case path == "/api/v1/realmguard/progress":
			if r.Method == http.MethodGet {
				required = "profile:read"
			} else {
				required = "profile:write"
			}
		case path == "/api/v1/realmguard/rankings":
			required = "rankings:read"
		case path == "/api/v1/realmguard/config" || path == "/api/v1/realmguard/version":
			required = "games:read"
		case strings.Contains(path, "/api-keys"):
			s.recordAccessDenied(r, []string{"a session, not an API key"})
			writeError(w, 403, "forbidden", "API keys cannot manage API keys")
			return
		case path == "/api/v1/ai/chat/completions":
			required = "ai:invoke"
		case strings.Contains(path, "/scores"):
			required = "scores:write"
		case strings.Contains(path, "/telemetry"):
			required = "sessions:write"
		case strings.Contains(path, "/sessions") && r.Method != http.MethodGet:
			required = "sessions:write"
		case strings.Contains(path, "/workflow"):
			required = "workflow:write"
		case strings.Contains(path, "/rankings"):
			required = "rankings:read"
		case path == "/api/v1/me" && r.Method == http.MethodPatch || path == "/api/v1/me/preferences" && r.Method == http.MethodPut:
			required = "profile:write"
		case strings.HasSuffix(path, "/favorite") && (r.Method == http.MethodPost || r.Method == http.MethodDelete):
			required = "profile:write"
		case strings.HasSuffix(path, "/join") && r.Method == http.MethodPost:
			required = "profile:write"
		case path == "/api/v1/me/achievements" && r.Method == http.MethodPost:
			required = "scores:write"
		case strings.Contains(path, "/me"):
			required = "profile:read"
		case strings.Contains(path, "/games") || strings.Contains(path, "/events") || strings.Contains(path, "/seasons") || strings.Contains(path, "/achievements") || strings.Contains(path, "/notices") || strings.Contains(path, "/banners"):
			required = "games:read"
		default:
			required = "api:access"
		}
		if !p.Can(required) {
			s.recordAccessDenied(r, []string{required})
			writeError(w, 403, "insufficient_scope", "API key requires "+required)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p Principal) Can(permission string) bool {
	if p.Role == "admin" && p.AuthType != "api_key" {
		return true
	}
	for _, have := range p.Permissions {
		if have == "*" || have == permission || (strings.HasSuffix(have, ":*") && strings.HasPrefix(permission, strings.TrimSuffix(have, "*"))) {
			return true
		}
	}
	return false
}

func (s *Server) audit(r *http.Request, action, resourceType, resourceID string, detail any) {
	// A server without a database cannot record anything, and now that refusals
	// are audited this is reached from middleware that runs before any handler
	// has needed one.
	if s.DB == nil {
		return
	}
	p, _ := principalFrom(r)
	body, _ := json.Marshal(detail)
	if len(body) == 0 {
		body = []byte("{}")
	}
	_, err := s.DB.Exec(context.WithoutCancel(r.Context()), `INSERT INTO audit_logs(actor_id,action,resource_type,resource_id,remote_addr,user_agent,detail)
		VALUES(NULLIF($1,'00000000-0000-0000-0000-000000000000')::uuid,$2,$3,$4,$5,$6,$7)`, p.UserID.String(), action, resourceType, resourceID, s.clientIP(r), r.UserAgent(), body)
	if err != nil {
		s.Log.Warn("write audit log", "error", err)
	}
}

func (s *Server) clientIP(r *http.Request) string {
	var service struct {
		TrustProxy bool `json:"trust_proxy"`
	}
	if s.setting(r.Context(), "service", &service) == nil && service.TrustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			if ip := net.ParseIP(forwarded); ip != nil {
				return ip.String()
			}
		}
		if forwarded := strings.TrimSpace(r.Header.Get("X-Real-IP")); forwarded != "" {
			if ip := net.ParseIP(forwarded); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// requestBaseURL is the canonical address of this deployment: the configured
// public URL when there is one, otherwise wherever the request arrived. It
// backs the OIDC redirect URI, the session cookie's Secure flag and HSTS, all
// of which want the single canonical answer.
func (s *Server) requestBaseURL(r *http.Request) string {
	var service struct {
		PublicURL string `json:"public_url"`
	}
	if s.setting(r.Context(), "service", &service) == nil && service.PublicURL != "" {
		return canonicalBaseURL(service.PublicURL)
	}
	return s.observedBaseURL(r)
}

// canonicalBaseURL puts a configured public URL into the single form its
// readers expect: a lowercase scheme with no trailing slash.
//
// A URL scheme is case-insensitive, so "HTTPS://games.example.com" names the
// same deployment as the lowercase spelling and the settings screen accepts it
// — url.Parse lowercases the scheme before the check reads it. Everything that
// consumes the stored value afterwards compares the raw string instead, and all
// three of those comparisons test a "https://" prefix: the session cookie's
// Secure flag, the HSTS header and the status screen's TLS indicator. One
// capital letter therefore stripped Secure from the session cookie of a service
// that is served over TLS, which is the one place the capital matters.
func canonicalBaseURL(value string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(value), "/")
	scheme, rest, separated := strings.Cut(trimmed, "://")
	if !separated {
		return trimmed
	}
	return strings.ToLower(scheme) + "://" + rest
}

// observedBaseURL is the address this particular request actually came in on.
func (s *Server) observedBaseURL(r *http.Request) string {
	var service struct {
		TrustProxy bool `json:"trust_proxy"`
	}
	_ = s.setting(r.Context(), "service", &service)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if service.TrustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		}
	}
	host := r.Host
	if service.TrustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwarded != "" {
			host = forwarded
		}
	}
	return scheme + "://" + host
}

// browserOrigins lists the origins a browser may legitimately present for this
// deployment: the canonical public URL, and the address the request arrived on.
//
// Accepting both is what lets an intranet service be reached by IP, short name
// and FQDN without state-changing requests failing. It keeps the CSRF property
// intact: a page on another site presents its own origin, which matches
// neither, and it cannot forge the browser-set Origin header. A host an
// attacker points at this service carries none of this service's cookies, so
// matching on the observed address gives them nothing.
func (s *Server) browserOrigins(r *http.Request) []string {
	origins := []string{s.requestBaseURL(r)}
	if observed := s.observedBaseURL(r); observed != origins[0] {
		origins = append(origins, observed)
	}
	return origins
}

func (s *Server) originAllowed(r *http.Request, origin string) bool {
	for _, allowed := range s.browserOrigins(r) {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeError(w, 400, "invalid_id", "invalid resource identifier")
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid_json", "invalid request body: "+err.Error())
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "invalid_json", "request body must contain one JSON value")
		return false
	}
	return true
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, 400, "invalid_json", "invalid request body")
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid_json", "invalid request body: "+err.Error())
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "invalid_json", "request body must contain one JSON value")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// dbError maps a query failure onto the public error contract. Anything that is
// not a missing row is a server fault and is logged with the request identity;
// the client still only learns that the request failed.
func (s *Server) dbError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "resource not found")
		return
	}
	s.logRequestError(r, err)
	s.serverError(w, r, 500, "internal_error", "internal server error", err)
}

// serverError reports a fault the caller cannot act on. The cause goes to the
// log with the request identity; the client is told exactly what it was told
// before.
//
// Two dozen 5xx paths used to return with the error still in hand and nothing
// written down. When the published Defense content stopped decoding, the API
// answered 500 invalid_published_content and the log stayed silent, so there
// was no way to learn which field had gone wrong.
func (s *Server) serverError(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	s.logRequestError(r, cause)
	writeError(w, status, code, message)
}

func (s *Server) logRequestError(r *http.Request, err error) {
	if s.Log == nil || err == nil || errors.Is(err, context.Canceled) {
		return
	}
	attrs := []any{"error", err}
	if r != nil {
		attrs = append(attrs, "method", r.Method, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()))
	}
	s.Log.Error("request failed", attrs...)
}

func pageParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	return
}

// likeEscaper defuses the three characters ILIKE reads as syntax. PostgreSQL
// escapes LIKE patterns with a backslash by default, so the backslash itself
// has to be doubled before it can protect the wildcards.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// searchPattern turns what an operator typed into a substring pattern that
// matches that text and nothing else.
//
// The search boxes are plain substring filters, but the term used to be pasted
// straight between two wildcards. A term containing % or _ — "50%", "user_id",
// a Windows path in a user agent — then reached PostgreSQL as a wildcard of its
// own, and a single "%" or "_" matched every row: the filter looked applied and
// silently did nothing. An empty term stays empty so callers can keep testing
// it to skip the filter entirely.
func searchPattern(q string) string {
	if q == "" {
		return ""
	}
	return "%" + likeEscaper.Replace(q) + "%"
}

// safeReturnTo keeps the screen a login sends the reader back to on this
// service. Only a path is accepted: anything naming another host is replaced
// with the portal root.
//
// A protocol-relative "//host/path" carries no scheme, so it is not absolute
// and its path still starts with a single slash — the authority hides in the
// host instead, which is why the host is checked directly.
func safeReturnTo(value string) string {
	if value == "" {
		return "/"
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "" || u.Opaque != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	// "///host" leaves an empty authority behind, and browsers read the extra
	// slashes as the start of one anyway.
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	return u.String()
}

func tokenPrefix(raw string) string {
	if len(raw) > 12 {
		return raw[:12]
	}
	return raw
}

func hexHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Server) setting(ctx context.Context, key string, dst any) error {
	raw, err := s.settingRaw(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode setting %s: %w", key, err)
	}
	return nil
}
