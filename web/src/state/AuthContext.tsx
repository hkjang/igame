import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { api, ApiError } from '../api/client';
import type { PublicConfig, User, VersionInfo } from '../types';

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

  useEffect(() => {
    let mounted = true;
    Promise.allSettled([api.publicConfig(), api.version(), api.me()]).then(([configResult, versionResult, userResult]) => {
      if (!mounted) return;
      if (configResult.status === 'fulfilled') setConfig({ ...fallbackConfig, ...configResult.value });
      if (versionResult.status === 'fulfilled') setVersion(versionResult.value);
      else if (configResult.status === 'fulfilled') setVersion({ version: configResult.value.version });
      if (userResult.status === 'fulfilled') setUser(userResult.value);
      else if (!(userResult.reason instanceof ApiError && userResult.reason.status === 401)) {
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
  }, []);
  const logout = useCallback(async () => {
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
