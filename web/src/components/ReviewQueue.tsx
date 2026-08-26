import { useState } from 'react';
import CheckCircleRounded from '@mui/icons-material/CheckCircleRounded';
import CloseRounded from '@mui/icons-material/CloseRounded';
import FactCheckRounded from '@mui/icons-material/FactCheckRounded';
import { Alert, Box, Button, Card, CardContent, Chip, Dialog, DialogActions, DialogContent, DialogTitle, LinearProgress, Stack, TextField, Typography } from '@mui/material';
import { api } from '../api/client';
import { optionLabel, rowLabel } from '../labels';
import { useAsync } from '../hooks/useAsync';
import { useSnackbar } from '../state/SnackbarContext';
import { ErrorPanel } from './ErrorPanel';

type Review = Awaited<ReturnType<typeof api.workflowReviews>>['items'][number];

/**
 * The values that tell an approver what they are approving.
 *
 * The queue used to list a request as "admin create · game" over the names of
 * the seventeen fields it would set, and never one of their values. Two pending
 * game registrations were identical on screen apart from their timestamps, so
 * there was nothing to decide between — and nothing to decide on.
 */
const SUMMARY_FIELDS: Array<[key: string, label: string]> = [
  ['slug', 'Slug'],
  ['game_url', '게임 URL'],
  ['game_type', '실행 방식'],
  ['status', '상태'],
  ['role', '역할'],
];

function summarize(payload: Record<string, unknown> | undefined) {
  const values = payload ?? {};
  const shown = SUMMARY_FIELDS.filter(([key]) => typeof values[key] === 'string' && String(values[key]).trim())
    .map(([key, label]) => [label, optionLabel(values[key])] as const);
  return { shown, remaining: Math.max(0, Object.keys(values).length - shown.length - 1) };
}

export function ReviewQueue({ enabled = true }: { enabled?: boolean }) {
  const { notify } = useSnackbar();
  const result = useAsync(() => enabled ? api.workflowReviews() : Promise.resolve({ items: [] }), [enabled]);
  const [target, setTarget] = useState<Review>();
  const [decision, setDecision] = useState<'approved' | 'rejected'>('approved');
  const [comment, setComment] = useState('');
  const [busy, setBusy] = useState(false);
  const open = (item: Review, next: 'approved' | 'rejected') => { setTarget(item); setDecision(next); setComment(''); };
  const submit = async () => {
    if (!target) return; setBusy(true);
    try { await api.reviewWorkflow(target.id, decision, comment); setTarget(undefined); await result.reload(); notify(decision === 'approved' ? '요청을 승인하고 적용했습니다.' : '요청을 반려했습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '검토 결과를 저장하지 못했습니다.', 'error'); }
    finally { setBusy(false); }
  };
  if (!enabled) return <Alert severity="success">승인 정책이 꺼져 있어 대기 요청이 생성되지 않습니다.</Alert>;
  return <Box><Stack direction="row" spacing={1.3} alignItems="center" mb={2}><FactCheckRounded color="primary" /><Box><Typography variant="h3">검토 대기</Typography><Typography color="text.secondary">팀장·운영자가 제출된 변경을 승인하거나 반려합니다.</Typography></Box></Stack>{result.loading && <LinearProgress />}{result.error && <ErrorPanel error={result.error} retry={() => void result.reload()} />}<Stack spacing={1.5}>{result.data?.items.filter((item) => item.status === 'pending').map((item) => <Card key={item.id}><CardContent sx={{ p: 2.5 }}><Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ md: 'center' }} gap={2}><Box sx={{ flex: 1 }}><Stack direction="row" spacing={1} alignItems="center"><Chip label="대기" size="small" color="warning" /><Typography fontWeight={800}>{item.requester_username}</Typography><Typography color="text.secondary">{item.action} · {item.resource_type}</Typography></Stack><Typography variant="h3" mt={1}>{rowLabel(item.payload ?? {})}</Typography><Typography variant="body2" color="text.secondary">요청 {new Date(item.created_at).toLocaleString('ko-KR')}{item.resource_id ? ` · 대상 ${item.resource_id}` : ''}</Typography>{(() => { const { shown, remaining } = summarize(item.payload); return <Stack direction="row" flexWrap="wrap" useFlexGap spacing={.7} mt={1.2}>{shown.map(([label, value]) => <Chip key={label} size="small" variant="outlined" label={`${label} ${value}`} />)}{remaining > 0 && <Chip size="small" label={`외 ${remaining}개 필드`} />}{shown.length === 0 && remaining === 0 && <Typography variant="body2" color="text.secondary">변경 내용 없음</Typography>}</Stack>; })()}</Box><Stack direction="row" spacing={1}><Button color="error" variant="outlined" startIcon={<CloseRounded />} onClick={() => open(item, 'rejected')}>반려</Button><Button color="success" variant="contained" startIcon={<CheckCircleRounded />} onClick={() => open(item, 'approved')}>검토·승인</Button></Stack></Stack></CardContent></Card>)}{!result.loading && !result.error && !result.data?.items.some((item) => item.status === 'pending') && <Alert severity="info">검토를 기다리는 요청이 없습니다.</Alert>}</Stack><Dialog open={Boolean(target)} onClose={() => !busy && setTarget(undefined)} fullWidth maxWidth="sm"><DialogTitle>{decision === 'approved' ? '요청 검토 및 승인' : '요청 검토 및 반려'}</DialogTitle><DialogContent><Typography color="text.secondary" mb={2}>{target?.requester_username}님의 {target?.resource_type} {target?.action} 요청</Typography><Typography fontWeight={750} mb={1}>요청 변경 내용</Typography><Box component="pre" tabIndex={0} aria-label="요청 변경 내용 JSON" sx={{ m: 0, mb: 2.5, p: 2, maxHeight: 280, overflow: 'auto', bgcolor: 'surface.code', border: 1, borderColor: 'divider', borderRadius: 2, color: 'surface.codeText', fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', fontSize: '.9rem', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify(target?.payload ?? {}, null, 2)}</Box><TextField label="검토 의견" multiline minRows={3} value={comment} onChange={(event) => setComment(event.target.value)} required={decision === 'rejected'} helperText={decision === 'rejected' ? '반려 사유를 입력해 주세요.' : '선택 사항'} /></DialogContent><DialogActions><Button onClick={() => setTarget(undefined)}>취소</Button><Button variant="contained" color={decision === 'approved' ? 'success' : 'error'} disabled={busy || (decision === 'rejected' && !comment.trim())} onClick={() => void submit()}>{decision === 'approved' ? '승인하고 적용' : '반려'}</Button></DialogActions></Dialog></Box>;
}
