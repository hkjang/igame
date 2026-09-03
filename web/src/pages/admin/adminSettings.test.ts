import { describe, expect, it } from 'vitest';
import { __testing } from './AdminSettingsPage';

const { playWindows, editedWindow, withEditedWindow, playPolicyProblem } = __testing;

describe('editedWindow', () => {
  it('shows nothing when the policy holds no window', () => {
    // A policy with no window lets everyone play at every hour. Filling the
    // fields with 11:30–13:30 showed a lunch-hour rule beside a switch that was
    // on, and saving carried none of it to the server.
    expect(editedWindow({ enabled: true })).toEqual({ days: [], start: '', end: '' });
    expect(editedWindow({ enabled: true, windows: [] })).toEqual({ days: [], start: '', end: '' });
  });

  it('shows the first window the policy holds', () => {
    const play = { windows: [{ days: [1, 2], start: '09:00', end: '18:00' }, { days: [6], start: '10:00', end: '12:00' }] };
    expect(editedWindow(play)).toEqual({ days: [1, 2], start: '09:00', end: '18:00' });
  });

  it('survives a policy whose windows are not a list', () => {
    expect(playWindows({ windows: 'every day' })).toEqual([]);
    expect(editedWindow({ windows: null })).toEqual({ days: [], start: '', end: '' });
  });
});

describe('withEditedWindow', () => {
  const play = { enabled: true, windows: [{ days: [1, 2, 3, 4, 5], start: '11:30', end: '13:30' }, { days: [6, 0], start: '10:00', end: '20:00' }] };

  it('keeps the windows this screen does not show', () => {
    // Checking a day used to save an array of exactly one window, deleting the
    // weekend rule an operator had set through the API with no warning at all.
    const next = withEditedWindow(play, { days: [1, 2, 3] });
    expect(next.windows).toEqual([{ days: [1, 2, 3], start: '11:30', end: '13:30' }, { days: [6, 0], start: '10:00', end: '20:00' }]);
  });

  it('leaves the rest of the policy alone', () => {
    const next = withEditedWindow({ ...play, daily_limits: { snake: 10 } }, { start: '12:00' });
    expect(next.enabled).toBe(true);
    expect(next.daily_limits).toEqual({ snake: 10 });
  });

  it('creates the first window when the policy had none', () => {
    expect(withEditedWindow({ enabled: true }, { start: '09:00' }).windows).toEqual([{ days: [], start: '09:00', end: '' }]);
  });

  it('drops a first window the operator emptied out', () => {
    // An empty window is not a window; the server refuses one with no hours.
    const cleared = withEditedWindow({ enabled: true, windows: [{ days: [], start: '09:00', end: '' }] }, { start: '' });
    expect(cleared.windows).toEqual([]);
  });
});

describe('playPolicyProblem', () => {
  it('accepts a policy with no window and one with a whole window', () => {
    expect(playPolicyProblem({ enabled: true })).toBe('');
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [1], start: '09:00', end: '18:00' }] })).toBe('');
  });

  it('names the missing half before the server answers in English', () => {
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [1], start: '09:00' }] })).toContain('허용 시작과 종료');
    expect(playPolicyProblem({ enabled: true, windows: [{ days: [1], end: '18:00' }] })).toContain('허용 시작과 종료');
  });

  it('checks the windows this screen does not show either', () => {
    const play = { windows: [{ days: [1], start: '09:00', end: '18:00' }, { days: [6], start: '', end: '12:00' }] };
    expect(playPolicyProblem(play)).not.toBe('');
  });
});
