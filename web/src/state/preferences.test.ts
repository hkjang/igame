import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { applyPreferences, clampFontScale, loadPreferences, resolveThemeMode, savePreferences } from './preferences';

describe('preferences', () => {
  beforeEach(() => { localStorage.clear(); vi.restoreAllMocks(); });
  afterEach(() => { localStorage.clear(); });

  it('clamps the font scale to the supported range', () => {
    expect(clampFontScale(90)).toBe(100);
    expect(clampFontScale(140)).toBe(125);
    expect(clampFontScale(115)).toBe(115);
    expect(clampFontScale(Number.NaN)).toBe(100);
  });

  it('round-trips stored preferences', () => {
    expect(savePreferences({ fontScale: 120, motion: false, theme: 'light' })).toBe(true);
    expect(loadPreferences()).toEqual({ fontScale: 120, motion: false, theme: 'light' });
  });

  it('falls back to defaults when nothing is stored', () => {
    expect(loadPreferences()).toEqual({ fontScale: 100, motion: true, theme: 'system' });
  });

  it('clamps a corrupted stored value instead of applying it', () => {
    localStorage.setItem('igame-font-scale', '9999');
    expect(loadPreferences().fontScale).toBe(125);
    localStorage.setItem('igame-font-scale', 'not-a-number');
    expect(loadPreferences().fontScale).toBe(100);
  });

  it('survives a browser that refuses site data', () => {
    // Replacing the global rather than spying on a method keeps this
    // independent of how the environment happens to implement Storage.
    const blocked = () => { throw new Error('blocked'); };
    vi.stubGlobal('localStorage', { getItem: blocked, setItem: blocked, removeItem: blocked, clear: blocked, key: blocked, length: 0 });
    expect(loadPreferences()).toEqual({ fontScale: 100, motion: true, theme: 'system' });
    expect(savePreferences({ fontScale: 110, motion: false, theme: 'dark' })).toBe(false);
    vi.unstubAllGlobals();
  });

  it('applies the scale and motion flag to the document', () => {
    // A percentage rather than a pixel size: the scale multiplies whatever the
    // reader has set as their browser's default font instead of replacing it,
    // and 100 leaves that setting exactly as they left it.
    applyPreferences({ fontScale: 125, motion: false, theme: 'dark' });
    expect(document.documentElement.style.fontSize).toBe('125%');
    expect(document.documentElement.dataset.motion).toBe('off');
    applyPreferences({ fontScale: 100, motion: true, theme: 'dark' });
    expect(document.documentElement.style.fontSize).toBe('100%');
    expect(document.documentElement.dataset.motion).toBe('on');
  });

  it('never pins the root to an absolute size', () => {
    // An absolute value overrode the reader's own default, so someone who had
    // raised it for legibility opened the portal and got 16px anyway.
    for (const fontScale of [100, 110, 125, 400, Number.NaN]) {
      applyPreferences({ fontScale, motion: true, theme: 'light' });
      expect(document.documentElement.style.fontSize).toMatch(/^\d+%$/);
    }
  });
});

describe('theme preference', () => {
  it('defaults to following the operating system', () => {
    expect(loadPreferences().theme).toBe('system');
  });

  it('only accepts the three supported values', () => {
    localStorage.setItem('igame-theme', 'neon');
    expect(loadPreferences().theme).toBe('system');
    for (const value of ['light', 'dark', 'system'] as const) {
      localStorage.setItem('igame-theme', value);
      expect(loadPreferences().theme).toBe(value);
    }
  });

  it('resolves an explicit choice without consulting the system', () => {
    const media = vi.fn();
    vi.stubGlobal('matchMedia', media);
    expect(resolveThemeMode('light')).toBe('light');
    expect(resolveThemeMode('dark')).toBe('dark');
    expect(media).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it('follows the system when asked to', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true }));
    expect(resolveThemeMode('system')).toBe('light');
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false }));
    expect(resolveThemeMode('system')).toBe('dark');
    vi.unstubAllGlobals();
  });

  it('keeps the original dark appearance when the browser cannot answer', () => {
    vi.stubGlobal('matchMedia', () => { throw new Error('unsupported'); });
    expect(resolveThemeMode('system')).toBe('dark');
    vi.unstubAllGlobals();
  });

  it('stamps the resolved scheme on the document so the browser paints controls to match', () => {
    applyPreferences({ fontScale: 100, motion: true, theme: 'light' });
    expect(document.documentElement.style.colorScheme).toBe('light');
    applyPreferences({ fontScale: 100, motion: true, theme: 'dark' });
    expect(document.documentElement.style.colorScheme).toBe('dark');
  });
});
