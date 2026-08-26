import { useRef, useState } from 'react';
import AnalyticsRounded from '@mui/icons-material/AnalyticsRounded';
import ApprovalRounded from '@mui/icons-material/ApprovalRounded';
import ArticleRounded from '@mui/icons-material/ArticleRounded';
import ViewCarouselRounded from '@mui/icons-material/ViewCarouselRounded';
import AutoAwesomeRounded from '@mui/icons-material/AutoAwesomeRounded';
import CategoryRounded from '@mui/icons-material/CategoryRounded';
import ChevronLeftRounded from '@mui/icons-material/ChevronLeftRounded';
import DashboardRounded from '@mui/icons-material/DashboardRounded';
import EmojiEventsRounded from '@mui/icons-material/EmojiEventsRounded';
import EventRounded from '@mui/icons-material/EventRounded';
import GamesRounded from '@mui/icons-material/GamesRounded';
import GroupsRounded from '@mui/icons-material/GroupsRounded';
import KeyRounded from '@mui/icons-material/KeyRounded';
import MenuRounded from '@mui/icons-material/MenuRounded';
import MilitaryTechRounded from '@mui/icons-material/MilitaryTechRounded';
import NotificationsRounded from '@mui/icons-material/NotificationsRounded';
import RedeemRounded from '@mui/icons-material/RedeemRounded';
import SecurityRounded from '@mui/icons-material/SecurityRounded';
import SettingsRounded from '@mui/icons-material/SettingsRounded';
import TuneRounded from '@mui/icons-material/TuneRounded';
import CastleRounded from '@mui/icons-material/CastleRounded';
import ShieldRounded from '@mui/icons-material/ShieldRounded';
import { alpha } from '@mui/material/styles';
import { AppBar, Box, Divider, Drawer, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Stack, Toolbar, Typography } from '@mui/material';
import { Link as RouterLink, NavLink, Outlet } from 'react-router-dom';
import { MAIN_CONTENT_ID, RouteChrome } from '../components/RouteChrome';
import { useAuth } from '../state/AuthContext';

const width = 276;
const menu: Array<{ to: string; label: string; icon: React.ReactNode; end?: boolean; adminOnly?: boolean }> = [
  { to: '/admin', label: '대시보드', icon: <DashboardRounded />, end: true },
  { to: '/admin/games', label: '게임', icon: <GamesRounded /> },
  { to: '/admin/categories', label: '카테고리', icon: <CategoryRounded /> },
  { to: '/admin/users', label: '사용자', icon: <GroupsRounded />, adminOnly: true },
  { to: '/admin/rankings', label: '랭킹', icon: <EmojiEventsRounded /> },
  { to: '/admin/seasons', label: '시즌', icon: <EventRounded /> },
  { to: '/admin/events', label: '이벤트', icon: <AutoAwesomeRounded /> },
  { to: '/admin/tournaments', label: '대회', icon: <MilitaryTechRounded /> },
  { to: '/admin/achievements', label: '업적', icon: <ApprovalRounded /> },
  { to: '/admin/rewards', label: '보상', icon: <RedeemRounded /> },
  { to: '/admin/notices', label: '공지', icon: <NotificationsRounded /> },
  { to: '/admin/banners', label: '배너', icon: <ViewCarouselRounded /> },
  { to: '/admin/analytics', label: '통계', icon: <AnalyticsRounded /> },
  { to: '/admin/realmguard', label: 'RealmGuard Designer', icon: <CastleRounded /> },
  { to: '/admin/defense', label: 'Defense Content Studio', icon: <ShieldRounded /> },
  { to: '/admin/audit', label: '감사 로그', icon: <ArticleRounded />, adminOnly: true },
  { to: '/admin/approvals', label: '검토·승인', icon: <TuneRounded /> },
  { to: '/admin/keys', label: '키 권한', icon: <KeyRounded />, adminOnly: true },
  { to: '/admin/security', label: 'OIDC·보안', icon: <SecurityRounded />, adminOnly: true },
  { to: '/admin/ai', label: 'AI 설정', icon: <AutoAwesomeRounded />, adminOnly: true },
  { to: '/admin/settings', label: '시스템 설정', icon: <SettingsRounded />, adminOnly: true },
];

