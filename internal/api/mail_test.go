package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/igame/internal/mail"
	"github.com/hkjang/igame/internal/secretbox"
)

// mailServer is a server with the mail setting seeded in the cache and a
// working secret box, so the handlers run up to the point where they would
// need the database.
func mailServer(t *testing.T, raw string) *Server {
	t.Helper()
	s := storedSetting(t, "mail", raw)
	box, err := secretbox.New([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	s.Secrets = box
	s.mailer = mail.NewService(nil, s.mailConfig, nil, nil)
	return s
}

func asAdmin(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey, Principal{UserID: uuid.New(), Username: "admin", Email: "admin@example.test", Role: "admin", AuthType: "session"}))
}

// The password is the one value the setting holds that must never come back.
// The list endpoint blanks it by name; this endpoint is the other way in.
func TestMailSettingIsReturnedWithoutThePassword(t *testing.T) {
	s := mailServer(t, `{"enabled":true,"smtp_host":"relay.corp","smtp_port":25,"username":"igame","password":"sealed-value","from_address":"igame@corp.example"}`)
	w := httptest.NewRecorder()
	s.getMailSetting(w, asAdmin(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/mail", nil)))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sealed-value") {
		t.Fatalf("the password came back: %s", w.Body.String())
	}
	var out struct {
		Setting            mail.Config `json:"setting"`
		PasswordConfigured bool        `json:"password_configured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.PasswordConfigured || out.Setting.Password != "" || out.Setting.SMTPHost != "relay.corp" || out.Setting.Security != "auto" || out.Setting.Timeout != 10 {
		t.Fatalf("unexpected setting: %+v configured=%v", out.Setting, out.PasswordConfigured)
	}
}

// A deployment that never saved the setting has no row, and reads as off.
func TestMailSettingDefaultsToOffWhenNeverSaved(t *testing.T) {
	s := mailServer(t, "")
	config, err := s.mailConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.SMTPPort != 25 || config.Security != "auto" {
		t.Fatalf("missing setting must be the disabled default, got %+v", config)
	}
	w := httptest.NewRecorder()
	s.getMailSetting(w, asAdmin(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/mail", nil)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestSavingMailRefusesWhatTheRelayWouldRefuse(t *testing.T) {
	cases := map[string]string{
		"enabled without a host":  `{"enabled":true,"from_address":"igame@corp.example"}`,
		"enabled without a from":  `{"enabled":true,"smtp_host":"relay.corp"}`,
		"unknown security":        `{"smtp_host":"relay.corp","security":"ssl"}`,
		"port out of range":       `{"smtp_host":"relay.corp","smtp_port":70000}`,
		"relative base url":       `{"base_url":"igame.corp"}`,
		"unknown field":           `{"smtp_host":"relay.corp","notify_everything":true}`,
		"from is not an address":  `{"from_address":"igame"}`,
		"timeout beyond the wait": `{"timeout_seconds":900}`,
	}
	for name, body := range cases {
		s := mailServer(t, "")
		code, errorCode, reached := settingWriteVerdict(s.putMailSetting, "/api/v1/admin/settings/mail", body)
		if reached || code != 400 {
			t.Errorf("%s: status %d code %s reached=%v", name, code, errorCode, reached)
		}
	}
}

// Same rule as the OIDC secret and the AI key: a blank password on save means
// "keep the stored one", so a stored value that cannot be read must stop the
// write rather than become an empty password.
func TestSavingMailRefusesWhenTheStoredPasswordCannotBeRead(t *testing.T) {
	s := mailServer(t, `{"smtp_host":`)
	code, errorCode, reached := settingWriteVerdict(s.putMailSetting, "/api/v1/admin/settings/mail", `{"enabled":true,"smtp_host":"relay.corp","username":"igame","from_address":"igame@corp.example"}`)
	if reached || code != 503 || errorCode != "mail_setting_unavailable" {
		t.Fatalf("status %d code %s reached=%v", code, errorCode, reached)
	}
	// A readable setting lets the same save through to the database.
	s = mailServer(t, `{"smtp_host":"old.corp","username":"igame","password":"sealed"}`)
	_, _, reached = settingWriteVerdict(s.putMailSetting, "/api/v1/admin/settings/mail", `{"enabled":true,"smtp_host":"relay.corp","username":"igame","from_address":"igame@corp.example"}`)
	if !reached {
		t.Fatal("a valid save with a readable stored setting must go on to write")
	}
}

// The test button sends with the saved setting, so it cannot prove anything
// while mail is off — and says so rather than sending nothing with a 200.
func TestSendingATestMailIsRefusedWhileMailIsOff(t *testing.T) {
	s := mailServer(t, `{"enabled":false,"smtp_host":"relay.corp","from_address":"igame@corp.example"}`)
	w := httptest.NewRecorder()
	s.sendTestMail(w, asAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/admin/mail/test", strings.NewReader(`{"recipient":"kim@corp.example"}`))))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "mail_disabled") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.sendTestMail(w, asAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/admin/mail/test", strings.NewReader(`{"recipient":"not-an-address"}`))))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_recipient") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

// A dead relay is a failed delivery, not a failed request: the reviewer's
// approval must return while the relay is still refusing connections.
func TestNotifyMailDoesNotFailTheRequestWhenTheRelayIsDown(t *testing.T) {
	s := mailServer(t, `{"enabled":true,"smtp_host":"127.0.0.1","smtp_port":1,"security":"none","from_address":"igame@corp.example","timeout_seconds":1}`)
	recipient := uuid.New()
	s.mailer = mail.NewService(nil, s.mailConfig, func(context.Context, []uuid.UUID) (map[uuid.UUID]string, error) {
		return map[uuid.UUID]string{recipient: "kim@corp.example"}, nil
	}, nil)
	r := asAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/workflow/reviews/x", nil))
	s.notifyMail(r, mail.ApprovalDecided("팀장", "게임 등록", "Snake", "approved", "", "workflow_request", "1", "/reviews"), []uuid.UUID{recipient})
	// Returning at all is the assertion; Wait only keeps the goroutine from
	// outliving the test.
	s.mailer.Wait()
}

func TestMailRoutesAreRegisteredForAdministrators(t *testing.T) {
	registered := map[string]bool{}
	_ = chi.Walk(New(nil, nil, nil).Router().(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered[method+" "+route] = true
		return nil
	})
	for _, want := range []string{"GET /api/v1/admin/mail/deliveries", "POST /api/v1/admin/mail/test"} {
		if !registered[want] {
			t.Fatalf("%s is not registered", want)
		}
	}
}
