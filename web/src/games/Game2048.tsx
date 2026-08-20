import { useCallback, useEffect, useState } from 'react';
import ArrowDownwardRounded from '@mui/icons-material/ArrowDownwardRounded';
import ArrowBackRounded from '@mui/icons-material/ArrowBackRounded';
import ArrowForwardRounded from '@mui/icons-material/ArrowForwardRounded';
import ArrowUpwardRounded from '@mui/icons-material/ArrowUpwardRounded';
import ReplayRounded from '@mui/icons-material/ReplayRounded';
import { alpha } from '@mui/material/styles';
import { Box, Button, IconButton, Stack, Typography } from '@mui/material';
import type { BuiltinGameProps } from './types';

type Direction = 'left' | 'right' | 'up' | 'down';
const colors: Record<number, string> = { 0: '#172536', 2: '#dbeafe', 4: '#b8e5f7', 8: '#67d7ff', 16: '#59b5e9', 32: '#638cf2', 64: '#8c73e8', 128: '#c66ee1', 256: '#e867b1', 512: '#ff718f', 1024: '#ff8c68', 2048: '#ffbd5c' };

function addTile(board: number[]): number[] {
  const empty = board.flatMap((value, index) => value === 0 ? [index] : []);
  if (!empty.length) return board;
  const next = [...board];
  const index = empty[Math.floor(Math.random() * empty.length)];
  next[index] = Math.random() < .9 ? 2 : 4;
  return next;
}
function freshBoard() { return addTile(addTile(Array<number>(16).fill(0))); }
function slide(line: number[]): { line: number[]; gained: number } {
  const values = line.filter(Boolean); const output: number[] = []; let gained = 0;
  for (let index = 0; index < values.length; index += 1) {
    if (values[index] === values[index + 1]) { const merged = values[index] * 2; output.push(merged); gained += merged; index += 1; }
    else output.push(values[index]);
  }
  return { line: [...output, ...Array<number>(4 - output.length).fill(0)], gained };
}
function moveBoard(board: number[], direction: Direction): { board: number[]; gained: number; changed: boolean } {
  const next = Array<number>(16).fill(0); let gained = 0;
  for (let outer = 0; outer < 4; outer += 1) {
    const indices = Array.from({ length: 4 }, (_, inner) => direction === 'left' ? outer * 4 + inner : direction === 'right' ? outer * 4 + (3 - inner) : direction === 'up' ? inner * 4 + outer : (3 - inner) * 4 + outer);
    const result = slide(indices.map((index) => board[index])); gained += result.gained;
    indices.forEach((index, inner) => { next[index] = result.line[inner]; });
  }
  return { board: next, gained, changed: next.some((value, index) => value !== board[index]) };
}
function canMove(board: number[]): boolean {
  if (board.includes(0)) return true;
  return board.some((value, index) => (index % 4 < 3 && value === board[index + 1]) || (index < 12 && value === board[index + 4]));
}

export function Game2048({ onStart, onFinish }: BuiltinGameProps) {
  const [board, setBoard] = useState(freshBoard);
  const [score, setScore] = useState(0);
  const [status, setStatus] = useState<'idle' | 'playing' | 'over'>('idle');
  const start = async () => { if (!await onStart()) return; setBoard(freshBoard()); setScore(0); setStatus('playing'); };
  const move = useCallback((direction: Direction) => {
    if (status !== 'playing') return;
    const result = moveBoard(board, direction);
    if (!result.changed) return;
    const next = addTile(result.board); const nextScore = score + result.gained;
    setBoard(next); setScore(nextScore);
    if (!canMove(next)) { setStatus('over'); void onFinish(nextScore, { highest_tile: Math.max(...next) }); }
  }, [board, onFinish, score, status]);
  useEffect(() => {
    const key = (event: KeyboardEvent) => {
      const directions: Record<string, Direction> = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' };
      if (directions[event.key]) { event.preventDefault(); move(directions[event.key]); }
    };
    window.addEventListener('keydown', key); return () => window.removeEventListener('keydown', key);
  }, [move]);
  return <Stack alignItems="center" spacing={2.5} width="100%"><Stack direction="row" justifyContent="space-between" alignItems="center" width="min(100%,440px)"><Box><Typography color="text.secondary">SCORE</Typography><Typography variant="h2" color="primary.main">{score.toLocaleString()}</Typography></Box><Button startIcon={<ReplayRounded />} variant="contained" onClick={() => void start()}>{status === 'idle' ? '게임 시작' : '새 게임'}</Button></Stack><Box aria-label="2048 게임판" sx={{ width: 'min(100%,440px)', aspectRatio: '1', display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: { xs: 1, sm: 1.4 }, p: { xs: 1, sm: 1.4 }, bgcolor: '#09131f', borderRadius: 3 }}>{board.map((value, index) => <Box key={index} sx={{ display: 'grid', placeItems: 'center', borderRadius: 2, bgcolor: colors[value] ?? '#ffcf63', color: value <= 4 ? '#173044' : '#061019', fontWeight: 900, fontSize: value >= 1024 ? '1.35rem' : 'clamp(1.3rem,5vw,2rem)', boxShadow: value ? `0 0 22px ${alpha(colors[value] ?? '#ffcf63', .22)}` : 'none' }}>{value || ''}</Box>)}</Box>{status === 'over' && <Typography color="warning.main" fontWeight={800}>더 움직일 수 없습니다. 새 게임에 도전하세요!</Typography>}<Box aria-label="방향 조작" sx={{ display: 'grid', gridTemplateColumns: 'repeat(3,52px)', gap: .7 }}><span /><IconButton aria-label="위로" onClick={() => move('up')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowUpwardRounded /></IconButton><span /><IconButton aria-label="왼쪽으로" onClick={() => move('left')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowBackRounded /></IconButton><IconButton aria-label="아래로" onClick={() => move('down')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowDownwardRounded /></IconButton><IconButton aria-label="오른쪽으로" onClick={() => move('right')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowForwardRounded /></IconButton></Box><Typography variant="body2" color="text.secondary">방향키 또는 화면 버튼으로 같은 숫자를 합치세요.</Typography></Stack>;
}
