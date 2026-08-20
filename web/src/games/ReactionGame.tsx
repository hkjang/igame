import { useEffect, useRef, useState } from 'react';
import BoltRounded from '@mui/icons-material/BoltRounded';
import { alpha } from '@mui/material/styles';
import { Box, Button, Stack, Typography } from '@mui/material';
import type { BuiltinGameProps } from './types';

type Phase = 'idle' | 'waiting' | 'ready' | 'early' | 'result';

export function ReactionGame({ onStart, onFinish }: BuiltinGameProps) {
  const [phase, setPhase] = useState<Phase>('idle');
  const [reaction, setReaction] = useState<number>();
  const timer = useRef<number | undefined>(undefined);
  const readyAt = useRef(0);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const start = async () => {
    if (!await onStart()) return; setReaction(undefined); setPhase('waiting');
    timer.current = window.setTimeout(() => { readyAt.current = performance.now(); setPhase('ready'); }, 1800 + Math.random() * 2800);
  };
  const press = () => {
    if (phase === 'waiting') { window.clearTimeout(timer.current); setPhase('early'); return; }
    if (phase === 'ready') {
      const value = Math.round(performance.now() - readyAt.current); const score = Math.max(100, 10000 - value * 20);
      setReaction(value); setPhase('result'); void onFinish(score, { reaction_ms: value });
    }
  };
  const color = phase === 'ready' ? '#73df9b' : phase === 'early' ? '#ff718f' : '#173047';
  const message = phase === 'waiting' ? '초록색이 될 때까지 기다리세요…' : phase === 'ready' ? '지금 누르세요!' : phase === 'early' ? '너무 빨랐어요!' : phase === 'result' ? `${reaction} ms` : '준비되면 시작하세요';
  return <Stack alignItems="center" spacing={3} width="100%"><Box component="button" type="button" onClick={press} disabled={!['waiting', 'ready'].includes(phase)} aria-label={message} sx={{ border: 1, borderColor: alpha(color, .7), width: 'min(100%,720px)', height: { xs: 320, md: 430 }, borderRadius: 4, bgcolor: color, color: phase === 'ready' ? '#06140b' : '#fff', cursor: ['waiting', 'ready'].includes(phase) ? 'pointer' : 'default', transition: 'background-color .12s', boxShadow: `0 0 80px ${alpha(color, .18)}`, display: 'grid', placeItems: 'center' }}><Stack alignItems="center"><BoltRounded sx={{ fontSize: 78 }} /><Typography variant="h2" sx={{ mt: 1 }}>{message}</Typography>{phase === 'result' && <Typography mt={1}>낮을수록 빠릅니다</Typography>}</Stack></Box><Button size="large" variant="contained" onClick={() => void start()}>{phase === 'idle' ? '테스트 시작' : '다시 도전'}</Button><Typography color="text.secondary">초록색으로 바뀐 뒤 패널을 누르세요. 먼저 누르면 무효입니다.</Typography></Stack>;
}
