import EmojiEventsRounded from '@mui/icons-material/EmojiEventsRounded';
import { Alert, Avatar, Box, Card, Container, FormControl, InputLabel, MenuItem, Select, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import { ErrorPanel } from '../components/ErrorPanel';
import { LoadingScreen } from '../components/LoadingScreen';
import { BUILTIN_GAMES } from '../data/builtinGames';
import { useAsync } from '../hooks/useAsync';

export function RankingsPage() {
  const [searchParams] = useSearchParams();
  const [game, setGame] = useState(searchParams.get('game') || 'snake');
  const [period, setPeriod] = useState('weekly');
  const [scope, setScope] = useState('individual');
  const catalog = useAsync(() => api.games(), []);
  const games = catalog.data?.items ?? BUILTIN_GAMES;
  useEffect(() => {
    if (!catalog.data) return;
    const match = catalog.data.items.find((item) => item.id === game || item.slug === game);
    const next = match?.id ?? catalog.data.items[0]?.id;
    if (next && next !== game) setGame(next);
  }, [catalog.data, game]);
  const result = useAsync(() => api.rankings(game, period, scope), [game, period, scope]);
  return <Container maxWidth="lg" sx={{ py: { xs: 4, md: 6 } }}>
    <Stack direction="row" spacing={1.5} alignItems="center"><EmojiEventsRounded color="warning" sx={{ fontSize: 42 }} /><Box><Typography variant="h1" sx={{ fontSize: { xs: '2.2rem', md: '3.2rem' } }}>랭킹</Typography><Typography color="text.secondary">개인과 조직의 최고 기록을 확인하세요.</Typography></Box></Stack>
    <Card sx={{ mt: 4, p: { xs: 2, md: 3 } }}><Stack direction={{ xs: 'column', md: 'row' }} spacing={2} alignItems={{ md: 'center' }}><FormControl sx={{ minWidth: 210 }}><InputLabel>게임</InputLabel><Select label="게임" value={games.some((item) => item.id === game) ? game : ''} onChange={(event) => setGame(event.target.value)} disabled={games.length === 0}>{games.map((item) => <MenuItem key={item.id} value={item.id}>{item.name}</MenuItem>)}</Select></FormControl><ToggleButtonGroup exclusive value={period} onChange={(_, value) => value && setPeriod(value)} aria-label="랭킹 기간"><ToggleButton value="daily">오늘</ToggleButton><ToggleButton value="weekly">주간</ToggleButton><ToggleButton value="monthly">월간</ToggleButton><ToggleButton value="all">전체</ToggleButton></ToggleButtonGroup><Box sx={{ flex: 1 }} /><ToggleButtonGroup exclusive value={scope} onChange={(_, value) => value && setScope(value)} aria-label="랭킹 범위"><ToggleButton value="individual">개인</ToggleButton><ToggleButton value="department">부서</ToggleButton><ToggleButton value="team">팀</ToggleButton></ToggleButtonGroup></Stack></Card>
    <Box mt={3}>{result.loading ? <LoadingScreen label="랭킹을 집계하는 중…" /> : result.error ? <ErrorPanel error={result.error} retry={() => void result.reload()} /> : (result.data?.items.length ?? 0) === 0 ? <Alert severity="info">아직 등록된 기록이 없습니다. 첫 기록의 주인공이 되어 보세요!</Alert> : <TableContainer component={Card}><Table aria-label="게임 랭킹"><TableHead><TableRow><TableCell width={90}>순위</TableCell><TableCell>{scope === 'individual' ? '플레이어' : scope === 'team' ? '팀' : '부서'}</TableCell>{scope === 'individual' ? <TableCell>소속</TableCell> : <TableCell>참여 인원</TableCell>}<TableCell>게임</TableCell><TableCell align="right">점수</TableCell></TableRow></TableHead><TableBody>{result.data?.items.map((entry) => <TableRow key={`${entry.rank}-${entry.user_id ?? entry.display_name}`} sx={entry.rank <= 3 ? { bgcolor: 'rgba(255,189,92,.045)' } : undefined}><TableCell><Stack direction="row" alignItems="center" spacing={1}>{entry.rank <= 3 && <EmojiEventsRounded color="warning" fontSize="small" />}<Typography fontWeight={800}>{entry.rank}</Typography></Stack></TableCell><TableCell><Stack direction="row" alignItems="center" spacing={1.3}><Avatar>{entry.display_name.slice(0, 1)}</Avatar><Typography fontWeight={700}>{entry.display_name}</Typography></Stack></TableCell><TableCell>{scope === 'individual' ? entry.department || entry.team || '—' : `${entry.members ?? 0}명`}</TableCell><TableCell>{entry.game_name || games.find((item) => item.id === game)?.name || '—'}</TableCell><TableCell align="right"><Typography fontWeight={800} color="primary.main">{entry.score.toLocaleString()}</Typography></TableCell></TableRow>)}</TableBody></Table></TableContainer>}</Box>
  </Container>;
}
