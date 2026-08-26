import { alpha, createTheme, type Theme } from '@mui/material/styles';

export type ThemeMode = 'light' | 'dark';

/**
 * Every colour the application shell uses lives here.
 *
 * Surfaces and accents used to be hex literals scattered across layouts and
 * pages, which meant the palette could not be changed in one place. They are
 * palette entries now, so `sx={{ bgcolor: 'surface.nav' }}` resolves through
 * the theme and a second palette is a contained change.
 */
declare module '@mui/material/styles' {
  interface Palette {
    surface: { ground: string; nav: string; sunken: string; code: string; codeText: string; overlay: string };
    accent: { ai: string; favorite: string };
  }
  interface PaletteOptions {
    surface?: { ground: string; nav: string; sunken: string; code: string; codeText: string; overlay: string };
    accent?: { ai: string; favorite: string };
  }
}

const palettes = {
  dark: {
    primary: { main: '#67d7ff', light: '#a5e9ff', dark: '#1596c6', contrastText: '#061019' },
    secondary: { main: '#9cf56b', contrastText: '#08140a' },
    background: { default: '#07101d', paper: '#111d2c' },
    text: { primary: '#f3f7fb', secondary: '#aab9ca' },
    error: { main: '#ff6b76' },
    warning: { main: '#ffbd5c' },
    success: { main: '#73df9b' },
    divider: 'rgba(188, 218, 240, .14)',
    surface: {
      ground: '#07101d',
      nav: '#091523',
      sunken: '#0b1725',
      code: '#050b12',
      codeText: '#c7eaff',
      overlay: '#0e1c2b',
    },
    accent: { ai: '#af8cff', favorite: '#ff718f' },
  },
  // The light ramp is not the dark one inverted: on a white ground the cyan and
  // lime of the dark theme fall far below the contrast the product commits to,
  // so each hue is darkened until it carries white text.
  light: {
    primary: { main: '#0b6a8f', light: '#3f93b6', dark: '#064a66', contrastText: '#ffffff' },
    secondary: { main: '#2e7d32', contrastText: '#ffffff' },
    background: { default: '#f2f6fa', paper: '#ffffff' },
    text: { primary: '#101d2b', secondary: '#4c5c6d' },
    error: { main: '#b3261e' },
    warning: { main: '#8a5300' },
    success: { main: '#1f6b3d' },
    divider: 'rgba(16, 29, 43, .16)',
    surface: {
      ground: '#f2f6fa',
      nav: '#ffffff',
      sunken: '#e5ecf4',
      // Code blocks stay dark in both modes, which is why they carry their own
      // text colour instead of inheriting text.primary.
      code: '#0f1d2e',
      codeText: '#d6e9f7',
      overlay: '#ffffff',
    },
    accent: { ai: '#5b3bb8', favorite: '#b4225b' },
  },
} as const;

const typography = {
  fontFamily: 'Pretendard, "Noto Sans KR", "Malgun Gothic", system-ui, -apple-system, sans-serif',
  fontSize: 16,
  htmlFontSize: 16,
  h1: { fontSize: 'clamp(2rem, 5vw, 3.8rem)', fontWeight: 800, lineHeight: 1.08, letterSpacing: '-0.035em' },
  h2: { fontSize: 'clamp(1.65rem, 3vw, 2.3rem)', fontWeight: 750, lineHeight: 1.2 },
  h3: { fontSize: '1.4rem', fontWeight: 700 },
  h4: { fontSize: '1.18rem', fontWeight: 700 },
  body1: { fontSize: '1rem', lineHeight: 1.65 },
  body2: { fontSize: '0.95rem', lineHeight: 1.55 },
  button: { fontSize: '0.97rem', fontWeight: 700, textTransform: 'none' as const },
};

function baseline(currentTheme: Theme) {
  const glow = currentTheme.palette.mode === 'dark' ? 0.34 : 0.14;
  return {
    html: { fontSize: 16, colorScheme: currentTheme.palette.mode },
    body: {
      minWidth: 320,
      minHeight: '100dvh',
      background: [
        `radial-gradient(circle at 14% -10%, ${alpha(currentTheme.palette.primary.main, glow)}, transparent 36rem)`,
        `radial-gradient(circle at 92% 22%, ${alpha(currentTheme.palette.success.main, glow / 4)}, transparent 32rem)`,
        currentTheme.palette.surface.ground,
      ].join(','),
    },
    '*:focus-visible': { outline: `3px solid ${currentTheme.palette.primary.main}`, outlineOffset: 2 },
    '::selection': { background: alpha(currentTheme.palette.primary.main, 0.28) },
    '.admin-scrollbar': {
      scrollbarWidth: 'thin',
      scrollbarColor: `${alpha(currentTheme.palette.primary.main, 0.45)} ${currentTheme.palette.surface.sunken}`,
      '&::-webkit-scrollbar': { width: 10 },
      '&::-webkit-scrollbar-track': { background: currentTheme.palette.surface.sunken, borderRadius: 10 },
      '&::-webkit-scrollbar-thumb': {
        background: alpha(currentTheme.palette.primary.main, 0.42),
        border: `2px solid ${currentTheme.palette.surface.sunken}`,
        borderRadius: 10,
      },
      '&::-webkit-scrollbar-thumb:hover': { background: alpha(currentTheme.palette.primary.main, 0.62) },
    },
    '.game-canvas': {
      maxWidth: '100%',
      borderRadius: 14,
      // Built-in games draw their own art, so their frame keeps the dark ground
      // in both modes rather than washing the canvas out.
      background: palettes.dark.surface.ground,
      touchAction: 'none' as const,
    },
  };
}

export function createAppTheme(mode: ThemeMode) {
  const palette = palettes[mode];
  return createTheme({
    cssVariables: true,
    palette: { mode, ...palette },
    typography,
    shape: { borderRadius: 14 },
    components: {
      MuiCssBaseline: { styleOverrides: (currentTheme) => baseline(currentTheme) },
      // whiteSpace: a button label is a single control, and the admin header
      // was breaking "새로고침" across two lines to fit beside the search box.
      // A toolbar that runs out of room wraps its buttons, not their words.
      MuiButton: { styleOverrides: { root: { minHeight: 44, borderRadius: 10, paddingInline: 18, whiteSpace: 'nowrap' } } },
      MuiIconButton: { styleOverrides: { root: { minWidth: 44, minHeight: 44 } } },
      MuiTextField: { defaultProps: { fullWidth: true, size: 'medium' } },
      MuiCard: {
        styleOverrides: {
          root: ({ theme: currentTheme }) => ({ backgroundImage: 'none', border: `1px solid ${currentTheme.palette.divider}` }),
        },
      },
      MuiTooltip: { styleOverrides: { tooltip: { fontSize: '0.9rem' } } },
      // Korean has no inter-word spaces at the character level, so the default
      // break-anywhere behaviour splits 어절 across lines ("있습 / 니다").
      // keep-all holds each word together; break-word still saves the layout
      // from an unbreakable string such as a long URL.
      MuiTypography: { styleOverrides: { root: { wordBreak: 'keep-all', overflowWrap: 'break-word' } } },
    },
  });
}
