import { useState } from 'react';
import DeleteSweepRounded from '@mui/icons-material/DeleteSweepRounded';
import RefreshRounded from '@mui/icons-material/RefreshRounded';
import SaveRounded from '@mui/icons-material/SaveRounded';
import { Alert, Box, Button, Card, CardContent, Chip, Divider, FormControlLabel, Grid, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material';
import { api, type TrackingViolation } from '../../api/client';
import { useAsync } from '../../hooks/useAsync';
import { useRetainFocus } from '../../hooks/useRetainFocus';
import { useSnackbar } from '../../state/SnackbarContext';

type Values = Record<string, unknown>;

/** The providers the server accepts, in the order the screen offers them. */
export const providers: Array<{ value: string; label: string; note: string }> = [
  // Momento first: the self-hosted collector is the one choice that keeps
  // visit data inside the network.
  { value: 'momento', label: 'Momento (사내 수집기)', note: '사내에서 직접 호스팅하는 수집기입니다. 데이터가 밖으로 나가지 않는 유일한 선택지이며, 기본으로 같은 오리진 프록시를 씁니다.' },
  { value: 'ga4', label: 'Google Analytics 4', note: 'googletagmanager.com과 google-analytics.com이 정책에 추가됩니다. 폐쇄망에서는 동작하지 않습니다.' },
  { value: 'gtm', label: 'Google Tag Manager', note: 'googletagmanager.com이 정책에 추가됩니다. 컨테이너가 부르는 태그의 출처는 아래 허용 출처에 직접 더해야 합니다.' },
  { value: 'matomo', label: 'Matomo', note: 'Matomo 서버 주소가 정책에 추가됩니다.' },
  { value: 'custom', label: '직접 붙여넣기', note: '스니펫에 적힌 http(s) 출처를 읽어 정책에 더합니다. 8KB까지 저장됩니다.' },
];

export const maxSnippetBytes = 8 * 1024;

/** UTF-8 length, which is what the server measures the snippet by. */
function byteLength(text: string): number {
  return new TextEncoder().encode(text).length;
}

/**
 * What stops this setting from being saved, said the way the screen says it,
 * or "" when nothing does. The server refuses the same things in English; the
 * screen names the missing field before the request leaves.
 */
export function trackingProblem(values: Values): string {
  const provider = String(values.provider ?? 'none');
  const text = (key: string) => String(values[key] ?? '').trim();
  if (byteLength(text('custom_snippet')) > maxSnippetBytes) return `붙여넣은 추적 코드는 ${maxSnippetBytes.toLocaleString()}바이트를 넘을 수 없습니다.`;
  for (const host of text('allowed_hosts').split(/[\s,]+/).filter(Boolean)) {
    if (!/^https?:\/\/[^\s;,'"]+$/.test(host)) return `허용 출처 "${host}"는 https://host 형태여야 합니다.`;
  }
  switch (provider) {
    case 'none':
      return '';
    case 'momento':
      if (!text('momento_url') || !text('momento_site_id')) return 'Momento 수집기 주소와 사이트 ID를 모두 입력하세요.';
      if (!/^https?:\/\/[^\s;,'"]+$/.test(text('momento_url'))) return 'Momento 수집기 주소는 https://로 시작하는 절대 주소여야 합니다.';
      return '';
    case 'ga4':
    case 'gtm':
      return text('measurement_id') ? '' : '측정 ID(Measurement ID 또는 GTM 컨테이너 ID)를 입력하세요.';
    case 'matomo':
      if (!text('matomo_url') || !text('matomo_site_id')) return 'Matomo 주소와 사이트 ID를 모두 입력하세요.';
      if (!/^https?:\/\/[^\s;,'"]+$/.test(text('matomo_url'))) return 'Matomo 주소는 https://로 시작하는 절대 주소여야 합니다.';
      return '';
    case 'custom':
      return text('custom_snippet') ? '' : '붙여넣을 추적 코드가 비어 있습니다.';
    default:
      return '알 수 없는 제공자입니다.';
  }
}

/**
 * The origins a saved setting will add to the policy, so the screen can say
 * what is about to be allowed before it is. Momento behind the proxy adds
 * nothing — the tracker loads from this origin — which is the whole point.
 */
export function policyOrigins(values: Values): string[] {
  const provider = String(values.provider ?? 'none');
  const text = (key: string) => String(values[key] ?? '').trim();
  const origins: string[] = [];
  const originOf = (raw: string) => { try { const url = new URL(raw); return /^https?:$/.test(url.protocol) ? url.origin : ''; } catch { return ''; } };
  if (provider === 'momento' && values.momento_proxy === false) origins.push(originOf(text('momento_url')));
  if (provider === 'ga4' || provider === 'gtm') origins.push('https://www.googletagmanager.com', 'https://www.google-analytics.com', 'https://analytics.google.com');
  if (provider === 'matomo') origins.push(originOf(text('matomo_url')));
  if (provider === 'custom') for (const match of text('custom_snippet').matchAll(/https?:\/\/[^"'`<>\s),;\\+]+/gi)) origins.push(originOf(match[0]));
  for (const host of text('allowed_hosts').split(/[\s,]+/).filter(Boolean)) origins.push(host.includes('*') ? host.replace(/\/+$/, '') : originOf(host));
  return [...new Set(origins.filter(Boolean))];
}

/** Add one origin to the comma separated allow list without duplicating it. */
export function withAllowedHost(values: Values, origin: string): Values {
  const current = String(values.allowed_hosts ?? '').split(/[\s,]+/).filter(Boolean);
  if (current.some((host) => host.toLowerCase() === origin.toLowerCase())) return values;
  return { ...values, allowed_hosts: [...current, origin].join(', ') };
}

function ViolationsPanel({ values, onAllow }: { values: Values; onAllow: (origin: string) => void }) {
  const { notify } = useSnackbar();
  const result = useAsync(() => api.trackingViolations(), []);
  const [busy, setBusy] = useState(false);
  const clear = async () => { setBusy(true); try { await api.clearTrackingViolations(); await result.reload(); notify('차단 기록을 비웠습니다. 화면을 다시 열어 아직 막히는 출처가 있는지 확인하세요.', 'success'); } catch (cause) { notify(cause instanceof Error ? cause.message : '비우지 못했습니다.', 'error'); } finally { setBusy(false); } };
  const items: TrackingViolation[] = result.data ?? [];
  const known = new Set(policyOrigins(values).map((origin) => origin.toLowerCase()));
  return <Box>
    <Stack direction="row" alignItems="center" justifyContent="space-between" mb={1}>
      <Typography variant="h4">브라우저가 차단한 출처</Typography>
      <Stack direction="row" spacing={1}>
        <Button size="small" startIcon={<RefreshRounded />} onClick={() => void result.reload()} disabled={result.loading}>새로 고침</Button>
        <Button size="small" color="error" startIcon={<DeleteSweepRounded />} onClick={() => void clear()} disabled={busy || items.length === 0}>기록 비우기</Button>
      </Stack>
    </Stack>
    <Typography color="text.secondary" variant="body2" mb={1.5}>추적이 켜져 있는 동안 콘텐츠 보안 정책이 거부한 주소입니다. 스니펫이 부르는 출처가 여기 보이면 <strong>허용</strong>을 눌러 허용 출처에 더한 뒤 저장하세요. 기록은 서버 메모리에만 최근 100건까지 남고, 재시작하면 사라집니다.</Typography>
    {result.error ? <Alert severity="error">{result.error.message}</Alert> : items.length === 0 ? <Alert severity="success">기록된 차단이 없습니다.</Alert> : <Table size="small"><TableHead><TableRow><TableCell>출처</TableCell><TableCell>지시어</TableCell><TableCell align="right">횟수</TableCell><TableCell>마지막</TableCell><TableCell /></TableRow></TableHead><TableBody>{items.map((item) => {
      const allowed = item.allowed || known.has(item.origin.toLowerCase());
      return <TableRow key={`${item.directive} ${item.origin}`}><TableCell sx={{ fontFamily: 'monospace' }}>{item.origin}</TableCell><TableCell>{item.directive}</TableCell><TableCell align="right">{item.count}</TableCell><TableCell>{new Date(item.last_seen).toLocaleString('ko-KR')}</TableCell><TableCell align="right">{allowed ? <Chip size="small" color="success" label="허용됨" /> : <Button size="small" variant="outlined" onClick={() => onAllow(item.origin)}>허용</Button>}</TableCell></TableRow>;
    })}</TableBody></Table>}
  </Box>;
}

export function TrackingSettings({ initial }: { initial: Values }) {
  const { notify } = useSnackbar();
  const [values, setValues] = useState<Values>({ enabled: false, provider: 'none', placement: 'head', include_admin: false, momento_proxy: true, momento_environment: 'prd', ...initial });
  const [busy, setBusy] = useState(false);
  const saveRef = useRetainFocus<HTMLButtonElement>(busy);
  const change = (key: string, value: unknown) => setValues((current) => ({ ...current, [key]: value }));
  const provider = String(values.provider ?? 'none');
  const enabled = Boolean(values.enabled);
  const problem = trackingProblem(values);
  const origins = policyOrigins(values);
  const save = async () => {
    if (problem) { notify(problem, 'error'); return; }
    setBusy(true);
    try { await api.saveAdminSetting('tracking', values); notify(enabled && provider !== 'none' ? '방문 추적을 저장했습니다. 다음 페이지 로드부터 스니펫이 붙습니다.' : '방문 추적을 저장했습니다. 스니펫은 붙지 않습니다.', 'success'); } catch (cause) { notify(cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error'); } finally { setBusy(false); }
  };
  const text = (key: string, label: string, extra: Record<string, unknown> = {}) => <TextField label={label} value={String(values[key] ?? '')} onChange={(event) => change(key, event.target.value)} {...extra} />;
  return <Card><CardContent sx={{ p: { xs: 2.5, md: 3.5 } }}>
    <Typography variant="h3">방문 추적</Typography>
    <Typography color="text.secondary" mt={.7}>관리자가 화면에서 방문 추적 스크립트를 붙입니다. 기본값은 꺼짐이며, 켜면 요청마다 nonce를 만들어 스니펫의 모든 script 태그에 붙이고 같은 nonce를 정책에 넣습니다 — 정책을 'unsafe-inline'으로 푸는 일은 없습니다.</Typography>
    <Divider sx={{ my: 3 }} />
    <Stack spacing={2.5}>
      <FormControlLabel control={<Switch checked={enabled} onChange={(event) => change('enabled', event.target.checked)} />} label="방문 추적 사용" />
      <TextField select label="제공자" value={provider} onChange={(event) => change('provider', event.target.value)} helperText={providers.find((item) => item.value === provider)?.note ?? '아무 스니펫도 붙지 않습니다.'}><MenuItem value="none">사용 안 함</MenuItem>{providers.map((item) => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}</TextField>
      {provider === 'momento' && <Stack spacing={2}>
        <Grid container spacing={2}><Grid size={{ xs: 12, md: 6 }}>{text('momento_url', 'Momento 수집기 주소', { placeholder: 'https://momento.company.local' })}</Grid><Grid size={{ xs: 12, sm: 6, md: 3 }}>{text('momento_site_id', '사이트 ID', { placeholder: 'igame' })}</Grid><Grid size={{ xs: 12, sm: 6, md: 3 }}>{text('momento_environment', '환경', { placeholder: 'prd', helperText: 'data-environment 값' })}</Grid></Grid>
        <FormControlLabel control={<Switch checked={values.momento_proxy !== false} onChange={(event) => change('momento_proxy', event.target.checked)} />} label="같은 오리진 프록시 사용 (/momento/* → 수집기)" />
        <Alert severity={values.momento_proxy !== false ? 'success' : 'info'}>{values.momento_proxy !== false ? '스니펫이 /momento/tracker.js를 부르고 이벤트를 /momento로 보냅니다. 서버가 수집기로 넘기므로 정책에 외부 출처가 등장하지 않습니다.' : '브라우저가 수집기를 직접 부릅니다. 수집기 주소가 script-src·connect-src·img-src에 추가됩니다.'}</Alert>
      </Stack>}
      {(provider === 'ga4' || provider === 'gtm') && text('measurement_id', provider === 'ga4' ? '측정 ID' : 'GTM 컨테이너 ID', { placeholder: provider === 'ga4' ? 'G-XXXXXXXXXX' : 'GTM-XXXXXXX' })}
      {provider === 'matomo' && <Grid container spacing={2}><Grid size={{ xs: 12, md: 8 }}>{text('matomo_url', 'Matomo 주소', { placeholder: 'https://matomo.company.local' })}</Grid><Grid size={{ xs: 12, md: 4 }}>{text('matomo_site_id', '사이트 ID', { placeholder: '1' })}</Grid></Grid>}
      {provider === 'custom' && text('custom_snippet', '추적 코드', { multiline: true, minRows: 6, placeholder: '<script async src="https://tracker.company.local/t.js" data-site="igame"></script>', helperText: `${byteLength(String(values.custom_snippet ?? '')).toLocaleString()} / ${maxSnippetBytes.toLocaleString()}바이트. 모든 <script> 태그에 nonce가 자동으로 붙습니다.`, slotProps: { input: { sx: { fontFamily: 'monospace', fontSize: '.85rem' } } } })}
      {provider !== 'none' && <>
        {text('allowed_hosts', '허용 출처', { multiline: true, minRows: 2, placeholder: 'https://tracker.company.local, https://*.company.local', helperText: '스니펫에서 자동으로 읽지 못한 출처를 쉼표·줄바꿈으로 구분해 더합니다. https://host 형태만 받습니다.' })}
        <Grid container spacing={2}><Grid size={{ xs: 12, sm: 6 }}><TextField select label="삽입 위치" value={String(values.placement ?? 'head')} onChange={(event) => change('placement', event.target.value)}><MenuItem value="head">&lt;head&gt; 끝</MenuItem><MenuItem value="body">&lt;body&gt; 끝</MenuItem></TextField></Grid><Grid size={{ xs: 12, sm: 6 }}><FormControlLabel control={<Switch checked={Boolean(values.include_admin)} onChange={(event) => change('include_admin', event.target.checked)} />} label="관리 화면(/admin)에서도 추적" /></Grid></Grid>
        <Alert severity="info">{origins.length === 0 ? '이 설정은 정책에 외부 출처를 더하지 않습니다. script-src에는 요청별 nonce만 추가됩니다.' : <>저장하면 script-src · connect-src · img-src에 다음 출처가 더해집니다: {origins.map((origin) => <Chip key={origin} size="small" label={origin} sx={{ ml: .5, fontFamily: 'monospace' }} />)}</>}</Alert>
        <Typography color="text.secondary" variant="body2">API·MCP·상태 경로에는 붙지 않고 정책도 더 좁습니다. 로그인 화면에도 붙지만 스니펫은 개인 식별 값을 보내지 않습니다.</Typography>
      </>}
      {problem && provider !== 'none' && <Alert severity="warning">{problem}</Alert>}
      {enabled && provider !== 'none' && <><Divider /><ViolationsPanel values={values} onAllow={(origin) => { setValues((current) => withAllowedHost(current, origin)); notify(`${origin}을 허용 출처에 더했습니다. 저장해야 적용됩니다.`, 'info'); }} /></>}
    </Stack>
    <Button ref={saveRef} variant="contained" startIcon={<SaveRounded />} onClick={() => void save()} disabled={busy} sx={{ mt: 3 }}>설정 저장</Button>
  </CardContent></Card>;
}
