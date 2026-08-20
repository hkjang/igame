import { useMemo, useState } from 'react';
import AddRounded from '@mui/icons-material/AddRounded';
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded';
import EditRounded from '@mui/icons-material/EditRounded';
import RefreshRounded from '@mui/icons-material/RefreshRounded';
import {
  Box, Button, Card, Chip, Container, Dialog, DialogActions, DialogContent, DialogTitle,
  FormControlLabel, IconButton, LinearProgress, MenuItem, Stack, Switch, Table, TableBody,
  TableCell, TableContainer, TableHead, TableRow, TextField, Tooltip, Typography,
} from '@mui/material';
import { api } from '../../api/client';
import { ErrorPanel } from '../../components/ErrorPanel';
import { useAsync } from '../../hooks/useAsync';
import { useSnackbar } from '../../state/SnackbarContext';

type Row = Record<string, unknown> & { id?: string; code?: string; slug?: string };
type FieldType = 'text' | 'number' | 'boolean' | 'date' | 'textarea' | 'select' | 'tags' | 'json';

interface Field {
  key: string;
  label: string;
  type?: FieldType;
  options?: string[];
  required?: boolean;
  readonly?: boolean;
  lookup?: 'categories' | 'games';
}

interface ResourceConfig {
  title: string;
  description: string;
  singular: string;
  fields: Field[];
  readonly?: boolean;
  updateOnly?: boolean;
  defaults?: Row;
}

