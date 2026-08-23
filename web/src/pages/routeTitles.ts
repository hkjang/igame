/** Page names used for the browser tab and the route-change announcement. */
const ROUTE_TITLES: Array<[RegExp, string]> = [
  [/^\/$/, '홈'],
  [/^\/login$/, '로그인'],
  [/^\/games$/, '모든 게임'],
  [/^\/games\/[^/]+$/, '게임 플레이'],
  [/^\/rankings$/, '랭킹'],
  [/^\/events$/, '이벤트'],
  [/^\/notices$/, '공지사항'],
  [/^\/ai$/, 'AI Game Lab'],
  [/^\/reviews$/, '승인함'],
  [/^\/developer$/, '개발자 센터'],
  [/^\/realmguard\/preview\/[^/]+$/, 'RealmGuard 미리보기'],
  [/^\/defense\/[^/]+\/preview\/[^/]+$/, 'Defense Series 미리보기'],
  [/^\/profile$/, '내 프로필'],
  [/^\/profile\/keys$/, '개인 키 관리'],
  [/^\/profile\/preferences$/, '개인화 설정'],
  [/^\/admin$/, '관리 대시보드'],
  [/^\/admin\/games$/, '게임 관리'],
  [/^\/admin\/categories$/, '카테고리 관리'],
  [/^\/admin\/users$/, '사용자 관리'],
  [/^\/admin\/rankings$/, '랭킹 관리'],
  [/^\/admin\/seasons$/, '시즌 관리'],
  [/^\/admin\/events$/, '이벤트 관리'],
  [/^\/admin\/tournaments$/, '대회 관리'],
  [/^\/admin\/achievements$/, '업적 관리'],
  [/^\/admin\/rewards$/, '보상 관리'],
  [/^\/admin\/notices$/, '공지 관리'],
  [/^\/admin\/banners$/, '배너 관리'],
  [/^\/admin\/analytics$/, '통계'],
  [/^\/admin\/realmguard$/, 'RealmGuard Designer'],
  [/^\/admin\/defense$/, 'Defense Content Studio'],
  [/^\/admin\/audit$/, '감사 로그'],
  [/^\/admin\/approvals$/, '검토·승인 설정'],
  [/^\/admin\/keys$/, '키 권한 설정'],
  [/^\/admin\/security$/, 'OIDC·보안 설정'],
  [/^\/admin\/ai$/, 'AI 설정'],
  [/^\/admin\/settings$/, '시스템 설정'],
];

/** Resolves the page name for a pathname, falling back for unmapped routes. */
export function titleForPath(pathname: string): string {
  const normalized = pathname.length > 1 ? pathname.replace(/\/+$/, '') || '/' : pathname;
  for (const [pattern, title] of ROUTE_TITLES) {
    if (pattern.test(normalized)) return title;
  }
  return '페이지를 찾을 수 없습니다';
}

/** Composes the browser tab title. */
export function documentTitleForPath(pathname: string, serviceName: string): string {
  return `${titleForPath(pathname)} · ${serviceName}`;
}
