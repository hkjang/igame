import { type FormEvent, useState } from 'react';
import ArrowForwardRounded from '@mui/icons-material/ArrowForwardRounded';
import LockRounded from '@mui/icons-material/LockRounded';
import SportsEsportsRounded from '@mui/icons-material/SportsEsportsRounded';
import { alpha } from '@mui/material/styles';
import { Alert, Box, Button, Card, CardContent, CircularProgress, Divider, Stack, TextField, Typography } from '@mui/material';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../state/AuthContext';

export function LoginPage() {
  const { user, config, version, login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  if (user) return <Navigate to="/" replace />;
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setSubmitting(true); setError('');
    try {
      await login(username, password);
      const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname ?? '/';
      navigate(from, { replace: true });
    } catch (cause) { setError(cause instanceof Error ? cause.message : '로그인에 실패했습니다.'); }
    finally { setSubmitting(false); }
  };
  return (
    <Box sx={{ minHeight: '100dvh', display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(420px, .85fr) 1.15fr' } }}>
      <Stack justifyContent="center" sx={{ p: { xs: 3, sm: 7, xl: 11 }, position: 'relative', zIndex: 1 }}>
        <Stack direction="row" alignItems="center" spacing={1.3} mb={6}><Box sx={{ width: 48, height: 48, borderRadius: 2.5, display: 'grid', placeItems: 'center', bgcolor: 'primary.main', color: 'primary.contrastText' }}><SportsEsportsRounded fontSize="large" /></Box><Typography variant="h3" sx={{ fontWeight: 900 }}>igame</Typography></Stack>
        <Box maxWidth={510}><Typography variant="h1">잠깐의 플레이,<br /><Box component="span" color="primary.main">새로운 연결.</Box></Typography><Typography mt={2} color="text.secondary" sx={{ fontSize: '1.1rem' }}>동료들과 가볍게 경쟁하고 함께 즐기는 사내 게임 플랫폼입니다.</Typography></Box>
        <Stack direction="row" spacing={1} mt={5} flexWrap="wrap" useFlexGap>{['6가지 기본 게임', '팀·부서 랭킹', '사내 SSO'].map((label) => <Box key={label} sx={{ px: 1.5, py: .8, border: 1, borderColor: 'divider', borderRadius: 99, color: 'text.secondary' }}>{label}</Box>)}</Stack>
      </Stack>
      <Stack justifyContent="center" alignItems="center" sx={(theme) => ({ p: 3, bgcolor: alpha(theme.palette.surface.overlay, .62), borderLeft: { lg: 1 }, borderColor: 'divider' })}>
        <Card sx={{ width: '100%', maxWidth: 500, boxShadow: 12 }}><CardContent sx={{ p: { xs: 3, sm: 5 } }}>
          <Typography variant="h2">로그인</Typography><Typography color="text.secondary" mt={1} mb={3}>{config.display_name ?? config.name}에 오신 것을 환영합니다.</Typography>
          {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
          {config.oidc_enabled && <Button fullWidth size="large" variant="contained" endIcon={<ArrowForwardRounded />} href={config.oidc_login_url || '/api/v1/auth/oidc/login'}>사내 SSO로 계속</Button>}
          {config.oidc_enabled && config.bootstrap_login_enabled !== false && <Divider sx={{ my: 3 }}>또는 관리자 로그인</Divider>}
          {config.bootstrap_login_enabled !== false && <Box component="form" onSubmit={(event) => void submit(event)}><Stack spacing={2}>
            <TextField label="관리자 아이디" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required inputProps={{ minLength: 1 }} />
            <TextField label="비밀번호" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required />
            <Button type="submit" fullWidth size="large" variant={config.oidc_enabled ? 'outlined' : 'contained'} startIcon={submitting ? <CircularProgress size={20} /> : <LockRounded />} disabled={submitting}>관리자 로그인</Button>
          </Stack></Box>}
          {!config.oidc_enabled && config.bootstrap_login_enabled === false && <Alert severity="warning">사용 가능한 로그인 방식이 없습니다. 시스템 관리자에게 문의하세요.</Alert>}
          <Typography mt={4} variant="body2" color="text.secondary" textAlign="center">igame v{version.version}{version.commit ? ` · ${version.commit.slice(0, 8)}` : ''}{version.buildDate ? ` · ${new Date(version.buildDate).toLocaleDateString('ko-KR')}` : ''}</Typography>
        </CardContent></Card>
      </Stack>
    </Box>
  );
}
