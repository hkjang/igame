import { useCallback, useEffect, useState } from 'react';

/**
 * Holds back an action that would throw away unsaved edits until the author
 * confirms it.
 *
 * The content studios load a section into a JSON editor and reload it whenever
 * the section, game or version changes. Without this, switching a dropdown
 * silently discarded however long someone had spent editing.
 */
export function useUnsavedGuard(dirty: boolean) {
  const [pending, setPending] = useState<{ run: () => void } | null>(null);

  // Covers leaving the app entirely: closing the tab or a hard reload.
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  const guard = useCallback((action: () => void) => {
    if (dirty) setPending({ run: action });
    else action();
  }, [dirty]);

  const discard = useCallback(() => {
    setPending((current) => { current?.run(); return null; });
  }, []);

  const keepEditing = useCallback(() => setPending(null), []);

  return { guard, askingToDiscard: pending !== null, discard, keepEditing };
}
