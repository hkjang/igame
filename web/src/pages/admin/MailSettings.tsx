import { useState } from 'react';
import RefreshRounded from '@mui/icons-material/RefreshRounded';
import SaveRounded from '@mui/icons-material/SaveRounded';
import SendRounded from '@mui/icons-material/SendRounded';
import { Alert, Box, Button, Card, CardContent, Chip, Divider, FormControlLabel, Grid, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material';
import { api, type MailDelivery } from '../../api/client';
import { useAsync } from '../../hooks/useAsync';
import { useRetainFocus } from '../../hooks/useRetainFocus';
import { useAuth } from '../../state/AuthContext';
import { useSnackbar } from '../../state/SnackbarContext';

type Values = Record<string, unknown>;

/** The events that can be switched off one at a time, in the order the screen lists them. */
export const mailEvents: Array<{ key: string; label: string; note: string }> = [
  { key: 'notify_approval_requested', label: '승인 요청 도착', note: '게임 등록·수정 요청과 RealmGuard·Defense 콘텐츠 게시 요청이 검토 대기열에 들어오면 관리자와 요청자 팀의 팀장에게 보냅니다.' },
  { key: 'notify_approval_decided', label: '승인 결과', note: '검토자가 승인하거나 반려하면 요청한 사람에게 결과와 반려 사유를 보냅니다.' },
  { key: 'notify_ranking_moderated', label: '내 점수 조정', note: '운영자가 점수를 랭킹에서 제외하거나 검토 대상으로 표시하면 그 점수의 주인에게 보냅니다.' },
];

export const securityOptions: Array<{ value: string; label: string; note: string }> = [
  { value: 'auto', label: '자동', note: '서버가 STARTTLS를 알리면 쓰고, 아니면 평문으로 보냅니다. 사내 릴레이의 기본값입니다.' },
  { value: 'none', label: '없음', note: '항상 평문으로 보냅니다.' },
  { value: 'starttls', label: 'STARTTLS', note: '서버가 STARTTLS를 지원하지 않으면 실패합니다.' },
  { value: 'tls', label: 'TLS', note: '연결부터 TLS로 시작합니다(보통 465 포트).' },
];

/** The defaults the server fills in when a field is absent, so the screen shows them. */
export const mailDefaults: Values = { enabled: false, smtp_host: '', smtp_port: 25, security: 'auto', skip_tls_verify: false, username: '', password: '', from_address: '', from_name: 'igame', base_url: '', timeout_seconds: 10 };

/**
 * What stops this setting from being saved, said the way the screen says it,
 * or "" when nothing does. The server refuses the same things in English; the
 * screen names the field before the request leaves.
 */
export function mailProblem(values: Values): string {
  const text = (key: string) => String(values[key] ?? '').trim();
  const port = Number(values.smtp_port ?? 25);
  const timeout = Number(values.timeout_seconds ?? 10);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return '포트는 1과 65535 사이여야 합니다.';
  if (!Number.isInteger(timeout) || timeout < 1 || timeout > 300) return '제한 시간은 1초와 300초 사이여야 합니다.';
  if (!securityOptions.some((option) => option.value === text('security'))) return '보안 방식을 고르세요.';
  if (text('from_address') && !/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(text('from_address'))) return '보내는 주소는 이메일 주소여야 합니다.';
  if (text('base_url') && !/^https?:\/\/[^\s]+$/.test(text('base_url'))) return '서비스 주소는 https://로 시작하는 절대 주소여야 합니다.';
  if (!values.enabled) return '';
  if (!text('smtp_host')) return '릴레이 주소를 입력하세요.';
  if (!text('from_address')) return '보내는 주소를 입력하세요.';
  return '';
}

/** The setting as the server will store it: numbers as numbers, a blank password left out so the stored one is kept. */
export function mailPayload(values: Values): Values {
  const payload: Values = { ...values, smtp_port: Number(values.smtp_port ?? 25), timeout_seconds: Number(values.timeout_seconds ?? 10) };
  for (const key of ['smtp_host', 'username', 'from_address', 'from_name', 'base_url']) payload[key] = String(values[key] ?? '').trim();
  if (!payload.password) delete payload.password;
  return payload;
}

const statusLabel: Record<string, { label: string; color: 'default' | 'success' | 'error' | 'warning' }> = {
  sent: { label: '보냄', color: 'success' },
  failed: { label: '실패', color: 'error' },
  queued: { label: '대기', color: 'warning' },
};

const eventLabel: Record<string, string> = { approval_requested: '승인 요청', approval_decided: '승인 결과', ranking_moderated: '점수 조정', test: '시험 발송' };

function DeliveriesPanel({ revision }: { revision: number }) {
  const [status, setStatus] = useState('');
  const result = useAsync(() => api.mailDeliveries(status), [status, revision]);
  const items: MailDelivery[] = result.data?.items ?? [];
  const summary = result.data?.summary;
  return <Box>
    <Stack direction="row" alignItems="center" justifyContent="space-between" mb={1} flexWrap="wrap" gap={1}>
      <Typography variant="h4">발송 기록</Typography>
      <Stack direction="row" spacing={1} alignItems="center">
        <TextField select size="small" label="상태" value={status} onChange={(event) => setStatus(event.target.value)} sx={{ minWidth: 120 }}><MenuItem value="">전체</MenuItem><MenuItem value="sent">보냄</MenuItem><MenuItem value="failed">실패</MenuItem><MenuItem value="queued">대기</MenuItem></TextField>
        <Button size="small" startIcon={<RefreshRounded />} onClick={() => void result.reload()} disabled={result.loading}>새로 고침</Button>
      </Stack>
    </Stack>
    <Typography color="text.secondary" variant="body2" mb={1.5}>시도마다 한 줄씩 남습니다 — 언제, 어떤 이벤트로, 누구에게, 어떤 제목이었고, 되었는지. 본문은 기록하지 않습니다.{summary && summary.total > 0 ? ` 전체 ${summary.total.toLocaleString()}건: 보냄 ${(summary.status.sent ?? 0).toLocaleString()} · 실패 ${(summary.status.failed ?? 0).toLocaleString()} · 대기 ${(summary.status.queued ?? 0).toLocaleString()}.` : ''}</Typography>
    {result.error ? <Alert severity="error">{result.error.message}</Alert> : items.length === 0 ? <Alert severity="info">기록된 발송이 없습니다.</Alert> : <Table size="small"><TableHead><TableRow><TableCell>시각</TableCell><TableCell>이벤트</TableCell><TableCell>받는 사람</TableCell><TableCell>제목</TableCell><TableCell>상태</TableCell><TableCell>오류</TableCell></TableRow></TableHead><TableBody>{items.map((item) => {
      const state = statusLabel[item.status] ?? { label: item.status, color: 'default' as const };
      return <TableRow key={item.id}><TableCell sx={{ whiteSpace: 'nowrap' }}>{new Date(item.created_at).toLocaleString('ko-KR')}</TableCell><TableCell>{eventLabel[item.event] ?? item.event}</TableCell><TableCell sx={{ fontFamily: 'monospace' }}>{item.recipient}</TableCell><TableCell>{item.subject}</TableCell><TableCell><Chip size="small" color={state.color} label={`${state.label}${item.attempts > 1 ? ` (${item.attempts}회)` : ''}`} /></TableCell><TableCell sx={{ color: 'error.main', fontSize: '.8rem', maxWidth: 320, wordBreak: 'break-all' }}>{item.error_message ?? ''}</TableCell></TableRow>;
    })}</TableBody></Table>}
  </Box>;
}

export function MailSettings({ initial, secretStored }: { initial: Values; secretStored: boolean }) {
  const { notify } = useSnackbar();
  const { user } = useAuth();
  const [values, setValues] = useState<Values>({ ...mailDefaults, ...initial, password: '' });
  const [busy, setBusy] = useState(false);
  const [testing, setTesting] = useState(false);
  const [recipient, setRecipient] = useState(user?.email ?? '');
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string }>();
  const [revision, setRevision] = useState(0);
  const saveRef = useRetainFocus<HTMLButtonElement>(busy);
  const change = (key: string, value: unknown) => setValues((current) => ({ ...current, [key]: value }));
  const enabled = Boolean(values.enabled);
  const problem = mailProblem(values);
  const security = String(values.security ?? 'auto');
  const save = async () => {
    if (problem) { notify(problem, 'error'); return; }
    setBusy(true);
    try { await api.saveAdminSetting('mail', mailPayload(values)); notify(enabled ? '메일 알림을 저장했습니다. 시험 발송으로 릴레이가 받는지 확인하세요.' : '메일 알림을 저장했습니다. 꺼져 있으므로 아무것도 보내지 않습니다.', 'success'); } catch (cause) { notify(cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error'); } finally { setBusy(false); }
  };
  const sendTest = async () => {
    setTesting(true);
    setTestResult(undefined);
    try { const outcome = await api.sendTestMail(recipient.trim()); setTestResult({ ok: true, message: `${outcome.recipient}(으)로 보냈습니다. 받은 편지함을 확인하세요.` }); } catch (cause) { setTestResult({ ok: false, message: cause instanceof Error ? cause.message : '보내지 못했습니다.' }); } finally { setTesting(false); setRevision((current) => current + 1); }
  };
  const text = (key: string, label: string, extra: Record<string, unknown> = {}) => <TextField label={label} value={String(values[key] ?? '')} onChange={(event) => change(key, event.target.value)} {...extra} />;
  return <Card><CardContent sx={{ p: { xs: 2.5, md: 3.5 } }}>
    <Typography variant="h3">메일 알림</Typography>
    <Typography color="text.secondary" mt={.7}>사내 SMTP 릴레이로 사람이 기다리는 일만 보냅니다. 기본값은 꺼짐이고, 메일은 요청과 별도로 배경에서 나가므로 릴레이가 멈춰 있어도 화면은 평소처럼 동작합니다. 자기가 한 일은 자기에게 보내지 않습니다.</Typography>
    <Divider sx={{ my: 3 }} />
    <Stack spacing={2.5}>
      <FormControlLabel control={<Switch checked={enabled} onChange={(event) => change('enabled', event.target.checked)} />} label="메일 알림 사용" />
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 8 }}>{text('smtp_host', '릴레이 주소 (mail.smtp_host)', { placeholder: 'smtp.company.local' })}</Grid>
        <Grid size={{ xs: 6, md: 2 }}><TextField label="포트" type="number" value={Number(values.smtp_port ?? 25)} onChange={(event) => change('smtp_port', Number(event.target.value))} inputProps={{ min: 1, max: 65535 }} helperText="사내 릴레이는 대개 25" /></Grid>
        <Grid size={{ xs: 6, md: 2 }}><TextField label="제한 시간(초)" type="number" value={Number(values.timeout_seconds ?? 10)} onChange={(event) => change('timeout_seconds', Number(event.target.value))} inputProps={{ min: 1, max: 300 }} /></Grid>
      </Grid>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 6 }}><TextField select label="보안" value={security} onChange={(event) => change('security', event.target.value)} helperText={securityOptions.find((option) => option.value === security)?.note}>{securityOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid>
        <Grid size={{ xs: 12, md: 6 }}><FormControlLabel control={<Switch checked={Boolean(values.skip_tls_verify)} disabled={security === 'none'} onChange={(event) => change('skip_tls_verify', event.target.checked)} />} label="인증서 검증 건너뛰기 (사내 사설 인증서일 때만)" /></Grid>
      </Grid>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 6 }}>{text('username', '사용자 이름', { helperText: '인증 없는 릴레이면 비워 둡니다. 비우면 비밀번호도 저장하지 않습니다.' })}</Grid>
        <Grid size={{ xs: 12, md: 6 }}><TextField label="비밀번호" type="password" value={String(values.password ?? '')} onChange={(event) => change('password', event.target.value)} disabled={!String(values.username ?? '').trim()} helperText={secretStored ? '저장된 비밀번호가 있습니다. 바꿀 때만 입력하세요. 설정 API는 비밀번호를 돌려주지 않습니다.' : '비밀번호가 아직 저장되어 있지 않습니다.'} autoComplete="new-password" /></Grid>
      </Grid>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 6 }}>{text('from_address', '보내는 주소 (mail.from_address)', { placeholder: 'igame@company.local' })}</Grid>
        <Grid size={{ xs: 12, md: 6 }}>{text('from_name', '보내는 이름', { placeholder: 'igame' })}</Grid>
      </Grid>
      {text('base_url', '메일 속 링크가 가리킬 서비스 주소 (mail.base_url)', { placeholder: 'https://igame.company.local', helperText: '비우면 메일에 링크를 넣지 않습니다.' })}
      <Divider />
      <Typography variant="h4">보낼 이벤트</Typography>
      <Typography color="text.secondary" variant="body2">이 메일이 오지 않으면 누군가 손해를 보거나 화면을 계속 새로고침하는 일만 골랐습니다. 종류별로 끌 수 있습니다.</Typography>
      {mailEvents.map((event) => <FormControlLabel key={event.key} control={<Switch checked={values[event.key] !== false} onChange={(change_) => change(event.key, change_.target.checked)} />} label={<Box><Typography>{event.label}</Typography><Typography variant="body2" color="text.secondary">{event.note}</Typography></Box>} sx={{ alignItems: 'flex-start', '.MuiFormControlLabel-label': { pt: .6 } }} />)}
      {problem && <Alert severity="warning">{problem}</Alert>}
    </Stack>
    <Button ref={saveRef} variant="contained" startIcon={<SaveRounded />} onClick={() => void save()} disabled={busy} sx={{ mt: 3 }}>설정 저장</Button>
    <Divider sx={{ my: 3 }} />
    <Typography variant="h4">시험 발송</Typography>
    <Typography color="text.secondary" variant="body2" mt={.5} mb={1.5}>저장된 설정으로 실제 한 통을 보내고 릴레이의 대답을 그 자리에서 보여 줍니다. 릴레이 설정은 한 번에 맞는 일이 드뭅니다 — 먼저 저장한 뒤 누르세요.</Typography>
    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-start' }}>
      <TextField label="받는 사람" value={recipient} onChange={(event) => setRecipient(event.target.value)} placeholder="me@company.local" sx={{ flex: 1 }} size="small" />
      <Button variant="outlined" startIcon={<SendRounded />} onClick={() => void sendTest()} disabled={testing || !recipient.trim()}>시험 발송</Button>
    </Stack>
    {testResult && <Alert severity={testResult.ok ? 'success' : 'error'} sx={{ mt: 1.5 }}>{testResult.message}</Alert>}
    <Divider sx={{ my: 3 }} />
    <DeliveriesPanel revision={revision} />
  </CardContent></Card>;
}
