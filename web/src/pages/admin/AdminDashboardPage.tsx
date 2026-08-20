import AnalyticsRounded from '@mui/icons-material/AnalyticsRounded';
import GroupsRounded from '@mui/icons-material/GroupsRounded';
import PlayCircleRounded from '@mui/icons-material/PlayCircleRounded';
import SportsEsportsRounded from '@mui/icons-material/SportsEsportsRounded';
import { Alert, Box, Card, CardContent, Container, Grid, LinearProgress, Stack, Typography } from '@mui/material';
import { api } from '../../api/client';
import { useAsync } from '../../hooks/useAsync';

interface Dashboard { users?: number; dau?: number; wau?: number; mau?: number; sessions_today?: number; scores_today?: number; game_launches?: number; active_games?: number; avg_session_seconds?: number; pending_approvals?: number; popular_games?: Array<{ name: string; launches: number }> }
const cards = [
  { key: 'users', label: '활성 사용자', icon: <GroupsRounded />, color: '#67d7ff' },
  { key: 'sessions_today', label: '오늘 게임 실행', icon: <PlayCircleRounded />, color: '#9cf56b' },
  { key: 'active_games', label: '운영 게임', icon: <SportsEsportsRounded />, color: '#af8cff' },
  { key: 'scores_today', label: '오늘 등록 점수', icon: <AnalyticsRounded />, color: '#ffbd5c' },
] as const;

export function AdminDashboardPage() {
  const result = useAsync(() => api.request<Dashboard>('/api/v1/admin/dashboard'), []);
  const data = result.data ?? {};
  return <Container maxWidth="xl" sx={{ py: { xs: 3, lg: 5 } }}><Typography variant="h1" sx={{ fontSize: { xs: '2.1rem', lg: '3rem' } }}>서비스 대시보드</Typography><Typography color="text.secondary" mt={1}>igame 운영 상태와 참여 지표를 한눈에 확인합니다.</Typography>{result.loading && <LinearProgress sx={{ mt: 3 }} />}{result.error && <Alert severity="warning" sx={{ mt: 3 }}>{result.error.message}</Alert>}<Grid container spacing={2.5} mt={1}>{cards.map((card) => <Grid key={card.key} size={{ xs: 12, sm: 6, xl: 3 }}><Card><CardContent sx={{ p: 3 }}><Stack direction="row" justifyContent="space-between"><Box><Typography color="text.secondary">{card.label}</Typography><Typography sx={{ fontSize: '2.25rem', fontWeight: 900, mt: .5 }}>{(data[card.key] ?? 0).toLocaleString()}</Typography></Box><Box sx={{ width: 48, height: 48, display: 'grid', placeItems: 'center', borderRadius: 2, bgcolor: `${card.color}1f`, color: card.color }}>{card.icon}</Box></Stack></CardContent></Card></Grid>)}</Grid><Grid container spacing={2.5} mt={.5}><Grid size={{ xs: 12, lg: 8 }}><Card><CardContent sx={{ p: 3 }}><Typography variant="h3">인기 게임</Typography><Stack spacing={2} mt={3}>{data.popular_games?.length ? data.popular_games.map((game) => <Box key={game.name}><Stack direction="row" justifyContent="space-between"><Typography fontWeight={700}>{game.name}</Typography><Typography color="text.secondary">{game.launches.toLocaleString()}회</Typography></Stack><LinearProgress variant="determinate" value={Math.min(100, game.launches / Math.max(...data.popular_games!.map((item) => item.launches)) * 100)} sx={{ mt: .7, height: 7, borderRadius: 5 }} /></Box>) : <Typography color="text.secondary">집계된 실행 데이터가 없습니다.</Typography>}</Stack></CardContent></Card></Grid><Grid size={{ xs: 12, lg: 4 }}><Card><CardContent sx={{ p: 3 }}><Typography variant="h3">운영 요약</Typography><Stack spacing={2} mt={3}><Stack direction="row" justifyContent="space-between"><Typography color="text.secondary">주간 사용자</Typography><Typography fontWeight={800}>{data.wau ?? 0}</Typography></Stack><Stack direction="row" justifyContent="space-between"><Typography color="text.secondary">월간 사용자</Typography><Typography fontWeight={800}>{data.mau ?? 0}</Typography></Stack><Stack direction="row" justifyContent="space-between"><Typography color="text.secondary">승인 대기</Typography><Typography fontWeight={800} color={(data.pending_approvals ?? 0) > 0 ? 'warning.main' : 'text.primary'}>{data.pending_approvals ?? 0}</Typography></Stack></Stack></CardContent></Card></Grid></Grid></Container>;
}
