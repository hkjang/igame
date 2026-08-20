export type TelemetrySleep = (milliseconds: number) => Promise<void>;

export const REALMGUARD_OPTIONAL_TELEMETRY_LIMIT = 128;

const requiredAttestationEvents = new Set([
  'realmguard.battle.ready',
  'realmguard.wave.start',
  'realmguard.wave.complete',
  'realmguard.tower.build',
  'realmguard.tower.upgrade',
  'realmguard.tower.sell',
  'realmguard.battle.complete',
]);

export function isRequiredRealmGuardTelemetry(event: string): boolean {
  return requiredAttestationEvents.has(event);
}

const sleep: TelemetrySleep = (milliseconds) => new Promise((resolve) => window.setTimeout(resolve, milliseconds));

export function createRealmGuardUUID(): string {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  const bytes = new Uint8Array(16);
  if (globalThis.crypto?.getRandomValues) globalThis.crypto.getRandomValues(bytes);
  else for (let index = 0; index < bytes.length; index += 1) bytes[index] = Math.floor(Math.random() * 256);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export async function retryRealmGuardTelemetry(
  send: () => Promise<void>,
  options: { backoffMs?: number[]; sleep?: TelemetrySleep; shouldRetry?: (error: unknown) => boolean } = {},
): Promise<void> {
  const backoffMs = options.backoffMs ?? [150, 450];
  const wait = options.sleep ?? sleep;
  const shouldRetry = options.shouldRetry ?? ((error: unknown) => {
    const status = error && typeof error === 'object' && 'status' in error ? Number((error as { status?: unknown }).status) : undefined;
    return status === undefined || !Number.isFinite(status) || status >= 500;
  });
  let lastError: unknown;
  for (let attempt = 0; attempt <= backoffMs.length; attempt += 1) {
    try { await send(); return; }
    catch (cause) { lastError = cause; if (!shouldRetry(cause)) break; }
    if (attempt < backoffMs.length) await wait(backoffMs[attempt]);
  }
  throw lastError instanceof Error ? lastError : new Error('전투 검증 로그를 전송하지 못했습니다.');
}

export function realmGuardEventPayload(
  battleId: string,
  sequence: number,
  data: Record<string, unknown> = {},
  eventId = createRealmGuardUUID(),
): Record<string, unknown> {
  return { ...data, battle_id: battleId, sequence, client_event_id: eventId, occurred_at: new Date().toISOString() };
}
