import { useCallback, useEffect, useRef, useState } from 'react';
import { createGameHub, GameHubError, type GameHubClient } from '@igame/gamehub-js';
import { useSnackbar } from '../state/SnackbarContext';

export function useGameRuntime(gameId: string) {
  const { notify } = useSnackbar();
  const client = useRef<GameHubClient | null>(null);
  const startedAt = useRef(0);
  const [online, setOnline] = useState(true);

  useEffect(() => {
    client.current = createGameHub({ gameId, onError: () => undefined });
    return () => { client.current = null; };
  }, [gameId]);

  const start = useCallback(async (metadata: Record<string, unknown> = {}) => {
    startedAt.current = performance.now();
    try {
      await client.current?.start({ client: 'builtin-react', ...metadata });
      setOnline(true);
      return true;
    } catch (cause) {
      if (cause instanceof GameHubError && cause.code === 'realmguard_config_stale') {
        setOnline(true);
        notify('RealmGuard 콘텐츠가 갱신되었습니다. 최신 설정을 다시 불러왔습니다. 시작 버튼을 다시 눌러 주세요.', 'warning');
        return false;
      }
      setOnline(false);
      if (cause instanceof GameHubError && cause.code === 'play_policy_denied') {
        notify('관리자가 설정한 플레이 허용 시간이 아닙니다.', 'error');
        return false;
      }
      notify(`${cause instanceof Error ? cause.message : '세션을 만들 수 없습니다.'} 연습 모드로 계속합니다.`, 'warning');
      return true;
    }
  }, [notify]);

  const finish = useCallback(async (score: number, metadata: Record<string, unknown> = {}) => {
    if (!client.current?.session) return;
    const duration = Math.max(0, Math.round((performance.now() - startedAt.current) / 1000));
    try {
      await client.current.submitScore({ score, metadata: { ...metadata, duration } });
      await client.current.finish({ score, duration, result: metadata });
      notify(`점수 ${score.toLocaleString()}점을 기록했습니다.`, 'success');
    } catch (cause) {
      notify(cause instanceof Error ? cause.message : '점수를 저장하지 못했습니다.', 'error');
    }
  }, [notify]);
  const telemetry = useCallback(async (event: string, data: Record<string, unknown> = {}) => {
    if (!client.current?.session) throw new GameHubError('기록 세션이 없어 검증 로그를 전송할 수 없습니다.', { code: 'telemetry_session_required' });
    const { client_event_id: clientEventId, sequence, occurred_at: occurredAt, ...payload } = data;
    await client.current.telemetry({
      event, payload,
      clientEventId: typeof clientEventId === 'string' ? clientEventId : undefined,
      sequence: Number.isInteger(sequence) ? Number(sequence) : undefined,
      occurredAt: typeof occurredAt === 'string' ? occurredAt : undefined,
    });
  }, []);
  const completeAuthoritatively = useCallback(async (payload: unknown) => {
    if (!client.current?.session) return undefined;
    return client.current.completeAuthoritatively({
      path: '/api/v1/realmguard/results',
      payload: payload && typeof payload === 'object' ? payload as Record<string, unknown> : {},
    });
  }, []);
  const isRecording = useCallback(() => Boolean(client.current?.session), []);
  return { start, finish, telemetry, completeAuthoritatively, isRecording, online };
}
