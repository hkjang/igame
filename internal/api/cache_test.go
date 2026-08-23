package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// A server with no database pool proves the cache answers without a query: any
// fall-through would dereference a nil *pgxpool.Pool and panic.
func TestSettingServedFromCacheWithoutQuerying(t *testing.T) {
	s := &Server{}
	s.storeSetting("service", settingEntry{raw: []byte(`{"timezone":"Asia/Seoul"}`), expires: time.Now().Add(settingsTTL)})
	var cfg struct {
		Timezone string `json:"timezone"`
	}
	if err := s.setting(context.Background(), "service", &cfg); err != nil {
		t.Fatalf("cached setting returned %v", err)
	}
	if cfg.Timezone != "Asia/Seoul" {
		t.Fatalf("timezone=%q, want Asia/Seoul", cfg.Timezone)
	}
}

func TestAbsentSettingIsCachedAsMissing(t *testing.T) {
	s := &Server{}
	s.storeSetting("approval", settingEntry{missing: true, expires: time.Now().Add(settingsTTL)})
	if err := s.setting(context.Background(), "approval", &struct{}{}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing setting returned %v, want pgx.ErrNoRows", err)
	}
}

func TestExpiredAndInvalidatedSettingsAreNotServed(t *testing.T) {
	s := &Server{}
	s.storeSetting("service", settingEntry{raw: []byte(`{}`), expires: time.Now().Add(-time.Second)})
	if _, ok := s.cachedSetting("service", time.Now()); ok {
		t.Fatal("an expired entry was served from the cache")
	}
	s.storeSetting("service", settingEntry{raw: []byte(`{}`), expires: time.Now().Add(settingsTTL)})
	s.invalidateSetting("service")
	if _, ok := s.cachedSetting("service", time.Now()); ok {
		t.Fatal("invalidateSetting left the entry in place")
	}
}

func TestLoadLocationReusesParsedZone(t *testing.T) {
	first, err := loadLocation("Asia/Seoul")
	if err != nil {
		t.Fatalf("load Asia/Seoul: %v", err)
	}
	second, err := loadLocation("Asia/Seoul")
	if err != nil || first != second {
		t.Fatalf("second load returned %v, %v; want the cached location", second, err)
	}
	if _, err := loadLocation("Not/AZone"); err == nil {
		t.Fatal("an unknown zone was accepted")
	}
}
