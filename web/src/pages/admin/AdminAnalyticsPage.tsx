import { Alert, Card, CardContent, Container, Grid, LinearProgress, Stack, Typography } from '@mui/material';
import { api } from '../../api/client';
import { useAsync } from '../../hooks/useAsync';

interface Analytics { dau?: number; wau?: number; mau?: number; retention?: number; avg_session_seconds?: number; completion_rate?: number; ranking_participation?: number; event_participation?: number }
const metrics: Array<[keyof Analytics, string, string]> = [['dau', 'DAU', '명'], ['wau', 'WAU', '명'], ['mau', 'MAU', '명'], ['retention', '7일 재방문', '%'], ['avg_session_seconds', '평균 세션', '초'], ['completion_rate', '완료율', '%'], ['ranking_participation', '랭킹 참여율', '%'], ['event_participation', '이벤트 참여율', '%']];
export function AdminAnalyticsPage() {
  const result = useAsync(() => api.request<Analytics>('/api/v1/admin/analytics'), []);
  return <Container maxWidth="xl" sx={{ py: 5 }}><Typography variant="h1" sx={{ fontSize: '3rem' }}>서비스 통계</Typography><Typography color="text.secondary" mt={1}>사내에 저장된 익명화 운영 지표입니다.</Typography>{result.loading && <LinearProgress sx={{ mt: 3 }} />}{result.error && <Alert severity="warning" sx={{ mt: 3 }}>{result.error.message}</Alert>}<Grid container spacing={2.5} mt={1}>{metrics.map(([key, label, unit]) => <Grid key={key} size={{ xs: 12, sm: 6, lg: 3 }}><Card><CardContent sx={{ p: 3 }}><Typography color="text.secondary">{label}</Typography><Stack direction="row" alignItems="baseline" spacing={.7} mt={1}><Typography sx={{ fontSize: '2.3rem', fontWeight: 900 }}>{result.data?.[key] ?? 0}</Typography><Typography color="text.secondary">{unit}</Typography></Stack></CardContent></Card></Grid>)}</Grid></Container>;
}
