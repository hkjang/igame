import { describe, expect, it } from 'vitest';
import { documentTitleForPath, titleForPath } from './routeTitles';

describe('routeTitles', () => {
  it('names portal and admin routes', () => {
    expect(titleForPath('/')).toBe('홈');
    expect(titleForPath('/games')).toBe('모든 게임');
    expect(titleForPath('/notices')).toBe('공지사항');
    expect(titleForPath('/games/realmguard')).toBe('게임 플레이');
    expect(titleForPath('/profile/keys')).toBe('개인 키 관리');
    expect(titleForPath('/admin')).toBe('관리 대시보드');
    expect(titleForPath('/admin/defense')).toBe('Defense Content Studio');
    expect(titleForPath('/defense/cyber-fortress/preview/abc')).toBe('Defense Series 미리보기');
  });

  it('does not let a nested path borrow a parent title', () => {
    expect(titleForPath('/games/snake/extra')).toBe('페이지를 찾을 수 없습니다');
    expect(titleForPath('/admin/unknown')).toBe('페이지를 찾을 수 없습니다');
  });

  it('ignores a trailing slash', () => {
    expect(titleForPath('/games/')).toBe('모든 게임');
    expect(titleForPath('/')).toBe('홈');
  });

  it('composes the tab title with the configured service name', () => {
    expect(documentTitleForPath('/rankings', '사내 게임')).toBe('랭킹 · 사내 게임');
  });
});
