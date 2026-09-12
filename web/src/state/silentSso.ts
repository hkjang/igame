import type { PublicConfig } from '../types';

/**
 * Silent SSO: when the identity provider still holds a session for the
 * visitor, sign them in without showing the login screen.
 *
 * The attempt is a top-level navigation to the login endpoint with
 * prompt=none. The provider never renders anything for it — either a code
 * comes straight back and the ordinary flow finishes, or the callback lands
 * on /login?sso=none because there was no session. That refusal is an
 * ordinary answer, not an error. What makes it dangerous is retrying: a
 * second attempt on the page it lands on bounces the browser between the
 * provider and the portal forever, and the visitor sees nothing but flicker.
 * Everything in this module exists to make sure the attempt happens at most
 * once per tab session.
 */

// sessionStorage rather than localStorage on purpose: a new tab should try
// again (the visitor may have signed in to the provider since), while a
// reload after a refusal must not.
const ATTEMPTED_KEY = 'igame.sso.silentAttempted';
const SIGNED_OUT_KEY = 'igame.sso.signedOut';

/** Where the silent attempt sends the browser. */
export const SILENT_LOGIN_PATH = '/api/v1/auth/oidc/login';

export type StorageSource = () => Storage;

const sessionStorageSource: StorageSource = () => window.sessionStorage;

function readFlag(key: string, source: StorageSource): boolean {
  try {
    return source().getItem(key) === 'true';
  } catch {
    // Private modes and blocked site data throw here. Reading that as "not
    // yet attempted" would start the loop this flag prevents, so an
    // unreadable store counts as already attempted.
    return true;
  }
}

function writeFlag(key: string, value: boolean, source: StorageSource) {
  try {
    if (value) source().setItem(key, 'true');
    else source().removeItem(key);
  } catch {
    // Nothing to do: readFlag already fails closed for this store.
  }
}

/** Records that the visitor signed out on purpose, which suppresses auto-login. */
export function markSignedOut(source: StorageSource = sessionStorageSource) {
  writeFlag(SIGNED_OUT_KEY, true, source);
  writeFlag(ATTEMPTED_KEY, true, source);
}

/** Lifts the suppression once a session exists again. */
export function clearSilentSsoState(source: StorageSource = sessionStorageSource) {
  writeFlag(SIGNED_OUT_KEY, false, source);
  writeFlag(ATTEMPTED_KEY, false, source);
}

/**
 * Paths where a silent attempt must never start.
 *
 * The login screen is where a refusal lands, so an attempt from there is the
 * loop itself. API, MCP, health and proxy paths are not screens at all — the
 * portal never renders on them, but the rule says so explicitly rather than
 * relying on that.
 */
export function silentSsoAllowedPath(pathname: string): boolean {
  if (pathname === '/login' || pathname.startsWith('/login/')) return false;
  return !['/api/', '/mcp', '/healthz', '/readyz', '/momento/'].some((prefix) => pathname === prefix || pathname.startsWith(prefix));
}

/**
 * Decides whether to try signing in without showing the login screen.
 *
 * Three guards stack against the redirect loop: the once-per-tab flag in
 * sessionStorage, the sign-out suppression, and the sso= marker the callback
 * puts in the address when the provider had no session — that last one still
 * holds when the browser's storage was cleared in between.
 */
export function shouldAttemptSilentSso(
  config: Pick<PublicConfig, 'oidc_enabled' | 'oidc_auto_login'>,
  location: Pick<Location, 'pathname' | 'search'>,
  source: StorageSource = sessionStorageSource,
): boolean {
  if (!config.oidc_enabled || !config.oidc_auto_login) return false;
  if (!silentSsoAllowedPath(location.pathname)) return false;
  const sso = new URLSearchParams(location.search).get('sso');
  if (sso === 'none' || sso === 'error') return false;
  if (readFlag(SIGNED_OUT_KEY, source)) return false;
  if (readFlag(ATTEMPTED_KEY, source)) return false;
  return true;
}

/** Only a path on this service may be returned to; anything else becomes the portal root. */
export function safeReturnTo(value: string): string {
  return value.startsWith('/') && !value.startsWith('//') ? value : '/';
}

/** Builds the address of a silent attempt that comes back to returnTo. */
export function silentLoginUrl(returnTo: string): string {
  return `${SILENT_LOGIN_PATH}?prompt=none&return_to=${encodeURIComponent(safeReturnTo(returnTo))}`;
}

/**
 * Sends the browser to the provider for one silent attempt. The attempted
 * flag is written before leaving so that even an interrupted navigation
 * counts.
 */
export function beginSilentSso(returnTo: string, source: StorageSource = sessionStorageSource) {
  writeFlag(ATTEMPTED_KEY, true, source);
  window.location.assign(silentLoginUrl(returnTo));
}
