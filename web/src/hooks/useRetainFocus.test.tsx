import { afterEach, describe, expect, it } from 'vitest';
import { useState } from 'react';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useRetainFocus } from './useRetainFocus';

afterEach(cleanup);

function Form({ onSubmit }: { onSubmit?: () => void }) {
  const [busy, setBusy] = useState(false);
  const ref = useRetainFocus<HTMLButtonElement>(busy);
  return (
    <>
      <input aria-label="somewhere else" />
      <button ref={ref} disabled={busy} onClick={() => { setBusy(true); onSubmit?.(); }}>저장</button>
      <button onClick={() => setBusy(false)}>finish</button>
    </>
  );
}

/**
 * What a browser does when a focused element becomes disabled: it fires blur and
 * focus lands on the body. jsdom leaves focus on the disabled element, so the
 * two halves are staged by hand.
 */
const disableBlur = (button: HTMLElement) => act(() => {
  fireEvent.blur(button);
  const parked = document.createElement('input');
  document.body.append(parked);
  parked.focus();
  parked.remove();
});

describe('useRetainFocus', () => {
  it('gives focus back to a button that was disabled out from under it', () => {
    render(<Form />);
    const save = screen.getByRole('button', { name: '저장' });
    act(() => save.focus());
    expect(document.activeElement).toBe(save);

    act(() => { fireEvent.click(save); });
    disableBlur(save);
    expect(document.activeElement).toBe(document.body);

    act(() => { fireEvent.click(screen.getByRole('button', { name: 'finish' })); });
    expect(document.activeElement).toBe(save);
  });

  it('leaves focus alone when the form moved it somewhere deliberate', () => {
    render(<Form />);
    const save = screen.getByRole('button', { name: '저장' });
    act(() => save.focus());
    act(() => { fireEvent.click(save); });
    disableBlur(save);
    const elsewhere = screen.getByLabelText('somewhere else');
    act(() => elsewhere.focus());
    act(() => { fireEvent.click(screen.getByRole('button', { name: 'finish' })); });
    expect(document.activeElement).toBe(elsewhere);
  });

  it('does nothing when the user blurred the button themselves', () => {
    render(<Form />);
    const save = screen.getByRole('button', { name: '저장' });
    act(() => save.focus());
    act(() => save.blur());          // still enabled: the user tabbed away
    act(() => { fireEvent.click(save); });
    act(() => { fireEvent.click(screen.getByRole('button', { name: 'finish' })); });
    expect(document.activeElement).not.toBe(save);
  });
});
