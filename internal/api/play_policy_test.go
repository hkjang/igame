package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// playPolicyServer seeds the settings cache so playAllowed runs without a
// database. A raw value of "" stands for a key that was never stored; a value
// that is not valid JSON stands for one that cannot be read right now, which is
// what a transient failure looks like to the caller.
func playPolicyServer(t *testing.T, policy, service string) *Server {
	t.Helper()
	s := &Server{Now: func() time.Time { return time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC) }} // 12:00 KST
	for key, raw := range map[string]string{"play_policy": policy, "service": service} {
		entry := settingEntry{raw: []byte(raw), expires: time.Now().Add(time.Hour)}
		if raw == "" {
			entry = settingEntry{missing: true, expires: time.Now().Add(time.Hour)}
		}
		s.storeSetting(key, entry)
	}
	return s
}

// playVerdict asks the policy about a game on a server with no database. Every
// read the policy needs is cached, so a decision that touches the pool is one
// the policy could not make on its own: the nil pool panics, and that panic is
// the evidence.
func playVerdict(s *Server, gameSlug string) (allowed bool, denial string, err error, reachedDatabase bool) {
	defer func() {
		if recover() != nil {
			reachedDatabase = true
		}
	}()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey, Principal{UserID: uuid.New(), Role: "user"}))
	allowed, denial, err = s.playAllowed(r, uuid.New(), gameSlug)
	return allowed, denial, err, false
}

func TestPlayPolicyRefusesWhenItCannotBeRead(t *testing.T) {
	// A policy that cannot be read is not a policy that is switched off. Reading
	// the failure as the zero value lifted the time windows and the daily limits
	// at once, for exactly as long as the database was unwell.
	s := playPolicyServer(t, `{"enabled":`, `{"timezone":"Asia/Seoul"}`)

	allowed, _, err, reached := playVerdict(s, "snake")
	if reached {
		t.Fatal("an unreadable policy queried the database")
	}
	if err == nil {
		t.Fatalf("an unreadable policy returned allowed=%v with no error", allowed)
	}
	if allowed {
		t.Fatal("an unreadable policy allowed play")
	}
}

func TestPlayPolicyRefusesWhenServiceSettingsCannotBeRead(t *testing.T) {
	// The windows and the day boundary are both read in the service time zone,
	// so an unreadable zone moves the hours the policy names.
	s := playPolicyServer(t, `{"enabled":true,"windows":[{"start":"09:00","end":"18:00"}],"daily_limits":{}}`, `{"timezone":`)

	allowed, _, err, reached := playVerdict(s, "snake")
	if reached {
		t.Fatal("unreadable service settings queried the database")
	}
	if err == nil || allowed {
		t.Fatalf("unreadable service settings returned allowed=%v err=%v, want a refusal", allowed, err)
	}
}

func TestPlayPolicyAllowsWhenNoneIsStored(t *testing.T) {
	// An absent key is a deployment that never configured a policy. That has to
	// keep meaning "no restriction", which is why the refusal above checks for
	// something other than a missing row.
	s := playPolicyServer(t, "", "")

	allowed, _, err, reached := playVerdict(s, "snake")
	if err != nil || !allowed {
		t.Fatalf("no stored policy returned allowed=%v err=%v, want allowed", allowed, err)
	}
	if reached {
		t.Fatal("no stored policy still queried the database")
	}
}

func TestPlayPolicyAllowsWhenDisabled(t *testing.T) {
	s := playPolicyServer(t, `{"enabled":false,"windows":[],"daily_limits":{"snake":1}}`, `{"timezone":"Asia/Seoul"}`)

	allowed, _, err, reached := playVerdict(s, "snake")
	if err != nil || !allowed {
		t.Fatalf("a disabled policy returned allowed=%v err=%v, want allowed", allowed, err)
	}
	if reached {
		t.Fatal("a disabled policy consulted the daily limit")
	}
}

func TestPlayPolicyDeniesOutsideTheWindowWithoutQuerying(t *testing.T) {
	s := playPolicyServer(t, `{"enabled":true,"windows":[{"start":"20:00","end":"21:00"}],"daily_limits":{"snake":60}}`, `{"timezone":"Asia/Seoul"}`)

	allowed, denial, err, reached := playVerdict(s, "snake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed || denial == "" {
		t.Fatalf("12:00 KST outside 20:00-21:00 returned allowed=%v denial=%q", allowed, denial)
	}
	if reached {
		t.Fatal("a request refused by the time window still queried the database")
	}
}

func TestPlayPolicyUsesTheSlugTheCallerAlreadyRead(t *testing.T) {
	// The slug used to come from a second lookup whose failure left the limit
	// keyed by "", which no policy names — so the limit vanished. It is now the
	// slug the session handler already read and checked.
	s := playPolicyServer(t, `{"enabled":true,"windows":[],"daily_limits":{"snake":60}}`, `{"timezone":"Asia/Seoul"}`)

	if _, _, _, reached := playVerdict(s, "snake"); !reached {
		t.Fatal("a game with a daily limit never summed today's play time")
	}
	allowed, _, err, reached := playVerdict(s, "tetris")
	if err != nil || !allowed {
		t.Fatalf("a game with no daily limit returned allowed=%v err=%v, want allowed", allowed, err)
	}
	if reached {
		t.Fatal("a game with no daily limit summed play time anyway")
	}
}

func TestPlayPolicyRefusesWhenTodaysPlayTimeCannotBeSummed(t *testing.T) {
	// A failed sum used to leave used=0, which is below every limit, so the one
	// query that enforces the daily limit could switch it off by failing. The
	// pool below points at a port nothing listens on, so the query fails the way
	// an unreachable database does.
	s := playPolicyServer(t, `{"enabled":true,"windows":[],"daily_limits":{"snake":60}}`, `{"timezone":"Asia/Seoul"}`)
	pool, poolErr := pgxpool.New(context.Background(), "postgres://igame:igame@127.0.0.1:1/igame")
	if poolErr != nil {
		t.Fatalf("build pool: %v", poolErr)
	}
	defer pool.Close()
	s.DB = pool

	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey, Principal{UserID: uuid.New(), Role: "user"}))
	allowed, _, err := s.playAllowed(r, uuid.New(), "snake")
	if err == nil || allowed {
		t.Fatalf("a failed sum returned allowed=%v err=%v, want a refusal", allowed, err)
	}
}
