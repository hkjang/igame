#!/usr/bin/env node
import { createRequire } from 'node:module';

const requireFromWorkingDirectory = createRequire(`${process.cwd()}/package.json`);
let chromium;
try {
  ({ chromium } = requireFromWorkingDirectory('playwright'));
} catch (error) {
  console.error('Playwright is not installed in the working directory. Install playwright@1.55.0 with browser download disabled, then retry.');
  throw error;
}

const baseURL = (process.env.IGAME_BASE_URL ?? process.argv[2] ?? 'http://127.0.0.1:8080').replace(/\/$/, '');
const username = process.env.IGAME_USERNAME ?? process.argv[3] ?? '';
const password = process.env.IGAME_PASSWORD ?? process.argv[4] ?? '';
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

  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' });
  await visible(page.getByRole('heading', { name: '로그인' }), 'login heading');
  await page.getByLabel('관리자 아이디').fill(username);
  await page.getByLabel('비밀번호').fill(password);
  await Promise.all([
    page.waitForURL((url) => url.pathname !== '/login', { timeout: 20_000 }),
    page.getByRole('button', { name: '관리자 로그인' }).click(),
  ]);

  diagnostics.collecting = true;

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
  await page.waitForLoadState('networkidle');

  await page.goto(`${baseURL}/admin/realmguard`, { waitUntil: 'networkidle' });
  await page.reload({ waitUntil: 'networkidle' });
  await visible(page.getByRole('heading', { name: 'RealmGuard Designer' }), 'RealmGuard Designer heading');

  const versionsResponse = await context.request.get(`${baseURL}/api/v1/admin/realmguard/versions`);
  if (!versionsResponse.ok()) throw new Error(`RealmGuard versions returned HTTP ${versionsResponse.status()}`);
  const versions = await versionsResponse.json();
  const previewVersion = versions.items?.find((item) => item.status === 'published') ?? versions.items?.[0];
  if (!previewVersion?.id) throw new Error('No RealmGuard version is available for preview');

  await page.goto(`${baseURL}/realmguard/preview/${previewVersion.id}`, { waitUntil: 'networkidle' });
  await page.reload({ waitUntil: 'networkidle' });
  await visible(page.getByRole('heading', { name: 'Designer 연습 미리보기' }), 'RealmGuard preview heading');
  await visible(page.getByText('연습 전용입니다.', { exact: false }).first(), 'practice-only warning');
  await visible(page.getByRole('button', { name: '수호전 시작' }), 'preview start button');

  await page.waitForTimeout(750);
  const failures = [
    ...diagnostics.externalRequests.map((value) => `external request: ${value}`),
    ...diagnostics.failedRequests.map((value) => `request failed: ${value}`),
    ...diagnostics.badResponses.map((value) => `HTTP error: ${value}`),
    ...diagnostics.consoleErrors.map((value) => `console error: ${value}`),
    ...diagnostics.pageErrors.map((value) => `page error: ${value}`),
  ];
  if (failures.length > 0) throw new Error(`Browser diagnostics failed:\n${failures.join('\n')}`);

  console.log(`Browser smoke passed: login, RealmGuard canvas, Designer, preview, refresh, and zero external HTTP requests (${baseURL})`);
  await context.close();
} finally {
  await browser.close();
}
