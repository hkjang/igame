export const FONT_SCALE_MIN = 100;
export const FONT_SCALE_MAX = 125;
export const FONT_SCALE_STEP = 5;

const FONT_SCALE_KEY = 'igame-font-scale';
const MOTION_KEY = 'igame-motion';
const THEME_KEY = 'igame-theme';

/** 'system' follows the operating system's own light/dark setting. */
export type ThemePreference = 'system' | 'light' | 'dark';

export interface Preferences {
  /** Percentage of the 16px base body size, 100–125. */
  fontScale: number;
  motion: boolean;
  theme: ThemePreference;
}

export const DEFAULT_PREFERENCES: Preferences = { fontScale: FONT_SCALE_MIN, motion: true, theme: 'system' };

function readTheme(value: string | null): ThemePreference {
  return value === 'light' || value === 'dark' ? value : 'system';
}

/** Resolves the preference against the OS setting into an actual mode. */
export function resolveThemeMode(preference: ThemePreference): 'light' | 'dark' {
  if (preference !== 'system') return preference;
  try {
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  } catch {
    // Without matchMedia the product's original dark appearance is the default.
    return 'dark';
  }
}

export function clampFontScale(value: number): number {
  if (!Number.isFinite(value)) return FONT_SCALE_MIN;
  return Math.min(FONT_SCALE_MAX, Math.max(FONT_SCALE_MIN, Math.round(value)));
}

// A browser with site data blocked throws on any localStorage access. These are
// personalisation preferences: losing them must never stop the app from
// starting or leave a click handler to throw.
function read(key: string): string | null {
  try { return localStorage.getItem(key); } catch { return null; }
}

function write(key: string, value: string): boolean {
  try { localStorage.setItem(key, value); return true; } catch { return false; }
}

export function loadPreferences(): Preferences {
  const stored = read(FONT_SCALE_KEY);
  return {
    fontScale: stored === null ? FONT_SCALE_MIN : clampFontScale(Number(stored)),
    motion: read(MOTION_KEY) !== 'off',
    theme: readTheme(read(THEME_KEY)),
  };
}

/** Applies preferences to the document. Safe to call before React mounts. */
export function applyPreferences({ fontScale, motion, theme }: Preferences): void {
  // A percentage, not a pixel size: an absolute value overrode whatever the
  // reader had set as their browser's default font, so someone who had raised
  // it for legibility opened the portal and got 16px anyway. This scale
  // multiplies their setting instead of replacing it, and 100 leaves it alone.
  document.documentElement.style.fontSize = `${clampFontScale(fontScale)}%`;
  document.documentElement.dataset.motion = motion ? 'on' : 'off';
  // Setting the scheme before React mounts keeps the browser from painting
  // scrollbars and form controls in the wrong mode for a frame.
  document.documentElement.style.colorScheme = resolveThemeMode(theme);
}

/** Persists preferences and reports whether the browser accepted them. */
export function savePreferences({ fontScale, motion, theme }: Preferences): boolean {
  const scaleStored = write(FONT_SCALE_KEY, String(clampFontScale(fontScale)));
  const motionStored = write(MOTION_KEY, motion ? 'on' : 'off');
  const themeStored = write(THEME_KEY, theme);
  return scaleStored && motionStored && themeStored;
}
