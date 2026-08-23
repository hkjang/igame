import { Card, CardContent, Container, Grid, Skeleton, Stack, Typography } from '@mui/material';
import { api } from '../../api/client';
import { ErrorPanel } from '../../components/ErrorPanel';
import { useAsync } from '../../hooks/useAsync';

interface Analytics { dau?: number; wau?: number; mau?: number; retention?: number; avg_session_seconds?: number; completion_rate?: number; ranking_participation?: number; event_participation?: number }

const metrics: Array<[keyof Analytics, string, string]> = [
  ['dau', 'DAU', '명'], ['wau', 'WAU', '명'], ['mau', 'MAU', '명'], ['retention', '7일 재방문', '%'],
  ['avg_session_seconds', '평균 세션', '초'], ['completion_rate', '완료율', '%'],
  ['ranking_participation', '랭킹 참여율', '%'], ['event_participation', '이벤트 참여율', '%'],
];

export function AdminAnalyticsPage() {
  const result = useAsync(() => api.request<Analytics>('/api/v1/admin/analytics'), []);
  return (
    <Container maxWidth="xl" sx={{ py: 5 }}>
      <Typography variant="h1" sx={{ fontSize: { xs: '2.1rem', lg: '3rem' } }}>서비스 통계</Typography>
      <Typography color="text.secondary" mt={1}>사내에 저장된 익명화 운영 지표입니다.</Typography>
      {result.error && <ErrorPanel error={result.error} retry={() => void result.reload()} />}
      <Grid container spacing={2.5} mt={1}>
        {metrics.map(([key, label, unit]) => (
          <Grid key={key} size={{ xs: 12, sm: 6, lg: 3 }}>
            <Card>
              <CardContent sx={{ p: 3 }}>
                <Typography color="text.secondary">{label}</Typography>
                <Stack direction="row" alignItems="baseline" spacing={0.7} mt={1}>
                  {/* A failed request must never look like a real zero: an operator
                      has to be able to tell "no activity" from "not loaded". */}
                  {result.loading
                    ? <Skeleton variant="text" width={90} sx={{ fontSize: '2.3rem' }} />
                    : <Typography sx={{ fontSize: '2.3rem', fontWeight: 900 }} aria-label={result.error ? `${label} 불러오지 못함` : undefined}>
                        {result.error ? '—' : (result.data?.[key] ?? 0).toLocaleString('ko-KR')}
                      </Typography>}
                  {!result.loading && !result.error && <Typography color="text.secondary">{unit}</Typography>}
                </Stack>
              </CardContent>
            </Card>
          </Grid>
        ))}
      </Grid>
    </Container>
  );
}
