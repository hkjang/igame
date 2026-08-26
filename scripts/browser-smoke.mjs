#!/usr/bin/env node
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

let AxeBuilder;
try {
  AxeBuilder = requireFromWorkingDirectory('@axe-core/playwright').default;
} catch (error) {
  console.error('@axe-core/playwright is not installed in the working directory. Install @axe-core/playwright@4.13.0, then retry.');
  throw error;
}

/**
 * Routes the accessibility pass walks.
 *
 * Every one of these has failed at least once: a combobox with a visible label
 * that named nothing, a JSON editor whose aria-label landed on the wrapper
 * instead of the textarea, a drawer putting anchors straight inside a <ul>, and
 * a disabled field whose helper text — the reason it was disabled — sat at
 * 2.67:1. None of them show up in a screenshot.
 */
const accessibilityRoutes = [
  '/', '/games', '/rankings', '/events', '/notices', '/reviews', '/developer',
  '/profile', '/profile/keys', '/profile/preferences',
  '/admin', '/admin/games', '/admin/users', '/admin/rankings', '/admin/seasons',
  '/admin/notices', '/admin/realmguard', '/admin/defense', '/admin/analytics', '/admin/settings',
];

/**
 * Widths the layout sweep checks.
 *
 * The Defense Studio's header pushed the page 69px wider than a 1280px window
 * and put its primary action off screen, while collapsing 새 Draft into an 86px
 * column of wrapped text 274px tall. Nothing caught it: the screenshots taken
 * during development were 1440 and 390, and 1280 sits between them. The first
 * run of this sweep then found the portal's own header 88px over at 900.
 *
 * It walks the same routes as the accessibility pass rather than keeping a
 * second list, because a route worth checking for one is worth checking for the
 * other, and two lists drift.
 */
const layoutWidths = [1280, 900, 768];

const baseURL = (process.env.IGAME_BASE_URL ?? process.argv[2] ?? 'http://127.0.0.1:8080').replace(/\/$/, '');
const username = process.env.IGAME_USERNAME ?? process.argv[3] ?? '';
const password = process.env.IGAME_PASSWORD ?? process.argv[4] ?? '';
const requireDesignerDraft = process.env.IGAME_REQUIRE_DESIGNER_DRAFT === 'true';
const requireDefenseDraft = process.env.IGAME_REQUIRE_DEFENSE_DRAFT === 'true';
const screenshotDirectory = process.env.IGAME_SCREENSHOT_DIR?.trim() ?? '';
if (!username || !password) {
  console.error('Usage: IGAME_USERNAME=<user> IGAME_PASSWORD=<password> [IGAME_BASE_URL=http://127.0.0.1:8080] node scripts/browser-smoke.mjs');
  process.exit(2);
}

const serviceOrigin = new URL(baseURL).origin;
const diagnostics = {
  collecting: false,
  consoleErrors: [],
  pageErrors: [],
  failedRequests: [],
  badResponses: [],
  externalRequests: [],
};

function describeRequest(request) {
  return `${request.method()} ${request.url()}`;
}

function isExternalHTTP(url) {
  try {
    const parsed = new URL(url);
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && parsed.origin !== serviceOrigin;
  } catch {
    return false;
  }
}

async function visible(locator, label, timeout = 20_000) {
  await locator.waitFor({ state: 'visible', timeout });
  if (!(await locator.isVisible())) throw new Error(`${label} is not visible`);
}

async function elementBox(locator, label) {
  const box = await locator.boundingBox();
  if (!box || box.width <= 0 || box.height <= 0) throw new Error(`${label} has no measurable layout box`);
  return box;
}

function boxesOverlap(first, second) {
  return first.x < second.x + second.width
    && first.x + first.width > second.x
    && first.y < second.y + second.height
    && first.y + first.height > second.y;
}

function boxIsInside(inner, outer) {
  return inner.x >= outer.x && inner.y >= outer.y
    && inner.x + inner.width <= outer.x + outer.width
    && inner.y + inner.height <= outer.y + outer.height;
}

