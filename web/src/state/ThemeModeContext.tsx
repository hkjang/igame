import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { CssBaseline, ThemeProvider } from '@mui/material';
import { createAppTheme, type ThemeMode } from '../theme';
import { applyPreferences, loadPreferences, resolveThemeMode, savePreferences, type ThemePreference } from './preferences';

interface ThemeModeValue {
  /** What the user chose, including 'system'. */
  preference: ThemePreference;
  /** What that resolves to right now. */
  mode: ThemeMode;
  setPreference: (next: ThemePreference) => void;
  /** Flips between light and dark, leaving 'system' behind deliberately. */
  toggle: () => void;
}

const ThemeModeContext = createContext<ThemeModeValue | null>(null);

export function ThemeModeProvider({ children }: { children: ReactNode }) {
  const [preference, setStoredPreference] = useState<ThemePreference>(() => loadPreferences().theme);
  const [mode, setMode] = useState<ThemeMode>(() => resolveThemeMode(preference));

  // While following the system, track it: a workstation switching to night mode
  // at dusk should carry the portal with it.
  useEffect(() => {
    setMode(resolveThemeMode(preference));
    if (preference !== 'system') return;
    let media: MediaQueryList;
    try {
      media = window.matchMedia('(prefers-color-scheme: light)');
    } catch {
      return;
    }
    const follow = () => setMode(resolveThemeMode('system'));
    media.addEventListener('change', follow);
    return () => media.removeEventListener('change', follow);
  }, [preference]);

  useEffect(() => {
    document.documentElement.style.colorScheme = mode;
  }, [mode]);

  const setPreference = useCallback((next: ThemePreference) => {
    setStoredPreference(next);
    // Appearance applies immediately and persists; unlike the text-size slider
    // there is nothing here worth staging behind a save button.
    const current = loadPreferences();
    savePreferences({ ...current, theme: next });
    applyPreferences({ ...current, theme: next });
  }, []);

  const toggle = useCallback(() => setPreference(resolveThemeMode(preference) === 'dark' ? 'light' : 'dark'), [preference, setPreference]);

  const theme = useMemo(() => createAppTheme(mode), [mode]);
  const value = useMemo(() => ({ preference, mode, setPreference, toggle }), [preference, mode, setPreference, toggle]);

  return (
    <ThemeModeContext.Provider value={value}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        {children}
      </ThemeProvider>
    </ThemeModeContext.Provider>
  );
}

export function useThemeMode(): ThemeModeValue {
  const value = useContext(ThemeModeContext);
  if (!value) throw new Error('useThemeMode must be used inside ThemeModeProvider');
  return value;
}
