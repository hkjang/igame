import { useState } from 'react';
import AdminPanelSettingsRounded from '@mui/icons-material/AdminPanelSettingsRounded';
import EmojiEventsRounded from '@mui/icons-material/EmojiEventsRounded';
import EventRounded from '@mui/icons-material/EventRounded';
import ExtensionRounded from '@mui/icons-material/ExtensionRounded';
import HomeRounded from '@mui/icons-material/HomeRounded';
import KeyRounded from '@mui/icons-material/KeyRounded';
import LogoutRounded from '@mui/icons-material/LogoutRounded';
import MenuRounded from '@mui/icons-material/MenuRounded';
import PersonRounded from '@mui/icons-material/PersonRounded';
import SettingsRounded from '@mui/icons-material/SettingsRounded';
import SportsEsportsRounded from '@mui/icons-material/SportsEsportsRounded';
import SmartToyRounded from '@mui/icons-material/SmartToyRounded';
import FactCheckRounded from '@mui/icons-material/FactCheckRounded';
import { AppBar, Avatar, Box, Button, Chip, Container, Divider, IconButton, ListItemIcon, Menu, MenuItem, Stack, Toolbar, Tooltip, Typography } from '@mui/material';
import { Link as RouterLink, NavLink, Outlet, useNavigate } from 'react-router-dom';
import { useAuth } from '../state/AuthContext';

const baseNavigation = [
  { to: '/', label: '홈', icon: <HomeRounded /> },
  { to: '/games', label: '모든 게임', icon: <SportsEsportsRounded /> },
  { to: '/rankings', label: '랭킹', icon: <EmojiEventsRounded /> },
  { to: '/events', label: '이벤트', icon: <EventRounded /> },
  { to: '/ai', label: 'AI Game Lab', icon: <SmartToyRounded />, aiOnly: true },
  { to: '/reviews', label: '승인함', icon: <FactCheckRounded />, approvalOnly: true },
  { to: '/developer', label: '개발자 센터', icon: <ExtensionRounded /> },
];

export function PortalLayout() {
  const { user, version, config, logout } = useAuth();
  const navigate = useNavigate();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const isAdmin = [user?.role, ...(user?.roles ?? [])].some((role) => role && ['admin', 'operator', 'system_admin', 'service_admin'].includes(role));
  const isReviewer = [user?.role, ...(user?.roles ?? [])].some((role) => role && ['manager', 'operator', 'admin'].includes(role));
  const navigation = baseNavigation.filter((item) => (!item.aiOnly || config.ai_enabled) && (!item.approvalOnly || (config.approval_enabled && isReviewer)));
  const close = () => setAnchor(null);
  const doLogout = async () => { close(); await logout(); navigate('/login', { replace: true }); };
  return (
    <Box sx={{ minHeight: '100vh' }}>
      <AppBar position="sticky" elevation={0} sx={{ borderBottom: 1, borderColor: 'divider', bgcolor: 'rgba(7,16,29,.9)', backdropFilter: 'blur(16px)' }}>
        <Container maxWidth="xl"><Toolbar disableGutters sx={{ minHeight: 72, gap: 2 }}>
          <Stack component={RouterLink} to="/" direction="row" alignItems="center" spacing={1} aria-label="igame 홈">
            <Box sx={{ width: 38, height: 38, borderRadius: 2, display: 'grid', placeItems: 'center', color: '#061019', bgcolor: 'primary.main' }}><SportsEsportsRounded /></Box>
            <Typography variant="h4" sx={{ fontWeight: 900, letterSpacing: '-.04em' }}>{config.display_name ?? 'igame'}</Typography>
          </Stack>
          <Stack component="nav" aria-label="주 메뉴" direction="row" spacing={.5} sx={{ ml: 3, display: { xs: 'none', md: 'flex' } }}>
            {navigation.map((item) => <Button key={item.to} component={NavLink} to={item.to} end={item.to === '/'} color="inherit" startIcon={item.icon} sx={{ color: 'text.secondary', '&.active': { color: 'primary.main', bgcolor: 'rgba(103,215,255,.08)' } }}>{item.label}</Button>)}
          </Stack>
          <Box sx={{ flex: 1 }} />
          <Chip label={`Lv.${user?.level ?? 1}`} size="small" color="secondary" variant="outlined" sx={{ display: { xs: 'none', sm: 'flex' } }} />
          <Tooltip title="프로필 메뉴"><IconButton aria-label="프로필 메뉴 열기" onClick={(event) => setAnchor(event.currentTarget)}><Avatar src={user?.avatar_url} sx={{ width: 38, height: 38, bgcolor: 'primary.dark' }}>{(user?.display_name || user?.username || '?').slice(0, 1)}</Avatar></IconButton></Tooltip>
          <IconButton aria-label="모바일 메뉴" sx={{ display: { md: 'none' } }} onClick={(event) => setAnchor(event.currentTarget)}><MenuRounded /></IconButton>
          <Menu anchorEl={anchor} open={Boolean(anchor)} onClose={close} slotProps={{ paper: { className: 'admin-scrollbar', sx: { mt: 1, width: 285, maxHeight: 'min(620px,80vh)', border: 1, borderColor: 'divider' } } }}>
            <Box sx={{ px: 2, py: 1.5 }}><Typography fontWeight={750}>{user?.display_name || user?.username}</Typography><Typography variant="body2" color="text.secondary">{user?.department || '소속 정보 없음'}</Typography></Box>
            <Divider />
            <MenuItem component={RouterLink} to="/profile" onClick={close}><ListItemIcon><PersonRounded /></ListItemIcon>내 프로필</MenuItem>
            <MenuItem component={RouterLink} to="/profile/keys" onClick={close}><ListItemIcon><KeyRounded /></ListItemIcon>개인 키 관리</MenuItem>
            <MenuItem component={RouterLink} to="/profile/preferences" onClick={close}><ListItemIcon><SettingsRounded /></ListItemIcon>개인화 설정</MenuItem>
            <Divider />
            {navigation.map((item) => <MenuItem key={item.to} component={RouterLink} to={item.to} onClick={close} sx={{ display: { md: 'none' } }}><ListItemIcon>{item.icon}</ListItemIcon>{item.label}</MenuItem>)}
            {isAdmin && <MenuItem component={RouterLink} to="/admin" onClick={close}><ListItemIcon><AdminPanelSettingsRounded /></ListItemIcon>서비스 관리</MenuItem>}
            <Divider />
            <Box sx={{ px: 2, py: 1 }}><Typography variant="body2" color="text.secondary">igame v{version.version}{version.commit ? ` · ${version.commit.slice(0, 8)}` : ''}</Typography></Box>
            <MenuItem onClick={() => void doLogout()}><ListItemIcon><LogoutRounded /></ListItemIcon>로그아웃</MenuItem>
          </Menu>
        </Toolbar></Container>
      </AppBar>
      <Box component="main"><Outlet /></Box>
      <Box component="footer" sx={{ mt: 8, borderTop: 1, borderColor: 'divider', py: 3 }}><Container maxWidth="xl"><Typography variant="body2" color="text.secondary">{config.display_name ?? 'igame'} · igame platform · v{version.version}</Typography></Container></Box>
    </Box>
  );
}
