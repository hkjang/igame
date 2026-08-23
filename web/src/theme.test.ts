import { describe, expect, it } from 'vitest';
import { createAppTheme, type ThemeMode } from './theme';

const modes: ThemeMode[] = ['dark', 'light'];

function channel(value: number): number {
  const ratio = value / 255;
  return ratio <= 0.03928 ? ratio / 12.92 : ((ratio + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  const value = hex.replace('#', '');
  const full = value.length === 3 ? value.split('').map((c) => c + c).join('') : value;
  const [r, g, b] = [0, 2, 4].map((offset) => parseInt(full.slice(offset, offset + 2), 16));
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/** WCAG 2.1 relative contrast between two opaque colours. */
function contrast(foreground: string, background: string): number {
  const [light, dark] = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
  return (light + 0.05) / (dark + 0.05);
}

describe('theme palettes', () => {
  it.each(modes)('%s exposes every surface the shell paints on', (mode) => {
    const { palette } = createAppTheme(mode);
    for (const key of ['ground', 'nav', 'sunken', 'code', 'codeText', 'overlay'] as const) {
      expect(palette.surface[key]).toMatch(/^#[0-9a-f]{6}$/i);
    }
    expect(palette.accent.ai).toMatch(/^#[0-9a-f]{6}$/i);
    expect(palette.accent.favorite).toMatch(/^#[0-9a-f]{6}$/i);
    expect(palette.surface.ground).toBe(palette.background.default);
    expect(palette.mode).toBe(mode);
  });

  // The product states contrast as a requirement, and a light palette is where
  // a dark-theme hue quietly stops being readable.
  it.each(modes)('%s keeps body text at AA contrast on both surfaces', (mode) => {
    const { palette } = createAppTheme(mode);
    expect(contrast(palette.text.primary, palette.background.default)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(palette.text.primary, palette.background.paper)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(palette.text.secondary, palette.background.paper)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(modes)('%s keeps filled buttons readable', (mode) => {
    const { palette } = createAppTheme(mode);
    expect(contrast(palette.primary.contrastText, palette.primary.main)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(palette.secondary.contrastText, palette.secondary.main)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(modes)('%s keeps code blocks readable against their own surface', (mode) => {
    const { palette } = createAppTheme(mode);
    expect(contrast(palette.surface.codeText, palette.surface.code)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(modes)('%s keeps status colours legible as text on paper', (mode) => {
    const { palette } = createAppTheme(mode);
    // These carry meaning on their own, so they are held to large-text AA.
    for (const status of [palette.error.main, palette.warning.main, palette.success.main, palette.accent.ai]) {
      expect(contrast(status, palette.background.paper)).toBeGreaterThanOrEqual(3);
    }
  });

  it.each(modes)('%s keeps the accessibility commitments the README makes', (mode) => {
    const theme = createAppTheme(mode);
    expect(theme.typography.fontSize).toBe(16);
    expect(theme.typography.htmlFontSize).toBe(16);
    const button = theme.components?.MuiButton?.styleOverrides?.root as { minHeight?: number } | undefined;
    const iconButton = theme.components?.MuiIconButton?.styleOverrides?.root as { minHeight?: number } | undefined;
    expect(button?.minHeight).toBeGreaterThanOrEqual(44);
    expect(iconButton?.minHeight).toBeGreaterThanOrEqual(44);
  });
});
