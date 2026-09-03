import { describe, expect, it } from 'vitest';
import { __testing } from './AdminSettingsPage';

const { playWindows, withWindowAt, withNewWindow, withoutWindowAt, playPolicyProblem } = __testing;

describe('playWindows', () => {
  it('reads the windows a stored policy holds', () => {
    const play = { windows: [{ days: [1, 2], start: '09:00', end: '18:00' }, { days: [6], start: '10:00', end: '12:00' }] };
    expect(playWindows(play)).toHaveLength(2);
  });

  it('survives a policy with no windows, or windows that are not a list', () => {
    // A policy with no window lets everyone play at every hour. The screen has
    // to say so rather than draw a window nobody set.
    expect(playWindows({ enabled: true })).toEqual([]);
    expect(playWindows({ windows: 'every day' })).toEqual([]);
    expect(playWindows({ windows: null })).toEqual([]);
  });
});

describe('withWindowAt', () => {
  const play = { enabled: true, windows: [{ days: [1, 2, 3, 4, 5], start: '11:30', end: '13:30' }, { days: [6, 0], start: '10:00', end: '20:00' }] };

  it('edits one window and leaves the others alone', () => {
    // Checking a day used to save an array of exactly one window, deleting the
    // weekend rule an operator had set through the API with no warning at all.
    const next = withWindowAt(play, 0, { days: [1, 2, 3] });
    expect(next.windows).toEqual([{ days: [1, 2, 3], start: '11:30', end: '13:30' }, { days: [6, 0], start: '10:00', end: '20:00' }]);
  });

  it('edits a window this screen once could not reach', () => {
    const next = withWindowAt(play, 1, { end: '22:00' });
    expect(next.windows).toEqual([{ days: [1, 2, 3, 4, 5], start: '11:30', end: '13:30' }, { days: [6, 0], start: '10:00', end: '22:00' }]);
  });

  it('leaves the rest of the policy alone', () => {
    const next = withWindowAt({ ...play, daily_limits: { snake: 10 } }, 0, { start: '12:00' });
    expect(next.enabled).toBe(true);
    expect(next.daily_limits).toEqual({ snake: 10 });
  });
});

describe('withNewWindow and withoutWindowAt', () => {
  it('adds an empty window for the operator to fill in', () => {
    expect(withNewWindow({ enabled: true }).windows).toEqual([{ days: [], start: '', end: '' }]);
    const play = { windows: [{ days: [1], start: '09:00', end: '18:00' }] };
    expect(withNewWindow(play).windows).toEqual([{ days: [1], start: '09:00', end: '18:00' }, { days: [], start: '', end: '' }]);
  });

  it('removes only the window it names', () => {
    const play = { enabled: true, windows: [{ days: [1], start: '09:00', end: '18:00' }, { days: [6], start: '10:00', end: '12:00' }] };
    expect(withoutWindowAt(play, 0).windows).toEqual([{ days: [6], start: '10:00', end: '12:00' }]);
    expect(withoutWindowAt(play, 1).windows).toEqual([{ days: [1], start: '09:00', end: '18:00' }]);
    expect(withoutWindowAt(play, 0).enabled).toBe(true);
  });
});

describe('playPolicyProblem', () => {
  it('accepts a policy with no window and one with a whole window', () => {
    expect(playPolicyProblem({ enabled: true })).toBe('');
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [1], start: '09:00', end: '18:00' }] })).toBe('');
  });

  it('accepts a window that runs past midnight', () => {
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [5], start: '22:00', end: '02:00' }] })).toBe('');
  });

  it('names the missing half before the server answers in English', () => {
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [1], start: '09:00' }] })).toContain('시작과 종료 시각을 모두');
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [1], end: '18:00' }] })).toContain('시작과 종료 시각을 모두');
  });

  it('refuses a window that ends at the minute it opens', () => {
    // Two identical times draw a window of no length, and the server used to
    // read one as the whole day: the tightest looking policy allowed the most.
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [1], start: '09:00', end: '09:00' }] })).toContain('시각이 같은 시간대');
  });

  it('checks every window, including the ones added last', () => {
    const play = { windows: [{ days: [1], start: '09:00', end: '18:00' }, { days: [6], start: '', end: '12:00' }] };
    expect(playPolicyProblem(play)).not.toBe('');
  });
});
