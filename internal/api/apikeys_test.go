package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEffectiveKeyPermissionsTracksCurrentPolicyAndRole(t *testing.T) {
	policy := apiKeyPolicy{
		AvailablePermissions: []string{"games:read", "scores:write", "admin:*"},
		RolePermissions: map[string][]string{
			"user":  {"games:read"},
			"admin": {"games:read", "scores:write", "admin:*"},
		},
	}
	stored := []string{"games:read", "scores:write", "admin:*"}

	if got, want := effectiveKeyPermissions(Principal{Role: "user"}, stored, policy), []string{"games:read"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("user effective permissions = %v, want %v", got, want)
	}

	policy.AvailablePermissions = []string{"games:read", "scores:write"}
	if got, want := effectiveKeyPermissions(Principal{Role: "admin"}, stored, policy), []string{"games:read", "scores:write"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("admin effective permissions after global removal = %v, want %v", got, want)
	}
}

func TestPolicyDecodeCannotMutateCanonicalAllowlist(t *testing.T) {
	want := append([]string(nil), allowedAPIKeyPermissions...)
	policy := apiKeyPolicy{AvailablePermissions: append([]string(nil), allowedAPIKeyPermissions...)}
	if err := json.Unmarshal([]byte(`{"available_permissions":["api:access"]}`), &policy); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allowedAPIKeyPermissions, want) {
		t.Fatalf("canonical permission allowlist mutated: got %v, want %v", allowedAPIKeyPermissions, want)
	}
}

func TestAPIKeyProfileReadCannotMutateProfileState(t *testing.T) {
	request := func(method, path string, permissions ...string) int {
		r := httptest.NewRequest(method, path, nil)
		principal := Principal{Role: "user", AuthType: "api_key", Permissions: permissions}
		r = r.WithContext(context.WithValue(r.Context(), principalKey, principal))
		w := httptest.NewRecorder()
		(&Server{}).enforceAPIKeyPermissions(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(w, r)
		return w.Code
	}

	if got := request(http.MethodGet, "/api/v1/realmguard/progress", "profile:read"); got != http.StatusNoContent {
		t.Fatalf("profile read GET status = %d", got)
	}
	for _, target := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/v1/realmguard/progress"},
		{http.MethodPatch, "/api/v1/me"},
		{http.MethodPut, "/api/v1/me/preferences"},
	} {
		if got := request(target.method, target.path, "profile:read"); got != http.StatusForbidden {
			t.Fatalf("profile:read mutation %s %s status = %d", target.method, target.path, got)
		}
		if got := request(target.method, target.path, "profile:write"); got != http.StatusNoContent {
			t.Fatalf("profile:write mutation %s %s status = %d", target.method, target.path, got)
		}
	}
}
