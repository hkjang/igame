import { type ChangeEvent, useEffect, useRef, useState } from 'react';
import KeyboardRounded from '@mui/icons-material/KeyboardRounded';
import { Box, Button, LinearProgress, Stack, TextField, Typography } from '@mui/material';
import type { BuiltinGameProps } from './types';

const sentences = [
  '좋은 게임은 짧은 휴식에도 새로운 에너지를 줍니다.',
  '동료와 함께 도전하면 평범한 기록도 즐거운 추억이 됩니다.',
  '정확한 입력이 빠른 입력보다 먼저입니다.',
  '작은 성공을 쌓아 오늘의 최고 기록을 만들어 보세요.',
  '안전한 키 관리는 신뢰할 수 있는 서비스의 시작입니다.',
  '게임을 즐기고 다시 힘차게 업무로 돌아갑니다.',
];

export function TypingGame({ onStart, onFinish }: BuiltinGameProps) {
  const [playing, setPlaying] = useState(false);
  const [seconds, setSeconds] = useState(60);
  const [text, setText] = useState('');
  const [index, setIndex] = useState(0);
  const [correct, setCorrect] = useState(0);
  const [total, setTotal] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const finishRef = useRef(onFinish); finishRef.current = onFinish;
  const scoreRef = useRef({ correct, total }); scoreRef.current = { correct, total };
  useEffect(() => {
    if (!playing) return;
    const interval = window.setInterval(() => setSeconds((value) => {
      if (value > 1) return value - 1;
      window.clearInterval(interval); setPlaying(false);
      const state = scoreRef.current; const accuracy = state.total ? state.correct / state.total : 0; const score = Math.round(state.correct * 10 * accuracy);
      void finishRef.current(score, { correct_characters: state.correct, total_characters: state.total, accuracy: Math.round(accuracy * 100) });
      return 0;
    }), 1000);
    return () => window.clearInterval(interval);
  }, [playing]);
  const start = async () => { if (!await onStart()) return; setPlaying(true); setSeconds(60); setText(''); setIndex(0); setCorrect(0); setTotal(0); window.setTimeout(() => input.current?.focus(), 0); };
  const type = (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.value; setText(value);
    if (value.endsWith('\n') || value === sentences[index]) {
      const typed = value.trimEnd(); const target = sentences[index]; let matches = 0;
      for (let i = 0; i < typed.length; i += 1) if (typed[i] === target[i]) matches += 1;
      setCorrect((current) => current + matches); setTotal((current) => current + Math.max(typed.length, target.length)); setText(''); setIndex((current) => (current + 1) % sentences.length);
    }
  };
  const accuracy = total ? Math.round(correct / total * 100) : 100;
  return <Stack width="min(100%,820px)" spacing={3}><Stack direction="row" justifyContent="space-between" alignItems="center"><Box><Typography color="text.secondary">남은 시간</Typography><Typography variant="h2" color={seconds <= 10 ? 'warning.main' : 'primary.main'}>{seconds}초</Typography></Box><Stack direction="row" spacing={3}><Box textAlign="right"><Typography color="text.secondary">정확도</Typography><Typography variant="h3">{accuracy}%</Typography></Box><Box textAlign="right"><Typography color="text.secondary">정확한 글자</Typography><Typography variant="h3">{correct}</Typography></Box></Stack></Stack><LinearProgress variant="determinate" value={seconds / 60 * 100} sx={{ height: 8, borderRadius: 5 }} /><Box sx={{ minHeight: 160, p: { xs: 2, md: 4 }, borderRadius: 3, bgcolor: '#07101d', border: 1, borderColor: 'divider', display: 'grid', placeItems: 'center' }}><Typography variant="h3" textAlign="center" lineHeight={1.7}>{sentences[index].split('').map((character, charIndex) => <Box key={charIndex} component="span" sx={{ color: charIndex < text.length ? text[charIndex] === character ? 'secondary.main' : 'error.main' : 'text.primary', textDecoration: charIndex < text.length && text[charIndex] !== character ? 'underline' : 'none' }}>{character}</Box>)}</Typography></Box><TextField inputRef={input} value={text} onChange={type} disabled={!playing} placeholder="위 문장을 입력하세요" multiline minRows={2} inputProps={{ 'aria-label': '타이핑 입력' }} /><Button size="large" variant="contained" startIcon={<KeyboardRounded />} onClick={() => void start()}>{playing ? '처음부터' : seconds === 0 ? '다시 도전' : '게임 시작'}</Button><Typography color="text.secondary" textAlign="center">문장을 모두 입력하면 자동으로 다음 문장으로 넘어갑니다.</Typography></Stack>;
}
