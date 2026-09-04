package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hkjang/igame/internal/version"
	"github.com/jackc/pgx/v5"
)

// growthTables are the append-only tables the operator is responsible for
// pruning. igame ships no retention job on purpose, so the console at least has
// to show how much has accumulated.
var growthTables = []string{"audit_logs", "game_telemetry", "game_sessions", "scores"}

type tableUsage struct {
	Table string `json:"table"`
	// Rows is an estimate from the planner statistics: count(*) on a telemetry
	// table with millions of rows is a sequential scan nobody wants on a
	// dashboard load.
	Rows      int64 `json:"rows"`
	Estimated bool  `json:"estimated"`
}

type publishedContent struct {
	Slug        string `json:"slug"`
	Label       string `json:"label"`
	VersionNo   int    `json:"version_no"`
	PublishedAt string `json:"published_at,omitempty"`
}

// adminStatus answers "what state is this installation in" in one request.
//
// The runtime image has no shell, so an operator cannot look inside the
// container. This gathers the things they would otherwise have to hunt for
// across five settings pages and two content studios.
func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	started := time.Now()
	dbErr := s.DB.Ping(ctx)
	latency := time.Since(started).Milliseconds()
	stat := s.DB.Stat()
	database := map[string]any{
		"reachable":            dbErr == nil,
		"latency_ms":           latency,
		"connections_in_use":   stat.AcquiredConns(),
		"connections_idle":     stat.IdleConns(),
		"connections_max":      stat.MaxConns(),
		"acquire_wait_seconds": stat.AcquireDuration().Seconds(),
	}

	var oidcCfg oidcSetting
	_ = s.setting(ctx, "oidc", &oidcCfg)
	var aiCfg aiSetting
	_ = s.setting(ctx, "ai", &aiCfg)
	var approvalCfg approvalSetting
	_ = s.setting(ctx, "approval", &approvalCfg)
	var service struct {
		Timezone              string `json:"timezone"`
		PublicURL             string `json:"public_url"`
		TrustProxy            bool   `json:"trust_proxy"`
		BootstrapLoginEnabled *bool  `json:"bootstrap_login_enabled"`
	}
	_ = s.setting(ctx, "service", &service)
	var playPolicy struct {
		Enabled bool `json:"enabled"`
	}
	_ = s.setting(ctx, "play_policy", &playPolicy)
	bootstrapEnabled := service.BootstrapLoginEnabled == nil || *service.BootstrapLoginEnabled

	writeJSON(w, 200, map[string]any{
		"service": map[string]any{
			"version": version.Version, "commit": version.Commit, "build_date": version.BuildDate,
			"timezone": firstString(service.Timezone, "Asia/Seoul"),
			// Whether HTTPS is actually declared decides the session cookie's
			// Secure flag and HSTS, so it belongs on a status screen.
			"public_url": service.PublicURL, "trust_proxy": service.TrustProxy,
			"https": strings.HasPrefix(canonicalBaseURL(service.PublicURL), "https://"),
		},
		"database": database,
		"policies": map[string]any{
			"approval_enabled": approvalCfg.Enabled, "oidc_enabled": oidcCfg.Enabled,
			"ai_enabled": aiCfg.Enabled, "bootstrap_login_enabled": bootstrapEnabled,
			"play_policy_enabled": playPolicy.Enabled,
		},
		"published": s.publishedContentStatus(ctx),
		"storage":   s.storageUsage(ctx),
	})
}

// publishedContentStatus reports the snapshot each built-in game is serving.
func (s *Server) publishedContentStatus(ctx context.Context) []publishedContent {
	items := make([]publishedContent, 0, 1+len(defenseGameSlugs))
	if realm, err := s.loadRealmGuardPublished(ctx); err == nil {
		items = append(items, publishedContent{Slug: "realmguard", Label: realm.Label, VersionNo: realm.VersionNo, PublishedAt: optionalTime(realm.PublishedAt)})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		s.Log.Warn("status: read published RealmGuard content", "error", err)
	}
	for _, slug := range defenseGameSlugs {
		defense, err := s.loadDefensePublished(ctx, slug)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				s.Log.Warn("status: read published Defense content", "game", slug, "error", err)
			}
			continue
		}
		items = append(items, publishedContent{Slug: slug, Label: defense.Label, VersionNo: defense.VersionNo, PublishedAt: optionalTime(defense.PublishedAt)})
	}
	return items
}

func optionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// storageUsage reads planner row estimates for the tables that only grow.
func (s *Server) storageUsage(ctx context.Context) []tableUsage {
	usage := make([]tableUsage, 0, len(growthTables))
	for _, table := range growthTables {
		entry := tableUsage{Table: table, Estimated: true}
		var estimate float64
		if err := s.DB.QueryRow(ctx, `SELECT reltuples FROM pg_class WHERE oid = to_regclass($1)`, table).Scan(&estimate); err != nil {
			s.Log.Warn("status: read table estimate", "table", table, "error", err)
			continue
		}
		if estimate < 0 {
			// A table that has never been analyzed reports -1; counting a small
			// unanalyzed table is cheap and beats showing nothing.
			var exact int64
			if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+table+` LIMIT 100000) sample`).Scan(&exact); err == nil {
				entry.Rows = exact
				entry.Estimated = exact >= 100000
			}
		} else {
			entry.Rows = int64(estimate)
		}
		usage = append(usage, entry)
	}
	return usage
}
