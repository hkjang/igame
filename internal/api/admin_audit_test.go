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

// These settings decide who may sign in and with what role, and whether changes
// need review at all. The audit row used to say only which setting was written.
func TestAuditSettingChangeNamesWhatMoved(t *testing.T) {
	before := []byte(`{"enabled":true,"admin_groups":["ops"],"client_secret":"old","issuer":"https://a"}`)
	after := []byte(`{"enabled":false,"admin_groups":["ops","everyone"],"client_secret":"new","issuer":"https://a"}`)
	changes := auditSettingChange(before, after)

	if _, untouched := changes["issuer"]; untouched {
		t.Fatal("a field that did not move should not be recorded")
	}
	enabled, ok := changes["enabled"].(map[string]any)
	if !ok || enabled["from"] != true || enabled["to"] != false {
		t.Fatalf("enabled recorded as %v, want true -> false", changes["enabled"])
	}
	if _, ok := changes["admin_groups"].(map[string]any); !ok {
		t.Fatalf("granting a group admin was not recorded: %v", changes["admin_groups"])
	}
	// The value must never reach a table that is exported to CSV.
	if changes["client_secret"] != "replaced" {
		t.Fatalf("client_secret recorded as %v, want the word replaced", changes["client_secret"])
	}
}

func TestAuditSettingChangeRecordsRemovalAndNoChange(t *testing.T) {
	removed := auditSettingChange([]byte(`{"issuer":"https://a","api_key":"k"}`), []byte(`{}`))
	issuer, ok := removed["issuer"].(map[string]any)
	if !ok || issuer["from"] != "https://a" || issuer["to"] != nil {
		t.Fatalf("a removed field recorded as %v", removed["issuer"])
	}
	if removed["api_key"] != "removed" {
		t.Fatalf("a removed secret recorded as %v, want the word removed", removed["api_key"])
	}
	same := auditSettingChange([]byte(`{"enabled":true}`), []byte(`{"enabled":true}`))
	if same["changed"] != "nothing" {
		t.Fatalf("an unchanged write recorded as %v", same)
	}
	notObject := auditSettingChange([]byte(`{}`), []byte(`"a string"`))
	if notObject["changed"] != "value is not an object" {
		t.Fatalf("a non-object value recorded as %v", notObject)
	}
}
