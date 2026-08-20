import { Alert, Container, Typography } from '@mui/material';
import { ReviewQueue } from '../components/ReviewQueue';
import { RealmGuardReviewQueue } from '../components/RealmGuardReviewQueue';
import { useAuth } from '../state/AuthContext';

export function ReviewsPage() {
  const { user, config } = useAuth();
  const reviewer = [user?.role, ...(user?.roles ?? [])].some((role) => role && ['manager', 'operator', 'admin'].includes(role));
  if (!reviewer) return <Container maxWidth="md" sx={{ py: 6 }}><Alert severity="error">승인함은 팀장 또는 서비스 운영자만 이용할 수 있습니다.</Alert></Container>;
  return <Container maxWidth="lg" sx={{ py: { xs: 4, md: 6 } }}><Typography variant="h1" sx={{ fontSize: { xs: '2.2rem', md: '3.2rem' } }}>승인함</Typography><Typography color="text.secondary" mt={1} mb={4}>서비스 변경 요청을 검토하고 적용 여부를 결정합니다.</Typography><ReviewQueue enabled={Boolean(config.approval_enabled)} /><RealmGuardReviewQueue enabled={Boolean(config.approval_enabled)} /></Container>;
}
