import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { api, ApiError, onSessionExpired } from '../api/client';
import type { PublicConfig, User, VersionInfo } from '../types';
import { beginSilentSso, clearSilentSsoState, markSignedOut, shouldAttemptSilentSso } from './silentSso';

interface AuthState {
  user: User | null;
  config: PublicConfig;
  version: VersionInfo;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
}

const fallbackConfig: PublicConfig = {
  name: 'igame', version: 'dev', oidc_enabled: false, oidc_login_url: '/api/v1/auth/oidc/login', bootstrap_login_enabled: true,
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [config, setConfig] = useState(fallbackConfig);
  const [version, setVersion] = useState<VersionInfo>({ version: 'dev' });
  const [loading, setLoading] = useState(true);
  const signedIn = useRef(false);
  signedIn.current = user !== null;

  // An expired session must drop the user back to the login screen instead of
  // leaving every page showing an authentication error it cannot recover from.
  useEffect(() => {
    onSessionExpired(() => { if (signedIn.current) setUser(null); });
  }, []);

  useEffect(() => {
    let mounted = true;
    Promise.allSettled([api.publicConfig(), api.version(), api.me()]).then(([configResult, versionResult, userResult]) => {
      if (!mounted) return;
      const loaded = configResult.status === 'fulfilled' ? { ...fallbackConfig, ...configResult.value } : fallbackConfig;
      setConfig(loaded);
      if (versionResult.status === 'fulfilled') setVersion(versionResult.value);
      else if (configResult.status === 'fulfilled') setVersion({ version: configResult.value.version });
      if (userResult.status === 'fulfilled') {
        setUser(userResult.value);
        // A session exists again, so a sign-out earlier in this tab no longer
        // has to suppress the next silent attempt.
        clearSilentSsoState();
      } else if (userResult.reason instanceof ApiError && userResult.reason.status === 401) {
        // Nobody is signed in here, but the provider may still hold a session.
        // The attempt is a top-level navigation away from this page; loading
        // stays true so the login screen is not drawn for the instant before
        // the browser leaves. Every guard against retrying lives in the rule.
        if (shouldAttemptSilentSso(loaded, window.location)) {
          beginSilentSso(window.location.pathname + window.location.search + window.location.hash);
          return;
        }
      } else {
        // Public config can still render a useful login screen when the API is unavailable.
        setUser(null);
      }
      setLoading(false);
    });
    return () => { mounted = false; };
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    await api.login(username, password);
    setUser(await api.me());
    clearSilentSsoState();
  }, []);
  const logout = useCallback(async () => {
    // Signing the visitor straight back in silently would make sign-out look
    // broken, so the suppression is written before the session goes away.
    markSignedOut();
    try { await api.logout(); } finally { setUser(null); }
  }, []);
  const refreshUser = useCallback(async () => setUser(await api.me()), []);
  const value = useMemo(() => ({ user, config, version, loading, login, logout, refreshUser }), [user, config, version, loading, login, logout, refreshUser]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used inside AuthProvider');
  return value;
}
