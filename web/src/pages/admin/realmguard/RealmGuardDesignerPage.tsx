import { useEffect, useMemo, useState } from 'react';
import AddRounded from '@mui/icons-material/AddRounded';
import BugReportRounded from '@mui/icons-material/BugReportRounded';
import CheckCircleRounded from '@mui/icons-material/CheckCircleRounded';
import OpenInNewRounded from '@mui/icons-material/OpenInNewRounded';
import PublishRounded from '@mui/icons-material/PublishRounded';
import SaveRounded from '@mui/icons-material/SaveRounded';
import ScienceRounded from '@mui/icons-material/ScienceRounded';
import { Alert, Box, Button, Card, CardContent, Chip, CircularProgress, Container, Dialog, DialogActions, DialogContent, DialogTitle, Divider, FormControl, Grid, InputLabel, MenuItem, Paper, Select, Stack, Tab, Tabs, TextField, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { ErrorPanel } from '../../../components/ErrorPanel';
import { useAsync } from '../../../hooks/useAsync';
import { useAuth } from '../../../state/AuthContext';
import { useSnackbar } from '../../../state/SnackbarContext';
import type { RealmSection } from '../../../games/realmguard/types';
import { realmGuardDesignerAPI, type RealmGuardVersionRecord } from './api';

const sections: Array<{ id: RealmSection; label: string }> = [
  { id: 'stages', label: 'Stages' }, { id: 'waves', label: 'Waves' }, { id: 'enemies', label: 'Enemies' },
  { id: 'bosses', label: 'Bosses' }, { id: 'towers', label: 'Towers' }, { id: 'heroes', label: 'Heroes' },
  { id: 'skills', label: 'Skills' }, { id: 'balance', label: 'Balance' },
];
type DesignerTab = RealmSection | 'versions' | 'telemetry';
const editableStatuses = new Set(['draft', 'testing']);
const statusColor: Record<string, 'default' | 'info' | 'warning' | 'success'> = { draft: 'default', testing: 'info', pending_approval: 'warning', approved: 'success', published: 'success' };

function ContentSummary({ data }: { data: unknown }) {
  if (Array.isArray(data)) return <Grid container spacing={1}>{data.slice(0, 12).map((raw, index) => { const item = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}; return <Grid key={String(item.id ?? index)} size={{ xs: 12, sm: 6, lg: 4 }}><Paper variant="outlined" sx={{ p: 1.5 }}><Typography fontWeight={800}>{String(item.name ?? item.label ?? item.id ?? `항목 ${index + 1}`)}</Typography><Typography variant="body2" color="text.secondary">{String(item.id ?? '')}</Typography></Paper></Grid>; })}<Grid size={12}><Typography variant="body2" color="text.secondary">총 {data.length}개 항목 · JSON 편집 후 서버 검증으로 참조 무결성을 확인합니다.</Typography></Grid></Grid>;
  if (data && typeof data === 'object') return <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}>{Object.keys(data).map((key) => <Chip key={key} label={key} />)}</Stack>;
  return <Typography color="text.secondary">표시할 데이터가 없습니다.</Typography>;
}

const numericKeys = new Set(['number', 'starting_gold', 'lives', 'cost', 'damage', 'range', 'fire_rate', 'projectile_speed', 'hp', 'speed', 'armor', 'reward', 'life_damage', 'radius', 'respawn_seconds', 'cooldown', 'unlock_stage']);
const quickKeys = ['id', 'name', 'label', 'title', 'subtitle', 'description', 'number', 'mode', 'theme', 'gimmick', 'starting_gold', 'lives', 'cost', 'damage', 'range', 'fire_rate', 'projectile_speed', 'hp', 'speed', 'armor', 'reward', 'life_damage', 'radius', 'respawn_seconds', 'cooldown', 'unlock_stage'];
const structuredKeys = ['path', 'paths', 'tower_spots', 'entries', 'branches', 'traits'];

