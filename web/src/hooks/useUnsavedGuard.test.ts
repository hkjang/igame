import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useUnsavedGuard } from './useUnsavedGuard';

describe('useUnsavedGuard', () => {
  it('runs the action immediately when nothing is unsaved', () => {
    const action = vi.fn();
    const { result } = renderHook(() => useUnsavedGuard(false));
    act(() => result.current.guard(action));
    expect(action).toHaveBeenCalledTimes(1);
    expect(result.current.askingToDiscard).toBe(false);
  });

  it('holds the action back until the author discards', () => {
    const action = vi.fn();
    const { result } = renderHook(() => useUnsavedGuard(true));
    act(() => result.current.guard(action));
    expect(action).not.toHaveBeenCalled();
    expect(result.current.askingToDiscard).toBe(true);

    act(() => result.current.discard());
    expect(action).toHaveBeenCalledTimes(1);
    expect(result.current.askingToDiscard).toBe(false);
  });

  it('drops the action when the author keeps editing', () => {
    const action = vi.fn();
    const { result } = renderHook(() => useUnsavedGuard(true));
    act(() => result.current.guard(action));
    act(() => result.current.keepEditing());
    expect(action).not.toHaveBeenCalled();
    expect(result.current.askingToDiscard).toBe(false);
  });

  it('warns before unload only while there are unsaved edits', () => {
    const add = vi.spyOn(window, 'addEventListener');
    const remove = vi.spyOn(window, 'removeEventListener');
    const { rerender, unmount } = renderHook(({ dirty }) => useUnsavedGuard(dirty), { initialProps: { dirty: false } });
    expect(add).not.toHaveBeenCalledWith('beforeunload', expect.any(Function));

    rerender({ dirty: true });
    expect(add).toHaveBeenCalledWith('beforeunload', expect.any(Function));

    rerender({ dirty: false });
    expect(remove).toHaveBeenCalledWith('beforeunload', expect.any(Function));
    unmount();
  });
});
