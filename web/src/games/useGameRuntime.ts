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
  return { start, finish, online };
}
