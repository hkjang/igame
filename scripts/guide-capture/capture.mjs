#!/usr/bin/env node
// Takes the screen captures docs/USER_GUIDE.md and docs/ADMIN_GUIDE.md embed.
//
// It resolves Playwright from the working directory the same way
// scripts/browser-smoke.mjs does, so it runs inside the pinned browser image
// the release gate already uses instead of adding a dependency to this
// repository.
//
// Before capturing it creates the notices, events and season the screens show,
// through the same API an operator uses, so the audit trail the admin guide
// shows is a real one. It writes no system setting: a capture run must not be
// able to change how a service is configured. The users and finished sessions
// the API cannot create come from scripts/guide-capture/seed.sql, applied
// before this script starts.
import { createRequire } from 'node:module';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';

const requireFromWorkingDirectory = createRequire(`${process.cwd()}/package.json`);
let chromium;
try {
  ({ chromium } = requireFromWorkingDirectory('playwright'));
} catch (error) {
  console.error('Playwright is not installed in the working directory. Install playwright@1.55.0 with browser download disabled, then retry.');
  throw error;
}

const baseURL = (process.env.IGAME_GUIDE_CAPTURE_URL ?? '').replace(/\/$/, '');
const username = process.env.IGAME_GUIDE_CAPTURE_USER ?? '';
const password = process.env.IGAME_GUIDE_CAPTURE_PASSWORD ?? '';
const outputDirectory = process.env.IGAME_GUIDE_CAPTURE_DIR ?? '';
if (!baseURL || !username || !password || !outputDirectory) {
  console.error('Usage: IGAME_GUIDE_CAPTURE_URL=… IGAME_GUIDE_CAPTURE_USER=… IGAME_GUIDE_CAPTURE_PASSWORD=… IGAME_GUIDE_CAPTURE_DIR=… node scripts/guide-capture/capture.mjs');
  process.exit(2);
}

// The desktop size the guide standard fixes. Captures taken at another width
// would not line up with the rest of the guide set.
const viewport = { width: 1440, height: 900 };
const dayMs = 24 * 60 * 60 * 1000;
const at = (days) => new Date(Date.now() + days * dayMs).toISOString();

await mkdir(outputDirectory, { recursive: true });

