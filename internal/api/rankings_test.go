package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// privacySetting seeds the settings cache so the rankings handler runs without
// a database. An empty raw value stands for a policy that cannot be read.
func privacySetting(t *testing.T, raw string) *Server {
	t.Helper()
	s := &Server{}
	entry := settingEntry{raw: []byte(raw), expires: time.Now().Add(time.Hour)}
	if raw == "" {
		entry = settingEntry{missing: true, expires: time.Now().Add(time.Hour)}
	}
	s.storeSetting("privacy", entry)
	return s
}

// rankingsVerdict runs the handler on a server with no database. The privacy
// gate has to decide before anything is aggregated, so a refused request comes
// back with a status while an allowed one reaches the nil pool and panics —
// which is exactly the evidence that the gate let it through.
func rankingsVerdict(s *Server, target string) (code int, errorCode string, reachedDatabase bool) {
	defer func() {
		if recover() != nil {
			reachedDatabase = true
		}
	}()
	w := httptest.NewRecorder()
	s.rankings(w, httptest.NewRequest(http.MethodGet, target, nil))
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body.Error.Code, false
}

func TestGroupedRankingsRefusedWhenOrganizationNamesAreHidden(t *testing.T) {
	// show_department off means the organization is not public. Aggregating by
	// department or team would hand back the very names it hides, together with
	// each group's head count and total score.
	s := privacySetting(t, `{"ranking_name":"nickname","show_department":false}`)

	for _, group := range []string{"department", "team"} {
		code, errorCode, reached := rankingsVerdict(s, "/api/v1/rankings?game_id=snake&group="+group)
		if reached {
			t.Fatalf("group=%s queried the database before consulting the privacy policy", group)
		}
		if code != http.StatusForbidden {
			t.Fatalf("group=%s returned %d, want 403", group, code)
		}
		if errorCode != "organization_ranking_hidden" {
			t.Fatalf("group=%s returned code %s, want organization_ranking_hidden", group, errorCode)
		}
	}
}

func TestGroupedRankingsAllowedWhenOrganizationNamesArePublic(t *testing.T) {
	s := privacySetting(t, `{"ranking_name":"nickname","show_department":true}`)

	for _, group := range []string{"", "individual", "department", "team"} {
		code, _, reached := rankingsVerdict(s, "/api/v1/rankings?game_id=snake&group="+group)
		if !reached {
			t.Fatalf("group=%q was refused with %d before it could be aggregated", group, code)
		}
	}
}

func TestRankingsRefuseAnUnreadablePrivacyPolicy(t *testing.T) {
	// An absent or unreadable policy is not a public one: falling back to the
	// zero value would publish exactly what the setting exists to withhold.
	s := privacySetting(t, "")

	for _, group := range []string{"", "individual", "department", "team"} {
		code, errorCode, reached := rankingsVerdict(s, "/api/v1/rankings?game_id=snake&group="+group)
		if reached {
			t.Fatalf("group=%q was aggregated without a readable privacy policy", group)
		}
		if code != http.StatusServiceUnavailable {
			t.Fatalf("group=%q returned %d, want 503", group, code)
		}
		if errorCode != "privacy_setting_unavailable" {
			t.Fatalf("group=%q returned code %s, want privacy_setting_unavailable", group, errorCode)
		}
	}
}

func TestRankingsRejectAnUnknownGroupBeforeQuerying(t *testing.T) {
	s := privacySetting(t, `{"ranking_name":"nickname","show_department":true}`)

	code, errorCode, reached := rankingsVerdict(s, "/api/v1/rankings?game_id=snake&group=company")
	if reached {
		t.Fatal("an unknown group reached the database")
	}
	if code != http.StatusBadRequest || errorCode != "invalid_group" {
		t.Fatalf("group=company returned %d/%s, want 400/invalid_group", code, errorCode)
	}
}

func TestRankingsStillRequireAGame(t *testing.T) {
	s := privacySetting(t, `{"ranking_name":"nickname","show_department":true}`)

	code, errorCode, _ := rankingsVerdict(s, "/api/v1/rankings?group=department")
	if code != http.StatusBadRequest || errorCode != "game_required" {
		t.Fatalf("returned %d/%s, want 400/game_required", code, errorCode)
	}
}
