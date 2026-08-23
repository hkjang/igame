package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
)

// settingsTTL bounds how long a system setting may be served from memory.
// Writes on this instance drop the entry immediately, so the TTL only bounds
// staleness for a second instance sharing the same database.
const settingsTTL = 5 * time.Second

// oidcProviderTTL bounds how long a discovered OIDC provider is reused. The
// discovery document changes rarely and a login performs two lookups (start and
// callback), so caching removes most provider round trips.
const oidcProviderTTL = time.Hour

type settingEntry struct {
	raw     []byte
	missing bool
	expires time.Time
}

type providerEntry struct {
	provider *oidc.Provider
	expires  time.Time
}

// settingRaw reads a system setting through the in-memory cache. Absent keys
// are cached as well: several settings are optional and would otherwise cost a
// query on every request that consults them.
func (s *Server) settingRaw(ctx context.Context, key string) ([]byte, error) {
	now := time.Now()
	if entry, ok := s.cachedSetting(key, now); ok {
		if entry.missing {
			return nil, pgx.ErrNoRows
		}
		return entry.raw, nil
	}
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT value FROM system_settings WHERE key=$1`, key).Scan(&raw)
	switch {
	case err == nil:
		s.storeSetting(key, settingEntry{raw: raw, expires: now.Add(settingsTTL)})
	case errors.Is(err, pgx.ErrNoRows):
		s.storeSetting(key, settingEntry{missing: true, expires: now.Add(settingsTTL)})
	default:
		// Transient failures must not be cached; the caller sees the error and
		// the next request retries the database.
		return nil, err
	}
	return raw, err
}

func (s *Server) cachedSetting(key string, now time.Time) (settingEntry, bool) {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	entry, ok := s.settingsCache[key]
	return entry, ok && now.Before(entry.expires)
}

func (s *Server) storeSetting(key string, entry settingEntry) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.settingsCache == nil {
		s.settingsCache = map[string]settingEntry{}
	}
	s.settingsCache[key] = entry
}

// invalidateSetting drops a cached setting after an administrator writes it so
// the change is visible on this instance without waiting for the TTL.
func (s *Server) invalidateSetting(key string) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	delete(s.settingsCache, key)
}

// serviceTimezone resolves a named location once and reuses it. time.LoadLocation
// parses the embedded zoneinfo archive on every call, which is wasteful on the
// request path.
var locationCache sync.Map // string -> *time.Location

func loadLocation(name string) (*time.Location, error) {
	if cached, ok := locationCache.Load(name); ok {
		return cached.(*time.Location), nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	locationCache.Store(name, location)
	return location, nil
}

// oidcProvider returns a cached OIDC provider for the issuer. A changed issuer
// naturally misses the cache, and writing the OIDC setting clears it.
func (s *Server) oidcProvider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	now := time.Now()
	s.providerMu.RLock()
	entry, ok := s.providerCache[issuer]
	s.providerMu.RUnlock()
	if ok && now.Before(entry.expires) {
		return entry.provider, nil
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	s.providerMu.Lock()
	if s.providerCache == nil {
		s.providerCache = map[string]providerEntry{}
	}
	s.providerCache[issuer] = providerEntry{provider: provider, expires: now.Add(oidcProviderTTL)}
	s.providerMu.Unlock()
	return provider, nil
}

func (s *Server) invalidateOIDCProviders() {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	clear(s.providerCache)
}