const browser = await chromium.launch({ headless: true, args: ['--disable-dev-shm-usage'] });
const captured = [];
try {
  const context = await browser.newContext({
    viewport,
    // The portal follows the operating system by default, and its dark screen is
    // what the product is shown in everywhere else.
    colorScheme: 'dark',
    locale: 'ko-KR',
    timezoneId: 'Asia/Seoul',
    reducedMotion: 'reduce',
  });
  const page = await context.newPage();

  async function api(method, path, body) {
    const response = await context.request.fetch(`${baseURL}${path}`, {
      method,
      ...(body === undefined ? {} : { data: body }),
    });
    if (!response.ok()) throw new Error(`${method} ${path} returned HTTP ${response.status()}: ${await response.text()}`);
    return response.status() === 204 ? null : response.json();
  }

  await api('POST', '/api/v1/auth/login', { username, password });

  const notices = [
    ['9월 사내 게임 리그 참가 신청 안내', '9월 15일부터 26일까지 부서 대항 리그를 진행합니다. 참가 신청은 이벤트 화면에서 직접 할 수 있으며, 부서별 상위 3인의 점수가 합산됩니다.', true],
    ['점심시간 플레이 시간 정책 변경', '플레이 허용 시간이 12:00~13:00과 18:00 이후로 조정되었습니다. 허용 시간 밖에서는 연습 모드로만 실행되며 기록은 남지 않습니다.', false],
    ['RealmGuard 콘텐츠 0.3.1 게시', '타워 분기 재조정과 영웅별 초상·전장 실루엣이 반영되었습니다. 기존 캠페인 진행도와 랭킹은 그대로 유지됩니다.', false],
  ];
  for (const [title, content, pinned] of notices) {
    await api('POST', '/api/v1/admin/notices', { title, content, status: 'published', pinned });
  }

  const events = [
    ['9월 부서 대항 스코어 어택', '부서별 상위 3인의 점수를 합산해 순위를 가립니다.', -3, 11, 'active'],
    ['점심시간 타임어택 토너먼트', '12:00~13:00 사이의 기록만 집계하는 짧은 토너먼트입니다.', -1, 6, 'active'],
    ['신규 입사자 환영 이벤트', '입사 3개월 이내 구성원 전용 이벤트입니다.', 7, 21, 'draft'],
  ];
  for (const [name, description, from, to, status] of events) {
    await api('POST', '/api/v1/admin/events', {
      name, description, event_type: 'score_attack', starts_at: at(from), ends_at: at(to), status,
    });
  }

  await api('POST', '/api/v1/admin/seasons', {
    name: '2026 3분기 시즌',
    description: '분기 단위로 랭킹을 초기화하고 상위 기록에 뱃지를 부여합니다.',
    starts_at: at(-20), ends_at: at(40), status: 'active',
  });

  // Two personal API keys, so the key screen is not captured empty. The values
  // they return are never displayed again and the service is thrown away with
  // them.
  for (const [name, permissions] of [
    ['사내 대시보드 연동', ['api:access', 'rankings:read', 'games:read']],
    ['MCP 클라이언트', ['mcp:access', 'games:read']],
  ]) {
    await api('POST', '/api/v1/me/api-keys', { name, permissions });
  }

  // A RealmGuard draft, so the Designer shows its editor rather than the empty
  // state. Creating a draft publishes nothing.
  await api('POST', '/api/v1/admin/realmguard/versions', {
    label: '9월 밸런스 조정 초안',
    notes: '9~10 스테이지의 보스 체력과 타워 분기 비용을 다시 맞춥니다.',
  });

  // One update as well, so the audit trail the admin guide shows is not made up
  // entirely of creations.
  const [pinnedNotice] = (await api('GET', '/api/v1/admin/notices?limit=1')).items;
  await api('PUT', `/api/v1/admin/notices/${pinnedNotice.id}`, {
    title: pinnedNotice.title,
    content: `${pinnedNotice.content} 참가 신청 마감은 9월 12일입니다.`,
    status: 'published',
    pinned: true,
  });

  async function settle() {
    await page.waitForLoadState('networkidle', { timeout: 30_000 }).catch(() => {});
    // MUI fades content in; a capture taken on the load event catches it
    // half-transparent.
    await page.waitForTimeout(1_200);
  }

  async function shot(name) {
    await settle();
    await page.screenshot({ path: join(outputDirectory, `${name}.png`), animations: 'disabled' });
    captured.push(name);
    console.log(`captured ${name}.png`);
  }

  async function visit(path, name) {
    await page.goto(`${baseURL}${path}`, { waitUntil: 'domcontentloaded' });
    await shot(name);
  }

  // The login capture must not come from a session that is already open, so it
  // is taken in a context of its own.
  const anonymous = await browser.newContext({ viewport, colorScheme: 'dark', locale: 'ko-KR', timezoneId: 'Asia/Seoul', reducedMotion: 'reduce' });
  const loginPage = await anonymous.newPage();
  await loginPage.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' });
  await loginPage.getByRole('heading', { name: '로그인' }).waitFor({ state: 'visible', timeout: 30_000 });
  await loginPage.getByLabel('관리자 아이디').fill(username);
  await loginPage.waitForTimeout(1_200);
  await loginPage.screenshot({ path: join(outputDirectory, 'login.png'), animations: 'disabled' });
  captured.push('login');
  console.log('captured login.png');
  await anonymous.close();

  await visit('/', 'home');
  await visit('/games', 'games');

  // A real play session, started the way a player starts one, and played far
  // enough that the board and the score show what a player actually sees.
  await page.goto(`${baseURL}/games/2048`, { waitUntil: 'domcontentloaded' });
  await settle();
  await page.getByRole('button', { name: '게임 시작' }).click();
  await page.waitForTimeout(800);
  for (const key of ['ArrowLeft', 'ArrowDown', 'ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp', 'ArrowLeft', 'ArrowDown', 'ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowDown']) {
    await page.keyboard.press(key);
    await page.waitForTimeout(160);
  }
  await shot('game-play');

  await visit('/rankings', 'rankings');
  await visit('/events', 'events');
  await visit('/notices', 'notices');
  await visit('/profile', 'profile');
  await visit('/profile/keys', 'profile-keys');
  await visit('/profile/preferences', 'profile-preferences');

  await visit('/admin', 'admin-dashboard');
  // The service status card is the first diagnostic point on an image with no
  // shell, and it sits below the fold at this window size.
  await page.mouse.wheel(0, 820);
  await page.waitForTimeout(800);
  await shot('admin-dashboard-status');
  await visit('/admin/games', 'admin-games');
  await visit('/admin/users', 'admin-users');
  await visit('/admin/audit', 'admin-audit');
  await visit('/admin/analytics', 'admin-analytics');
  await visit('/admin/notices', 'admin-notices');
  await visit('/admin/realmguard', 'admin-realmguard');
  await visit('/admin/settings', 'admin-settings');
  await visit('/admin/security', 'admin-security');
  await visit('/admin/approvals', 'admin-approvals');
} finally {
  await browser.close();
}

console.log(`wrote ${captured.length} captures to ${outputDirectory}`);
