import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyText } from './clipboard';

function setClipboard(value: unknown) {
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value });
}

// jsdom implements neither of these, so both are installed explicitly.
function setExecCommand(value: unknown) {
  Object.defineProperty(document, 'execCommand', { configurable: true, value });
}

afterEach(() => {
  vi.restoreAllMocks();
  setClipboard(undefined);
  setExecCommand(undefined);
});

describe('copyText', () => {
  it('uses the async clipboard when the page is a secure context', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });
    await expect(copyText('igk_secret')).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith('igk_secret');
  });

  it('falls back when the clipboard API is absent, as it is over plain HTTP', async () => {
    const exec = vi.fn().mockReturnValue(true);
    setExecCommand(exec);
    await expect(copyText('igk_secret')).resolves.toBe(true);
    expect(exec).toHaveBeenCalledWith('copy');
  });

  it('falls back when the clipboard API rejects', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) });
    setExecCommand(vi.fn().mockReturnValue(true));
    await expect(copyText('igk_secret')).resolves.toBe(true);
  });

  it('reports failure instead of claiming a copy that did not happen', async () => {
    setExecCommand(vi.fn().mockReturnValue(false));
    await expect(copyText('igk_secret')).resolves.toBe(false);
  });

  it('reports failure when no copy mechanism exists at all', async () => {
    await expect(copyText('igk_secret')).resolves.toBe(false);
  });

  it('leaves no stray node behind when the fallback throws', async () => {
    setExecCommand(vi.fn().mockImplementation(() => { throw new Error('boom'); }));
    const before = document.body.childElementCount;
    await expect(copyText('igk_secret')).resolves.toBe(false);
    expect(document.body.childElementCount).toBe(before);
  });

  it('does nothing for an empty value', async () => {
    const writeText = vi.fn();
    setClipboard({ writeText });
    await expect(copyText('')).resolves.toBe(false);
    expect(writeText).not.toHaveBeenCalled();
  });
});
