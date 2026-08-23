import SearchRounded from '@mui/icons-material/SearchRounded';
import { Alert, Box, Chip, Container, Grid, InputAdornment, Skeleton, Stack, TextField, Typography } from '@mui/material';
import { useMemo, useState } from 'react';
import { api } from '../api/client';
import { GameCard } from '../components/GameCard';
import { mergeGames } from '../data/builtinGames';
import { useAsync } from '../hooks/useAsync';
import { useSnackbar } from '../state/SnackbarContext';
import type { Game } from '../types';

export function GamesPage() {
  const { notify } = useSnackbar();
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('전체');
  const { data, loading, error, setData } = useAsync(async () => mergeGames((await api.games()).items), []);
  const games = data ?? mergeGames();
  const categories = ['전체', ...new Set(games.map((game) => game.category))];
  const filtered = useMemo(() => games.filter((game) => {
    const normalized = query.trim().toLocaleLowerCase('ko');
    const matches = !normalized || [game.name, game.description, game.category, ...game.tags].join(' ').toLocaleLowerCase('ko').includes(normalized);
    return matches && (category === '전체' || game.category === category);
  }), [games, query, category]);
  const favorite = async (game: Game) => {
    const next = !game.favorite;
    setData(games.map((item) => item.id === game.id ? { ...item, favorite: next } : item));
    try { await api.toggleFavorite(game.id, next); notify('즐겨찾기를 변경했습니다.', 'success'); }
    catch (cause) { setData(games); notify(cause instanceof Error ? cause.message : '변경하지 못했습니다.', 'error'); }
  };
  return <Container maxWidth="xl" sx={{ py: { xs: 4, md: 6 } }}>
    <Typography variant="h1" sx={{ fontSize: { xs: '2.2rem', md: '3.2rem' } }}>모든 게임</Typography><Typography color="text.secondary" mt={1}>원하는 게임을 찾고 즉시 플레이하세요.</Typography>
    <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} mt={4} mb={3} alignItems={{ md: 'center' }}><TextField value={query} onChange={(event) => setQuery(event.target.value)} placeholder="게임 이름이나 태그 검색" aria-label="게임 검색" sx={{ maxWidth: 520 }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded /></InputAdornment> } }} /><Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" role="group" aria-label="카테고리 필터">{categories.map((item) => <Chip key={item} label={item} clickable component="button" type="button" aria-pressed={category === item} color={category === item ? 'primary' : 'default'} variant={category === item ? 'filled' : 'outlined'} onClick={() => setCategory(item)} />)}</Stack></Stack>
    {error && <Alert severity="warning" sx={{ mb: 3 }}>서버 카탈로그를 불러오지 못해 기본 게임을 표시합니다. {error.message}</Alert>}
    <Grid container spacing={2.5}>{loading ? Array.from({ length: 5 }, (_, index) => <Grid key={index} size={{ xs: 12, sm: 6, md: 4, xl: 2.4 }}><Skeleton variant="rounded" height={390} /></Grid>) : filtered.map((game) => <Grid key={game.id} size={{ xs: 12, sm: 6, md: 4, xl: 2.4 }}><GameCard game={game} onFavorite={(item) => void favorite(item)} /></Grid>)}</Grid>
    {!loading && filtered.length === 0 && <Box textAlign="center" py={10}><Typography variant="h3">검색 결과가 없습니다</Typography><Typography color="text.secondary" mt={1}>검색어나 카테고리를 바꿔 보세요.</Typography></Box>}
    {/* Filtering rewrites the grid silently; announce the new count so it is not a screen-reader-only surprise. */}
    <Box role="status" aria-live="polite" sx={{ position: 'absolute', width: 1, height: 1, overflow: 'hidden', clipPath: 'inset(50%)', whiteSpace: 'nowrap' }}>{loading ? '' : `게임 ${filtered.length}개`}</Box>
  </Container>;
}
