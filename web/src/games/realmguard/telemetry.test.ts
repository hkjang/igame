import { describe, expect, it, vi } from 'vitest';
import { createRealmGuardUUID, isRequiredRealmGuardTelemetry, REALMGUARD_OPTIONAL_TELEMETRY_LIMIT, realmGuardEventPayload, retryRealmGuardTelemetry } from './telemetry';

describe('RealmGuard telemetry attestation', () => {
  it('reserves the session ledger for required attestation events', () => {
    expect(REALMGUARD_OPTIONAL_TELEMETRY_LIMIT).toBeLessThan(500);
    expect(isRequiredRealmGuardTelemetry('realmguard.wave.complete')).toBe(true);
    expect(isRequiredRealmGuardTelemetry('realmguard.tower.sell')).toBe(true);
    expect(isRequiredRealmGuardTelemetry('realmguard.battle.complete')).toBe(true);
    expect(isRequiredRealmGuardTelemetry('realmguard.barracks.block')).toBe(false);
    expect(isRequiredRealmGuardTelemetry('realmguard.hero.skill')).toBe(false);
  });

  it('creates backend-decodable UUID event identities', () => {
    expect(createRealmGuardUUID()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
  });

  it('retries a failed send up to three times with the same event identity', async () => {
    const battleId = '11111111-1111-4111-8111-111111111111';
    const eventId = '22222222-2222-4222-8222-222222222222';
    const payload = realmGuardEventPayload(battleId, 7, { wave: 2 }, eventId);
    const send = vi.fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockRejectedValueOnce(new Error('timeout'))
      .mockResolvedValueOnce(undefined);
    const wait = vi.fn().mockResolvedValue(undefined);

    await retryRealmGuardTelemetry(() => send(payload), { backoffMs: [10, 20], sleep: wait });

    expect(send).toHaveBeenCalledTimes(3);
    expect(send.mock.calls.every(([sent]) => sent === payload)).toBe(true);
    expect(wait.mock.calls).toEqual([[10], [20]]);
    expect(payload).toMatchObject({ battle_id: battleId, sequence: 7, client_event_id: eventId, wave: 2 });
    expect(payload.occurred_at).toEqual(expect.any(String));
  });

  it('surfaces a persistent failure after the bounded retry budget', async () => {
    const send = vi.fn().mockRejectedValue(new Error('offline'));
    await expect(retryRealmGuardTelemetry(send, { backoffMs: [0, 0], sleep: async () => undefined })).rejects.toThrow('offline');
    expect(send).toHaveBeenCalledTimes(3);
  });

  it('does not retry a non-retryable protocol rejection', async () => {
    const rejection = Object.assign(new Error('sequence gap'), { status: 409 });
    const send = vi.fn().mockRejectedValue(rejection);
    await expect(retryRealmGuardTelemetry(send, { backoffMs: [0, 0], sleep: async () => undefined })).rejects.toThrow('sequence gap');
    expect(send).toHaveBeenCalledTimes(1);
  });
});
