import { useState } from 'react';
import AddRounded from '@mui/icons-material/AddRounded';
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded';
import InfoOutlined from '@mui/icons-material/InfoOutlined';
import SaveRounded from '@mui/icons-material/SaveRounded';
import { Alert, Box, Button, Card, CardContent, Checkbox, Container, Divider, FormControlLabel, Grid, MenuItem, Stack, Switch, TextField, Typography } from '@mui/material';
import { api } from '../../api/client';
import { ErrorPanel } from '../../components/ErrorPanel';
import { LoadingScreen } from '../../components/LoadingScreen';
import { useAsync } from '../../hooks/useAsync';
import { useSnackbar } from '../../state/SnackbarContext';
import { ReviewQueue } from '../../components/ReviewQueue';
import { useRetainFocus } from '../../hooks/useRetainFocus';

type Values = Record<string, unknown>;
const allPermissions = ['api:access', 'mcp:access', 'games:read', 'sessions:write', 'scores:write', 'rankings:read', 'profile:read', 'profile:write', 'ai:invoke', 'workflow:write', 'admin:*'];
const roles = ['user', 'manager', 'operator', 'admin'];
const roleLabels: Record<string, string> = { user: '일반 사용자', manager: '팀장', operator: '게임 운영자', admin: '서비스 관리자' };

function SettingCard({ title, description, children, onSave, busy }: { title: string; description: string; children: React.ReactNode; onSave: () => void; busy: boolean }) {
  // Saving disables this button, which blurred it and dropped focus to the body.
  const saveRef = useRetainFocus<HTMLButtonElement>(busy);
  return <Card><CardContent sx={{ p: { xs: 2.5, md: 3.5 } }}><Typography variant="h3">{title}</Typography><Typography color="text.secondary" mt={.7}>{description}</Typography><Divider sx={{ my: 3 }} />{children}<Button ref={saveRef} variant="contained" startIcon={<SaveRounded />} onClick={onSave} disabled={busy} sx={{ mt: 3 }}>설정 저장</Button></CardContent></Card>;
}

/**
 * Whether a write-only secret is already stored.
 *
 * The server never sends the secret back, so the field is always blank and
 * "저장된 Secret은 표시되지 않습니다" read the same whether one had been saved
 * years ago or never at all. The list endpoint reports the fact beside the
 * settings — it used to report it *inside* them, which is what broke saving.
 */
function secretHelp(stored: boolean, hasOne: string, hasNone: string) {
  // Both sentences are written out by the caller rather than built from the
  // field's name: Korean picks 이/가 by the last sound of the word before it,
  // and "Client Secret" and "API Key" do not take the same one.
  return stored ? `${hasOne} 바꿀 때만 새 값을 입력하세요. 비워두면 그대로 유지됩니다.` : hasNone;
}

