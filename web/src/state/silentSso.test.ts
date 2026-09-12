import { describe, expect, it } from 'vitest';
import { clearSilentSsoState, markSignedOut, safeReturnTo, shouldAttemptSilentSso, silentLoginUrl, silentSsoAllowedPath } from './silentSso';

// An in-memory Storage that stands for one tab's sessionStorage.
function memoryStorage(): Storage {
  const entries = new Map<string, string>();
  return {
    getItem: (key) => entries.get(key) ?? null,
    setItem: (key, value) => { entries.set(key, String(value)); },
    removeItem: (key) => { entries.delete(key); },
    clear: () => { entries.clear(); },
    key: (index) => [...entries.keys()][index] ?? null,
    get length() { return entries.size; },
  };
}

const enabled = { oidc_enabled: true, oidc_auto_login: true };
const home = { pathname: '/', search: '' };

describe('shouldAttemptSilentSso', () => {
  it('does nothing unless the administrator turned auto_login on', () => {
    const store = memoryStorage();
    expect(shouldAttemptSilentSso({ oidc_enabled: true, oidc_auto_login: false }, home, () => store)).toBe(false);
    expect(shouldAttemptSilentSso({ oidc_enabled: false, oidc_auto_login: true }, home, () => store)).toBe(false);
    expect(shouldAttemptSilentSso({ oidc_enabled: true }, home, () => store)).toBe(false);
    expect(shouldAttemptSilentSso(enabled, home, () => store)).toBe(true);
  });

  it('tries once per tab session: a refusal followed by a reload does not try again', () => {
    const store = memoryStorage();
    expect(shouldAttemptSilentSso(enabled, home, () => store)).toBe(true);
    // beginSilentSso would navigate away; record the attempt the same way.
    store.setItem('igame.sso.silentAttempted', 'true');
    expect(shouldAttemptSilentSso(enabled, home, () => store)).toBe(false);
    // A new tab has an empty sessionStorage and tries again.
    expect(shouldAttemptSilentSso(enabled, home, memoryStorage)).toBe(true);
  });

  it('does not try again on the address the callback lands a refusal on', () => {
    // sessionStorage may have been cleared between the attempt and the
    // landing; the marker in the address must be enough on its own.
    const store = memoryStorage();
    expect(shouldAttemptSilentSso(enabled, { pathname: '/', search: '?sso=none' }, () => store)).toBe(false);
    expect(shouldAttemptSilentSso(enabled, { pathname: '/', search: '?sso=error' }, () => store)).toBe(false);
    expect(shouldAttemptSilentSso(enabled, { pathname: '/', search: '?tab=2' }, () => store)).toBe(true);
  });

  it('stays quiet after a deliberate sign-out until a session exists again', () => {
    const store = memoryStorage();
    markSignedOut(() => store);
    expect(shouldAttemptSilentSso(enabled, home, () => store)).toBe(false);
    clearSilentSsoState(() => store);
    expect(shouldAttemptSilentSso(enabled, home, () => store)).toBe(true);
  });

  it('treats a store it cannot read as already attempted', () => {
    // Private modes throw on access. Reading that as "not yet attempted"
    // would retry on every landing, which is exactly the loop.
    const throwing = () => { throw new DOMException('blocked', 'SecurityError'); };
    expect(shouldAttemptSilentSso(enabled, home, throwing)).toBe(false);
  });

  it('never starts from the login screen or a non-screen path', () => {
    const store = memoryStorage();
    for (const pathname of ['/login', '/login/', '/api/v1/me', '/mcp', '/healthz', '/readyz', '/momento/tracker.js']) {
      expect(shouldAttemptSilentSso(enabled, { pathname, search: '' }, () => store), pathname).toBe(false);
      expect(silentSsoAllowedPath(pathname), pathname).toBe(false);
    }
    for (const pathname of ['/', '/games/snake', '/rankings', '/admin/settings']) {
      expect(silentSsoAllowedPath(pathname), pathname).toBe(true);
    }
  });
});

describe('silentLoginUrl', () => {
  it('asks for prompt=none and carries the deep link back', () => {
    expect(silentLoginUrl('/games/snake?tab=2')).toBe('/api/v1/auth/oidc/login?prompt=none&return_to=%2Fgames%2Fsnake%3Ftab%3D2');
  });

  it('only returns to a path on this service', () => {
    expect(safeReturnTo('/games')).toBe('/games');
    expect(safeReturnTo('//evil.example/games')).toBe('/');
    expect(safeReturnTo('https://evil.example/')).toBe('/');
    expect(safeReturnTo('')).toBe('/');
    expect(silentLoginUrl('//evil.example')).toBe('/api/v1/auth/oidc/login?prompt=none&return_to=%2F');
  });
});