function QuickEditor({ data, onChange }: { data: unknown; onChange: (data: unknown) => void }) {
  const [selected, setSelected] = useState(0);
  if (Array.isArray(data)) {
    const index = Math.min(selected, Math.max(0, data.length - 1));
    const item = data[index] && typeof data[index] === 'object' ? data[index] as Record<string, unknown> : {};
    const update = (key: string, value: unknown) => onChange(data.map((entry, itemIndex) => itemIndex === index ? { ...(entry as Record<string, unknown>), [key]: value } : entry));
    return <Stack spacing={2}><TextField select label="편집 항목" value={index} onChange={(event) => setSelected(Number(event.target.value))}>{data.map((entry, itemIndex) => { const row = entry && typeof entry === 'object' ? entry as Record<string, unknown> : {}; return <MenuItem key={String(row.id ?? itemIndex)} value={itemIndex}>{String(row.name ?? row.label ?? row.id ?? `항목 ${itemIndex + 1}`)}</MenuItem>; })}</TextField><Grid container spacing={1.5}>{quickKeys.filter((key) => key in item).map((key) => <Grid key={key} size={{ xs: 12, sm: 6 }}><TextField label={key} value={String(item[key] ?? '')} type={numericKeys.has(key) ? 'number' : 'text'} onChange={(event) => update(key, numericKeys.has(key) ? Number(event.target.value) : event.target.value)} /></Grid>)}</Grid>{structuredKeys.filter((key) => key in item).map((key) => <TextField key={key} label={`${key} JSON`} multiline minRows={3} value={JSON.stringify(item[key], null, 2)} onChange={(event) => { try { update(key, JSON.parse(event.target.value)); } catch { /* 전체 JSON 편집기에서 오류를 확인합니다. */ } }} helperText={`${Array.isArray(item[key]) ? item[key].length : 0}개 · 좌표/그룹을 JSON 배열로 편집`} />)}</Stack>;
  }
  if (data && typeof data === 'object') {
    const value = data as Record<string, unknown>;
    const difficulties = value.difficulties && typeof value.difficulties === 'object' ? value.difficulties as Record<string, Record<string, unknown>> : undefined;
    if (difficulties) return <Stack spacing={2}><Typography fontWeight={800}>난이도 밸런스</Typography>{(['casual', 'normal', 'veteran'] as const).map((difficulty) => <Paper key={difficulty} variant="outlined" sx={{ p: 1.5 }}><Typography fontWeight={800} mb={1}>{difficulty}</Typography><Grid container spacing={1}>{['enemy_hp', 'enemy_speed', 'gold', 'difficulty_bonus'].map((key) => <Grid key={key} size={{ xs: 6, md: 3 }}><TextField label={key} type="number" value={Number(difficulties[difficulty]?.[key] ?? 0)} onChange={(event) => onChange({ ...value, difficulties: { ...difficulties, [difficulty]: { ...difficulties[difficulty], [key]: Number(event.target.value) } } })} /></Grid>)}</Grid></Paper>)}<Grid container spacing={1.5}>{['endless_ramp', 'endless_wave_bonus', 'sell_refund_rate', 'clear_time_target_ms', 'clear_time_bonus_divisor', 'duration_tolerance_ms', 'min_wave_duration_ms'].filter((key) => key in value).map((key) => <Grid key={key} size={{ xs: 12, sm: 6 }}><TextField label={key} type="number" value={Number(value[key])} onChange={(event) => onChange({ ...value, [key]: Number(event.target.value) })} /></Grid>)}</Grid>{['tower_upgrade_cost', 'hero_level_xp'].filter((key) => key in value).map((key) => <TextField key={key} label={`${key} JSON`} multiline minRows={2} value={JSON.stringify(value[key])} onChange={(event) => { try { onChange({ ...value, [key]: JSON.parse(event.target.value) }); } catch { /* 전체 JSON 편집기에서 오류를 표시합니다. */ } }} />)}</Stack>;
  }
  return <Alert severity="info">구조화 편집기가 지원하지 않는 값은 전체 JSON 편집기를 사용하세요.</Alert>;
}

function VersionsPanel({ items, reload, isAdmin }: { items: RealmGuardVersionRecord[]; reload: () => Promise<unknown>; isAdmin: boolean }) {
  const { notify } = useSnackbar();
  const [busy, setBusy] = useState('');
  const action = async (id: string, kind: 'test' | 'publish') => {
    setBusy(`${id}:${kind}`);
    try {
      if (kind === 'test') {
        const response = await realmGuardDesignerAPI.testVersion(id);
        notify(`검증 통과: ${Object.entries(response.validation).map(([key, value]) => `${key} ${String(value)}`).join(' · ')}`, 'success');
      } else {
        const response = await realmGuardDesignerAPI.publishVersion(id);
        notify(response.approval_required && !response.published ? '승인함으로 게시 요청을 보냈습니다.' : '버전을 게시했습니다.', response.approval_required && !response.published ? 'info' : 'success');
      }
      await reload();
    } catch (cause) { notify(cause instanceof Error ? cause.message : '버전 작업에 실패했습니다.', 'error'); }
    finally { setBusy(''); }
  };
  return <Stack spacing={1.5}>{items.map((version) => <Card key={version.id} variant={version.status === 'published' ? 'elevation' : 'outlined'}><CardContent sx={{ p: 2.5 }}><Stack direction={{ xs: 'column', md: 'row' }} gap={2} alignItems={{ md: 'center' }}><Box flex={1}><Stack direction="row" flexWrap="wrap" useFlexGap spacing={1} alignItems="center"><Typography variant="h4">{version.label}</Typography><Chip size="small" color={statusColor[version.status] ?? 'default'} label={version.status} /><Chip size="small" variant="outlined" label={`#${version.version_no}`} /></Stack><Typography color="text.secondary" mt={.8}>{version.notes || '릴리스 노트 없음'}</Typography><Typography variant="body2" color="text.secondary" mt={1}>content {version.content_version} · stage {version.stage_version} · balance {version.balance_version} · asset {version.asset_version}</Typography></Box><Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}><Button component={RouterLink} to={`/realmguard/preview/${version.id}`} target="_blank" variant="outlined" startIcon={<OpenInNewRounded />}>연습 미리보기</Button>{editableStatuses.has(version.status) && <Button disabled={Boolean(busy)} onClick={() => void action(version.id, 'test')} startIcon={busy === `${version.id}:test` ? <CircularProgress size={18} /> : <ScienceRounded />}>검증</Button>}{isAdmin && version.status === 'pending_approval' && <Button component={RouterLink} to="/reviews" color="success" startIcon={<CheckCircleRounded />}>승인·반려</Button>}{['testing', 'approved'].includes(version.status) && <Button variant="contained" disabled={Boolean(busy)} onClick={() => void action(version.id, 'publish')} startIcon={<PublishRounded />}>{version.status === 'testing' ? '게시 요청' : '게시'}</Button>}</Stack></Stack></CardContent></Card>)}{items.length === 0 && <Alert severity="info">아직 콘텐츠 버전이 없습니다. 게시본에서 새 Draft를 만드세요.</Alert>}</Stack>;
}

