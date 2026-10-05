package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hkjang/igame/internal/secretbox"
)

// The OIDC and AI settings are saved with a bare `UPDATE system_settings SET
// ... WHERE key=...`. An UPDATE that matches no row is not an error: Exec
// returns a nil error and nought rows affected, so both handlers carry on and
// answer 200 with the setting the caller sent, having written nothing. The OIDC
// handler also records an `oidc.update` audit entry naming the groups it just
// granted administrator — a change that never happened.
//
// Both handlers already allow for the row being absent: each reads the current
// value immediately beforehand and lets pgx.ErrNoRows through deliberately, so
// the code assumes exactly the state in which its own write is discarded. The
// general setting save in the same file (putSetting) has always written with
// `INSERT ... ON CONFLICT(key) DO UPDATE`, which leaves two write contracts on
// one table.
//
// Reachability, stated plainly: migrations/001_initial.sql seeds the `oidc` and
// `ai` rows, and no product code path deletes from system_settings, so a
// correctly migrated installation has the rows and this is not an outage. These
// tests pin the contract that a successful save has actually saved, and that an
// audit entry describes a change that took place.
//
// They drive the real router against a real database with a real administrator
// session, and are skipped when IGAME_TEST_DSN is unset; `make test-db DSN=...`
// runs them.

type settingWriteFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	admin  uuid.UUID
	cookie string
}

