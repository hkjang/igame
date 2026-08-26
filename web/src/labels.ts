/**
 * Korean for the values the platform stores.
 *
 * The whole product is written in Korean and then leaked its schema wherever a
 * stored value was rendered straight into the page: a status chip said
 * `pending_approval`, a game's 실행 방식 said `iframe`, and the Defense games'
 * own HUD sat a 상태 ready between 웨이브 1/8 and 12 차단.
 *
 * Keyed by the value, because a value that appears in several places means the
 * same thing in each. Where it does not — a game that is 서비스 중 against a
 * user who is 활성 — the caller passes its own wording.
 */
export const OPTION_LABELS: Record<string, string> = {
  // Row lifecycle, shared by games, seasons, events, tournaments and notices.
  draft: '초안',
  active: '활성',
  maintenance: '점검 중',
  disabled: '사용 중지',
  closed: '종료',
  cancelled: '취소',
  published: '게시 중',

  // Score moderation.
  valid: '정상',
  flagged: '검토 필요',
  excluded: '집계 제외',

  // How a game is run.
  embedded: '내장 실행',
  iframe: 'iframe 삽입',
  external: '외부 링크',

  // Which end of the leaderboard wins.
  desc: '높은 점수가 상위',
  asc: '낮은 점수가 상위',

  // Roles.
  user: '일반',
  manager: '매니저',
  operator: '운영자',
  admin: '관리자',

  // How an event or a tournament is contested.
  score_attack: '점수 경쟁',
  time_attack: '기록 경쟁',
  team_battle: '팀 대항',
  department_battle: '부서 대항',
  attendance: '출석',
  survival: '생존',
  bracket: '토너먼트',

  // Reward kinds.
  badge: '배지',
  title: '칭호',
  avatar_frame: '아바타 프레임',

  // A change request moving through review.
  pending: '검토 대기',
  approved: '승인됨',
  rejected: '반려됨',
  applied: '반영됨',

  // A content version moving towards release.
  testing: '검증 중',
  pending_approval: '승인 대기',
  archived: '보관',

  // What a battle is doing right now.
  ready: '준비',
  playing: '진행 중',
  paused: '일시정지',
  victory: '승리',
  defeat: '패배',
};

/**
 * The label for one stored value.
 *
 * Falls back to the value itself rather than to a blank or a placeholder: a
 * value with no label is usually one left behind by an older schema, and it is
 * exactly what the person looking at the screen needs to see.
 */
export function optionLabel(value: unknown, overrides?: Record<string, string>): string {
  const key = String(value);
  return overrides?.[key] ?? OPTION_LABELS[key] ?? key;
}
