package api

import (
	"reflect"
	"testing"
)

// Granting admin is the most consequential thing the user endpoint does, and the
// audit entry used to record only the value it was set to — which cannot tell a
// promotion from a request that set the role the account already had.
func TestAuditUserChangeRecordsWhatMoved(t *testing.T) {
	for _, item := range []struct {
		name string
		got  map[string]any
		want map[string]any
	}{
		{
			"a promotion says where it came from",
			auditUserChange("user", "admin", "active", "active", false),
			map[string]any{"role": map[string]string{"from": "user", "to": "admin"}},
		},
		{
			"setting the role it already had is not a change",
			auditUserChange("admin", "admin", "active", "active", false),
			map[string]any{"changed": "profile fields only"},
		},
		{
			"locking an account is recorded",
			auditUserChange("user", "user", "active", "disabled", false),
			map[string]any{"status": map[string]string{"from": "active", "to": "disabled"}},
		},
		{
			// It lets the operator sign in as that person, and it left no trace.
			"a password reset is recorded on its own",
			auditUserChange("user", "user", "active", "active", true),
			map[string]any{"password_reset": true},
		},
		{
			"a demotion and a lock in one request record both",
			auditUserChange("admin", "user", "active", "disabled", true),
			map[string]any{
				"role":           map[string]string{"from": "admin", "to": "user"},
				"status":         map[string]string{"from": "active", "to": "disabled"},
				"password_reset": true,
			},
		},
	} {
		t.Run(item.name, func(t *testing.T) {
			if !reflect.DeepEqual(item.got, item.want) {
				t.Fatalf("audit detail is %v, want %v", item.got, item.want)
			}
		})
	}
}