const configs: Record<string, ResourceConfig> = {
  games: {
    title: '게임 관리', description: '메타데이터 기반으로 게임을 등록하고 공개 상태를 관리합니다.', singular: '게임',
    defaults: { game_type: 'iframe', status: 'draft', ranking_enabled: true, achievement_enabled: true, season_enabled: true, min_players: 1, max_players: 1, version: '1.0.0', score_order: 'desc', score_rules: {} },
    fields: [
      { key: 'name', label: '게임명', required: true }, { key: 'slug', label: 'Slug', required: true },
      { key: 'description', label: '설명', type: 'textarea' }, { key: 'category_id', label: '카테고리', lookup: 'categories' },
      { key: 'tags', label: '태그', type: 'tags' }, { key: 'game_url', label: '게임 URL', required: true },
      { key: 'game_type', label: '실행 방식', type: 'select', options: ['embedded', 'iframe', 'external'] },
      { key: 'thumbnail_url', label: '썸네일 URL' }, { key: 'banner_url', label: '배너 URL' },
      { key: 'multiplayer', label: '멀티플레이', type: 'boolean' }, { key: 'ranking_enabled', label: '랭킹 사용', type: 'boolean' },
      { key: 'achievement_enabled', label: '업적 사용', type: 'boolean' }, { key: 'season_enabled', label: '시즌 사용', type: 'boolean' },
      { key: 'min_players', label: '최소 인원', type: 'number' }, { key: 'max_players', label: '최대 인원', type: 'number' },
      { key: 'status', label: '상태', type: 'select', options: ['draft', 'active', 'maintenance', 'disabled'] },
      { key: 'version', label: '버전' }, { key: 'developer', label: '개발자' },
      { key: 'score_order', label: '점수 정렬', type: 'select', options: ['desc', 'asc'] },
      { key: 'score_rules', label: '점수 검증 규칙 JSON', type: 'json' },
    ],
  },
  categories: {
    title: '카테고리', description: '게임 탐색에 사용할 분류와 표시 순서를 관리합니다.', singular: '카테고리', defaults: { sort_order: 0 },
    fields: [{ key: 'name', label: '이름', required: true }, { key: 'slug', label: 'Slug', required: true }, { key: 'description', label: '설명' }, { key: 'sort_order', label: '표시 순서', type: 'number' }],
  },
  users: {
    title: '사용자', description: '사용자 역할, 조직·팀 및 서비스 상태를 관리합니다.', singular: '사용자', updateOnly: true,
    fields: [{ key: 'username', label: '아이디', readonly: true }, { key: 'display_name', label: '이름' }, { key: 'department', label: '소속' }, { key: 'team', label: '팀' }, { key: 'role', label: '역할', type: 'select', options: ['user', 'manager', 'operator', 'admin'] }, { key: 'status', label: '상태', type: 'select', options: ['active', 'disabled'] }],
  },
  rankings: {
    title: '랭킹 운영', description: '이상 점수를 제외하거나 다시 유효화합니다.', singular: '랭킹 기록', updateOnly: true,
    fields: [{ key: 'display_name', label: '사용자', readonly: true }, { key: 'game_name', label: '게임', readonly: true }, { key: 'score', label: '점수', type: 'number', readonly: true }, { key: 'created_at', label: '기록 시각', readonly: true }, { key: 'status', label: '상태', type: 'select', options: ['valid', 'flagged', 'excluded'] }],
  },
  seasons: {
    title: '시즌', description: '기간별 랭킹 시즌과 기록 보관 정책을 운영합니다.', singular: '시즌', defaults: { status: 'draft' },
    fields: [{ key: 'name', label: '시즌명', required: true }, { key: 'description', label: '설명', type: 'textarea' }, { key: 'starts_at', label: '시작일', type: 'date', required: true }, { key: 'ends_at', label: '종료일', type: 'date', required: true }, { key: 'status', label: '상태', type: 'select', options: ['draft', 'active', 'closed'] }],
  },
  events: {
    title: '이벤트', description: '사내 캠페인과 부서·팀 대항 이벤트를 만듭니다.', singular: '이벤트', defaults: { event_type: 'score_attack', status: 'draft', rules: {} },
    fields: [{ key: 'name', label: '이벤트명', required: true }, { key: 'description', label: '설명', type: 'textarea' }, { key: 'event_type', label: '유형', type: 'select', options: ['score_attack', 'time_attack', 'team_battle', 'department_battle', 'attendance'] }, { key: 'game_id', label: '게임', lookup: 'games' }, { key: 'starts_at', label: '시작일', type: 'date', required: true }, { key: 'ends_at', label: '종료일', type: 'date', required: true }, { key: 'status', label: '상태', type: 'select', options: ['draft', 'active', 'closed', 'cancelled'] }, { key: 'rules', label: '이벤트 규칙 JSON', type: 'json' }],
  },
  tournaments: {
    title: '대회', description: '토너먼트와 팀 대항전을 구성합니다.', singular: '대회', defaults: { format: 'score_attack', status: 'draft', max_participants: 128, rules: {} },
    fields: [{ key: 'name', label: '대회명', required: true }, { key: 'description', label: '설명', type: 'textarea' }, { key: 'game_id', label: '게임', lookup: 'games' }, { key: 'format', label: '방식', type: 'select', options: ['score_attack', 'time_attack', 'survival', 'bracket', 'team_battle'] }, { key: 'max_participants', label: '최대 참가자', type: 'number' }, { key: 'starts_at', label: '시작일', type: 'date', required: true }, { key: 'ends_at', label: '종료일', type: 'date', required: true }, { key: 'status', label: '상태', type: 'select', options: ['draft', 'active', 'closed', 'cancelled'] }, { key: 'rules', label: '대회 규칙 JSON', type: 'json' }],
  },
  achievements: {
    title: '업적', description: '게임별·포털 공통 업적 조건을 관리합니다.', singular: '업적', defaults: { xp: 0, active: true, criteria: {} },
    fields: [{ key: 'code', label: '코드', required: true }, { key: 'name', label: '이름', required: true }, { key: 'description', label: '설명' }, { key: 'game_id', label: '게임', lookup: 'games' }, { key: 'icon_url', label: '아이콘 URL' }, { key: 'criteria', label: '조건 JSON', type: 'json' }, { key: 'xp', label: 'XP', type: 'number' }, { key: 'active', label: '사용', type: 'boolean' }],
  },
  rewards: {
    title: '보상', description: '금전 가치가 없는 배지·칭호·프레임을 관리합니다.', singular: '보상', defaults: { type: 'badge', enabled: true, metadata: {} },
    fields: [{ key: 'name', label: '보상명', required: true }, { key: 'description', label: '설명' }, { key: 'type', label: '유형', type: 'select', options: ['badge', 'title', 'avatar_frame'] }, { key: 'metadata', label: '메타데이터 JSON', type: 'json' }, { key: 'enabled', label: '사용', type: 'boolean' }],
  },
  notices: {
    title: '공지', description: '사용자 포털에 게시할 안내를 작성합니다.', singular: '공지', defaults: { status: 'draft', pinned: false },
    fields: [{ key: 'title', label: '제목', required: true }, { key: 'content', label: '내용', type: 'textarea', required: true }, { key: 'status', label: '상태', type: 'select', options: ['draft', 'published'] }, { key: 'pinned', label: '상단 고정', type: 'boolean' }, { key: 'published_at', label: '게시 시각', type: 'date' }],
  },
  banners: {
    title: '배너', description: '홈과 이벤트 영역에 노출할 배너를 관리합니다.', singular: '배너', defaults: { enabled: true, sort_order: 0 },
    fields: [{ key: 'title', label: '제목', required: true }, { key: 'image_url', label: '이미지 URL', required: true }, { key: 'link_url', label: '연결 URL' }, { key: 'starts_at', label: '노출 시작', type: 'date' }, { key: 'ends_at', label: '노출 종료', type: 'date' }, { key: 'sort_order', label: '표시 순서', type: 'number' }, { key: 'enabled', label: '사용', type: 'boolean' }],
  },
  audit: {
    title: '감사 로그', description: '관리 변경과 보안 관련 활동을 추적합니다.', singular: '로그', readonly: true,
    fields: [{ key: 'created_at', label: '시각' }, { key: 'actor_username', label: '수행자' }, { key: 'action', label: '작업' }, { key: 'resource_type', label: '대상 유형' }, { key: 'resource_id', label: '대상 ID' }, { key: 'remote_addr', label: 'IP' }],
  },
};

