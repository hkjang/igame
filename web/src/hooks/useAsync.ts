import { useCallback, useEffect, useRef, useState } from 'react';

export function useAsync<T>(loader: () => Promise<T>, dependencies: readonly unknown[] = []) {
  const [data, setData] = useState<T>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error>();
  // Only the newest load may publish. Without this a slow earlier request —
  // a changed filter, a double click on reload — can resolve last and overwrite
  // the current result.
  const generation = useRef(0);
  const reload = useCallback(async () => {
    const attempt = ++generation.current;
    setLoading(true); setError(undefined);
    try {
      const result = await loader();
      if (attempt === generation.current) { setData(result); setLoading(false); }
      return result;
    } catch (cause) {
      if (attempt === generation.current) {
        setError(cause instanceof Error ? cause : new Error(String(cause)));
        setLoading(false);
      }
      return undefined;
    }
  }, dependencies); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { void reload(); }, [reload]);
  return { data, loading, error, reload, setData };
}
