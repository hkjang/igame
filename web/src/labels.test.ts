import { describe, expect, it } from 'vitest';
import { OPTION_LABELS, optionLabel } from './labels';

describe('optionLabel', () => {
  it('shows a value it has no label for rather than nothing', () => {
    // A value with no label is usually one an older schema left behind, and it
    // is exactly what the person looking at the screen needs to see.
    expect(optionLabel('retired')).toBe('retired');
    expect(optionLabel(undefined)).toBe('undefined');
  });

  it('lets a caller say what the shared wording gets wrong', () => {
    expect(optionLabel('active')).toBe('활성');
    expect(optionLabel('active', { active: '서비스 중' })).toBe('서비스 중');
  });

  it('covers every state a battle and a content version can be in', () => {
    // These reach a player and an operator directly, with no field definition
    // in between to catch a value that was never given Korean.
    for (const status of ['ready', 'playing', 'paused', 'victory', 'defeat'])
      expect(OPTION_LABELS[status]).toBeTruthy();
    for (const status of ['draft', 'testing', 'pending_approval', 'approved', 'published', 'archived'])
      expect(OPTION_LABELS[status]).toBeTruthy();
    for (const status of ['pending', 'approved', 'rejected', 'applied'])
      expect(OPTION_LABELS[status]).toBeTruthy();
  });

  it('is Korean throughout', () => {
    const notKorean = Object.entries(OPTION_LABELS)
      .filter(([, label]) => !/[가-힣]/.test(label))
      .map(([value]) => value);
    // iframe 삽입 keeps the word an operator has to type; everything else that
    // carries no Korean at all is a value that was never translated.
    expect(notKorean).toEqual([]);
  });
});
