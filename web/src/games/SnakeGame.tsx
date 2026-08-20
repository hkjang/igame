import { useCallback, useEffect, useRef, useState } from 'react';
import ArrowDownwardRounded from '@mui/icons-material/ArrowDownwardRounded';
import ArrowBackRounded from '@mui/icons-material/ArrowBackRounded';
import ArrowForwardRounded from '@mui/icons-material/ArrowForwardRounded';
import ArrowUpwardRounded from '@mui/icons-material/ArrowUpwardRounded';
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded';
import { Box, Button, IconButton, Stack, Typography } from '@mui/material';
import type { BuiltinGameProps } from './types';

interface Point { x: number; y: number }
type Direction = 'up' | 'down' | 'left' | 'right';
const gridSize = 20;
const opposite: Record<Direction, Direction> = { up: 'down', down: 'up', left: 'right', right: 'left' };
const vectors: Record<Direction, Point> = { up: { x: 0, y: -1 }, down: { x: 0, y: 1 }, left: { x: -1, y: 0 }, right: { x: 1, y: 0 } };
const initialSnake = (): Point[] => [{ x: 10, y: 10 }, { x: 9, y: 10 }, { x: 8, y: 10 }];
function randomFood(body: Point[]): Point {
  const free: Point[] = [];
  for (let y = 0; y < gridSize; y += 1) for (let x = 0; x < gridSize; x += 1) if (!body.some((part) => part.x === x && part.y === y)) free.push({ x, y });
  return free[Math.floor(Math.random() * free.length)] ?? { x: 0, y: 0 };
}

export function SnakeGame({ onStart, onFinish }: BuiltinGameProps) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const bodyRef = useRef<Point[]>(initialSnake());
  const foodRef = useRef<Point>(randomFood(bodyRef.current));
  const directionRef = useRef<Direction>('right');
  const queuedDirection = useRef<Direction>('right');
  const scoreRef = useRef(0);
  const [body, setBody] = useState(bodyRef.current);
  const [food, setFood] = useState(foodRef.current);
  const [score, setScore] = useState(0);
  const [status, setStatus] = useState<'idle' | 'playing' | 'over'>('idle');

  const turn = useCallback((direction: Direction) => {
    if (opposite[directionRef.current] !== direction) queuedDirection.current = direction;
  }, []);
  useEffect(() => {
    const handle = (event: KeyboardEvent) => {
      const map: Record<string, Direction> = { ArrowUp: 'up', w: 'up', ArrowDown: 'down', s: 'down', ArrowLeft: 'left', a: 'left', ArrowRight: 'right', d: 'right' };
      if (map[event.key]) { event.preventDefault(); turn(map[event.key]); }
    };
    window.addEventListener('keydown', handle); return () => window.removeEventListener('keydown', handle);
  }, [turn]);
  useEffect(() => {
    if (status !== 'playing') return;
    const timer = window.setInterval(() => {
      directionRef.current = queuedDirection.current;
      const vector = vectors[directionRef.current]; const current = bodyRef.current;
      const head = { x: current[0].x + vector.x, y: current[0].y + vector.y };
      const collided = head.x < 0 || head.y < 0 || head.x >= gridSize || head.y >= gridSize || current.some((part) => part.x === head.x && part.y === head.y);
      if (collided) { window.clearInterval(timer); setStatus('over'); void onFinish(scoreRef.current, { length: current.length, apples: scoreRef.current / 100 }); return; }
      const ate = head.x === foodRef.current.x && head.y === foodRef.current.y;
      const next = [head, ...current]; if (!ate) next.pop();
      else { scoreRef.current += 100; setScore(scoreRef.current); foodRef.current = randomFood(next); setFood(foodRef.current); }
      bodyRef.current = next; setBody(next);
    }, 125);
    return () => window.clearInterval(timer);
  }, [onFinish, status]);
  useEffect(() => {
    const context = canvas.current?.getContext('2d'); if (!context) return;
    const size = canvas.current!.width; const cell = size / gridSize;
    context.fillStyle = '#06101b'; context.fillRect(0, 0, size, size);
    context.strokeStyle = 'rgba(160,200,225,.055)'; context.lineWidth = 1;
    for (let point = 0; point <= gridSize; point += 1) { context.beginPath(); context.moveTo(point * cell, 0); context.lineTo(point * cell, size); context.stroke(); context.beginPath(); context.moveTo(0, point * cell); context.lineTo(size, point * cell); context.stroke(); }
    context.fillStyle = '#ff718f'; context.beginPath(); context.arc((food.x + .5) * cell, (food.y + .5) * cell, cell * .34, 0, Math.PI * 2); context.fill();
    body.forEach((part, index) => { context.fillStyle = index === 0 ? '#9cf56b' : '#58c987'; context.beginPath(); context.roundRect(part.x * cell + 2, part.y * cell + 2, cell - 4, cell - 4, 5); context.fill(); });
  }, [body, food]);
  const start = async () => {
    if (!await onStart()) return; const next = initialSnake(); bodyRef.current = next; directionRef.current = 'right'; queuedDirection.current = 'right'; scoreRef.current = 0; foodRef.current = randomFood(next); setBody(next); setFood(foodRef.current); setScore(0); setStatus('playing');
  };
  return <Stack alignItems="center" spacing={2.5} width="100%"><Stack width="min(100%,520px)" direction="row" justifyContent="space-between" alignItems="center"><Box><Typography color="text.secondary">SCORE</Typography><Typography variant="h2" color="secondary.main">{score}</Typography></Box><Button variant="contained" startIcon={<PlayArrowRounded />} onClick={() => void start()}>{status === 'playing' ? '다시 시작' : status === 'over' ? '다시 도전' : '게임 시작'}</Button></Stack><canvas ref={canvas} className="game-canvas" width={440} height={440} role="img" aria-label={`Snake 게임판, 점수 ${score}`} />{status === 'over' && <Typography color="warning.main" fontWeight={800}>충돌했습니다! 최종 점수 {score}점</Typography>}<Box aria-label="Snake 방향 조작" sx={{ display: 'grid', gridTemplateColumns: 'repeat(3,52px)', gap: .7 }}><span /><IconButton aria-label="위로" onClick={() => turn('up')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowUpwardRounded /></IconButton><span /><IconButton aria-label="왼쪽으로" onClick={() => turn('left')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowBackRounded /></IconButton><IconButton aria-label="아래로" onClick={() => turn('down')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowDownwardRounded /></IconButton><IconButton aria-label="오른쪽으로" onClick={() => turn('right')} disabled={status !== 'playing'} sx={{ bgcolor: 'action.hover' }}><ArrowForwardRounded /></IconButton></Box><Typography color="text.secondary">방향키, WASD 또는 화면 버튼으로 움직이세요.</Typography></Stack>;
}
