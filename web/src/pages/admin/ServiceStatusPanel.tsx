import CheckCircleRounded from '@mui/icons-material/CheckCircleRounded';
import CancelRounded from '@mui/icons-material/CancelRounded';
import StorageRounded from '@mui/icons-material/StorageRounded';
import { Alert, Card, CardContent, Chip, Divider, Grid, Skeleton, Stack, Tooltip, Typography } from '@mui/material';
import { api } from '../../api/client';
import { ErrorPanel } from '../../components/ErrorPanel';
import { useAsync } from '../../hooks/useAsync';

interface ServiceStatus {
  service: { version: string; commit: string; build_date: string; timezone: string; public_url: string; trust_proxy: boolean; https: boolean };
  database: { reachable: boolean; latency_ms: number; connections_in_use: number; connections_idle: number; connections_max: number; acquire_wait_seconds: number };
  policies: Record<string, boolean>;
  published: Array<{ slug: string; label: string; version_no: number; published_at?: string }>;
  storage: Array<{ table: string; rows: number; estimated: boolean }>;
}

const policyLabels: Array<[string, string]> = [
  ['oidc_enabled', '사내 SSO'],
  ['bootstrap_login_enabled', '관리자 로그인'],
  ['approval_enabled', '승인 흐름'],
  ['ai_enabled', 'AI 기능'],
  ['play_policy_enabled', '플레이 시간 정책'],
];

const gameNames: Record<string, string> = {
  realmguard: 'RealmGuard',
  'office-guardians': 'Office Guardians',
  'cyber-fortress': 'Cyber Fortress',
  'ai-nexus-defense': 'AI Nexus Defense',
};

const tableNames: Record<string, string> = {
  audit_logs: '감사 로그',
  game_telemetry: '게임 텔레메트리',
  game_sessions: '게임 세션',
  scores: '점수',
};

function PolicyChip({ label, on }: { label: string; on: boolean }) {
  return <Chip
    size="small"
    icon={on ? <CheckCircleRounded /> : <CancelRounded />}
    label={`${label} ${on ? '사용' : '미사용'}`}
    color={on ? 'success' : 'default'}
    variant={on ? 'filled' : 'outlined'}
  />;
}

/**
 * Gathers the operational facts an operator would otherwise hunt for across
 * five settings pages and two content studios. The runtime image has no shell,
 * so the console is the only place to see them.
 */
export function ServiceStatusPanel() {
  const status = useAsync(() => api.request<ServiceStatus>('/api/v1/admin/status'), []);
  if (status.error) return <ErrorPanel error={status.error} retry={() => void status.reload()} />;
  const data = status.data;
  const db = data?.database;

  return (
    <Card>
      <CardContent sx={{ p: 3 }}>
        <Stack direction="row" spacing={1} alignItems="center" mb={2}>
          <StorageRounded color="primary" />
          <Typography variant="h3">서비스 상태</Typography>
        </Stack>

        {status.loading ? <Skeleton variant="rounded" height={220} /> : data && (
          <Grid container spacing={3}>
            <Grid size={{ xs: 12, md: 6 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom>데이터베이스</Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap alignItems="center">
                <Chip size="small" color={db?.reachable ? 'success' : 'error'} label={db?.reachable ? `연결됨 · ${db.latency_ms}ms` : '연결 실패'} />
                <Tooltip title="사용 중 / 유휴 / 최대 커넥션">
                  <Chip size="small" variant="outlined" label={`커넥션 ${db?.connections_in_use ?? 0}·${db?.connections_idle ?? 0}/${db?.connections_max ?? 0}`} />
                </Tooltip>
              </Stack>

              <Typography variant="body2" color="text.secondary" gutterBottom mt={2.5}>서비스</Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                <Chip size="small" variant="outlined" label={`v${data.service.version}`} />
                <Chip size="small" variant="outlined" label={data.service.timezone} />
                <Chip size="small" color={data.service.https ? 'success' : 'default'} variant={data.service.https ? 'filled' : 'outlined'} label={data.service.https ? 'HTTPS 공개 URL' : '공개 URL 미설정 또는 HTTP'} />
                {data.service.trust_proxy && <Chip size="small" variant="outlined" label="프록시 헤더 신뢰" />}
              </Stack>

              <Typography variant="body2" color="text.secondary" gutterBottom mt={2.5}>정책</Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                {policyLabels.map(([key, label]) => <PolicyChip key={key} label={label} on={Boolean(data.policies[key])} />)}
              </Stack>
            </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom>게시된 콘텐츠</Typography>
              {data.published.length === 0
                ? <Typography color="text.secondary">게시된 콘텐츠 버전이 없습니다.</Typography>
                : <Stack spacing={0.8}>
                    {data.published.map((item) => (
                      <Stack key={item.slug} direction="row" justifyContent="space-between" alignItems="center" spacing={2}>
                        <Typography>{gameNames[item.slug] ?? item.slug}</Typography>
                        <Typography color="text.secondary" variant="body2" textAlign="right">
                          {item.label} · #{item.version_no}
                          {item.published_at ? ` · ${new Date(item.published_at).toLocaleDateString('ko-KR')}` : ''}
                        </Typography>
                      </Stack>
                    ))}
                  </Stack>}

              <Divider sx={{ my: 2.5 }} />
              <Typography variant="body2" color="text.secondary" gutterBottom>누적 데이터</Typography>
              <Stack spacing={0.8}>
                {data.storage.map((item) => (
                  <Stack key={item.table} direction="row" justifyContent="space-between" spacing={2}>
                    <Typography>{tableNames[item.table] ?? item.table}</Typography>
                    <Typography color="text.secondary" variant="body2">
                      {item.estimated ? '약 ' : ''}{item.rows.toLocaleString('ko-KR')}행
                    </Typography>
                  </Stack>
                ))}
              </Stack>
              <Alert severity="info" sx={{ mt: 2 }}>
                자동 보존·삭제 job이 없습니다. 행 수는 통계 기반 추정치이며, 보존 기간은 조직 정책에 따라 직접 관리해야 합니다.
              </Alert>
            </Grid>
          </Grid>
        )}
      </CardContent>
    </Card>
  );
}
