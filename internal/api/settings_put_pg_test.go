package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// system_settings.value is jsonb and every editable setting is an object that
// validateSetting reads into a struct. Unmarshalling the JSON literal `null`
// into a struct is a no-op that reports no error, so `{"value": null}` passes
// every key's validation and stores jsonb `null` — the setting then reads back
// as the zero value of its struct, which is how `play_policy` or `privacy`
// silently reverts to defaults. Whether the write lands is PostgreSQL's
// business, so these tests drive the real router against a real database. They
// are skipped when IGAME_TEST_DSN is unset; `make test-db DSN=...` runs them.

type settingFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	cookie string
}

func newSettingFixture(t *testing.T) settingFixture {
	t.Helper()
	pool := migratedPool(t)
	f := settingFixture{pool: pool}

	admin := insertTestUser(t, pool, "admin")
	f.cookie = insertTestSession(t, pool, admin)

	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// preserve remembers a setting the test is about to write and puts the stored
// value back afterwards. Setting keys are global and the API fixture shares the
// default schema, so a test that edits `service` edits it for every other test
// in the package.
func (f settingFixture) preserve(t *testing.T, key string) {
	t.Helper()
	value, at, ok := f.row(t, key)
	t.Cleanup(func() {
		ctx := context.Background()
		if !ok {
			_, _ = f.pool.Exec(ctx, `DELETE FROM system_settings WHERE key=$1`, key)
			return
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO system_settings(key,value,updated_at) VALUES($1,$2::jsonb,$3)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_by=NULL,updated_at=excluded.updated_at`, key, value, at); err != nil {
			t.Errorf("restore setting %s: %v", key, err)
		}
	})
}

// row reports what the column actually holds, which is the thing a `null` write
// destroys.
func (f settingFixture) row(t *testing.T, key string) (value string, at time.Time, ok bool) {
	t.Helper()
	err := f.pool.QueryRow(context.Background(), `SELECT value::text,updated_at FROM system_settings WHERE key=$1`, key).Scan(&value, &at)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", time.Time{}, false
		}
		t.Fatal(err)
	}
	return value, at, true
}

func (f settingFixture) jsonType(t *testing.T, key string) string {
	t.Helper()
	var kind string
	if err := f.pool.QueryRow(context.Background(), `SELECT jsonb_typeof(value) FROM system_settings WHERE key=$1`, key).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	return kind
}

func (f settingFixture) auditCount(t *testing.T, key string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='setting.update' AND resource_id=$1`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// put sends raw JSON so the tests can send `value` shapes Go's typed encoder
// would never produce for a struct field.
func (f settingFixture) put(t *testing.T, key, body string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPut, f.server.URL+"/api/v1/admin/settings/"+key, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil && err != io.EOF {
		t.Fatalf("PUT %s: %v", key, err)
	}
	return response.StatusCode, out
}

func TestPutSettingRejectsNonObjectValue(t *testing.T) {
	f := newSettingFixture(t)

	for _, key := range []string{"service", "play_policy"} {
		for _, value := range []string{`null`, `[]`, `[{"timezone":"UTC"}]`, `5`, `"UTC"`, `true`} {
			t.Run(key+"/"+value, func(t *testing.T) {
				f.preserve(t, key)
				before, beforeAt, ok := f.row(t, key)
				if !ok {
					t.Fatalf("setting %s is missing; the seed should have created it", key)
				}
				audits := f.auditCount(t, key)

				status, out := f.put(t, key, `{"value":`+value+`}`)
				// Read the row either way: when the request is wrongly
				// accepted, the row is the evidence of what it did.
				after, afterAt, _ := f.row(t, key)
				if status != http.StatusBadRequest {
					t.Fatalf("%s value %s: status %d, want 400; row is now %s (%s)", key, value, status, after, f.jsonType(t, key))
				}
				envelope, _ := out["error"].(map[string]any)
				if code, _ := envelope["code"].(string); code != "invalid_setting" {
					t.Fatalf("%s value %s: error %v, want code invalid_setting", key, value, out)
				}
				// A rejected write must leave the stored setting, its
				// timestamp and the audit trail exactly as they were.
				if after != before || !afterAt.Equal(beforeAt) {
					t.Fatalf("%s value %s: row changed from %s (%s) to %s (%s)", key, value, before, beforeAt, after, afterAt)
				}
				if got := f.auditCount(t, key); got != audits {
					t.Fatalf("%s value %s: audit entries for setting.update went from %d to %d; a refused write must not be audited", key, value, audits, got)
				}
			})
		}
	}
}

func TestPutSettingAcceptsObjectValue(t *testing.T) {
	f := newSettingFixture(t)

	t.Run("service object is stored and audited", func(t *testing.T) {
		f.preserve(t, "service")
		before, beforeAt, _ := f.row(t, "service")
		audits := f.auditCount(t, "service")

		status, out := f.put(t, "service", `{"value":{"display_name":"Settings PUT test","timezone":"UTC"}}`)
		if status != http.StatusOK {
			t.Fatalf("status %d %v, want 200", status, out)
		}
		after, afterAt, _ := f.row(t, "service")
		if after == before || !afterAt.After(beforeAt) {
			t.Fatalf("row is %s at %s, want the new value with a newer timestamp (was %s at %s)", after, afterAt, before, beforeAt)
		}
		if kind := f.jsonType(t, "service"); kind != "object" {
			t.Fatalf("stored value is %s, want object", kind)
		}
		// The value has to be readable back through the setting cache the
		// write invalidates, not just present in the column.
		var stored struct {
			DisplayName string `json:"display_name"`
			Timezone    string `json:"timezone"`
		}
		if err := json.Unmarshal([]byte(after), &stored); err != nil {
			t.Fatal(err)
		}
		if stored.DisplayName != "Settings PUT test" || stored.Timezone != "UTC" {
			t.Fatalf("stored value is %+v, want the posted display_name and timezone", stored)
		}
		if got := f.auditCount(t, "service"); got != audits+1 {
			t.Fatalf("audit entries for setting.update went from %d to %d, want one more", audits, got)
		}
	})

	t.Run("empty object is still a valid setting", func(t *testing.T) {
		f.preserve(t, "play_policy")
		status, out := f.put(t, "play_policy", `{"value":{}}`)
		if status != http.StatusOK {
			t.Fatalf("status %d %v, want 200", status, out)
		}
		if kind := f.jsonType(t, "play_policy"); kind != "object" {
			t.Fatalf("stored value is %s, want object", kind)
		}
	})

	// The invalidated cache must serve the new value, which is what makes a
	// silent revert to defaults observable to a caller.
	t.Run("stored timezone is served back", func(t *testing.T) {
		f.preserve(t, "service")
		if status, out := f.put(t, "service", `{"value":{"display_name":"Settings PUT test","timezone":"UTC"}}`); status != http.StatusOK {
			t.Fatalf("status %d %v, want 200", status, out)
		}
		request, err := http.NewRequest(http.MethodGet, f.server.URL+"/api/v1/admin/settings/service", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var out struct {
			Value struct {
				Timezone string `json:"timezone"`
			} `json:"value"`
		}
		if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || out.Value.Timezone != "UTC" {
			t.Fatalf("GET returned %d with timezone %q, want 200 and UTC", response.StatusCode, out.Value.Timezone)
		}
	})
}
