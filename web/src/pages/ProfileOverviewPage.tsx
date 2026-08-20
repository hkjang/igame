import { type FormEvent, useState } from 'react';
import SaveRounded from '@mui/icons-material/SaveRounded';
import EmojiEventsRounded from '@mui/icons-material/EmojiEventsRounded';
import HistoryRounded from '@mui/icons-material/HistoryRounded';
import LockResetRounded from '@mui/icons-material/LockResetRounded';
import { Alert, Button, Card, CardContent, Chip, FormControlLabel, Grid, LinearProgress, Stack, Switch, TextField, Typography } from '@mui/material';
import { api } from '../api/client';
import { useAsync } from '../hooks/useAsync';
import { useAuth } from '../state/AuthContext';
import { useSnackbar } from '../state/SnackbarContext';

export function ProfileOverviewPage() {
  const { user, refreshUser } = useAuth();
  const { notify } = useSnackbar();
  const [nickname, setNickname] = useState(user?.nickname ?? '');
  const [rankingOptOut, setRankingOptOut] = useState(user?.ranking_opt_out ?? false);
  const [saving, setSaving] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [passwordConfirm, setPasswordConfirm] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);
  const activity = useAsync(async () => {
    const [history, achievements] = await Promise.all([api.playHistory(), api.myAchievements()]);
    return { history: history.items, achievements: achievements.items };
  }, []);
  const save = async (event: FormEvent) => {
    event.preventDefault(); setSaving(true);
    try { await api.updateProfile({ nickname, ranking_opt_out: rankingOptOut }); await refreshUser(); notify('프로필을 저장했습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '프로필을 저장하지 못했습니다.', 'error'); }
    finally { setSaving(false); }
  };
  const changePassword = async (event: FormEvent) => {
    event.preventDefault();
    if (newPassword.length < 12) { notify('새 비밀번호는 12자 이상이어야 합니다.', 'warning'); return; }
    if (newPassword !== passwordConfirm) { notify('새 비밀번호 확인이 일치하지 않습니다.', 'warning'); return; }
    setChangingPassword(true);
    try {
      await api.changePassword(currentPassword, newPassword);
      setCurrentPassword(''); setNewPassword(''); setPasswordConfirm('');
      notify('비밀번호를 변경했습니다.', 'success');
    } catch (cause) { notify(cause instanceof Error ? cause.message : '비밀번호를 변경하지 못했습니다.', 'error'); }
    finally { setChangingPassword(false); }
  };
  return <Grid container spacing={3}><Grid size={{ xs: 12, md: 7 }}><Card><CardContent sx={{ p: 3 }}><Typography variant="h3">기본 정보</Typography><Stack component="form" onSubmit={(event) => void save(event)} spacing={2.2} mt={3}><TextField label="아이디" value={user?.username ?? ''} disabled /><TextField label="이름" value={user?.display_name ?? ''} disabled helperText="SSO에서 제공하는 정보입니다." /><TextField label="소속" value={user?.department ?? ''} disabled /><TextField label="팀" value={user?.team ?? ''} disabled /><TextField label="랭킹 닉네임" value={nickname} onChange={(event) => setNickname(event.target.value)} inputProps={{ maxLength: 30 }} helperText="관리자 공개 정책이 닉네임일 때 표시됩니다." /><FormControlLabel control={<Switch checked={rankingOptOut} onChange={(event) => setRankingOptOut(event.target.checked)} />} label="공개 랭킹 참여 안 함" /><Button type="submit" variant="contained" startIcon={<SaveRounded />} disabled={saving} sx={{ alignSelf: 'flex-start' }}>변경 저장</Button></Stack></CardContent></Card></Grid><Grid size={{ xs: 12, md: 5 }}><Card><CardContent sx={{ p: 3 }}><Typography variant="h3">게임 성장</Typography><Typography sx={{ fontSize: '3rem', fontWeight: 900, color: 'secondary.main', mt: 2 }}>Lv.{user?.level ?? 1}</Typography><Typography color="text.secondary">누적 {user?.xp ?? 0} XP</Typography></CardContent></Card></Grid><Grid size={{ xs: 12, md: 7 }}><Card><CardContent sx={{ p: 3 }}><Stack direction="row" spacing={1} alignItems="center"><LockResetRounded color="primary" /><Typography variant="h3">로컬 비밀번호 변경</Typography></Stack><Typography color="text.secondary" mt={1}>Bootstrap 또는 로컬 계정만 사용합니다. OIDC 계정의 비밀번호는 사내 인증 시스템에서 변경하세요.</Typography><Stack component="form" onSubmit={(event) => void changePassword(event)} spacing={2} mt={2.5}><TextField label="현재 비밀번호" type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required /><TextField label="새 비밀번호" type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required helperText="12자 이상" inputProps={{ minLength: 12 }} /><TextField label="새 비밀번호 확인" type="password" autoComplete="new-password" value={passwordConfirm} onChange={(event) => setPasswordConfirm(event.target.value)} required error={Boolean(passwordConfirm && newPassword !== passwordConfirm)} /><Button type="submit" variant="outlined" startIcon={<LockResetRounded />} disabled={changingPassword || !currentPassword || !newPassword || !passwordConfirm} sx={{ alignSelf: 'flex-start' }}>비밀번호 변경</Button></Stack></CardContent></Card></Grid>{activity.loading && <Grid size={{ xs: 12 }}><LinearProgress /></Grid>}{activity.error && <Grid size={{ xs: 12 }}><Alert severity="warning">활동 기록을 불러오지 못했습니다. {activity.error.message}</Alert></Grid>}<Grid size={{ xs: 12, md: 7 }}><Card><CardContent sx={{ p: 3 }}><Stack direction="row" spacing={1} alignItems="center"><HistoryRounded color="primary" /><Typography variant="h3">최근 플레이</Typography></Stack><Stack spacing={1.5} mt={2}>{activity.data?.history.slice(0, 5).map((item) => <Stack key={item.id} direction="row" alignItems="center" justifyContent="space-between" sx={{ p: 1.5, bgcolor: 'action.hover', borderRadius: 2 }}><Stack><Typography fontWeight={750}>{item.game_name}</Typography><Typography variant="body2" color="text.secondary">{new Date(item.started_at).toLocaleString('ko-KR')}</Typography></Stack><Stack alignItems="end"><Typography fontWeight={800}>{item.score?.toLocaleString() ?? '—'}점</Typography>{item.verified !== undefined && <Chip size="small" label={item.verified ? '검증됨' : '검토 중'} color={item.verified ? 'success' : 'warning'} />}</Stack></Stack>)}{!activity.loading && !activity.data?.history.length && <Typography color="text.secondary">아직 플레이 기록이 없습니다.</Typography>}</Stack></CardContent></Card></Grid><Grid size={{ xs: 12, md: 5 }}><Card><CardContent sx={{ p: 3 }}><Stack direction="row" spacing={1} alignItems="center"><EmojiEventsRounded color="warning" /><Typography variant="h3">획득 업적</Typography></Stack><Stack spacing={1.5} mt={2}>{activity.data?.achievements.slice(0, 5).map((item) => <Stack key={item.id ?? item.code} sx={{ p: 1.5, bgcolor: 'action.hover', borderRadius: 2 }}><Typography fontWeight={750}>{item.name}</Typography><Typography variant="body2" color="text.secondary">{item.description || item.code}</Typography></Stack>)}{!activity.loading && !activity.data?.achievements.length && <Typography color="text.secondary">첫 업적에 도전해 보세요.</Typography>}</Stack></CardContent></Card></Grid></Grid>;
}
