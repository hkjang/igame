import { safeReturnTo } from '../state/silentSso';

/**
 * Where the visitor was headed before they landed on the login screen.
 *
 * RequireAuth passes it in router state. A silent SSO attempt that the
 * provider refused arrives by full navigation instead, so the callback puts
 * the deep link in the address as return_to; only a path on this service is
 * honoured. Everything else starts at the portal root.
 */
export function loginDestination(state: unknown, search: string): string {
  const from = (state as { from?: { pathname?: string; search?: string } } | null)?.from;
  if (from?.pathname) return from.pathname + (from.search ?? '');
  return safeReturnTo(new URLSearchParams(search).get('return_to') ?? '');
}

/** The SSO button's address; the deep link rides along so SSO lands there too. */
export function ssoLoginHref(loginUrl: string, from: string): string {
  return from === '/' ? loginUrl : `${loginUrl}?return_to=${encodeURIComponent(from)}`;
}