export function RealmGuardDesignerPage() {
  const { user } = useAuth();
  const { notify } = useSnackbar();
  const isAdmin = [user?.role, ...(user?.roles ?? [])].includes('admin');
  const [tab, setTab] = useState<DesignerTab>('stages');
  const versions = useAsync(() => realmGuardDesignerAPI.versions(), []);
  const editable = versions.data?.items.find((item) => editableStatuses.has(item.status));
  const [versionId, setVersionId] = useState('');
  useEffect(() => { if (!versionId && editable) setVersionId(editable.id); }, [editable, versionId]);
  const section = useAsync(() => sections.some((item) => item.id === tab) && versionId ? realmGuardDesignerAPI.section(tab as RealmSection, versionId) : Promise.resolve(undefined), [tab, versionId]);
  const telemetry = useAsync(() => tab === 'telemetry' ? realmGuardDesignerAPI.telemetry(30) : Promise.resolve(undefined), [tab]);
  const [editor, setEditor] = useState('');
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [draft, setDraft] = useState({ label: '', notes: '', asset_version: '' });
  useEffect(() => { if (section.data) setEditor(JSON.stringify(section.data.data, null, 2)); }, [section.data]);
  const parsed = useMemo(() => { try { return { value: JSON.parse(editor) as unknown }; } catch (error) { return { error: error instanceof Error ? error.message : 'JSON 오류' }; } }, [editor]);
  const updateParsed = (value: unknown) => setEditor(JSON.stringify(value, null, 2));
  const save = async () => {
    if (!versionId || !sections.some((item) => item.id === tab) || parsed.error) return;
    setSaving(true);
    try {
      const checksum = section.data?.version.checksum;
      if (!checksum) throw new Error('편집 버전 checksum이 없습니다. Draft를 새로고침해 주세요.');
      await realmGuardDesignerAPI.saveSection(tab as RealmSection, parsed.value, versionId, checksum);
      notify(`${tab} Draft를 저장했습니다.`, 'success'); await Promise.all([section.reload(), versions.reload()]);
    }
    catch (cause) { notify(cause instanceof Error ? cause.message : 'Draft를 저장하지 못했습니다.', 'error'); }
    finally { setSaving(false); }
  };
  const create = async () => {
    setSaving(true);
    try { const response = await realmGuardDesignerAPI.createVersion(draft); setCreateOpen(false); setDraft({ label: '', notes: '', asset_version: '' }); setVersionId(response.version.id); setTab('stages'); await versions.reload(); notify('게시본을 복제한 새 Draft를 만들었습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : 'Draft를 만들지 못했습니다.', 'error'); }
    finally { setSaving(false); }
  };
  return <Container maxWidth="xl" sx={{ py: 4 }}><Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" gap={2}><Box><Typography variant="h1" sx={{ fontSize: { xs: '2.1rem', md: '3rem' } }}>RealmGuard Designer</Typography><Typography color="text.secondary" mt={1}>콘텐츠 Draft를 편집하고 검증·승인·게시합니다. 게시본과 실제 플레이는 같은 스냅샷을 사용합니다.</Typography></Box><Stack direction="row" spacing={1}><FormControl size="small" sx={{ minWidth: 220 }}><InputLabel>편집 버전</InputLabel><Select label="편집 버전" value={versionId} onChange={(event) => setVersionId(event.target.value)}>{versions.data?.items.filter((item) => editableStatuses.has(item.status)).map((item) => <MenuItem key={item.id} value={item.id}>{item.label} · {item.status}</MenuItem>)}</Select></FormControl><Button variant="contained" startIcon={<AddRounded />} onClick={() => setCreateOpen(true)}>새 Draft</Button></Stack></Stack><Alert severity="info" sx={{ mt: 2 }}>미리보기는 연습 전용입니다. 세션·점수·진행도·랭킹을 저장하지 않습니다.</Alert><Tabs value={tab} onChange={(_, value: DesignerTab) => setTab(value)} variant="scrollable" scrollButtons="auto" sx={{ mt: 3, borderBottom: 1, borderColor: 'divider' }}>{sections.map((item) => <Tab key={item.id} value={item.id} label={item.label} />)}<Tab value="versions" label="Versions" /><Tab value="telemetry" label="Telemetry" /></Tabs><Box mt={3}>{tab === 'versions' ? versions.error ? <ErrorPanel error={versions.error} retry={() => void versions.reload()} /> : <VersionsPanel items={versions.data?.items ?? []} reload={versions.reload} isAdmin={isAdmin} /> : tab === 'telemetry' ? telemetry.error ? <ErrorPanel error={telemetry.error} retry={() => void telemetry.reload()} /> : telemetry.loading ? <CircularProgress /> : <Card><CardContent><Stack direction="row" spacing={1} alignItems="center"><BugReportRounded color="primary" /><Typography variant="h3">최근 30일 운영 지표</Typography></Stack><Box component="pre" className="admin-scrollbar" tabIndex={0} sx={{ mt: 2, p: 2, maxHeight: 620, overflow: 'auto', bgcolor: '#050b12', borderRadius: 2, fontSize: '1rem', whiteSpace: 'pre-wrap' }}>{JSON.stringify(telemetry.data ?? {}, null, 2)}</Box></CardContent></Card> : !versionId ? <Alert severity="warning">편집 가능한 Draft가 없습니다. 새 Draft를 먼저 만드세요.</Alert> : section.error ? <ErrorPanel error={section.error} retry={() => void section.reload()} /> : section.loading ? <CircularProgress /> : <Grid container spacing={2}><Grid size={{ xs: 12, lg: 5 }}><Card><CardContent><Typography variant="h3">구조화 빠른 편집</Typography><Typography color="text.secondary" mb={2}>항목을 선택해 이름·수치·경로·그룹을 편집합니다.</Typography>{parsed.error ? <Alert severity="error">{parsed.error}</Alert> : <QuickEditor data={parsed.value} onChange={updateParsed} />}<Divider sx={{ my: 2 }} /><ContentSummary data={parsed.value} /></CardContent></Card></Grid><Grid size={{ xs: 12, lg: 7 }}><Card><CardContent><Stack direction="row" justifyContent="space-between" alignItems="center"><Box><Typography variant="h3">{tab} 전체 JSON</Typography><Typography color="text.secondary">고급 필드 편집 후 Versions에서 서버 전체 검증을 실행하세요.</Typography></Box><Button variant="contained" startIcon={saving ? <CircularProgress size={18} /> : <SaveRounded />} disabled={saving || Boolean(parsed.error)} onClick={() => void save()}>Draft 저장</Button></Stack><Divider sx={{ my: 2 }} /><TextField value={editor} onChange={(event) => setEditor(event.target.value)} multiline fullWidth minRows={20} maxRows={32} aria-label={`${tab} JSON 편집기`} inputProps={{ spellCheck: false }} sx={{ '& textarea': { fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', fontSize: '1rem', lineHeight: 1.55 } }} /></CardContent></Card></Grid></Grid>}</Box><Dialog open={createOpen} onClose={() => !saving && setCreateOpen(false)} fullWidth maxWidth="sm"><DialogTitle>새 RealmGuard Draft</DialogTitle><DialogContent><Stack spacing={2} mt={1}><TextField label="버전 라벨" value={draft.label} onChange={(event) => setDraft({ ...draft, label: event.target.value })} placeholder="비우면 서버가 자동 생성" /><TextField label="릴리스 노트" multiline minRows={4} value={draft.notes} onChange={(event) => setDraft({ ...draft, notes: event.target.value })} /><TextField label="Asset Version" value={draft.asset_version} onChange={(event) => setDraft({ ...draft, asset_version: event.target.value })} placeholder="비우면 현재 게시본 유지" /></Stack></DialogContent><DialogActions><Button onClick={() => setCreateOpen(false)}>취소</Button><Button variant="contained" disabled={saving} onClick={() => void create()}>Draft 만들기</Button></DialogActions></Dialog></Container>;
}

export default RealmGuardDesignerPage;
