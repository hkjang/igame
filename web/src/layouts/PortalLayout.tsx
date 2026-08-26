import { useRef, useState } from 'react';
import AdminPanelSettingsRounded from '@mui/icons-material/AdminPanelSettingsRounded';
import EmojiEventsRounded from '@mui/icons-material/EmojiEventsRounded';
import EventRounded from '@mui/icons-material/EventRounded';
import CampaignRounded from '@mui/icons-material/CampaignRounded';
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
import DarkModeRounded from '@mui/icons-material/DarkModeRounded';
import LightModeRounded from '@mui/icons-material/LightModeRounded';
import { alpha } from '@mui/material/styles';
import { AppBar, Avatar, Box, Button, Chip, Container, Divider, IconButton, ListItemIcon, Menu, MenuItem, Stack, Toolbar, Tooltip, Typography } from '@mui/material';
import { Link as RouterLink, NavLink, Outlet, useNavigate } from 'react-router-dom';
import { MAIN_CONTENT_ID, RouteChrome } from '../components/RouteChrome';
import { useAuth } from '../state/AuthContext';
import { useThemeMode } from '../state/ThemeModeContext';

const baseNavigation = [
  { to: '/', label: '홈', icon: <HomeRounded /> },
  { to: '/games', label: '모든 게임', icon: <SportsEsportsRounded /> },
  { to: '/rankings', label: '랭킹', icon: <EmojiEventsRounded /> },
  { to: '/events', label: '이벤트', icon: <EventRounded /> },
  { to: '/notices', label: '공지사항', icon: <CampaignRounded /> },
  { to: '/ai', label: 'AI Game Lab', icon: <SmartToyRounded />, aiOnly: true },
  { to: '/reviews', label: '승인함', icon: <FactCheckRounded />, approvalOnly: true },
  { to: '/developer', label: '개발자 센터', icon: <ExtensionRounded /> },
];

