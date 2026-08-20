import type { ReactNode } from 'react';
import { Alert, Button, Container, Stack, Typography } from '@mui/material';
import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { LoadingScreen } from './components/LoadingScreen';
import { AdminLayout } from './layouts/AdminLayout';
import { PortalLayout } from './layouts/PortalLayout';
import { ProfileLayout } from './layouts/ProfileLayout';
import { DeveloperPage } from './pages/DeveloperPage';
import { EventsPage } from './pages/EventsPage';
import { GamePlayPage } from './pages/GamePlayPage';
import { GamesPage } from './pages/GamesPage';
import { HomePage } from './pages/HomePage';
import { LoginPage } from './pages/LoginPage';
import { NotFoundPage } from './pages/NotFoundPage';
import { PersonalKeysPage } from './pages/PersonalKeysPage';
import { PreferencesPage } from './pages/PreferencesPage';
import { ProfileOverviewPage } from './pages/ProfileOverviewPage';
import { RankingsPage } from './pages/RankingsPage';
import { AdminAnalyticsPage } from './pages/admin/AdminAnalyticsPage';
import { AdminDashboardPage } from './pages/admin/AdminDashboardPage';
import { AdminResourcePage } from './pages/admin/AdminResourcePage';
import { AdminSettingsPage } from './pages/admin/AdminSettingsPage';
import { AIGameLabPage } from './pages/AIGameLabPage';
import { ReviewsPage } from './pages/ReviewsPage';
import { RealmGuardPreviewPage } from './pages/RealmGuardPreviewPage';
import { RealmGuardDesignerPage } from './pages/admin/realmguard/RealmGuardDesignerPage';
import { useAuth } from './state/AuthContext';

function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth(); const location = useLocation();
  if (loading) return <LoadingScreen label="igame을 준비하는 중…" />;
  return user ? children : <Navigate to="/login" state={{ from: location }} replace />;
}

type ServiceRole = 'admin' | 'operator';

function RequireAdmin({ children, roles = ['admin', 'operator'] }: { children: ReactNode; roles?: ServiceRole[] }) {
  const { user } = useAuth();
  const allowed = [user?.role, ...(user?.roles ?? [])].some((role) => role && roles.includes(role as ServiceRole));
  if (allowed) return children;
  return <Container maxWidth="sm"><Stack minHeight="75vh" justifyContent="center"><Alert severity="error"><Typography variant="h3" mb={1}>관리 권한이 필요합니다</Typography>{roles.length === 1 ? '이 설정은 서비스 관리자만 이용할 수 있습니다.' : '이 영역은 서비스 관리자 또는 운영자만 이용할 수 있습니다.'}</Alert><Button href="/" sx={{ mt: 2 }}>사용자 포털로</Button></Stack></Container>;
}

const adminOnly = (page: ReactNode) => <RequireAdmin roles={['admin']}>{page}</RequireAdmin>;

export function App() {
  return <Routes>
    <Route path="/login" element={<LoginPage />} />
    <Route element={<RequireAuth><PortalLayout /></RequireAuth>}>
      <Route index element={<HomePage />} />
      <Route path="games" element={<GamesPage />} />
      <Route path="games/:slug" element={<GamePlayPage />} />
      <Route path="rankings" element={<RankingsPage />} />
      <Route path="events" element={<EventsPage />} />
      <Route path="ai" element={<AIGameLabPage />} />
      <Route path="reviews" element={<ReviewsPage />} />
      <Route path="realmguard/preview/:id" element={<RealmGuardPreviewPage />} />
      <Route path="developer" element={<DeveloperPage />} />
      <Route path="profile" element={<ProfileLayout />}>
        <Route index element={<ProfileOverviewPage />} />
        <Route path="keys" element={<PersonalKeysPage />} />
        <Route path="preferences" element={<PreferencesPage />} />
      </Route>
    </Route>
    <Route path="admin" element={<RequireAuth><RequireAdmin><AdminLayout /></RequireAdmin></RequireAuth>}>
      <Route index element={<AdminDashboardPage />} />
      {['games', 'categories', 'rankings', 'seasons', 'events', 'tournaments', 'achievements', 'rewards', 'notices', 'banners'].map((resource) => <Route key={resource} path={resource} element={<AdminResourcePage resource={resource} />} />)}
      <Route path="users" element={adminOnly(<AdminResourcePage resource="users" />)} />
      <Route path="audit" element={adminOnly(<AdminResourcePage resource="audit" />)} />
      <Route path="analytics" element={<AdminAnalyticsPage />} />
      <Route path="realmguard" element={<RealmGuardDesignerPage />} />
      <Route path="approvals" element={<AdminSettingsPage section="approval" />} />
      <Route path="keys" element={adminOnly(<AdminSettingsPage section="api_keys" />)} />
      <Route path="security" element={adminOnly(<AdminSettingsPage section="oidc" />)} />
      <Route path="ai" element={adminOnly(<AdminSettingsPage section="ai" />)} />
      <Route path="settings" element={adminOnly(<AdminSettingsPage section="general" />)} />
    </Route>
    <Route path="*" element={<NotFoundPage />} />
  </Routes>;
}