export function AdminLayout() {
  const { user } = useAuth();
  const [mobileOpen, setMobileOpen] = useState(false);
  const mainRef = useRef<HTMLElement>(null);
  const isAdmin = [user?.role, ...(user?.roles ?? [])].includes('admin');
  const navigation = (
    <Stack sx={{ height: '100%', bgcolor: 'surface.nav' }}>
      <Toolbar sx={{ gap: 1.4, minHeight: 72 }}><Box sx={{ width: 36, height: 36, bgcolor: 'primary.main', color: 'primary.contrastText', display: 'grid', placeItems: 'center', borderRadius: 2 }}><GamesRounded /></Box><Box><Typography fontWeight={900}>igame</Typography><Typography variant="body2" color="text.secondary">서비스 관리</Typography></Box></Toolbar>
      <Divider />
      <List className="admin-scrollbar" component="nav" aria-label="관리자 메뉴" sx={{ p: 1.2, overflowY: 'auto', flex: 1 }}>
        {menu.filter((item) => !item.adminOnly || isAdmin).map((item) => <ListItemButton key={item.to} component={NavLink} to={item.to} end={item.end} onClick={() => setMobileOpen(false)} sx={(theme) => ({ mb: .35, borderRadius: 2, color: 'text.secondary', '&.active': { color: 'primary.main', bgcolor: alpha(theme.palette.primary.main, .11), boxShadow: `inset 3px 0 ${theme.palette.primary.main}` } })}><ListItemIcon sx={{ color: 'inherit', minWidth: 42 }}>{item.icon}</ListItemIcon><ListItemText primary={item.label} primaryTypographyProps={{ fontWeight: 650 }} /></ListItemButton>)}
      </List>
      {/* component="nav": these items render as anchors, and an <a> is not a
          child a <ul> is allowed to have. */}
      <Divider /><List component="nav" sx={{ p: 1.2 }}><ListItemButton component={RouterLink} to="/" sx={{ borderRadius: 2 }}><ListItemIcon><ChevronLeftRounded /></ListItemIcon><ListItemText primary="사용자 포털로" /></ListItemButton></List>
    </Stack>
  );
  return (
    <Box sx={{ minHeight: '100dvh', display: 'flex', bgcolor: 'surface.ground' }}>
      <RouteChrome mainRef={mainRef} />
      <AppBar position="fixed" elevation={0} sx={(theme) => ({ display: { lg: 'none' }, bgcolor: alpha(theme.palette.surface.ground, .95), color: 'text.primary', borderBottom: 1, borderColor: 'divider' })}><Toolbar><IconButton aria-label="관리자 메뉴 열기" onClick={() => setMobileOpen(true)}><MenuRounded /></IconButton><Typography fontWeight={800} ml={1}>igame 서비스 관리</Typography></Toolbar></AppBar>
      <Box component="aside" sx={{ width: { lg: width }, flexShrink: 0 }}>
        <Drawer variant="permanent" open sx={{ display: { xs: 'none', lg: 'block' }, '& .MuiDrawer-paper': { width, borderRightColor: 'divider' } }}>{navigation}</Drawer>
        <Drawer variant="temporary" open={mobileOpen} onClose={() => setMobileOpen(false)} ModalProps={{ keepMounted: true }} sx={{ display: { lg: 'none' }, '& .MuiDrawer-paper': { width } }}>{navigation}</Drawer>
      </Box>
      <Box component="main" id={MAIN_CONTENT_ID} ref={mainRef} tabIndex={-1} sx={{ flex: 1, minWidth: 0, pt: { xs: 9, lg: 0 }, outline: 'none' }}><Outlet /></Box>
    </Box>
  );
}
