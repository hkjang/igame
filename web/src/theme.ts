import { alpha, createTheme } from '@mui/material/styles';

export const theme = createTheme({
  cssVariables: true,
  palette: {
    mode: 'dark',
    primary: { main: '#67d7ff', light: '#a5e9ff', dark: '#1596c6', contrastText: '#061019' },
    secondary: { main: '#9cf56b', contrastText: '#08140a' },
    background: { default: '#07101d', paper: '#111d2c' },
    text: { primary: '#f3f7fb', secondary: '#aab9ca' },
    error: { main: '#ff6b76' },
    warning: { main: '#ffbd5c' },
    success: { main: '#73df9b' },
    divider: 'rgba(188, 218, 240, .14)',
  },
  typography: {
    fontFamily: 'Pretendard, "Noto Sans KR", "Malgun Gothic", system-ui, -apple-system, sans-serif',
    fontSize: 16,
    htmlFontSize: 16,
    h1: { fontSize: 'clamp(2rem, 5vw, 3.8rem)', fontWeight: 800, lineHeight: 1.08, letterSpacing: '-0.035em' },
    h2: { fontSize: 'clamp(1.65rem, 3vw, 2.3rem)', fontWeight: 750, lineHeight: 1.2 },
    h3: { fontSize: '1.4rem', fontWeight: 700 },
    h4: { fontSize: '1.18rem', fontWeight: 700 },
    body1: { fontSize: '1rem', lineHeight: 1.65 },
    body2: { fontSize: '0.95rem', lineHeight: 1.55 },
    button: { fontSize: '0.97rem', fontWeight: 700, textTransform: 'none' },
  },
  shape: { borderRadius: 14 },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        html: { fontSize: 16, colorScheme: 'dark' },
        body: { minWidth: 320, minHeight: '100vh' },
        '*:focus-visible': { outline: '3px solid #67d7ff', outlineOffset: 2 },
        '::selection': { background: alpha('#67d7ff', 0.28) },
      },
    },
    MuiButton: { styleOverrides: { root: { minHeight: 44, borderRadius: 10, paddingInline: 18 } } },
    MuiIconButton: { styleOverrides: { root: { minWidth: 44, minHeight: 44 } } },
    MuiTextField: { defaultProps: { fullWidth: true, size: 'medium' } },
    MuiCard: { styleOverrides: { root: { backgroundImage: 'none', border: '1px solid rgba(188,218,240,.12)' } } },
    MuiTooltip: { styleOverrides: { tooltip: { fontSize: '0.9rem' } } },
  },
});
