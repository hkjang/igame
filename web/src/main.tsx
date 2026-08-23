import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { App } from './App';
import { AuthProvider } from './state/AuthContext';
import { applyPreferences, loadPreferences } from './state/preferences';
import { SnackbarProvider } from './state/SnackbarContext';
import { ThemeModeProvider } from './state/ThemeModeContext';
import './styles.css';

applyPreferences(loadPreferences());

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeModeProvider>
      <SnackbarProvider>
        <BrowserRouter>
          <AuthProvider><App /></AuthProvider>
        </BrowserRouter>
      </SnackbarProvider>
    </ThemeModeProvider>
  </StrictMode>,
);
