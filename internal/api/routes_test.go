package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// apiKeyRouteScopes is the authoritative record of what every authenticated
// route demands from an API key. A new route must be added here deliberately;
// the test below fails on anything missing so a scope is never chosen by
// accident from the fall-through branch.
var apiKeyRouteScopes = map[string]string{
	"GET /api/v1/me":   "profile:read",
	"PATCH /api/v1/me": "profile:write",
	// changePassword additionally rejects every non-session principal, so no API
	// key reaches it whatever scope the path mapping assigns.
	"PUT /api/v1/me/password":                                       "profile:read",
	"GET /api/v1/me/preferences":                                    "profile:read",
	"PUT /api/v1/me/preferences":                                    "profile:write",
	"GET /api/v1/me/history":                                        "profile:read",
	"GET /api/v1/me/achievements":                                   "profile:read",
	"POST /api/v1/me/achievements":                                  "scores:write",
	"GET /api/v1/me/api-keys":                                       "denied",
	"POST /api/v1/me/api-keys":                                      "denied",
	"PATCH /api/v1/me/api-keys/{id}":                                "denied",
	"POST /api/v1/me/api-keys/{id}/rotate":                          "denied",
	"DELETE /api/v1/me/api-keys/{id}":                               "denied",
	"GET /api/v1/games":                                             "games:read",
	"GET /api/v1/games/{id}":                                        "games:read",
	"POST /api/v1/games/{id}/favorite":                              "profile:write",
	"DELETE /api/v1/games/{id}/favorite":                            "profile:write",
	"POST /api/v1/games/{id}/sessions":                              "sessions:write",
	"POST /api/v1/sessions/{id}/finish":                             "sessions:write",
	"POST /api/v1/scores":                                           "scores:write",
	"POST /api/v1/telemetry":                                        "sessions:write",
	"GET /api/v1/rankings":                                          "rankings:read",
	"GET /api/v1/rankings/{gameID}":                                 "rankings:read",
	"GET /api/v1/achievements":                                      "games:read",
	"GET /api/v1/seasons":                                           "games:read",
	"GET /api/v1/events":                                            "games:read",
	"GET /api/v1/events/{id}":                                       "games:read",
	"POST /api/v1/events/{id}/join":                                 "profile:write",
	"GET /api/v1/notices":                                           "games:read",
	"GET /api/v1/banners":                                           "games:read",
	"GET /api/v1/realmguard/config":                                 "games:read",
	"GET /api/v1/realmguard/version":                                "games:read",
	"GET /api/v1/realmguard/progress":                               "profile:read",
	"PUT /api/v1/realmguard/progress":                               "profile:write",
	"POST /api/v1/realmguard/results":                               "scores:write",
	"GET /api/v1/realmguard/rankings":                               "rankings:read",
	"GET /api/v1/defense/{slug}/config":                             "games:read",
	"GET /api/v1/defense/{slug}/version":                            "games:read",
	"GET /api/v1/defense/{slug}/progress":                           "profile:read",
	"POST /api/v1/defense/{slug}/results":                           "scores:write",
	"GET /api/v1/defense/{slug}/rankings":                           "rankings:read",
	"GET /api/v1/defense/{slug}/learning":                           "profile:read",
	"POST /api/v1/defense/{slug}/education/events/{eventID}/answer": "scores:write",
	"GET /api/v1/defense/{slug}/versions/{id}/preview":              "admin:*",
	"GET /api/v1/defense/versions/pending":                          "admin:*",
	"POST /api/v1/defense/versions/{id}/review":                     "admin:*",
	"GET /api/v1/realmguard/versions/pending":                       "admin:*",
	"POST /api/v1/realmguard/versions/{id}/approve":                 "admin:*",
	"POST /api/v1/realmguard/versions/{id}/review":                  "admin:*",
	"GET /api/v1/realmguard/versions/{id}/preview":                  "admin:*",
	"POST /api/v1/workflow/requests":                                "workflow:write",
	"GET /api/v1/workflow/requests":                                 "workflow:write",
	"POST /api/v1/workflow/requests/{id}/review":                    "workflow:write",
	"GET /api/v1/workflow/reviews":                                  "workflow:write",
	"POST /api/v1/workflow/reviews/{id}":                            "workflow:write",
	"POST /api/v1/ai/chat/completions":                              "ai:invoke",
}

// probeAPIKeyScope reports which permission unlocks a route, "denied" when no
// permission does, and "open" when the route needs none at all.
func probeAPIKeyScope(t *testing.T, method, pattern string) string {
	t.Helper()
	path := strings.NewReplacer("{id}", "b1f4a2c0-0000-4000-8000-000000000001",
		"{gameID}", "b1f4a2c0-0000-4000-8000-000000000002",
		"{eventID}", "b1f4a2c0-0000-4000-8000-000000000003",
		"{slug}", "cyber-fortress",
		"{section}", "stages",
		"{itemID}", "item-1",
		"{key}", "service").Replace(pattern)
	call := func(permissions ...string) int {
		r := httptest.NewRequest(method, path, nil)
		principal := Principal{Role: "user", AuthType: "api_key", Permissions: permissions}
		r = r.WithContext(context.WithValue(r.Context(), principalKey, principal))
		w := httptest.NewRecorder()
		(&Server{}).enforceAPIKeyPermissions(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(w, r)
		return w.Code
	}
	if call() == http.StatusNoContent {
		return "open"
	}
	granted := ""
	for _, permission := range allowedAPIKeyPermissions {
		if call(permission) == http.StatusNoContent {
			if granted != "" {
				t.Fatalf("%s %s is unlocked by both %s and %s", method, pattern, granted, permission)
			}
			granted = permission
		}
	}
	if granted == "" {
		return "denied"
	}
	return granted
}

func TestEveryAuthenticatedRouteHasADeliberateAPIKeyScope(t *testing.T) {
	seen := map[string]bool{}
	router, ok := New(nil, nil, nil).Router().(chi.Routes)
	if !ok {
		t.Fatal("Router() no longer returns a chi router; this test can no longer enumerate routes")
	}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/*")
		if !strings.HasPrefix(route, "/api/v1/") || strings.HasPrefix(route, "/api/v1/auth/") ||
			route == "/api/v1/version" || route == "/api/v1/public/config" {
			return nil
		}
		key := method + " " + route
		seen[key] = true
		got := probeAPIKeyScope(t, method, route)
		if strings.HasPrefix(route, "/api/v1/admin/") {
			if got != "admin:*" {
				t.Errorf("%s requires %q, want admin:*", key, got)
			}
			return nil
		}
		want, ok := apiKeyRouteScopes[key]
		if !ok {
			t.Errorf("%s is not listed in apiKeyRouteScopes (it currently requires %q); add it deliberately", key, got)
			return nil
		}
		if got != want {
			t.Errorf("%s requires %q, want %q", key, got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	for key := range apiKeyRouteScopes {
		if !seen[key] {
			t.Errorf("apiKeyRouteScopes lists %s, which the router no longer registers", key)
		}
	}
}

// api:access is the weakest scope every key carries, so no route may be
// reachable with it alone unless it is genuinely meant to be.
func TestNoRouteFallsThroughToBareAPIAccess(t *testing.T) {
	for key, scope := range apiKeyRouteScopes {
		if scope == "api:access" {
			t.Errorf("%s only requires api:access; give it a scope that matches what it does", key)
		}
	}
}