export function PortalLayout() {
  const { user, version, config, logout } = useAuth();
  const { mode, toggle } = useThemeMode();
  const navigate = useNavigate();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const mainRef = useRef<HTMLElement>(null);
  const isAdmin = [user?.role, ...(user?.roles ?? [])].some((role) => role && ['admin', 'operator', 'system_admin', 'service_admin'].includes(role));
  const isReviewer = [user?.role, ...(user?.roles ?? [])].some((role) => role && ['manager', 'operator', 'admin'].includes(role));
  const navigation = baseNavigation.filter((item) => (!item.aiOnly || config.ai_enabled) && (!item.approvalOnly || (config.approval_enabled && isReviewer)));
  const close = () => setAnchor(null);
  const doLogout = async () => { close(); await logout(); navigate('/login', { replace: true }); };
  return (
    <Box sx={{ minHeight: '100dvh', display: 'flex', flexDirection: 'column' }}>
      <RouteChrome mainRef={mainRef} />
      {/* The bar paints its own translucent ground, so it also has to set its
          own text colour: inheriting the default `primary.contrastText` put
          white text on the light ground and near-black on the dark one, which
          left the wordmark invisible in both themes. */}
      <AppBar position="sticky" elevation={0} color="transparent" sx={(theme) => ({ color: 'text.primary', borderBottom: 1, borderColor: 'divider', bgcolor: alpha(theme.palette.surface.ground, .9), backdropFilter: 'blur(16px)' })}>
        {/* flexWrap: a breakpoint only knows the width of the window. A reader who
            has raised their browser's default font gets the same 1280px window
            with a header that no longer fits it, and the account controls hung
            39px off the right edge. Wrapping answers both: the nav drops to its
            own row and nothing leaves the page. */}
        <Container maxWidth="xl"><Toolbar disableGutters sx={{ minHeight: 72, gap: 2, flexWrap: 'wrap', rowGap: 1, py: { xs: 0, } }}>
          <Stack component={RouterLink} to="/" direction="row" alignItems="center" spacing={1.25} aria-label="igame 홈" sx={{ textDecoration: 'none', color: 'inherit', '&:hover .brand-mark': { transform: 'rotate(-6deg) scale(1.04)' } }}>
            <Box className="brand-mark" sx={(theme) => ({ width: 38, height: 38, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: 'primary.contrastText', background: `linear-gradient(140deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`, boxShadow: `0 6px 18px ${alpha(theme.palette.primary.main, .35)}`, transition: 'transform .2s' })}><SportsEsportsRounded /></Box>
            <Typography variant="h4" sx={{ fontWeight: 900, letterSpacing: '-.04em', color: 'text.primary' }}>{config.display_name ?? 'igame'}</Typography>
          </Stack>
          {/* lg, not md: six Korean nav labels plus the brand and the account
              controls need about 1050px, so at md (900) the theme toggle and the
              avatar hung 88px off the right edge of the window. Below lg the
              menu button holds the same navigation. */}
          <Stack component="nav" aria-label="주 메뉴" direction="row" spacing={.5} sx={{ ml: 3, display: { xs: 'none', lg: 'flex' } }}>
            {navigation.map((item) => <Button key={item.to} component={NavLink} to={item.to} end={item.to === '/'} color="inherit" startIcon={item.icon} sx={(theme) => ({ color: 'text.secondary', '&.active': { color: 'primary.main', bgcolor: alpha(theme.palette.primary.main, .08) } })}>{item.label}</Button>)}
          </Stack>
          <Box sx={{ flex: 1 }} />
          <Chip label={`Lv.${user?.level ?? 1}`} size="small" color="secondary" variant="outlined" sx={{ display: { xs: 'none', sm: 'flex' } }} />
          <Tooltip title={mode === 'dark' ? '밝은 화면으로 전환' : '어두운 화면으로 전환'}><IconButton aria-label={mode === 'dark' ? '밝은 화면으로 전환' : '어두운 화면으로 전환'} onClick={toggle}>{mode === 'dark' ? <LightModeRounded /> : <DarkModeRounded />}</IconButton></Tooltip><Tooltip title="프로필 메뉴"><IconButton aria-label="프로필 메뉴 열기" onClick={(event) => setAnchor(event.currentTarget)}><Avatar src={user?.avatar_url} sx={{ width: 38, height: 38, bgcolor: 'primary.dark' }}>{(user?.display_name || user?.username || '?').slice(0, 1)}</Avatar></IconButton></Tooltip>
          <IconButton aria-label="모바일 메뉴" sx={{ display: { lg: 'none' } }} onClick={(event) => setAnchor(event.currentTarget)}><MenuRounded /></IconButton>
          <Menu anchorEl={anchor} open={Boolean(anchor)} onClose={close} slotProps={{ paper: { className: 'admin-scrollbar', sx: { mt: 1, width: 285, maxHeight: 'min(620px,80vh)', border: 1, borderColor: 'divider' } } }}>
            <Box sx={{ px: 2, py: 1.5 }}><Typography fontWeight={750}>{user?.display_name || user?.username}</Typography><Typography variant="body2" color="text.secondary">{user?.department || '소속 정보 없음'}</Typography></Box>
            <Divider />
            <MenuItem component={RouterLink} to="/profile" onClick={close}><ListItemIcon><PersonRounded /></ListItemIcon>내 프로필</MenuItem>
            <MenuItem component={RouterLink} to="/profile/keys" onClick={close}><ListItemIcon><KeyRounded /></ListItemIcon>개인 키 관리</MenuItem>
            <MenuItem component={RouterLink} to="/profile/preferences" onClick={close}><ListItemIcon><SettingsRounded /></ListItemIcon>개인화 설정</MenuItem>
            <Divider />
            {navigation.map((item) => <MenuItem key={item.to} component={RouterLink} to={item.to} onClick={close} sx={{ display: { lg: 'none' } }}><ListItemIcon>{item.icon}</ListItemIcon>{item.label}</MenuItem>)}
            {isAdmin && <MenuItem component={RouterLink} to="/admin" onClick={close}><ListItemIcon><AdminPanelSettingsRounded /></ListItemIcon>서비스 관리</MenuItem>}
            <Divider />
            <Box sx={{ px: 2, py: 1 }}><Typography variant="body2" color="text.secondary">igame v{version.version}{version.commit ? ` · ${version.commit.slice(0, 8)}` : ''}</Typography></Box>
            <MenuItem onClick={() => void doLogout()}><ListItemIcon><LogoutRounded /></ListItemIcon>로그아웃</MenuItem>
          </Menu>
        </Toolbar></Container>
      </AppBar>
      {/* flex:1 keeps the footer on the bottom edge instead of leaving it
          stranded mid-screen on a short page such as an empty ranking board. */}
      <Box component="main" id={MAIN_CONTENT_ID} ref={mainRef} tabIndex={-1} sx={{ outline: 'none', flex: 1 }}><Outlet /></Box>
      <Box component="footer" sx={{ mt: 8, borderTop: 1, borderColor: 'divider', py: 3 }}><Container maxWidth="xl"><Typography variant="body2" color="text.secondary">{config.display_name ?? 'igame'} · igame platform · v{version.version}</Typography></Container></Box>
    </Box>
  );
}
