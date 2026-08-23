import { useState } from 'react';
import AddRounded from '@mui/icons-material/AddRounded';
import AutorenewRounded from '@mui/icons-material/AutorenewRounded';
import ContentCopyRounded from '@mui/icons-material/ContentCopyRounded';
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded';
import KeyRounded from '@mui/icons-material/KeyRounded';
import EditRounded from '@mui/icons-material/EditRounded';
import { Alert, Box, Button, Card, CardContent, Checkbox, Chip, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, IconButton, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { api } from '../api/client';
import { copyText } from '../api/clipboard';
import { ErrorPanel } from '../components/ErrorPanel';
import { LoadingScreen } from '../components/LoadingScreen';
import { useAsync } from '../hooks/useAsync';
import { useSnackbar } from '../state/SnackbarContext';
import type { PersonalKey } from '../types';

const fallbackPermissions = ['games:read', 'sessions:write', 'scores:write', 'rankings:read', 'profile:read', 'profile:write', 'ai:invoke', 'workflow:write', 'api:access', 'mcp:access'];
const permissionLabels: Record<string, string> = { 'games:read': '게임 조회', 'sessions:write': '세션 생성', 'scores:write': '점수 제출', 'rankings:read': '랭킹 조회', 'profile:read': '프로필 조회', 'profile:write': '프로필·개인 설정 변경', 'ai:invoke': 'AI 호출', 'workflow:write': '승인 워크플로', 'api:access': 'REST API', 'mcp:access': 'MCP 도구' };

export function PersonalKeysPage() {
  const { notify } = useSnackbar();
  const result = useAsync(() => api.personalKeys(), []);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('내 게임 키');
  const [selected, setSelected] = useState(['games:read', 'sessions:write', 'scores:write']);
  const [secret, setSecret] = useState('');
  const [editing, setEditing] = useState<PersonalKey>();
  const [editName, setEditName] = useState('');
  const [editPermissions, setEditPermissions] = useState<string[]>([]);
  const [editExpiry, setEditExpiry] = useState('');
  const [busy, setBusy] = useState(false);
  const availablePermissions = result.data?.available_permissions ?? fallbackPermissions;
  const create = async () => {
    setBusy(true);
    try { const created = await api.createPersonalKey({ name, permissions: selected }); setSecret(created.secret); await result.reload(); notify('새 키를 만들었습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '키를 만들지 못했습니다.', 'error'); }
    finally { setBusy(false); }
  };
  const rotate = async (id: string) => {
    if (!window.confirm('이 키를 회전하면 기존 키는 즉시 사용할 수 없습니다. 계속할까요?')) return;
    try { const next = await api.rotatePersonalKey(id); setSecret(next.secret); setOpen(true); await result.reload(); notify('키를 안전하게 회전했습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '키를 회전하지 못했습니다.', 'error'); }
  };
  const revoke = async (id: string) => {
    if (!window.confirm('이 키를 폐기할까요? 이 작업은 되돌릴 수 없습니다.')) return;
    try { await api.revokePersonalKey(id); await result.reload(); notify('키를 폐기했습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '키를 폐기하지 못했습니다.', 'error'); }
  };
  const openCreate = () => {
    const allowedSelection = selected.filter((permission) => availablePermissions.includes(permission));
    setSelected(allowedSelection.length > 0 ? allowedSelection : availablePermissions.slice(0, 1));
    setSecret('');
    setOpen(true);
  };
  const openEdit = (key: PersonalKey) => { setEditing(key); setEditName(key.name); setEditPermissions(key.permissions.filter((permission) => availablePermissions.includes(permission))); setEditExpiry(key.expires_at ? new Date(key.expires_at).toISOString().slice(0, 10) : ''); };
  const saveEdit = async () => {
    if (!editing) return; setBusy(true);
    try { await api.updatePersonalKey(editing.id, { name: editName.trim(), permissions: editPermissions, expires_at: editExpiry ? new Date(`${editExpiry}T23:59:59`).toISOString() : undefined }); setEditing(undefined); await result.reload(); notify('키 이름과 권한을 변경했습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '키를 변경하지 못했습니다.', 'error'); }
    finally { setBusy(false); }
  };
  const copySecret = async () => {
    // The key is shown once, so a failed copy has to say so rather than
    // confirm something that did not happen.
    if (await copyText(secret)) notify('키를 복사했습니다.', 'success');
    else notify('브라우저가 복사를 허용하지 않았습니다. 키를 직접 선택해 복사해 주세요.', 'warning');
  };
  return <Stack spacing={2.5}><Alert severity="info">개인 API 키는 게임 SDK와 API 자동화에만 사용하세요. 키는 생성·회전 직후 한 번만 표시되며, 브라우저 게임 코드에 저장하면 안 됩니다.</Alert><Stack direction="row" justifyContent="space-between" alignItems="center"><Box><Typography variant="h3">개인 API 키</Typography><Typography color="text.secondary">최소 권한과 주기적 키 회전을 권장합니다.</Typography></Box><Button variant="contained" startIcon={<AddRounded />} onClick={openCreate} disabled={!result.loading && availablePermissions.length === 0}>새 키</Button></Stack>
    {result.loading ? <LoadingScreen /> : result.error ? <ErrorPanel error={result.error} retry={() => void result.reload()} /> : result.data?.items.length === 0 ? <Card><CardContent sx={{ textAlign: 'center', py: 7 }}><KeyRounded sx={{ fontSize: 56, color: 'text.secondary' }} /><Typography variant="h3" mt={2}>아직 키가 없습니다</Typography></CardContent></Card> : result.data?.items.map((key) => <Card key={key.id}><CardContent sx={{ p: 2.5 }}><Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} spacing={2}><Box sx={{ flex: 1 }}><Stack direction="row" spacing={1} alignItems="center"><Typography fontWeight={800}>{key.name}</Typography><Chip size="small" label={key.status === 'active' ? '활성' : '폐기'} color={key.status === 'active' ? 'success' : 'default'} /></Stack><Typography color="text.secondary" mt={.5}>{key.prefix}•••••••• · 생성 {new Date(key.created_at).toLocaleDateString('ko-KR')}{key.expires_at ? ` · 만료 ${new Date(key.expires_at).toLocaleDateString('ko-KR')}` : ''}</Typography><Stack direction="row" spacing={.7} useFlexGap flexWrap="wrap" mt={1.5}>{key.permissions.map((permission) => <Chip key={permission} label={permission} size="small" variant="outlined" />)}</Stack></Box>{key.status === 'active' && <Stack direction="row"><Tooltip title="이름·권한 변경"><IconButton aria-label={`${key.name} 권한 변경`} onClick={() => openEdit(key)}><EditRounded /></IconButton></Tooltip><Tooltip title="키 회전"><IconButton aria-label={`${key.name} 키 회전`} onClick={() => void rotate(key.id)}><AutorenewRounded /></IconButton></Tooltip><Tooltip title="키 폐기"><IconButton color="error" aria-label={`${key.name} 키 폐기`} onClick={() => void revoke(key.id)}><DeleteOutlineRounded /></IconButton></Tooltip></Stack>}</Stack></CardContent></Card>)}
    <Dialog open={open} onClose={() => { if (!busy) setOpen(false); }} fullWidth maxWidth="sm"><DialogTitle>{secret ? '새 키를 안전하게 보관하세요' : '개인 API 키 만들기'}</DialogTitle><DialogContent>{secret ? <Stack spacing={2} mt={1}><Alert severity="warning">이 값은 다시 표시되지 않습니다. 지금 복사해 안전한 비밀 저장소에 보관하세요.</Alert><TextField value={secret} multiline InputProps={{ readOnly: true, endAdornment: <Tooltip title="복사"><IconButton aria-label="키 복사" onClick={() => void copySecret()}><ContentCopyRounded /></IconButton></Tooltip> }} /></Stack> : <Stack spacing={2} mt={1}><TextField label="키 이름" value={name} onChange={(event) => setName(event.target.value)} inputProps={{ maxLength: 100 }} /><Typography fontWeight={700}>권한</Typography><Box>{availablePermissions.map((permission) => <FormControlLabel key={permission} sx={{ width: { xs: '100%', sm: '47%' } }} control={<Checkbox checked={selected.includes(permission)} onChange={(event) => setSelected(event.target.checked ? [...selected, permission] : selected.filter((item) => item !== permission))} />} label={permissionLabels[permission] ?? permission} />)}</Box></Stack>}</DialogContent><DialogActions><Button onClick={() => setOpen(false)}>{secret ? '완료' : '취소'}</Button>{!secret && <Button variant="contained" disabled={busy || !name.trim() || selected.length === 0} onClick={() => void create()}>키 만들기</Button>}</DialogActions></Dialog>
    <Dialog open={Boolean(editing)} onClose={() => !busy && setEditing(undefined)} fullWidth maxWidth="sm"><DialogTitle>개인 API 키 권한 변경</DialogTitle><DialogContent><Stack spacing={2} mt={1}><TextField label="키 이름" value={editName} onChange={(event) => setEditName(event.target.value)} inputProps={{ maxLength: 100 }} /><TextField label="만료일" type="date" value={editExpiry} onChange={(event) => setEditExpiry(event.target.value)} slotProps={{ inputLabel: { shrink: true } }} /><Typography fontWeight={700}>권한</Typography><Box>{availablePermissions.map((permission) => <FormControlLabel key={permission} sx={{ width: { xs: '100%', sm: '47%' } }} control={<Checkbox checked={editPermissions.includes(permission)} onChange={(event) => setEditPermissions(event.target.checked ? [...editPermissions, permission] : editPermissions.filter((item) => item !== permission))} />} label={permissionLabels[permission] ?? permission} />)}</Box><Alert severity="info">권한 변경은 즉시 적용되며 키 문자열 자체는 바뀌지 않습니다. 키 유출이 의심되면 회전하세요.</Alert></Stack></DialogContent><DialogActions><Button onClick={() => setEditing(undefined)}>취소</Button><Button variant="contained" disabled={busy || !editName.trim() || editPermissions.length === 0} onClick={() => void saveEdit()}>변경 저장</Button></DialogActions></Dialog>
  </Stack>;
}
