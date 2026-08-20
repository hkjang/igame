import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { CssBaseline, ThemeProvider } from '@mui/material';
import { BrowserRouter } from 'react-router-dom';
import { App } from './App';
import { AuthProvider } from './state/AuthContext';
import { SnackbarProvider } from './state/SnackbarContext';
import { theme } from './theme';
import './styles.css';

const savedScale = Number(localStorage.getItem('igame-font-scale') ?? 100);
if (Number.isFinite(savedScale)) document.documentElement.style.fontSize = `${16 * Math.min(125, Math.max(100, savedScale)) / 100}px`;
document.documentElement.dataset.motion = localStorage.getItem('igame-motion') === 'off' ? 'off' : 'on';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <SnackbarProvider>
        <BrowserRouter>
          <AuthProvider><App /></AuthProvider>
        </BrowserRouter>
      </SnackbarProvider>
    </ThemeProvider>
  </StrictMode>,
);
