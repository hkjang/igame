import { createContext, type ReactNode, useCallback, useContext, useMemo, useState } from 'react';
import { Alert, Snackbar } from '@mui/material';

type Severity = 'success' | 'info' | 'warning' | 'error';
interface Notice { message: string; severity: Severity }
interface SnackbarValue { notify: (message: string, severity?: Severity) => void }

const SnackbarContext = createContext<SnackbarValue>({ notify: () => undefined });

export function SnackbarProvider({ children }: { children: ReactNode }) {
  const [notice, setNotice] = useState<Notice | null>(null);
  const notify = useCallback((message: string, severity: Severity = 'info') => setNotice({ message, severity }), []);
  const value = useMemo(() => ({ notify }), [notify]);
  return (
    <SnackbarContext.Provider value={value}>
      {children}
      <Snackbar open={Boolean(notice)} autoHideDuration={5000} onClose={() => setNotice(null)} anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
        <Alert onClose={() => setNotice(null)} severity={notice?.severity ?? 'info'} variant="filled" sx={{ minWidth: 300 }}>
          {notice?.message}
        </Alert>
      </Snackbar>
    </SnackbarContext.Provider>
  );
}

export const useSnackbar = () => useContext(SnackbarContext);