function OIDCSettings({ initial, secretStored }: { initial: Values; secretStored: boolean }) {
  const { notify } = useSnackbar(); const [values, setValues] = useState(initial); const [busy, setBusy] = useState(false);
  const change = (key: string, value: unknown) => setValues({ ...values, [key]: value });
  const save = async () => { setBusy(true); try { const payload = { ...values }; if (!payload.client_secret) delete payload.client_secret; await api.saveAdminSetting('oidc', payload); notify('OIDC 설정을 저장했습니다. 다음 로그인부터 적용됩니다.', 'success'); } catch (cause) { notify(cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error'); } finally { setBusy(false); } };
  return <SettingCard title="Keycloak OIDC" description="Issuer, Client ID와 Secret만으로 사내 SSO를 연결합니다. Discovery 문서는 서버가 자동 조회합니다." onSave={() => void save()} busy={busy}><Stack spacing={2}><FormControlLabel control={<Switch checked={Boolean(values.enabled)} onChange={(event) => change('enabled', event.target.checked)} />} label="OIDC 로그인 사용" /><TextField label="Issuer URL" value={String(values.issuer ?? '')} onChange={(event) => change('issuer', event.target.value)} placeholder="https://keycloak.company.local/realms/igame" /><TextField label="Client ID" value={String(values.client_id ?? '')} onChange={(event) => change('client_id', event.target.value)} /><TextField label="Client Secret" type="password" value={String(values.client_secret ?? '')} onChange={(event) => change('client_secret', event.target.value)} helperText={secretHelp(secretStored, '저장된 Client Secret이 있습니다.', 'Client Secret이 아직 저장되어 있지 않습니다. Keycloak Client의 Secret을 입력하세요.')} autoComplete="new-password" /><TextField label="Scopes" value={Array.isArray(values.scopes) ? values.scopes.join(' ') : String(values.scopes ?? 'openid profile email')} onChange={(event) => change('scopes', event.target.value.split(/\s+/).filter(Boolean))} helperText="공백으로 구분" /><Grid container spacing={2}>{[['username_claim', '사용자 ID Claim', 'preferred_username'], ['display_name_claim', '이름 Claim', 'name'], ['email_claim', '이메일 Claim', 'email'], ['groups_claim', '그룹 Claim', 'groups'], ['department_claim', '부서 Claim', 'department'], ['team_claim', '팀 Claim', 'team']].map(([key, label, placeholder]) => <Grid key={key} size={{ xs: 12, sm: 6 }}><TextField label={label} placeholder={placeholder} value={String(values[key] ?? '')} onChange={(event) => change(key, event.target.value)} /></Grid>)}</Grid><Grid container spacing={2}>{[['admin_groups', '관리자 그룹'], ['operator_groups', '운영자 그룹'], ['manager_groups', '팀장 그룹']].map(([key, label]) => <Grid key={key} size={{ xs: 12, md: 4 }}><TextField label={label} multiline minRows={2} value={Array.isArray(values[key]) ? (values[key] as string[]).join('\n') : ''} onChange={(event) => change(key, event.target.value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean))} helperText="줄바꿈 또는 쉼표로 구분" /></Grid>)}</Grid><Alert icon={<InfoOutlined />} severity="info">Redirect URI는 현재 서비스 주소의 <strong>/api/v1/auth/oidc/callback</strong>을 Keycloak Client에 등록하세요.</Alert></Stack></SettingCard>;
}

function AISettings({ initial, secretStored }: { initial: Values; secretStored: boolean }) {
  const { notify } = useSnackbar(); const [values, setValues] = useState<Values>({ max_tokens: 4096, timeout_seconds: 120, ...initial }); const [busy, setBusy] = useState(false);
  const change = (key: string, value: unknown) => setValues({ ...values, [key]: value });
  const save = async () => { setBusy(true); try { const payload: Values = { ...values, max_tokens: Math.min(262144, Math.max(1, Number(values.max_tokens))) }; if (!payload.api_key) delete payload.api_key; await api.saveAdminSetting('ai', payload); notify('AI 설정을 저장했습니다.', 'success'); } catch (cause) { notify(cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error'); } finally { setBusy(false); } };
  return <SettingCard title="AI Runtime" description="브라우저에는 API Key를 노출하지 않고 Go 서버가 모든 AI 요청을 스트리밍으로 중계합니다." onSave={() => void save()} busy={busy}><Stack spacing={2}><FormControlLabel control={<Switch checked={Boolean(values.enabled)} onChange={(event) => change('enabled', event.target.checked)} />} label="AI 기능 사용" /><TextField label="OpenAI 호환 API Base URL" value={String(values.base_url ?? '')} onChange={(event) => change('base_url', event.target.value)} placeholder="http://ai-gateway.local/v1" /><TextField label="API Key" type="password" value={String(values.api_key ?? '')} onChange={(event) => change('api_key', event.target.value)} helperText={`${secretHelp(secretStored, '저장된 API Key가 있습니다.', 'API Key가 아직 저장되어 있지 않습니다.')} 서버에서 암호화되어 저장됩니다.`} autoComplete="new-password" /><TextField label="기본 모델" value={String(values.default_model ?? '')} onChange={(event) => change('default_model', event.target.value)} placeholder="company-llm" /><Grid container spacing={2}><Grid size={{ xs: 12, sm: 6 }}><TextField label="최대 출력 토큰" type="number" value={Number(values.max_tokens)} onChange={(event) => change('max_tokens', Number(event.target.value))} inputProps={{ min: 1, max: 262144 }} helperText="최대 256K (262,144)" /></Grid><Grid size={{ xs: 12, sm: 6 }}><TextField label="타임아웃(초)" type="number" value={Number(values.timeout_seconds)} onChange={(event) => change('timeout_seconds', Number(event.target.value))} inputProps={{ min: 10, max: 3600 }} /></Grid></Grid><FormControlLabel control={<Switch checked disabled />} label="스트리밍 응답 기본 사용 (고정)" /><Alert severity="warning">256K 출력은 모델 한도와 메모리·응답 시간을 크게 사용합니다. 실제 모델 한도에 맞게 설정하세요.</Alert></Stack></SettingCard>;
}

function ApprovalSettings({ initial }: { initial: Values }) {
  const { notify } = useSnackbar(); const [values, setValues] = useState<Values>({ separation_of_duties: true, ...initial }); const [busy, setBusy] = useState(false); const enabled = Boolean(values.enabled);
  const save = async () => { setBusy(true); try { await api.saveAdminSetting('approval', values); notify(enabled ? '팀장 검토·승인 프로세스를 적용했습니다.' : '검토·승인·반려 단계를 제외했습니다.', 'success'); } catch (cause) { notify(cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error'); } finally { setBusy(false); } };
  return <Stack spacing={3}><SettingCard title="검토·승인 정책" description="필요한 조직에서만 팀장 검토 단계를 사용합니다. 끄면 제출 즉시 반영되어 승인·반려 상태가 생성되지 않습니다." onSave={() => void save()} busy={busy}><Stack spacing={2}><FormControlLabel control={<Switch checked={enabled} onChange={(event) => setValues({ ...values, enabled: event.target.checked, manager_required: event.target.checked })} />} label="팀장 검토·승인 프로세스 사용" /><Alert severity={enabled ? 'warning' : 'success'}>{enabled ? '제출 → 팀장 검토 → 승인 또는 반려 절차가 적용됩니다.' : '현재 검토·승인·반려 프로세스가 제외되어 있습니다.'}</Alert><FormControlLabel control={<Switch checked={Boolean(values.manager_required)} disabled={!enabled} onChange={(event) => setValues({ ...values, manager_required: event.target.checked })} />} label="팀장 권한 보유자만 승인 가능" /><FormControlLabel control={<Switch checked={values.separation_of_duties !== false} disabled={!enabled} onChange={(event) => setValues({ ...values, separation_of_duties: event.target.checked })} />} label="요청자와 승인자 분리 (자기 요청 승인 금지)" />{enabled && values.separation_of_duties === false && <Alert severity="warning">자기 요청을 직접 승인할 수 있어 권한 분리 효과가 낮아집니다. 운영 환경에서는 켜는 것을 권장합니다.</Alert>}</Stack></SettingCard><ReviewQueue enabled={enabled} /></Stack>;
}

function KeyPolicySettings({ initial }: { initial: Values }) {
  const { notify } = useSnackbar(); const [available, setAvailable] = useState<string[]>(Array.isArray(initial.available_permissions) ? initial.available_permissions as string[] : allPermissions); const [maxKeys, setMaxKeys] = useState(Number(initial.max_keys ?? 10)); const [ttl, setTtl] = useState(Number(initial.max_ttl_days ?? 365)); const [rolePermissions, setRolePermissions] = useState<Record<string, string[]>>((initial.role_permissions as Record<string, string[]>) ?? {}); const [busy, setBusy] = useState(false);
  const toggle = (role: string, permission: string) => { const current = rolePermissions[role] ?? []; setRolePermissions({ ...rolePermissions, [role]: current.includes(permission) ? current.filter((item) => item !== permission) : [...current, permission] }); };
  const toggleAvailable = (permission: string) => {
    if (available.includes(permission)) {
      setAvailable(available.filter((item) => item !== permission));
      setRolePermissions(Object.fromEntries(Object.entries(rolePermissions).map(([role, permissions]) => [role, permissions.filter((item) => item !== permission)])));
    } else setAvailable([...available, permission]);
  };
  const save = async () => { setBusy(true); try { await api.saveAdminSetting('api_keys', { available_permissions: available, role_permissions: rolePermissions, max_keys: maxKeys, max_ttl_days: ttl }); notify('키 권한 정책을 저장했습니다.', 'success'); } catch (cause) { notify(cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error'); } finally { setBusy(false); } };
  return <SettingCard title="개인 키 권한 정책" description="역할별로 발급 가능한 API·MCP 권한을 제한하고 키 수명과 개수를 통제합니다." onSave={() => void save()} busy={busy}><Stack spacing={3}><Grid container spacing={2}><Grid size={{ xs: 12, sm: 6 }}><TextField label="사용자당 최대 키" type="number" value={maxKeys} onChange={(event) => setMaxKeys(Number(event.target.value))} inputProps={{ min: 1, max: 100 }} /></Grid><Grid size={{ xs: 12, sm: 6 }}><TextField label="최대 유효기간(일)" type="number" value={ttl} onChange={(event) => setTtl(Number(event.target.value))} inputProps={{ min: 1, max: 3650 }} /></Grid></Grid><Box><Typography variant="h4">서비스 전체 허용 권한</Typography><Typography color="text.secondary" mt={.5} mb={1}>끄는 권한은 모든 역할의 배정에서도 자동 제거됩니다.</Typography><Stack direction="row" flexWrap="wrap" useFlexGap>{allPermissions.map((permission) => <FormControlLabel key={permission} sx={{ minWidth: { xs: '100%', md: 210 } }} control={<Checkbox checked={available.includes(permission)} onChange={() => toggleAvailable(permission)} />} label={permission} />)}</Stack></Box><Divider />{roles.map((role) => <Box key={role}><Typography fontWeight={800} mb={1}>{roleLabels[role]}</Typography><Stack direction="row" flexWrap="wrap" useFlexGap>{available.map((permission) => <FormControlLabel key={permission} sx={{ minWidth: { xs: '100%', md: 210 } }} control={<Checkbox checked={(rolePermissions[role] ?? []).includes(permission)} onChange={() => toggle(role, permission)} />} label={permission} />)}</Stack>{available.length === 0 && <Alert severity="warning">서비스 전체 허용 권한이 없습니다.</Alert>}<Divider sx={{ mt: 1 }} /></Box>)}</Stack></SettingCard>;
}

type PlayWindow = { days?: number[]; start?: string; end?: string };

/** The allowed-play windows a stored policy actually holds. */
function playWindows(play: Values): PlayWindow[] {
  return Array.isArray(play.windows) ? (play.windows as PlayWindow[]) : [];
}

/**
 * Apply an edit to one window, leaving the other windows and the rest of the
 * policy as they are.
 *
 * The API stores any number of windows and the server checks them all, so the
 * screen edits them all. It used to show and save only the first, which meant
 * checking a day here deleted the weekend rule somebody had set through the API
 * with nothing on screen to say a window had gone.
 */
function withWindowAt(play: Values, index: number, next: Partial<PlayWindow>): Values {
  return { ...play, windows: playWindows(play).map((window, at) => (at === index ? { ...window, ...next } : window)) };
}

/** Add an empty window for the operator to fill in. */
function withNewWindow(play: Values): Values {
  return { ...play, windows: [...playWindows(play), { days: [], start: '', end: '' }] };
}

/** Remove one window. A policy with none left restricts no hour at all. */
function withoutWindowAt(play: Values, index: number): Values {
  return { ...play, windows: playWindows(play).filter((_, at) => at !== index) };
}

/**
 * What stops this policy from being saved, said the way the screen says it, or
 * "" when nothing does. A window needs both bounds, and needs them to differ:
 * the server refuses either one with its own English sentence, and a window
 * that ends at the minute it opens allows only that minute.
 */
function playPolicyProblem(play: Values): string {
  for (const window of playWindows(play)) {
    if (!window.start || !window.end) return '허용 시간대의 시작과 종료 시각을 모두 입력하세요. 쓰지 않는 시간대는 삭제하세요.';
    if (window.start === window.end) return '시작과 종료 시각이 같은 시간대는 저장할 수 없습니다. 시각을 다시 지정하거나, 시간 제한 없이 허용하려면 그 시간대를 삭제하세요.';
  }
  return '';
}

function GeneralSettings({ settings }: { settings: Record<string, Values> }) {
  const { notify } = useSnackbar(); const [service, setService] = useState(settings.service ?? {}); const [privacy, setPrivacy] = useState(settings.privacy ?? {}); const [play, setPlay] = useState(settings.play_policy ?? {}); const [limitsText, setLimitsText] = useState(JSON.stringify(settings.play_policy?.daily_limits ?? {}, null, 2)); const [busy, setBusy] = useState(false);
  const save = async () => { setBusy(true); try { const dailyLimits = JSON.parse(limitsText) as Record<string, number>; const playPayload = { ...play, daily_limits: dailyLimits }; const problem = playPolicyProblem(playPayload); if (problem) { notify(problem, 'error'); return; } await Promise.all([api.saveAdminSetting('service', service), api.saveAdminSetting('privacy', privacy), api.saveAdminSetting('play_policy', playPayload)]); setPlay(playPayload); notify('시스템 설정을 저장했습니다.', 'success'); } catch (cause) { notify(cause instanceof SyntaxError ? '게임별 일일 제한 JSON 형식을 확인해 주세요.' : cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error'); } finally { setBusy(false); } };
  const windows = playWindows(play);
  return <SettingCard title="서비스 정책" description="포털 표시, 개인정보 공개 범위와 근무시간 플레이 정책을 구성합니다." onSave={() => void save()} busy={busy}><Stack spacing={3}><Typography variant="h4">일반</Typography><Grid container spacing={2}><Grid size={{ xs: 12, sm: 6 }}><TextField label="서비스 표시 이름" value={String(service.display_name ?? 'igame')} onChange={(event) => setService({ ...service, display_name: event.target.value })} /></Grid><Grid size={{ xs: 12, sm: 6 }}><TextField label="기준 시간대" value={String(service.timezone ?? 'Asia/Seoul')} onChange={(event) => setService({ ...service, timezone: event.target.value })} /></Grid><Grid size={{ xs: 12 }}><TextField label="서비스 공개 URL" value={String(service.public_url ?? '')} onChange={(event) => setService({ ...service, public_url: event.target.value })} placeholder="https://igame.company.local" /></Grid><Grid size={{ xs: 12, sm: 6 }}><TextField label="허용 Frame Origin" multiline minRows={2} value={Array.isArray(service.allowed_frame_origins) ? service.allowed_frame_origins.join('\n') : ''} onChange={(event) => setService({ ...service, allowed_frame_origins: event.target.value.split('\n').map((item) => item.trim()).filter(Boolean) })} helperText="한 줄에 하나" /></Grid><Grid size={{ xs: 12, sm: 6 }}><TextField label="허용 Connect Origin" multiline minRows={2} value={Array.isArray(service.allowed_connect_origins) ? service.allowed_connect_origins.join('\n') : ''} onChange={(event) => setService({ ...service, allowed_connect_origins: event.target.value.split('\n').map((item) => item.trim()).filter(Boolean) })} helperText="한 줄에 하나" /></Grid></Grid><Stack><FormControlLabel control={<Switch checked={service.bootstrap_login_enabled !== false} onChange={(event) => setService({ ...service, bootstrap_login_enabled: event.target.checked })} />} label="Bootstrap 관리자 로그인 사용" /><FormControlLabel control={<Switch checked={Boolean(service.trust_proxy)} onChange={(event) => setService({ ...service, trust_proxy: event.target.checked })} />} label="신뢰 프록시 헤더 사용" /></Stack>{service.bootstrap_login_enabled === false && !settings.oidc?.enabled && <Alert severity="error">OIDC와 Bootstrap 로그인을 모두 끄면 다음 로그인부터 관리자도 접근할 수 없습니다. 먼저 OIDC 연결을 검증하세요.</Alert>}<Divider /><Typography variant="h4">개인정보와 랭킹</Typography><TextField select label="랭킹 표시 이름" value={String(privacy.ranking_name ?? 'nickname')} onChange={(event) => setPrivacy({ ...privacy, ranking_name: event.target.value })}><MenuItem value="real_name">실명</MenuItem><MenuItem value="nickname">닉네임</MenuItem></TextField><Stack direction={{ xs: 'column', sm: 'row' }}><FormControlLabel control={<Switch checked={Boolean(privacy.show_department)} onChange={(event) => setPrivacy({ ...privacy, show_department: event.target.checked })} />} label="조직명 공개" /><FormControlLabel control={<Switch checked={privacy.ranking_opt_out !== false} onChange={(event) => setPrivacy({ ...privacy, ranking_opt_out: event.target.checked })} />} label="랭킹 Opt-out 허용" /></Stack><Divider /><Typography variant="h4">플레이 시간 정책</Typography><FormControlLabel control={<Switch checked={Boolean(play.enabled)} onChange={(event) => setPlay({ ...play, enabled: event.target.checked })} />} label="게임 허용 시간 제한 사용" />{Boolean(play.enabled) && windows.length === 0 && <Alert severity="warning">허용 시간대가 비어 있어 시간 제한은 적용되지 않습니다. 아래 게임별 일일 제한만 적용됩니다.</Alert>}{windows.map((window, index) => <Box key={index} sx={{ border: 1, borderColor: 'divider', borderRadius: 2, p: 2 }}><Stack direction="row" justifyContent="space-between" alignItems="center" mb={1}><Typography fontWeight={800}>허용 시간대 {index + 1}</Typography><Button color="error" size="small" startIcon={<DeleteOutlineRounded />} onClick={() => setPlay(withoutWindowAt(play, index))}>삭제</Button></Stack><Stack direction="row" flexWrap="wrap" useFlexGap>{['일', '월', '화', '수', '목', '금', '토'].map((label, day) => <FormControlLabel key={label} control={<Checkbox checked={(window.days ?? []).includes(day)} onChange={() => setPlay(withWindowAt(play, index, { days: (window.days ?? []).includes(day) ? (window.days ?? []).filter((item) => item !== day) : [...(window.days ?? []), day] }))} />} label={label} />)}</Stack><Typography color="text.secondary" variant="body2" mb={1.5}>요일을 선택하지 않으면 모든 요일에 적용됩니다.</Typography><Grid container spacing={2}><Grid size={{ xs: 12, sm: 6 }}><TextField label="허용 시작" type="time" value={String(window.start ?? '')} onChange={(event) => setPlay(withWindowAt(play, index, { start: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} disabled={!play.enabled} /></Grid><Grid size={{ xs: 12, sm: 6 }}><TextField label="허용 종료" type="time" value={String(window.end ?? '')} onChange={(event) => setPlay(withWindowAt(play, index, { end: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} disabled={!play.enabled} /></Grid></Grid></Box>)}<Box><Button startIcon={<AddRounded />} onClick={() => setPlay(withNewWindow(play))}>허용 시간대 추가</Button></Box><Typography color="text.secondary" variant="body2">시간대를 여러 개 두면 그중 하나에만 들어도 게임을 열 수 있습니다. 종료가 시작보다 이르면 자정을 넘겨 다음 날까지 이어집니다.</Typography><TextField label="게임별 일일 제한(분) JSON" multiline minRows={3} value={limitsText} onChange={(event) => setLimitsText(event.target.value)} helperText={'예: {"snake":10,"2048":20}'} /></Stack></SettingCard>;
}

export const __testing = { playWindows, withWindowAt, withNewWindow, withoutWindowAt, playPolicyProblem };

export function AdminSettingsPage({ section }: { section: 'oidc' | 'ai' | 'approval' | 'api_keys' | 'general' }) {
  const result = useAsync(() => api.adminSettings(), []);
  const title = { oidc: 'OIDC · 보안', ai: 'AI 설정', approval: '검토·승인', api_keys: '키 권한', general: '시스템 설정' }[section];
  const revision = result.data?.updated_at ? JSON.stringify(result.data.updated_at) : 'initial';
  return <Container maxWidth="lg" sx={{ py: { xs: 3, lg: 5 } }}><Typography variant="h1" sx={{ fontSize: { xs: '2.1rem', lg: '3rem' } }}>{title}</Typography><Typography color="text.secondary" mt={1} mb={3}>환경변수 변경 없이 관리자 페이지에서 운영 정책을 안전하게 관리합니다.</Typography>{result.loading ? <LoadingScreen /> : result.error ? <ErrorPanel error={result.error} retry={() => void result.reload()} /> : result.data && (section === 'oidc' ? <OIDCSettings key={revision} initial={result.data.settings.oidc ?? {}} secretStored={Boolean(result.data.secrets?.oidc?.client_secret)} /> : section === 'ai' ? <AISettings key={revision} initial={result.data.settings.ai ?? {}} secretStored={Boolean(result.data.secrets?.ai?.api_key)} /> : section === 'approval' ? <ApprovalSettings key={revision} initial={result.data.settings.approval ?? {}} /> : section === 'api_keys' ? <KeyPolicySettings key={revision} initial={result.data.settings.api_keys ?? {}} /> : <GeneralSettings key={revision} settings={result.data.settings} />)}</Container>;
}
