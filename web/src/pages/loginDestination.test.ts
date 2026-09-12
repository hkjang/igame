import { describe, expect, it } from 'vitest';
import { loginDestination, ssoLoginHref } from './loginDestination';

describe('loginDestination', () => {
  it('prefers the screen RequireAuth recorded, query included', () => {
    expect(loginDestination({ from: { pathname: '/games/snake', search: '?tab=2' } }, '')).toBe('/games/snake?tab=2');
    expect(loginDestination({ from: { pathname: '/rankings' } }, '?return_to=%2Fgames')).toBe('/rankings');
  });

  it('reads the deep link a refused silent attempt left in the address', () => {
    expect(loginDestination(null, '?sso=none&return_to=%2Fgames%2Fsnake')).toBe('/games/snake');
    expect(loginDestination(undefined, '?sso=none')).toBe('/');
  });

  it('never leaves this service', () => {
    expect(loginDestination(null, '?return_to=%2F%2Fevil.example%2F')).toBe('/');
    expect(loginDestination(null, '?return_to=https%3A%2F%2Fevil.example')).toBe('/');
  });
});

describe('ssoLoginHref', () => {
  it('carries the deep link only when there is one', () => {
    expect(ssoLoginHref('/api/v1/auth/oidc/login', '/')).toBe('/api/v1/auth/oidc/login');
    expect(ssoLoginHref('/api/v1/auth/oidc/login', '/games/snake?tab=2')).toBe('/api/v1/auth/oidc/login?return_to=%2Fgames%2Fsnake%3Ftab%3D2');
  });
});