function display(value: unknown): React.ReactNode {
  if (typeof value === 'boolean') return <Chip size="small" color={value ? 'success' : 'default'} label={value ? '사용' : '미사용'} />;
  if (value === null || value === undefined || value === '') return '—';
  if (typeof value === 'object') return JSON.stringify(value);
  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(value)) return new Date(value).toLocaleString('ko-KR');
  return String(value);
}

function toInputValue(value: unknown, type?: FieldType): unknown {
  if (value === null || value === undefined) return type === 'boolean' ? false : '';
  if (type === 'date' && typeof value === 'string') {
    const date = new Date(value);
    if (!Number.isNaN(date.getTime())) {
      const offset = date.getTimezoneOffset() * 60_000;
      return new Date(date.getTime() - offset).toISOString().slice(0, 16);
    }
  }
  if (type === 'tags' && Array.isArray(value)) return value.join(', ');
  if (type === 'json' && typeof value !== 'string') return JSON.stringify(value, null, 2);
  return value;
}

function buildPayload(config: ResourceConfig, form: Row): Row {
  const payload: Row = {};
  for (const field of config.fields) {
    if (field.readonly) continue;
    let value = form[field.key];
    if (field.type === 'tags') {
      value = Array.isArray(value) ? value : String(value ?? '').split(',').map((item) => item.trim()).filter(Boolean);
    } else if (field.type === 'json') {
      if (typeof value === 'string') value = value.trim() ? JSON.parse(value) : {};
    } else if (field.type === 'date') {
      value = value ? new Date(String(value)).toISOString() : undefined;
    }
    if (value === '' && !field.required) continue;
    if (value !== undefined) payload[field.key] = value;
  }
  return payload;
}

