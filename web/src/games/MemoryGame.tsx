import { useEffect, useRef, useState } from 'react';
import ReplayRounded from '@mui/icons-material/ReplayRounded';
import { Box, Button, Stack, Typography } from '@mui/material';
import type { BuiltinGameProps } from './types';

const symbols = ['★', '◆', '●', '▲', '☀', '☂', '♫', '⚡'];
interface CardState { id: number; symbol: string; matched: boolean }
function deck(): CardState[] {
  return [...symbols, ...symbols].map((symbol, id) => ({ id, symbol, matched: false })).sort(() => Math.random() - .5);
}

export function MemoryGame({ onStart, onFinish }: BuiltinGameProps) {
  const [cards, setCards] = useState(deck);
  const [open, setOpen] = useState<number[]>([]);
  const [moves, setMoves] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [locked, setLocked] = useState(false);
  const started = useRef(0);
  const timeout = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timeout.current), []);
  const start = async () => { if (!await onStart()) return; setCards(deck()); setOpen([]); setMoves(0); setLocked(false); setPlaying(true); started.current = Date.now(); };
  const flip = (index: number) => {
    if (!playing || locked || open.includes(index) || cards[index].matched) return;
    const nextOpen = [...open, index]; setOpen(nextOpen);
    if (nextOpen.length < 2) return;
    const nextMoves = moves + 1; setMoves(nextMoves); setLocked(true);
    if (cards[nextOpen[0]].symbol === cards[nextOpen[1]].symbol) {
      const nextCards = cards.map((card, cardIndex) => nextOpen.includes(cardIndex) ? { ...card, matched: true } : card);
      setCards(nextCards); setOpen([]); setLocked(false);
      if (nextCards.every((card) => card.matched)) {
        const seconds = Math.max(1, Math.round((Date.now() - started.current) / 1000));
        const score = Math.max(100, 10000 - nextMoves * 180 - seconds * 20);
        setPlaying(false); void onFinish(score, { moves: nextMoves, duration: seconds });
      }
    } else {
      timeout.current = window.setTimeout(() => { setOpen([]); setLocked(false); }, 700);
    }
  };
  const pairs = cards.filter((card) => card.matched).length / 2;
  return <Stack alignItems="center" spacing={2.5} width="100%"><Stack direction="row" justifyContent="space-between" alignItems="center" width="min(100%,580px)"><Box><Typography color="text.secondary">찾은 짝 {pairs}/8 · 시도 {moves}</Typography><Typography variant="h2">Memory Cards</Typography></Box><Button variant="contained" startIcon={<ReplayRounded />} onClick={() => void start()}>{playing ? '다시 시작' : '게임 시작'}</Button></Stack><Box sx={{ width: 'min(100%,580px)', display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: { xs: 1, sm: 1.4 } }}>{cards.map((card, index) => { const visible = card.matched || open.includes(index); return <Button key={card.id} aria-label={visible ? `${card.symbol} 카드` : `${index + 1}번 뒤집힌 카드`} onClick={() => flip(index)} sx={{ minWidth: 0, aspectRatio: '1', fontSize: { xs: '1.7rem', sm: '2.4rem' }, bgcolor: visible ? 'rgba(103,215,255,.18)' : '#1a3146', color: visible ? 'primary.main' : 'text.secondary', border: 1, borderColor: visible ? 'primary.dark' : 'divider', '&:hover': { bgcolor: visible ? 'rgba(103,215,255,.22)' : '#24435c' } }}>{visible ? card.symbol : '?'}</Button>; })}</Box>{!playing && pairs === 8 && <Typography color="secondary.main" fontWeight={800}>모든 짝을 찾았습니다! 새 기록에 도전하세요.</Typography>}<Typography color="text.secondary">카드 두 장을 선택해 같은 모양을 찾으세요.</Typography></Stack>;
}
