import KeyRounded from '@mui/icons-material/KeyRounded';
import PersonRounded from '@mui/icons-material/PersonRounded';
import TuneRounded from '@mui/icons-material/TuneRounded';
import { Avatar, Box, Container, Stack, Tab, Tabs, Typography } from '@mui/material';
import { Link as RouterLink, Outlet, useLocation } from 'react-router-dom';
import { useAuth } from '../state/AuthContext';

export function ProfileLayout() {
  const { user } = useAuth();
  const location = useLocation();
  const value = location.pathname.startsWith('/profile/keys') ? '/profile/keys' : location.pathname.startsWith('/profile/preferences') ? '/profile/preferences' : '/profile';
  return <Container maxWidth="lg" sx={{ py: { xs: 4, md: 6 } }}>
    <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} spacing={2.5}><Avatar src={user?.avatar_url} sx={{ width: 78, height: 78, bgcolor: 'primary.dark', fontSize: '2rem' }}>{(user?.display_name || user?.username || '?').slice(0, 1)}</Avatar><Box><Typography variant="h1" sx={{ fontSize: { xs: '2rem', md: '2.8rem' } }}>{user?.display_name || user?.username}</Typography><Typography color="text.secondary">{user?.department || '소속 정보 없음'} · 개인화 영역</Typography></Box></Stack>
    <Tabs value={value} variant="scrollable" scrollButtons="auto" aria-label="프로필 메뉴" sx={{ mt: 4, borderBottom: 1, borderColor: 'divider' }}><Tab component={RouterLink} icon={<PersonRounded />} iconPosition="start" label="프로필" value="/profile" to="/profile" /><Tab component={RouterLink} icon={<KeyRounded />} iconPosition="start" label="개인 API 키" value="/profile/keys" to="/profile/keys" /><Tab component={RouterLink} icon={<TuneRounded />} iconPosition="start" label="개인화" value="/profile/preferences" to="/profile/preferences" /></Tabs>
    <Box mt={3}><Outlet /></Box>
  </Container>;
}