export function AdminResourcePage({ resource }: { resource: string }) {
  const config = configs[resource] ?? { title: resource, description: '서비스 데이터를 관리합니다.', singular: '항목', fields: [{ key: 'name', label: '이름' }] };
  const { notify } = useSnackbar();
  const result = useAsync(() => api.adminList<Row>(resource), [resource]);
  const lookupResult = useAsync(async () => {
    const values: Record<'categories' | 'games', Array<{ value: string; label: string }>> = { categories: [], games: [] };
    if (config.fields.some((field) => field.lookup === 'categories')) {
      const response = await api.adminList<{ id: string; name: string }>('categories');
      values.categories = response.items.map((item) => ({ value: item.id, label: item.name }));
    }
    if (config.fields.some((field) => field.lookup === 'games')) {
      const response = await api.adminList<{ id: string; name: string }>('games');
      values.games = response.items.map((item) => ({ value: item.id, label: item.name }));
    }
    return values;
  }, [resource]);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Row>();
  const [form, setForm] = useState<Row>({});
  const [busy, setBusy] = useState(false);
  const columns = useMemo(() => config.fields.slice(0, 6), [config.fields]);
  const showForm = (row?: Row) => {
    const source = row ? { ...row } : { ...(config.defaults ?? {}) };
    const normalized: Row = {};
    for (const field of config.fields) normalized[field.key] = toInputValue(source[field.key], field.type);
    setEditing(row); setForm(normalized); setOpen(true);
  };
  const save = async () => {
    setBusy(true);
    try {
      const payload = buildPayload(config, form);
      const id = String(editing?.id ?? editing?.code ?? editing?.slug ?? '');
      if (editing) await api.adminUpdate(resource, id, payload); else await api.adminCreate(resource, payload);
      setOpen(false); await result.reload(); notify(`${config.singular}을 저장했습니다.`, 'success');
    } catch (cause) {
      notify(cause instanceof SyntaxError ? 'JSON 입력 형식을 확인해 주세요.' : cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error');
    } finally { setBusy(false); }
  };
  const remove = async (row: Row) => {
    if (!window.confirm(`${config.singular}을 삭제할까요?`)) return;
    const id = String(row.id ?? row.code ?? row.slug ?? '');
    try { await api.adminDelete(resource, id); await result.reload(); notify('삭제했습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '삭제하지 못했습니다.', 'error'); }
  };
  return <Container maxWidth="xl" sx={{ py: { xs: 3, lg: 5 } }}>
    <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'end' }} gap={2}>
      <Box><Typography variant="h1" sx={{ fontSize: { xs: '2.1rem', lg: '3rem' } }}>{config.title}</Typography><Typography color="text.secondary" mt={1}>{config.description}</Typography></Box>
      <Stack direction="row" spacing={1}><Button startIcon={<RefreshRounded />} onClick={() => void result.reload()}>새로고침</Button>{!config.readonly && !config.updateOnly && <Button variant="contained" startIcon={<AddRounded />} onClick={() => showForm()}>새 {config.singular}</Button>}</Stack>
    </Stack>
    {result.loading && <LinearProgress sx={{ mt: 3 }} />}
    <Box mt={3}>{result.error ? <ErrorPanel error={result.error} retry={() => void result.reload()} /> : <TableContainer component={Card}><Table aria-label={config.title}>
      <TableHead><TableRow>{columns.map((field) => <TableCell key={field.key}>{field.label}</TableCell>)}{!config.readonly && <TableCell align="right">관리</TableCell>}</TableRow></TableHead>
      <TableBody>{result.data?.items.map((row, index) => <TableRow key={String(row.id ?? row.code ?? index)}>{columns.map((field) => <TableCell key={field.key}>{display(row[field.key])}</TableCell>)}{!config.readonly && <TableCell align="right"><Tooltip title="수정"><IconButton onClick={() => showForm(row)}><EditRounded /></IconButton></Tooltip>{!config.updateOnly && <Tooltip title="삭제"><IconButton color="error" onClick={() => void remove(row)}><DeleteOutlineRounded /></IconButton></Tooltip>}</TableCell>}</TableRow>)}
        {result.data?.items.length === 0 && <TableRow><TableCell colSpan={columns.length + 1} align="center" sx={{ py: 7, color: 'text.secondary' }}>등록된 항목이 없습니다.</TableCell></TableRow>}
      </TableBody>
    </Table></TableContainer>}</Box>
    <Dialog open={open} onClose={() => !busy && setOpen(false)} fullWidth maxWidth="sm">
      <DialogTitle>{editing ? `${config.singular} 수정` : `새 ${config.singular}`}</DialogTitle>
      <DialogContent><Stack spacing={2} mt={1}>{config.fields.map((field) => field.type === 'boolean'
        ? <FormControlLabel key={field.key} control={<Switch checked={Boolean(form[field.key])} disabled={field.readonly} onChange={(event) => setForm({ ...form, [field.key]: event.target.checked })} />} label={field.label} />
        : <TextField key={field.key} label={field.label} type={field.type === 'number' ? 'number' : field.type === 'date' ? 'datetime-local' : 'text'} multiline={field.type === 'textarea' || field.type === 'json'} minRows={field.type === 'textarea' || field.type === 'json' ? 3 : undefined} required={field.required} disabled={field.readonly || (Boolean(field.lookup) && lookupResult.loading)} select={field.type === 'select' || Boolean(field.lookup)} value={String(form[field.key] ?? '')} onChange={(event) => setForm({ ...form, [field.key]: field.type === 'number' ? Number(event.target.value) : event.target.value })} helperText={field.type === 'tags' ? '쉼표로 구분' : field.type === 'json' ? '유효한 JSON 형식' : field.lookup && lookupResult.error ? '목록을 불러오지 못했습니다.' : undefined} slotProps={field.type === 'date' ? { inputLabel: { shrink: true } } : undefined}>{field.lookup && !field.required && <MenuItem value="">미지정</MenuItem>}{field.lookup ? lookupResult.data?.[field.lookup].map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>) : field.options?.map((option) => <MenuItem key={option} value={option}>{option}</MenuItem>)}</TextField>)}</Stack></DialogContent>
      <DialogActions><Button onClick={() => setOpen(false)}>취소</Button><Button variant="contained" onClick={() => void save()} disabled={busy}>저장</Button></DialogActions>
    </Dialog>
  </Container>;
}
