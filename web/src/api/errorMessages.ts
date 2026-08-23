/**
 * Korean text for the error codes a portal user can actually reach. The API
 * answers in English because it also serves scripts and the MCP endpoint, so
 * anything a person sees is translated here.
 *
 * Codes that are missing fall back to the server's own message on purpose:
 * content validation in the admin studios reports the exact field that failed,
 * and that detail is more useful than a generic sentence.
 */
const USER_FACING_MESSAGES: Record<string, string> = {
  // 공통
  network_error: '서버에 연결할 수 없습니다. 네트워크와 서비스 상태를 확인해 주세요.',
  internal_error: '서버에서 문제가 발생했습니다. 잠시 후 다시 시도해 주세요.',
  database_unavailable: '서비스가 아직 준비되지 않았습니다. 잠시 후 다시 시도해 주세요.',
  not_found: '요청한 항목을 찾을 수 없습니다.',
  unauthorized: '로그인이 필요합니다.',
  forbidden: '이 작업을 수행할 권한이 없습니다.',
  insufficient_scope: '사용 중인 키에 이 작업에 필요한 권한이 없습니다.',
  csrf_rejected: '허용되지 않은 출처의 요청입니다. 페이지를 새로 고친 뒤 다시 시도해 주세요.',

  // 로그인
  invalid_credentials: '아이디 또는 비밀번호가 올바르지 않습니다.',
  too_many_attempts: '로그인 시도가 너무 많습니다. 잠시 후 다시 시도해 주세요.',
  local_login_disabled: '관리자 로그인이 비활성화되어 있습니다. 사내 SSO로 로그인해 주세요.',
  session_required: '이 작업은 브라우저 로그인 세션에서만 할 수 있습니다.',
  oidc_disabled: '사내 SSO가 설정되어 있지 않습니다. 서비스 관리자에게 문의하세요.',
  oidc_discovery_failed: '사내 SSO 서버에 연결할 수 없습니다. 잠시 후 다시 시도해 주세요.',

  // 비밀번호
  weak_password: '새 비밀번호는 12자 이상이어야 합니다.',
  password_unchanged: '새 비밀번호가 현재 비밀번호와 같습니다.',
  invalid_current_password: '현재 비밀번호가 올바르지 않습니다.',

  // 개인 키
  key_limit: '활성 키 개수 한도에 도달했습니다. 사용하지 않는 키를 폐기한 뒤 다시 시도해 주세요.',
  invalid_expiry: '만료일이 허용된 최대 사용 기간을 벗어났습니다.',
  invalid_permissions: '선택한 권한 중 이 역할에 허용되지 않은 항목이 있습니다.',

  // 게임 세션과 점수
  invalid_session: '게임 세션이 만료되었거나 유효하지 않습니다. 게임을 다시 시작해 주세요.',
  session_finished: '이미 종료된 세션입니다.',
  session_rate_limited: '세션을 너무 자주 시작했습니다. 잠시 후 다시 시도해 주세요.',
  duplicate_score: '이 세션의 점수는 이미 기록되었습니다.',
  play_policy_denied: '지금은 플레이할 수 있는 시간이 아니거나 오늘의 플레이 한도를 초과했습니다.',
  authoritative_result_required: '이 게임의 기록은 게임 화면을 통해서만 저장됩니다.',
  defense_authoritative_result_required: '이 게임의 기록은 게임 화면을 통해서만 저장됩니다.',
  hero_locked: '아직 해금되지 않은 영웅입니다.',
  skill_locked: '아직 해금되지 않은 스킬입니다.',
  stage_locked: '아직 해금되지 않은 스테이지입니다.',
  education_not_enabled: '이 콘텐츠에는 교육 이벤트가 없습니다.',
  organization_ranking_hidden: '조직 단위 랭킹은 현재 공개되어 있지 않습니다.',

  // 게시된 콘텐츠 고정
  realmguard_version_required: '게임 콘텐츠 정보를 읽지 못했습니다. 페이지를 새로 고쳐 주세요.',
  defense_version_required: '게임 콘텐츠 정보를 읽지 못했습니다. 페이지를 새로 고쳐 주세요.',
  realmguard_config_stale: '게임 콘텐츠가 업데이트되었습니다. 페이지를 새로 고친 뒤 다시 시작해 주세요.',
  defense_config_stale: '게임 콘텐츠가 업데이트되었습니다. 페이지를 새로 고친 뒤 다시 시작해 주세요.',

  // AI
  ai_disabled: 'AI 기능이 설정되어 있지 않습니다. 서비스 관리자에게 문의하세요.',
  ai_upstream_unavailable: 'AI 서버에 연결할 수 없습니다. 잠시 후 다시 시도해 주세요.',
  max_tokens_exceeded: '요청한 토큰 수가 허용 한도를 초과했습니다.',

  // 검토와 승인
  team_required: '검토하려면 팀 정보가 필요합니다. 서비스 관리자에게 문의하세요.',
  different_team: '같은 팀에서 작성한 요청만 검토할 수 있습니다.',
  self_approval_forbidden: '본인이 올린 요청은 본인이 승인할 수 없습니다.',
  comment_required: '반려 사유를 입력해 주세요.',
  not_pending: '이미 처리된 요청입니다.',
  approval_not_enabled: '승인 흐름이 켜져 있지 않습니다.',
  stale_version: '다른 사용자가 먼저 수정했습니다. 최신 내용을 불러온 뒤 다시 시도해 주세요.',
};

/** Formats a Retry-After header into a Korean wait hint, when it carries one. */
export function retryHint(retryAfterSeconds: number | undefined): string {
  if (retryAfterSeconds === undefined || !Number.isFinite(retryAfterSeconds) || retryAfterSeconds <= 0) return '';
  if (retryAfterSeconds < 60) return ` 약 ${Math.ceil(retryAfterSeconds)}초 후에 다시 시도할 수 있습니다.`;
  return ` 약 ${Math.ceil(retryAfterSeconds / 60)}분 후에 다시 시도할 수 있습니다.`;
}

/**
 * Resolves the message to show for an API failure. `fallback` is whatever the
 * server said, which is kept for every code this table does not cover.
 */
export function localizedMessage(code: string | undefined, fallback: string, retryAfterSeconds?: number): string {
  const translated = code ? USER_FACING_MESSAGES[code] : undefined;
  if (!translated) return fallback;
  const hint = retryHint(retryAfterSeconds);
  // A wait hint replaces the generic "try again later" tail rather than stacking on it.
  return hint ? translated.replace(/\s*잠시 후 다시 시도해 주세요\.$/, '') + hint : translated;
}