func newSettingWriteFixture(t *testing.T) settingWriteFixture {
	t.Helper()
	pool := migratedPool(t)
	secrets, err := secretbox.New(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin := insertTestUser(t, pool, "admin")
	f := settingWriteFixture{pool: pool, admin: admin, cookie: insertTestSession(t, pool, admin)}
	f.server = httptest.NewServer(New(pool, secrets, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// call sends a request as the signed-in administrator and returns the status
// and the decoded body.
func (f settingWriteFixture) call(t *testing.T, method, path, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	request, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var out map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
		t.Fatalf("%s %s: decode response: %v", method, path, err)
	}
	return response.StatusCode, out
}

// storedRow reads what the database actually holds for a settings key.
func (f settingWriteFixture) storedRow(t *testing.T, key string) (value map[string]any, secret bool, updatedBy uuid.UUID, found bool) {
	t.Helper()
	var raw []byte
	var owner *uuid.UUID
	err := f.pool.QueryRow(context.Background(), `SELECT value,secret,updated_by FROM system_settings WHERE key=$1`, key).Scan(&raw, &secret, &owner)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, false, uuid.Nil, false
		}
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("stored %s value is not an object: %v", key, err)
	}
	if owner != nil {
		updatedBy = *owner
	}
	return value, secret, updatedBy, true
}

func (f settingWriteFixture) deleteRow(t *testing.T, key string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM system_settings WHERE key=$1`, key); err != nil {
		t.Fatal(err)
	}
}

// auditRows returns the detail of every audit entry for an action, newest last.
func (f settingWriteFixture) auditRows(t *testing.T, action string) []map[string]any {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT detail FROM audit_logs WHERE action=$1 AND actor_id=$2 ORDER BY created_at`, action, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var detail map[string]any
		if err := json.Unmarshal(raw, &detail); err != nil {
			t.Fatalf("audit detail for %s is not an object: %v", action, err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return details
}

const aiSettingBody = `{"enabled":true,"base_url":"https://ai.example.com","default_model":"local-model","api_key":"first-key","max_tokens":2048,"timeout_seconds":45}`

const oidcSettingBody = `{"enabled":true,"issuer":"https://idp.example.com/","client_id":"portal","client_secret":"first-secret","admin_groups":["platform-admins"]}`

func TestPutAISettingStoresTheSettingWhenNoRowExists(t *testing.T) {
	f := newSettingWriteFixture(t)
	f.deleteRow(t, "ai")

	status, body := f.call(t, http.MethodPut, "/api/v1/admin/settings/ai", aiSettingBody)
	if status != http.StatusOK {
		t.Fatalf("PUT ai with no row: status %d body %v, want 200", status, body)
	}

	value, secret, updatedBy, found := f.storedRow(t, "ai")
	if !found {
		t.Fatalf("PUT ai answered 200 and stored nothing; the setting the caller saved is gone")
	}
	if value["enabled"] != true || value["base_url"] != "https://ai.example.com" || value["default_model"] != "local-model" {
		t.Errorf("stored ai setting = %v, want the submitted enabled/base_url/default_model", value)
	}
	if value["max_tokens"] != float64(2048) || value["timeout_seconds"] != float64(45) {
		t.Errorf("stored ai limits = %v/%v, want 2048/45", value["max_tokens"], value["timeout_seconds"])
	}
	if !secret {
		t.Errorf("stored ai row has secret=false; the row holds a sealed API key")
	}
	if updatedBy != f.admin {
		t.Errorf("stored ai updated_by = %s, want the calling administrator %s", updatedBy, f.admin)
	}
	sealed, _ := value["api_key"].(string)
	if !strings.HasPrefix(sealed, "v1:") {
		t.Errorf("stored ai api_key = %q, want a sealed v1: value", sealed)
	}
}

func TestPutOIDCSettingStoresTheSettingWhenNoRowExists(t *testing.T) {
	f := newSettingWriteFixture(t)
	f.deleteRow(t, "oidc")

	status, body := f.call(t, http.MethodPut, "/api/v1/admin/settings/oidc", oidcSettingBody)
	if status != http.StatusOK {
		t.Fatalf("PUT oidc with no row: status %d body %v, want 200", status, body)
	}

	value, secret, updatedBy, found := f.storedRow(t, "oidc")
	// A successful save and an audit entry must agree about what happened. An
	// `oidc.update` row recording which group was granted administrator, beside
	// a settings table that was never written, is a false audit record.
	audits := f.auditRows(t, "oidc.update")
	if !found {
		t.Fatalf("PUT oidc answered 200 and stored nothing, yet left %d oidc.update audit entries: %v", len(audits), audits)
	}
	if len(audits) != 1 {
		t.Errorf("oidc.update audit entries = %d, want exactly 1 for one successful save", len(audits))
	}
	if value["enabled"] != true || value["issuer"] != "https://idp.example.com" || value["client_id"] != "portal" {
		t.Errorf("stored oidc setting = %v, want the submitted enabled/issuer/client_id", value)
	}
	if groups, ok := value["admin_groups"].([]any); !ok || len(groups) != 1 || groups[0] != "platform-admins" {
		t.Errorf("stored oidc admin_groups = %v, want [platform-admins]", value["admin_groups"])
	}
	if !secret {
		t.Errorf("stored oidc row has secret=false; the row holds a sealed client secret")
	}
	if updatedBy != f.admin {
		t.Errorf("stored oidc updated_by = %s, want the calling administrator %s", updatedBy, f.admin)
	}

	// The settings screen reads the value back through the cache the write just
	// dropped, so what an administrator sees next must be what they sent.
	status, read := f.call(t, http.MethodGet, "/api/v1/admin/settings/oidc", "")
	if status != http.StatusOK {
		t.Fatalf("GET oidc after the save: status %d, want 200", status)
	}
	var out struct {
		Enabled  bool   `json:"enabled"`
		Issuer   string `json:"issuer"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(read["setting"], &out); err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || out.Issuer != "https://idp.example.com" || out.ClientID != "portal" {
		t.Errorf("GET oidc read back %+v, want the setting just saved", out)
	}
	var configured bool
	if err := json.Unmarshal(read["client_secret_configured"], &configured); err != nil {
		t.Fatal(err)
	}
	if !configured {
		t.Error("GET oidc reports no client secret after one was saved")
	}
}

// With the row present the handlers already worked, and the change to how they
// write must not alter any of it: the existing value is replaced, a submission
// without a secret keeps the stored one byte for byte rather than sealing
// afresh, the row stays marked secret, and the audit entries keep the fields
// they had.
func TestPutSettingsKeepTheirBehaviourWhenTheRowExists(t *testing.T) {
	f := newSettingWriteFixture(t)

	t.Run("ai", func(t *testing.T) {
		if _, _, _, found := f.storedRow(t, "ai"); !found {
			t.Fatal("the migration no longer seeds the ai row; this test checks the row-present path")
		}
		if status, body := f.call(t, http.MethodPut, "/api/v1/admin/settings/ai", aiSettingBody); status != http.StatusOK {
			t.Fatalf("PUT ai: status %d body %v, want 200", status, body)
		}
		value, secret, updatedBy, _ := f.storedRow(t, "ai")
		firstSealed, _ := value["api_key"].(string)
		if !secret || updatedBy != f.admin || value["base_url"] != "https://ai.example.com" {
			t.Fatalf("first ai save: value %v secret %v updated_by %s", value, secret, updatedBy)
		}
		if firstSealed == "" || firstSealed == "first-key" {
			t.Fatalf("ai api_key was stored as %q, want it sealed", firstSealed)
		}

		// Every ordinary save from the settings screen arrives without the key.
		for _, submitted := range []string{``, `"********"`} {
			body := `{"enabled":true,"base_url":"https://ai.example.com/v2","default_model":"local-model","max_tokens":2048,"timeout_seconds":45}`
			if submitted != "" {
				body = `{"enabled":true,"base_url":"https://ai.example.com/v2","default_model":"local-model","api_key":` + submitted + `,"max_tokens":2048,"timeout_seconds":45}`
			}
			status, out := f.call(t, http.MethodPut, "/api/v1/admin/settings/ai", body)
			if status != http.StatusOK {
				t.Fatalf("PUT ai with api_key %s: status %d body %v, want 200", submitted, status, out)
			}
			var configured bool
			if err := json.Unmarshal(out["api_key_configured"], &configured); err != nil {
				t.Fatal(err)
			}
			if !configured {
				t.Errorf("PUT ai with api_key %s reports no key configured", submitted)
			}
			value, secret, _, _ := f.storedRow(t, "ai")
			if got, _ := value["api_key"].(string); got != firstSealed {
				t.Errorf("PUT ai with api_key %s rewrote the stored key: %q, want the carried-over %q", submitted, got, firstSealed)
			}
			if !secret {
				t.Errorf("PUT ai with api_key %s cleared secret on the row", submitted)
			}
			if value["base_url"] != "https://ai.example.com/v2" {
				t.Errorf("PUT ai with api_key %s did not update base_url: %v", submitted, value["base_url"])
			}
		}

		// The entry names what changed and never the key itself.
		for _, detail := range f.auditRows(t, "ai.update") {
			if len(detail) != 3 {
				t.Errorf("ai.update detail = %v, want exactly enabled/base_url/model", detail)
			}
			if _, ok := detail["enabled"]; !ok {
				t.Errorf("ai.update detail lost enabled: %v", detail)
			}
			if _, ok := detail["base_url"]; !ok {
				t.Errorf("ai.update detail lost base_url: %v", detail)
			}
			if detail["model"] != "local-model" {
				t.Errorf("ai.update detail model = %v, want local-model", detail["model"])
			}
		}
	})

	t.Run("oidc", func(t *testing.T) {
		if _, _, _, found := f.storedRow(t, "oidc"); !found {
			t.Fatal("the migration no longer seeds the oidc row; this test checks the row-present path")
		}
		if status, body := f.call(t, http.MethodPut, "/api/v1/admin/settings/oidc", oidcSettingBody); status != http.StatusOK {
			t.Fatalf("PUT oidc: status %d body %v, want 200", status, body)
		}
		value, secret, updatedBy, _ := f.storedRow(t, "oidc")
		firstSealed, _ := value["client_secret"].(string)
		if !secret || updatedBy != f.admin || value["issuer"] != "https://idp.example.com" {
			t.Fatalf("first oidc save: value %v secret %v updated_by %s", value, secret, updatedBy)
		}
		if firstSealed == "" || firstSealed == "first-secret" {
			t.Fatalf("oidc client_secret was stored as %q, want it sealed", firstSealed)
		}

		for _, submitted := range []string{``, `"********"`} {
			body := `{"enabled":true,"issuer":"https://idp.example.com","client_id":"portal-v2","admin_groups":["platform-admins"]}`
			if submitted != "" {
				body = `{"enabled":true,"issuer":"https://idp.example.com","client_id":"portal-v2","client_secret":` + submitted + `,"admin_groups":["platform-admins"]}`
			}
			status, out := f.call(t, http.MethodPut, "/api/v1/admin/settings/oidc", body)
			if status != http.StatusOK {
				t.Fatalf("PUT oidc with client_secret %s: status %d body %v, want 200", submitted, status, out)
			}
			var configured bool
			if err := json.Unmarshal(out["client_secret_configured"], &configured); err != nil {
				t.Fatal(err)
			}
			if !configured {
				t.Errorf("PUT oidc with client_secret %s reports no secret configured", submitted)
			}
			value, secret, _, _ := f.storedRow(t, "oidc")
			if got, _ := value["client_secret"].(string); got != firstSealed {
				t.Errorf("PUT oidc with client_secret %s resealed the stored secret: %q, want the carried-over %q", submitted, got, firstSealed)
			}
			if !secret {
				t.Errorf("PUT oidc with client_secret %s cleared secret on the row", submitted)
			}
			if value["client_id"] != "portal-v2" {
				t.Errorf("PUT oidc with client_secret %s did not update client_id: %v", submitted, value["client_id"])
			}
		}

		// auditSettingChange reports a replaced secret as "replaced" and never
		// copies it, and the group grant that decides who becomes an
		// administrator has to be visible.
		audits := f.auditRows(t, "oidc.update")
		if len(audits) == 0 {
			t.Fatal("no oidc.update audit entries for three saves")
		}
		first := audits[0]
		if first["client_secret"] != "replaced" {
			t.Errorf("oidc.update detail client_secret = %v, want \"replaced\"", first["client_secret"])
		}
		if _, ok := first["admin_groups"]; !ok {
			t.Errorf("oidc.update detail lost admin_groups: %v", first)
		}
		for _, detail := range audits {
			if strings.Contains(strings.ToLower(string(mustJSON(t, detail))), "first-secret") {
				t.Errorf("oidc.update detail carries the client secret: %v", detail)
			}
		}
	})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
