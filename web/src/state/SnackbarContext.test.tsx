import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { SnackbarProvider, useSnackbar } from './SnackbarContext';

// vitest runs without `globals`, so testing-library's automatic cleanup is
// never registered and renders would otherwise leak between tests.
afterEach(cleanup);

function Harness() {
  const { notify } = useSnackbar();
  return (
    <>
      <button onClick={() => notify('첫 번째', 'success')}>first</button>
      <button onClick={() => notify('두 번째', 'success')}>second</button>
      <button onClick={() => notify('실패했습니다', 'error')}>fail</button>
    </>
  );
}

function setup() {
  return render(<SnackbarProvider><Harness /></SnackbarProvider>);
}

describe('SnackbarProvider', () => {
  it('queues notices so a second one cannot erase the first', async () => {
    setup();
    await act(async () => {
      screen.getByText('first').click();
      screen.getByText('second').click();
    });
    expect(screen.getByText('첫 번째')).toBeInTheDocument();
    expect(screen.queryByText('두 번째')).not.toBeInTheDocument();

    await act(async () => { screen.getByLabelText('Close').click(); });
    expect(screen.getByText('두 번째')).toBeInTheDocument();
  });

  it('leaves an error on screen instead of hiding it on a timer', async () => {
    setup();
    await act(async () => { screen.getByText('fail').click(); });
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('실패했습니다');
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 50)); });
    expect(screen.getByText('실패했습니다')).toBeInTheDocument();
  });
});