async function waitForJSONArray(locator, minimumLength, label, timeout = 20_000) {
  const deadline = Date.now() + timeout;
  let lastValue = '';
  while (Date.now() < deadline) {
    try {
      if (await locator.isVisible()) {
        lastValue = await locator.inputValue();
        const parsed = JSON.parse(lastValue);
        if (Array.isArray(parsed) && parsed.length >= minimumLength) return parsed;
      }
    } catch {
      // The selected game/version is still switching; retry until it is stable.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`${label} did not load at least ${minimumLength} JSON items (last value length ${lastValue.length})`);
}

async function capture(page, name) {
  if (!screenshotDirectory) return;
  await mkdir(screenshotDirectory, { recursive: true });
  await page.screenshot({ path: join(screenshotDirectory, `${name}.png`), fullPage: true });
}

const defenseGames = [
  { slug: 'office-guardians', name: 'Office Guardians', education: false, minimumStages: 8 },
  { slug: 'cyber-fortress', name: 'Cyber Fortress', education: true, minimumStages: 10 },
  { slug: 'ai-nexus-defense', name: 'AI Nexus Defense', education: true, minimumStages: 10 },
];

const browser = await chromium.launch({ headless: true, args: ['--disable-dev-shm-usage'] });
try {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    ignoreHTTPSErrors: process.env.IGAME_IGNORE_HTTPS_ERRORS === 'true',
  });
  const page = await context.newPage();

  page.on('console', (message) => {
    if (diagnostics.collecting && message.type() === 'error') diagnostics.consoleErrors.push(message.text());
  });
  page.on('pageerror', (error) => {
    if (diagnostics.collecting) diagnostics.pageErrors.push(error.stack ?? error.message);
  });
  page.on('request', (request) => {
    if (diagnostics.collecting && isExternalHTTP(request.url())) diagnostics.externalRequests.push(describeRequest(request));
  });
  page.on('requestfailed', (request) => {
    if (diagnostics.collecting) diagnostics.failedRequests.push(`${describeRequest(request)}: ${request.failure()?.errorText ?? 'unknown error'}`);
  });
  page.on('response', (response) => {
    if (diagnostics.collecting && response.status() >= 400) diagnostics.badResponses.push(`${response.status()} ${response.request().method()} ${response.url()}`);
  });

  const versionResponse = await context.request.get(`${baseURL}/api/v1/version`);
  if (!versionResponse.ok()) throw new Error(`Version endpoint returned HTTP ${versionResponse.status()}`);
  const serviceVersion = (await versionResponse.json()).version;
  if (!serviceVersion) throw new Error('Version endpoint did not return a service version');

  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' });
  await visible(page.getByRole('heading', { name: '로그인' }), 'login heading');
  await visible(page.getByText(`igame v${serviceVersion}`, { exact: false }).first(), 'login service version');
  await page.getByLabel('관리자 아이디').fill(username);
  await page.getByLabel('비밀번호').fill(password);
  await Promise.all([
    page.waitForURL((url) => url.pathname !== '/login', { timeout: 20_000 }),
    page.getByRole('button', { name: '관리자 로그인' }).click(),
  ]);

  diagnostics.collecting = true;

  await page.goto(baseURL, { waitUntil: 'networkidle' });
  await capture(page, '01-home');

  await page.getByLabel('프로필 메뉴 열기').click();
  const profileMenu = page.getByRole('menu');
  await visible(profileMenu, 'profile context menu');
  await visible(profileMenu.getByText(`igame v${serviceVersion}`, { exact: false }), 'profile context service version');
  await page.keyboard.press('Escape');

  const publicConfigResponse = await context.request.get(`${baseURL}/api/v1/public/config`);
  if (!publicConfigResponse.ok()) throw new Error(`Public config returned HTTP ${publicConfigResponse.status()}`);
  const publicConfig = await publicConfigResponse.json();
  const approvalEnabled = publicConfig.approval_enabled === true;

  for (const game of defenseGames) {
    for (const suffix of ['', '-banner']) {
      const assetPath = `/assets/games/${game.slug}${suffix}.svg`;
      const assetResponse = await context.request.get(`${baseURL}${assetPath}`);
      if (assetResponse.status() !== 200) throw new Error(`${assetPath} returned HTTP ${assetResponse.status()}`);
      const contentType = assetResponse.headers()['content-type'] ?? '';
      if (!contentType.toLowerCase().includes('image/svg+xml')) throw new Error(`${assetPath} is not served as SVG`);
      const svg = await assetResponse.text();
      if (!/<svg\b/i.test(svg)) throw new Error(`${assetPath} does not contain an SVG root`);
      if (/(?:href|xlink:href)\s*=\s*["']\s*(?:https?:)?\/\//i.test(svg)
        || /url\(\s*["']?\s*(?:https?:)?\/\//i.test(svg)) {
        throw new Error(`${assetPath} references a remote asset`);
      }
    }
  }

  // The catalogue is the first screen with real artwork on it. Card art used to
  // resolve to its own aspect ratio and render taller than its frame, painting
  // over the title underneath, and a mis-sized visually-hidden live region added
  // a full screen of empty scroll below the grid. Both were invisible to unit
  // tests, so the browser gate measures them.
  await page.goto(`${baseURL}/games`, { waitUntil: 'networkidle' });
  // Specifically a card that carries artwork: the placeholder cards cannot show
  // the overflow this guards against, so matching the first card in the grid
  // would quietly assert nothing.
  const illustratedCard = page.locator('.MuiCard-root:has(img.game-art)').first();
  await visible(illustratedCard, 'illustrated game catalogue card');
  const artBox = await elementBox(illustratedCard.locator('img.game-art'), 'game card artwork');
  const frameBox = await elementBox(illustratedCard.locator('.game-art-frame'), 'game card artwork frame');
  if (!boxIsInside(artBox, { ...frameBox, height: frameBox.height + 1 })) {
    throw new Error(`Game card artwork ${JSON.stringify(artBox)} escapes its frame ${JSON.stringify(frameBox)}`);
  }
  const cardTitleBox = await elementBox(illustratedCard.getByRole('heading').first(), 'game card title');
  if (boxesOverlap(artBox, cardTitleBox)) throw new Error('Game card artwork covers the card title');
  const catalogueScroll = await page.evaluate(() => ({
    scroll: document.documentElement.scrollHeight,
    content: Math.ceil(document.querySelector('footer').getBoundingClientRect().bottom + window.scrollY),
  }));
  if (catalogueScroll.scroll > catalogueScroll.content + 48) {
    throw new Error(`Game catalogue scrolls ${catalogueScroll.scroll}px past its ${catalogueScroll.content}px of content`);
  }
  await capture(page, '01b-games');

  await page.goto(`${baseURL}/games/realmguard`, { waitUntil: 'networkidle' });
  await page.reload({ waitUntil: 'networkidle' });
  await visible(page.getByRole('heading', { name: 'RealmGuard', exact: true }).first(), 'RealmGuard page heading');
  const startButton = page.getByRole('button', { name: '수호전 시작' });
  await visible(startButton, 'RealmGuard start button');
  await startButton.click();
  const canvas = page.locator('[aria-label="RealmGuard 전장"] canvas');
  await visible(canvas, 'RealmGuard Phaser canvas', 30_000);
  const canvasSize = await canvas.evaluate((element) => ({ width: element.width, height: element.height }));
  if (canvasSize.width < 1 || canvasSize.height < 1) throw new Error('RealmGuard canvas has no render surface');
  await page.setViewportSize({ width: 375, height: 800 });
  const realmGuardBattleHUD = page.getByTestId('realmguard-battle-hud');
  await visible(realmGuardBattleHUD, 'RealmGuard narrow battle HUD');
  const narrowHUD = await realmGuardBattleHUD.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
    overflowX: getComputedStyle(element).overflowX,
    touchAction: getComputedStyle(element).touchAction,
    ancestorTouchAction: getComputedStyle(element.parentElement).touchAction,
  }));
  if (narrowHUD.scrollWidth <= narrowHUD.clientWidth || !['auto', 'scroll'].includes(narrowHUD.overflowX)
    || !narrowHUD.touchAction.includes('pan-x') || narrowHUD.ancestorTouchAction === 'none') {
    throw new Error(`RealmGuard narrow HUD is not horizontally reachable: ${JSON.stringify(narrowHUD)}`);
  }
  const realmGuardScrollLeft = await realmGuardBattleHUD.evaluate((element) => {
    element.scrollLeft = element.scrollWidth;
    return element.scrollLeft;
  });
  if (realmGuardScrollLeft <= 0) throw new Error('RealmGuard narrow HUD did not accept horizontal scrolling');
  const realmGuardPause = realmGuardBattleHUD.getByRole('button', { name: /일시정지|계속/ });
  await realmGuardPause.scrollIntoViewIfNeeded();
  await visible(realmGuardPause, 'RealmGuard narrow pause control');
  const realmGuardShell = page.getByTestId('realmguard-battle-shell');
  const realmGuardSkillPanel = page.getByTestId('realmguard-skill-panel');
  const realmGuardCommandPanel = page.getByTestId('realmguard-command-panel');
  const [realmShellBox, realmSkillBox, realmCommandBox] = await Promise.all([
    elementBox(realmGuardShell, 'RealmGuard narrow battle shell'),
    elementBox(realmGuardSkillPanel, 'RealmGuard narrow skill panel'),
    elementBox(realmGuardCommandPanel, 'RealmGuard narrow command panel'),
  ]);
  if (!boxIsInside(realmSkillBox, realmShellBox) || !boxIsInside(realmCommandBox, realmShellBox)
    || boxesOverlap(realmSkillBox, realmCommandBox)) {
    throw new Error(`RealmGuard narrow controls overlap or leave the battle shell: ${JSON.stringify({ realmShellBox, realmSkillBox, realmCommandBox })}`);
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.waitForLoadState('networkidle');

  await page.goto(`${baseURL}/admin/realmguard`, { waitUntil: 'networkidle' });
  await page.reload({ waitUntil: 'networkidle' });
  await visible(page.getByRole('heading', { name: 'RealmGuard Designer' }), 'RealmGuard Designer heading');
  const stagesEditor = page.locator('textarea[aria-label="stages JSON 편집기"]:not([aria-hidden="true"]), [aria-label="stages JSON 편집기"] textarea:not([aria-hidden="true"])').first();
  const noDraftNotice = page.getByText('편집 가능한 Draft가 없습니다.', { exact: false });
  await Promise.any([
    stagesEditor.waitFor({ state: 'visible', timeout: 20_000 }),
    noDraftNotice.waitFor({ state: 'visible', timeout: 20_000 }),
  ]).catch(() => { throw new Error('RealmGuard Designer rendered neither a stages editor nor a valid no-draft state'); });
  for (const invalidText of [/not valid JSON/i, /JSON 오류/i, /표시할 데이터가 없습니다/]) {
    if (await page.getByText(invalidText).first().isVisible().catch(() => false)) {
      throw new Error(`RealmGuard Designer rendered an invalid or empty editor state: ${invalidText}`);
    }
  }
  if (await stagesEditor.isVisible().catch(() => false)) {
    const editorText = await stagesEditor.inputValue();
    let stages;
    try {
      stages = JSON.parse(editorText);
    } catch (error) {
      throw new Error(`RealmGuard Designer stages editor does not contain valid JSON: ${error instanceof Error ? error.message : String(error)}`);
    }
    if (!Array.isArray(stages) || stages.length < 11) {
      throw new Error(`RealmGuard Designer stages editor is incomplete: expected at least 11 stages, got ${Array.isArray(stages) ? stages.length : typeof stages}`);
    }
    if (await page.getByRole('button', { name: 'Draft 저장' }).isDisabled()) {
      throw new Error('RealmGuard Designer loaded content but left Draft 저장 disabled');
    }
  } else {
    if (requireDesignerDraft) throw new Error('RealmGuard release smoke requires the API-created editable draft and its stages editor');
    await visible(noDraftNotice, 'no editable RealmGuard draft notice');
  }

  const versionsResponse = await context.request.get(`${baseURL}/api/v1/admin/realmguard/versions`);
  if (!versionsResponse.ok()) throw new Error(`RealmGuard versions returned HTTP ${versionsResponse.status()}`);
  const versions = await versionsResponse.json();
  const previewVersion = versions.items?.find((item) => item.status === 'published') ?? versions.items?.[0];
  if (!previewVersion?.id) throw new Error('No RealmGuard version is available for preview');

  await page.goto(`${baseURL}/realmguard/preview/${previewVersion.id}`, { waitUntil: 'networkidle' });
  await page.reload({ waitUntil: 'networkidle' });
  await visible(page.getByRole('heading', { name: 'Designer 연습 미리보기' }), 'RealmGuard preview heading');
  await visible(page.getByText('연습 전용입니다.', { exact: false }).first(), 'practice-only warning');
  const previewStart = page.getByRole('button', { name: '수호전 시작' });
  await visible(previewStart, 'preview start button');
  await previewStart.click();
  await visible(page.locator('[aria-label="RealmGuard 전장"] canvas'), 'RealmGuard preview Phaser canvas', 30_000);
  await page.setViewportSize({ width: 375, height: 800 });
  const practiceWarning = page.getByTestId('realmguard-practice-warning');
  const practiceSkillPanel = page.getByTestId('realmguard-skill-panel');
  const practiceCommandPanel = page.getByTestId('realmguard-command-panel');
  const practiceShell = page.getByTestId('realmguard-battle-shell');
  await visible(practiceWarning, 'RealmGuard narrow practice warning');
  const [practiceWarningBox, practiceSkillBox, practiceCommandBox, practiceShellBox] = await Promise.all([
    elementBox(practiceWarning, 'RealmGuard narrow practice warning'),
    elementBox(practiceSkillPanel, 'RealmGuard narrow practice skill panel'),
    elementBox(practiceCommandPanel, 'RealmGuard narrow practice command panel'),
    elementBox(practiceShell, 'RealmGuard narrow practice shell'),
  ]);
  if (!boxIsInside(practiceWarningBox, practiceShellBox) || !boxIsInside(practiceSkillBox, practiceShellBox)
    || !boxIsInside(practiceCommandBox, practiceShellBox) || boxesOverlap(practiceWarningBox, practiceSkillBox)
    || boxesOverlap(practiceSkillBox, practiceCommandBox)) {
    throw new Error(`RealmGuard practice HUD zones overlap or leave the battle shell: ${JSON.stringify({ practiceWarningBox, practiceSkillBox, practiceCommandBox, practiceShellBox })}`);
  }
  await page.setViewportSize({ width: 1440, height: 1000 });

  for (const game of defenseGames) {
    const configResponse = await context.request.get(`${baseURL}/api/v1/defense/${game.slug}/config`);
    if (!configResponse.ok()) throw new Error(`${game.name} config returned HTTP ${configResponse.status()}`);
    const publishedConfig = await configResponse.json();
    if (!publishedConfig.version?.id || !publishedConfig.game?.id || publishedConfig.game.slug !== game.slug) throw new Error(`${game.name} published config envelope is invalid`);
    const publishedEventIDs = new Set((publishedConfig.content?.events ?? []).map((event) => event.id));
    await page.goto(`${baseURL}/games/${game.slug}`, { waitUntil: 'networkidle' });
    await page.reload({ waitUntil: 'networkidle' });
    await visible(page.getByRole('heading', { name: game.name, exact: true }).first(), `${game.name} page heading`);
    await visible(page.getByTestId('defense-game-shell'), `${game.name} game shell`);
    await visible(page.getByTestId('defense-stage-select'), `${game.name} stage selector`);
    if (game.slug === 'office-guardians') {
      const stageTwoAction = page.getByTestId('defense-stage-stage-2');
      await visible(stageTwoAction, 'Office Guardians stage 2 action');
      if (await stageTwoAction.getAttribute('data-unlocked') !== 'true'
        || await stageTwoAction.getByRole('button').isDisabled()) {
        throw new Error('Office Guardians did not expose the stage 2 unlock returned by the authoritative result');
      }
    }
    if (game.slug === 'ai-nexus-defense') {
      await visible(page.getByRole('heading', { name: 'Model Tower 전략' }), 'AI model profile selector');
      for (const profileID of ['small', 'medium', 'large', 'reasoning', 'vision']) {
        await visible(page.getByTestId(`defense-profile-${profileID}`), `AI ${profileID} profile`);
      }
    }
    const defenseStart = page.getByTestId('defense-start');
    await visible(defenseStart, `${game.name} start button`);
    const sessionResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST'
      && new URL(response.url()).pathname === `/api/v1/games/${publishedConfig.game.id}/sessions`, { timeout: 20_000 });
    await defenseStart.click();
    const sessionResponse = await sessionResponsePromise;
    if (sessionResponse.status() !== 201) throw new Error(`${game.name} session returned HTTP ${sessionResponse.status()}`);
    const sessionRequest = sessionResponse.request().postDataJSON();
    const sessionBody = await sessionResponse.json();
    if (sessionRequest?.metadata?.defense_content_version_id !== publishedConfig.version.id
      || sessionBody.session?.defense_content_version_id !== publishedConfig.version.id) {
      throw new Error(`${game.name} did not pin the exact published Defense content UUID`);
    }
    await visible(page.getByTestId('defense-canvas'), `${game.name} canvas host`, 30_000);
    await visible(page.locator(`[aria-label="${game.name} 전장"] canvas`), `${game.name} Phaser canvas`, 30_000);
    if (game.slug === 'office-guardians') {
      await page.setViewportSize({ width: 375, height: 800 });
      const defenseBattleHUD = page.getByTestId('defense-battle-hud');
      await visible(defenseBattleHUD, 'Defense narrow battle HUD');
      const defenseNarrowHUD = await defenseBattleHUD.evaluate((element) => ({
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        overflowX: getComputedStyle(element).overflowX,
        touchAction: getComputedStyle(element).touchAction,
        ancestorTouchAction: getComputedStyle(element.parentElement).touchAction,
      }));
      if (defenseNarrowHUD.scrollWidth <= defenseNarrowHUD.clientWidth
        || !['auto', 'scroll'].includes(defenseNarrowHUD.overflowX)
        || !defenseNarrowHUD.touchAction.includes('pan-x')
        || defenseNarrowHUD.ancestorTouchAction === 'none') {
        throw new Error(`Defense narrow HUD is not horizontally reachable: ${JSON.stringify(defenseNarrowHUD)}`);
      }
      const defenseScrollLeft = await defenseBattleHUD.evaluate((element) => {
        element.scrollLeft = element.scrollWidth;
        return element.scrollLeft;
      });
      if (defenseScrollLeft <= 0) throw new Error('Defense narrow HUD did not accept horizontal scrolling');
      const defensePause = defenseBattleHUD.getByRole('button', { name: /일시정지|계속/ });
      await defensePause.scrollIntoViewIfNeeded();
      await visible(defensePause, 'Defense narrow pause control');
      await page.setViewportSize({ width: 1440, height: 1000 });
    }
    if (!game.education) await capture(page, '02-office-guardians-battle');
    if (game.slug === 'ai-nexus-defense') {
      await visible(page.getByTestId('defense-ai-resource-hud'), 'AI resource HUD');
      await page.setViewportSize({ width: 375, height: 800 });
      const defenseShell = page.getByTestId('defense-game-shell');
      const resourceHUD = page.getByTestId('defense-ai-resource-hud');
      const skillStack = page.getByTestId('defense-skill-stack');
      const commandPanel = page.getByTestId('defense-command-panel');
      await visible(skillStack, 'AI narrow skill stack');
      await visible(commandPanel, 'AI narrow command panel');
      const [shellBox, resourceBox, skillBox, commandBox] = await Promise.all([
        elementBox(defenseShell, 'AI narrow game shell'),
        elementBox(resourceHUD, 'AI narrow resource HUD'),
        elementBox(skillStack, 'AI narrow skill stack'),
        elementBox(commandPanel, 'AI narrow command panel'),
      ]);
      if (!boxIsInside(resourceBox, shellBox) || !boxIsInside(skillBox, shellBox)
        || boxesOverlap(resourceBox, skillBox) || boxesOverlap(skillBox, commandBox)) {
        throw new Error(`AI narrow HUD zones overlap or leave the battle shell: ${JSON.stringify({ shellBox, resourceBox, skillBox, commandBox })}`);
      }
      await page.setViewportSize({ width: 1440, height: 1000 });
      const rules = publishedConfig.content?.resource_rules ?? {};
      const computeStart = await page.getByTestId('defense-ai-resource-compute').getAttribute('data-start');
      const tokenStart = await page.getByTestId('defense-ai-resource-token').getAttribute('data-start');
      if (Number(computeStart) !== Number(rules.compute_start) || Number(tokenStart) !== Number(rules.token_start)) {
        throw new Error(`AI resource HUD did not use pinned start limits: ${computeStart} / ${tokenStart}`);
      }
    }

    if (game.education) {
      const choice = page.getByTestId('defense-choice-event');
      const nextWave = page.getByRole('button', { name: /다음 웨이브/ }).first();
      if (await nextWave.isVisible().catch(() => false)) await nextWave.click();
      await visible(choice, `${game.name} education choice`, 12_000);
      await visible(page.locator('[data-testid="defense-game-shell"][data-event-paused="true"]'), `${game.name} scene paused by education prompt`);
      // On the attribute, not the chip's text: the text is Korean for the
      // player and the attribute is the state the game is actually in.
      await visible(page.locator('[data-battle-status="paused"]'), `${game.name} paused battle status`);
      await capture(page, game.slug === 'cyber-fortress' ? '03-cyber-fortress-education' : '04-ai-nexus-hud-education');
      if (game.slug === 'ai-nexus-defense') {
        const rules = publishedConfig.content?.resource_rules ?? {};
        const computeAfterWaveStart = Number(rules.compute_start) - Number(rules.wave_compute_cost);
        const tokenAfterWaveStart = Number(rules.token_start) - Number(rules.wave_token_cost);
        const computeHUD = await page.getByTestId('defense-ai-resource-compute').getAttribute('data-remaining');
        const tokenHUD = await page.getByTestId('defense-ai-resource-token').getAttribute('data-remaining');
        if (Number(computeHUD) !== computeAfterWaveStart || Number(tokenHUD) !== tokenAfterWaveStart) {
          throw new Error(`AI resource HUD did not apply pinned wave costs: ${computeHUD} / ${tokenHUD}`);
        }
      }
      const answers = choice.getByTestId('defense-answer');
      if ((await answers.count()) < 2) throw new Error(`${game.name} education choice has fewer than two answers`);
      const answerID = await answers.first().getAttribute('data-answer-id');
      if (!answerID || !['A', 'B', 'C'].includes(answerID)) throw new Error(`${game.name} education answer is missing a neutral A/B/C data-answer-id`);
      const answerResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST'
        && new URL(response.url()).pathname.startsWith(`/api/v1/defense/${game.slug}/education/events/`), { timeout: 20_000 });
      await answers.first().click();
      const answerResponse = await answerResponsePromise;
      if (!answerResponse.ok()) throw new Error(`${game.name} education answer returned HTTP ${answerResponse.status()}`);
      const submittedEventID = decodeURIComponent(new URL(answerResponse.url()).pathname.split('/').at(-2) ?? '');
      if (!publishedEventIDs.has(submittedEventID)) throw new Error(`${game.name} submitted a fallback education event instead of a published event ID`);
      await visible(page.getByTestId('defense-choice-feedback'), `${game.name} server answer feedback`);
      const continueButton = page.getByTestId('defense-choice-continue');
      if (game.slug === 'ai-nexus-defense') {
        const resultResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST'
          && new URL(response.url()).pathname === '/api/v1/defense/ai-nexus-defense/results', { timeout: 240_000 });
        await continueButton.click();
        const speedButton = page.getByRole('button', { name: '1×', exact: true });
        await visible(speedButton, 'AI battle speed control');
        await speedButton.click();
        const resultPanel = page.getByTestId('defense-result');
        const resultDeadline = Date.now() + 240_000;
        while (!(await resultPanel.isVisible().catch(() => false))) {
          if (Date.now() >= resultDeadline) throw new Error('AI resource-depletion result did not appear before the deadline');
          const pendingChoice = page.getByTestId('defense-choice-event');
          if (!(await pendingChoice.isVisible().catch(() => false))) {
            await page.waitForTimeout(250);
            continue;
          }
          const pendingAnswers = pendingChoice.getByTestId('defense-answer');
          const pendingAnswerID = await pendingAnswers.first().getAttribute('data-answer-id');
          if (!pendingAnswerID || !['A', 'B', 'C'].includes(pendingAnswerID)) {
            throw new Error('AI follow-up education answer is missing a neutral A/B/C data-answer-id');
          }
          const pendingAnswerResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST'
            && new URL(response.url()).pathname.startsWith('/api/v1/defense/ai-nexus-defense/education/events/'), { timeout: 20_000 });
          await pendingAnswers.first().click();
          const pendingAnswerResponse = await pendingAnswerResponsePromise;
          if (!pendingAnswerResponse.ok()) throw new Error(`AI follow-up education answer returned HTTP ${pendingAnswerResponse.status()}`);
          const pendingEventID = decodeURIComponent(new URL(pendingAnswerResponse.url()).pathname.split('/').at(-2) ?? '');
          if (!publishedEventIDs.has(pendingEventID)) throw new Error('AI submitted a fallback follow-up education event instead of a published event ID');
          await visible(pendingChoice.getByTestId('defense-choice-feedback'), 'AI follow-up server answer feedback');
          await pendingChoice.getByTestId('defense-choice-continue').click();
        }
        const resultResponse = await resultResponsePromise;
        if (resultResponse.status() !== 201) throw new Error(`AI resource-depletion result returned HTTP ${resultResponse.status()}`);
        const resultBody = await resultResponse.json();
        if (resultBody.result?.verified !== true
          || resultBody.result?.stars !== 0
          || resultBody.result?.resource_state?.trust?.remaining !== 0
          || resultBody.result?.attestation?.resource_state?.trust?.remaining !== 0) {
          throw new Error(`AI resource-depletion result was not server verified: ${JSON.stringify(resultBody.result ?? {})}`);
        }
        await visible(resultPanel.getByText('방어선 붕괴', { exact: true }), 'AI depletion defeat heading');
        await visible(resultPanel.getByText('서버 검증을 완료해 진행도와 전용 랭킹에 반영했습니다.', { exact: true }), 'AI verified result notice');
        const trustResult = resultPanel.getByText('trust', { exact: true }).locator('..');
        await visible(trustResult.getByText('0', { exact: true }), 'AI depleted Trust result');
        if (await page.getByTestId('defense-choice-event').isVisible().catch(() => false)) {
          throw new Error('AI displayed a late education modal after terminal resource depletion');
        }
        if (await page.getByTestId('defense-game-shell').getAttribute('data-ai-depleted') !== 'true') {
          throw new Error('AI game shell did not expose the terminal resource-depletion state');
        }
        await capture(page, '04b-ai-nexus-depletion-result');
      } else if (await continueButton.isVisible().catch(() => false)) {
        await continueButton.click();
        await visible(page.locator('[data-testid="defense-game-shell"][data-event-paused="false"]'), `${game.name} scene resumed after education choice`);
      }
    }
  }

  for (const game of defenseGames) {
    await page.goto(`${baseURL}/admin/defense`, { waitUntil: 'networkidle' });
    await page.reload({ waitUntil: 'networkidle' });
    await visible(page.getByRole('heading', { name: 'Defense Content Studio' }), 'Defense Content Studio heading');
    await visible(page.getByTestId('defense-studio'), 'Defense Content Studio root');
    const gameSelect = page.getByRole('combobox', { name: '게임' });
    await visible(gameSelect, 'Defense Content Studio game selector');
    if (game.slug !== 'office-guardians') {
      const gameName = defenseGames.find((item) => item.slug === game.slug)?.name ?? game.slug;
      await Promise.all([
        page.waitForResponse((response) => response.request().method() === 'GET'
          && new URL(response.url()).pathname === `/api/v1/admin/defense/${game.slug}/drafts/stages`
          && response.status() === 200, { timeout: 20_000 }),
        (async () => {
          await gameSelect.click();
          await page.getByRole('option', { name: gameName, exact: true }).click();
        })(),
      ]);
    }
    const sectionSelect = page.getByRole('combobox', { name: '콘텐츠 영역' });
    await visible(sectionSelect, 'Defense Content Studio section selector');

    if (game.slug === 'office-guardians') {
      await page.getByRole('button', { name: '새 Draft', exact: true }).click();
      await visible(page.getByRole('heading', { name: '새 Office Guardians Draft' }), 'Office rollback Draft dialog');
      const rollbackSource = page.getByRole('combobox', { name: '복제할 기준 버전' });
      await visible(rollbackSource, 'rollback source selector');
      await rollbackSource.click();
      // 보관 is the Korean the option now carries for `archived`.
      const archivedSource = page.getByRole('option', { name: /v0\.3\.0 · 보관 · #1/ });
      await visible(archivedSource, 'historical published Defense source');
      await archivedSource.click();
      await page.getByLabel('Policy Version').fill('browser-rollback-policy-v0.3.0');
      await page
        .getByLabel('버전 라벨')
        .fill(`browser-rollback-draft-${process.pid}-${Date.now()}`);
      const rollbackResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST'
        && new URL(response.url()).pathname === '/api/v1/admin/defense/office-guardians/versions', { timeout: 20_000 });
      await page.getByRole('button', { name: 'Draft 만들기', exact: true }).click();
      const rollbackResponse = await rollbackResponsePromise;
      if (rollbackResponse.status() !== 201) throw new Error(`Office rollback Draft returned HTTP ${rollbackResponse.status()}`);
      const rollbackRequest = rollbackResponse.request().postDataJSON();
      const rollbackBody = await rollbackResponse.json();
      if (!rollbackRequest?.source_version_id
        || rollbackRequest.policy_version !== 'browser-rollback-policy-v0.3.0'
        || rollbackBody.version?.source_version_id !== rollbackRequest.source_version_id
        || rollbackBody.version?.policy_version !== rollbackRequest.policy_version) {
        throw new Error('Content Studio did not preserve the rollback source UUID and Policy Version');
      }
      await visible(page.getByText('browser-rollback-policy-v0.3.0', { exact: true }).first(), 'rollback Draft Policy Version');
    }

    const editor = page
      .getByTestId('defense-section-editor')
      .locator('textarea:not([aria-hidden="true"])')
      .first();
    const noDraft = page.getByText('편집 가능한 Draft가 없습니다.', { exact: false });
    if (requireDefenseDraft) {
      await editor.waitFor({ state: 'visible', timeout: 20_000 }).catch(() => {
        throw new Error(`${game.name} release smoke requires the API-created editable draft`);
      });
    } else {
      await Promise.any([
        editor.waitFor({ state: 'visible', timeout: 20_000 }),
        noDraft.waitFor({ state: 'visible', timeout: 20_000 }),
      ]).catch(() => { throw new Error(`${game.name} Studio rendered neither an editor nor a valid no-draft state`); });
    }

    if (await editor.isVisible().catch(() => false)) {
      await visible(page.getByTestId('defense-quick-editor'), `${game.name} structured quick editor`);
      if (game.slug === 'office-guardians') await capture(page, '05-defense-content-studio');
      await waitForJSONArray(
        editor,
        game.minimumStages,
        `${game.name} Studio stages editor`,
      );
      const saveButton = page.getByRole('button', { name: 'Draft 저장' });
      if (await saveButton.isDisabled()) throw new Error(`${game.name} Studio loaded content but left Draft 저장 disabled`);
      const saveResponsePromise = page.waitForResponse((response) => response.request().method() === 'PUT'
        && new URL(response.url()).pathname === `/api/v1/admin/defense/${game.slug}/drafts/stages`, { timeout: 20_000 });
      await saveButton.click();
      const saveResponse = await saveResponsePromise;
      if (!saveResponse.ok()) throw new Error(`${game.name} Studio Draft 저장 returned HTTP ${saveResponse.status()}`);

      await page.getByRole('tab', { name: 'Versions & Approval' }).click();
      await visible(page.getByRole('heading', { name: /^버전 검증과 (승인|게시)$/ }), `${game.name} version workflow`);
      await visible(page.getByRole('button', { name: '테스트' }), `${game.name} test action`);
      await visible(page.getByRole('button', { name: approvalEnabled ? /^(승인 요청|게시)$/ : '즉시 게시' }), `${game.name} publish workflow action`);
      await visible(page.getByRole('link', { name: '미리보기', exact: true }), `${game.name} version preview action`);
      if (!approvalEnabled) {
        if (await page.getByLabel('검토 의견').count()) throw new Error(`${game.name} rendered review controls while approval is disabled`);
        if (await page.getByRole('button', { name: /^(승인|반려)$/ }).count()) throw new Error(`${game.name} rendered approval actions while approval is disabled`);
      }

      await page.getByRole('tab', { name: 'Telemetry' }).click();
      await visible(page.getByRole('heading', { name: '최근 운영 Telemetry' }), `${game.name} telemetry report`);
      await page.getByRole('tab', { name: 'Education Report' }).click();
      await visible(page.getByRole('heading', { name: '교육 효과 Report' }), `${game.name} learning report`);
      await visible(page.getByTestId('defense-report-dashboard'), `${game.name} report dashboard`);
      await visible(page.getByText(game.education ? '참여자' : '플레이어', { exact: true }).first(), `${game.name} report metric definition`);
      if (new URL(page.url()).searchParams.get('view') !== 'report') {
        throw new Error(`${game.name} Education Report selection was not persisted in the URL`);
      }
      if (game.slug === 'office-guardians') {
        await page.reload({ waitUntil: 'networkidle' });
        await visible(page.getByRole('heading', { name: 'Defense Content Studio' }), 'Defense Content Studio report deep refresh');
        await visible(page.getByRole('heading', { name: '교육 효과 Report' }), 'Education Report restored after deep refresh');
        await visible(page.getByTestId('defense-report-dashboard'), 'report dashboard restored after deep refresh');
        if (new URL(page.url()).searchParams.get('view') !== 'report'
          || await page.getByRole('tab', { name: 'Education Report' }).getAttribute('aria-selected') !== 'true') {
          throw new Error('Education Report URL state and selected tab were not preserved across refresh');
        }
      }

      await page.getByRole('tab', { name: 'Content Editor' }).click();
      const previewButton = page.getByRole('link', { name: '미리보기', exact: true });
      await visible(previewButton, `${game.name} Studio preview button`);
      await Promise.all([
        page.waitForURL((url) => url.pathname.startsWith(`/defense/${game.slug}/preview/`), { timeout: 20_000 }),
        previewButton.click(),
      ]);
      await visible(page.getByTestId('defense-game-shell'), `${game.name} practice preview shell`);
    }
  }

  // A page is never wider than the window it is in. Checked before the
  // accessibility pass so both run against the same settled session.
  const layoutFailures = [];
  for (const width of layoutWidths) {
    await page.setViewportSize({ width, height: 900 });
    for (const route of accessibilityRoutes) {
      await page.goto(`${baseURL}${route}`, { waitUntil: 'networkidle' });
      await page.waitForTimeout(400);
      const overflow = await page.evaluate(() => ({ document: document.documentElement.scrollWidth, window: window.innerWidth }));
      if (overflow.document > overflow.window + 1) {
        layoutFailures.push(`${route} at ${width}px is ${overflow.document}px wide in a ${overflow.window}px window`);
      }
    }
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  if (layoutFailures.length > 0) throw new Error(`Horizontal overflow:\n${layoutFailures.join('\n')}`);

  // Accessibility last: it navigates away from wherever the flow ended, and a
  // violation here is a real defect rather than a flake, so it belongs with the
  // other things that fail the release.
  const accessibilityFailures = [];
  for (const route of accessibilityRoutes) {
    await page.goto(`${baseURL}${route}`, { waitUntil: 'networkidle' });
    await page.waitForTimeout(500);
    const report = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
    for (const violation of report.violations) {
      if (violation.impact !== 'serious' && violation.impact !== 'critical') continue;
      const where = violation.nodes.slice(0, 3).map((node) => node.target.join(' ')).join(' | ');
      accessibilityFailures.push(`${route} [${violation.impact}] ${violation.id}: ${violation.help} — ${where}`);
    }
  }
  if (accessibilityFailures.length > 0) throw new Error(`Accessibility violations:\n${accessibilityFailures.join('\n')}`);

  await page.waitForTimeout(750);
  const failures = [
    ...diagnostics.externalRequests.map((value) => `external request: ${value}`),
    ...diagnostics.failedRequests.map((value) => `request failed: ${value}`),
    ...diagnostics.badResponses.map((value) => `HTTP error: ${value}`),
    ...diagnostics.consoleErrors.map((value) => `console error: ${value}`),
    ...diagnostics.pageErrors.map((value) => `page error: ${value}`),
  ];
  if (failures.length > 0) throw new Error(`Browser diagnostics failed:\n${failures.join('\n')}`);

  console.log(`Browser smoke passed: login, RealmGuard, three Defense games, education choices, both content studios, preview, refresh, ${accessibilityRoutes.length} routes with no horizontal overflow at ${layoutWidths.join('/')}px, ${accessibilityRoutes.length} routes with no serious accessibility violation, and zero external HTTP requests (${baseURL})`);
  await context.close();
} finally {
  await browser.close();
}
