import { createContext, type ReactNode, useCallback, useContext, useMemo, useRef, useState } from 'react';
import { Alert, Snackbar } from '@mui/material';

type Severity = 'success' | 'info' | 'warning' | 'error';
interface Notice { id: number; message: string; severity: Severity }
interface SnackbarValue { notify: (message: string, severity?: Severity) => void }

const SnackbarContext = createContext<SnackbarValue>({ notify: () => undefined });

/**
 * Errors and warnings stay until they are dismissed. Confirmations describe
 * something the user just did and can fade on their own.
 */
function autoHideFor(severity: Severity): number | null {
  return severity === 'error' || severity === 'warning' ? null : 5000;
}

export function SnackbarProvider({ children }: { children: ReactNode }) {
  // Notices queue rather than overwrite: two results arriving together used to
  // mean the first was replaced before it could be read.
  const [queue, setQueue] = useState<Notice[]>([]);
  const nextId = useRef(0);
  const current = queue[0];

  const notify = useCallback((message: string, severity: Severity = 'info') => {
    const id = nextId.current++;
    setQueue((pending) => [...pending, { id, message, severity }]);
  }, []);

  const dismiss = useCallback((reason?: string) => {
    // A stray click elsewhere should not throw away a message still being read.
    if (reason === 'clickaway') return;
    setQueue((pending) => pending.slice(1));
  }, []);

  const value = useMemo(() => ({ notify }), [notify]);
  return (
    <SnackbarContext.Provider value={value}>
      {children}
      <Snackbar
        key={current?.id}
        open={Boolean(current)}
        autoHideDuration={current ? autoHideFor(current.severity) : null}
        onClose={(_, reason) => dismiss(reason)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={() => dismiss()}
          severity={current?.severity ?? 'info'}
          variant="filled"
          sx={{ minWidth: 300, maxWidth: 'min(560px, 92vw)' }}
        >
          {current?.message}
        </Alert>
      </Snackbar>
    </SnackbarContext.Provider>
  );
}

export const useSnackbar = () => useContext(SnackbarContext);
