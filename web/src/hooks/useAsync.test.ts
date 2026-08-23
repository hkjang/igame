import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useAsync } from './useAsync';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

describe('useAsync', () => {
  it('publishes the loaded value', async () => {
    const { result } = renderHook(() => useAsync(async () => 'loaded'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBe('loaded');
    expect(result.current.error).toBeUndefined();
  });

  it('ignores a superseded load that resolves last', async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const loads = [first.promise, second.promise];
    let index = 0;
    const { result } = renderHook(() => useAsync(() => loads[index++]));

    await act(async () => { void result.current.reload(); });
    await act(async () => { second.resolve('newest'); await second.promise; });
    expect(result.current.data).toBe('newest');

    await act(async () => { first.resolve('stale'); await first.promise; });
    expect(result.current.data).toBe('newest');
    expect(result.current.loading).toBe(false);
  });

  it('ignores a superseded failure', async () => {
    const failing = deferred<string>();
    const succeeding = deferred<string>();
    const loads = [failing.promise, succeeding.promise];
    let index = 0;
    const { result } = renderHook(() => useAsync(() => loads[index++]));

    await act(async () => { void result.current.reload(); });
    await act(async () => { succeeding.resolve('newest'); await succeeding.promise; });
    await act(async () => {
      failing.reject(new Error('stale failure'));
      await failing.promise.catch(() => undefined);
    });

    expect(result.current.error).toBeUndefined();
    expect(result.current.data).toBe('newest');
  });

  it('reports the newest failure', async () => {
    const { result } = renderHook(() => useAsync(async () => { throw new Error('boom'); }));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error?.message).toBe('boom');
  });
});
