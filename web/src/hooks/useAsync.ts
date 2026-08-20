import { useCallback, useEffect, useState } from 'react';

export function useAsync<T>(loader: () => Promise<T>, dependencies: readonly unknown[] = []) {
  const [data, setData] = useState<T>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error>();
  const reload = useCallback(async () => {
    setLoading(true); setError(undefined);
    try { const result = await loader(); setData(result); return result; }
    catch (cause) { setError(cause instanceof Error ? cause : new Error(String(cause))); return undefined; }
    finally { setLoading(false); }
  }, dependencies); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { void reload(); }, [reload]);
  return { data, loading, error, reload, setData };
}
