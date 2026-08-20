import { lazy, Suspense } from 'react';
import { Alert, Box, Button, Container, Stack, Typography } from '@mui/material';
import ArrowBackRounded from '@mui/icons-material/ArrowBackRounded';
import { Link as RouterLink, useParams } from 'react-router-dom';
import { ErrorPanel } from '../components/ErrorPanel';
import { LoadingScreen } from '../components/LoadingScreen';
import { useAsync } from '../hooks/useAsync';
import { useAuth } from '../state/AuthContext';
import { getRealmGuardPreview } from './admin/realmguard/api';

const RealmGuardGame = lazy(() => import('../games/realmguard/RealmGuardGame'));

export function RealmGuardPreviewPage() {
  const { id = '' } = useParams();
  const { user } = useAuth();
  const allowed = [user?.role, ...(user?.roles ?? [])].some((role) => role && ['manager', 'operator', 'admin'].includes(role));
  const designerRole = [user?.role, ...(user?.roles ?? [])].some((role) => role && ['operator', 'admin'].includes(role));
  const resource = useAsync(() => allowed && id ? getRealmGuardPreview(id) : Promise.reject(new Error('미리보기 권한이 필요합니다.')), [allowed, id]);
  if (!allowed) return <Container sx={{ py: 6 }}><Alert severity="error">RealmGuard 미리보기는 팀장·운영자·관리자만 이용할 수 있습니다.</Alert></Container>;
  if (resource.loading) return <LoadingScreen label="RealmGuard Draft를 검증하는 중…" />;
  if (resource.error || !resource.data) return <Container sx={{ py: 6 }}><ErrorPanel error={resource.error ?? new Error('미리보기를 불러오지 못했습니다.')} retry={() => void resource.reload()} /></Container>;
  return <Container maxWidth="xl" sx={{ py: 3 }}><Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={2} mb={2}><Box><Typography variant="h1" sx={{ fontSize: { xs: '2rem', md: '2.7rem' } }}>Designer 연습 미리보기</Typography><Typography color="text.secondary">{resource.data.version.label} · 서버 검증을 통과한 Draft 스냅샷</Typography></Box><Button component={RouterLink} to={designerRole ? '/admin/realmguard' : '/reviews'} startIcon={<ArrowBackRounded />}>{designerRole ? 'Designer로 돌아가기' : '승인함으로 돌아가기'}</Button></Stack><Alert severity="warning" sx={{ mb: 2 }}>연습 전용입니다. 게임 세션을 만들지 않으며 점수·별·진행도·랭킹을 저장하지 않습니다.</Alert><Suspense fallback={<LoadingScreen label="Phaser 엔진을 불러오는 중…" />}><RealmGuardGame preview={{ config: resource.data.config, label: resource.data.version.label }} onStart={async () => true} onFinish={async () => undefined} /></Suspense></Container>;
}

export default RealmGuardPreviewPage;
