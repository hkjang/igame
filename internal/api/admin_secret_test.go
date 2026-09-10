package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// storedSetting seeds the settings cache so the two secret-bearing PUT handlers
// run without a database. A raw value of "" stands for a key that has never
// been written; anything that is not JSON stands for a value the server cannot
// read right now, which is what a transient failure looks like to the caller.
func storedSetting(t *testing.T, key, raw string) *Server {
	t.Helper()
	s := &Server{}
	entry := settingEntry{raw: []byte(raw), expires: time.Now().Add(time.Hour)}
	if raw == "" {
		entry = settingEntry{missing: true, expires: time.Now().Add(time.Hour)}
	}
	s.storeSetting(key, entry)
	return s
}

// settingWriteVerdict submits a setting to handler on a server with no database.
// Reading the current value comes before the write, so a refused submission
// comes back with a status while an accepted one reaches the nil pool and
// panics — which is the evidence that it went on to store the value.
func settingWriteVerdict(handler func(http.ResponseWriter, *http.Request), path, body string) (code int, errorCode string, reachedDatabase bool) {
	defer func() {
		if recover() != nil {
			reachedDatabase = true
		}
	}()
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))
	var parsed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w.Code, parsed.Error.Code, false
}

// The settings screen is never given the stored client secret, so it submits an
// empty one and the handler carries the old value forward. That made the read
// of the old value load-bearing: when it failed, "no secret" and "the secret
// could not be read" were the same zero value, and the save wrote an empty
// client_secret over a working one — from a database blip, with a 200 and an
// audit entry saying the configuration was updated.
func TestSavingOIDCRefusesWhenTheStoredSecretCannotBeRead(t *testing.T) {
	s := storedSetting(t, "oidc", `{"issuer":`)

	code, errorCode, reached := settingWriteVerdict(s.putOIDCSetting, "/api/v1/admin/settings/oidc",
		`{"enabled":true,"issuer":"https://sso.example.com","client_id":"igame","client_secret":""}`)
	if reached {
		t.Fatal("the OIDC setting was written without reading the secret it carries forward")
	}
	if code != http.StatusServiceUnavailable {
		t.Fatalf("returned %d, want 503", code)
	}
	if errorCode != "oidc_setting_unavailable" {
		t.Fatalf("returned code %s, want oidc_setting_unavailable", errorCode)
	}
}

func TestSavingAIRefusesWhenTheStoredKeyCannotBeRead(t *testing.T) {
	s := storedSetting(t, "ai", `{"enabled":`)

	code, errorCode, reached := settingWriteVerdict(s.putAISetting, "/api/v1/admin/settings/ai",
		`{"enabled":true,"base_url":"https://api.example.com","default_model":"m","api_key":""}`)
	if reached {
		t.Fatal("the AI setting was written without reading the key it carries forward")
	}
	if code != http.StatusServiceUnavailable {
		t.Fatalf("returned %d, want 503", code)
	}
	if errorCode != "ai_setting_unavailable" {
		t.Fatalf("returned code %s, want ai_setting_unavailable", errorCode)
	}
}

// A key that was never written has no secret to lose, so the first save must
// still go through.
func TestSavingASettingThatWasNeverWrittenStillProceeds(t *testing.T) {
	for _, tc := range []struct {
		key     string
		handler func(*Server) func(http.ResponseWriter, *http.Request)
		path    string
		body    string
	}{
		{"oidc", func(s *Server) func(http.ResponseWriter, *http.Request) { return s.putOIDCSetting },
			"/api/v1/admin/settings/oidc",
			`{"enabled":true,"issuer":"https://sso.example.com","client_id":"igame","client_secret":""}`},
		{"ai", func(s *Server) func(http.ResponseWriter, *http.Request) { return s.putAISetting },
			"/api/v1/admin/settings/ai",
			`{"enabled":true,"base_url":"https://api.example.com","default_model":"m","api_key":""}`},
	} {
		s := storedSetting(t, tc.key, "")
		code, errorCode, reached := settingWriteVerdict(tc.handler(s), tc.path, tc.body)
		if !reached {
			t.Fatalf("%s: a first save was refused with %d/%s", tc.key, code, errorCode)
		}
	}
}
