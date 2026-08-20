package api

import (
	"encoding/json"
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
