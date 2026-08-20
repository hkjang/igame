import ArrowForwardRounded from '@mui/icons-material/ArrowForwardRounded';
import EmojiEventsRounded from '@mui/icons-material/EmojiEventsRounded';
import PlayCircleRounded from '@mui/icons-material/PlayCircleRounded';
import WhatshotRounded from '@mui/icons-material/WhatshotRounded';
import { alpha } from '@mui/material/styles';
import { Box, Button, Card, CardContent, Chip, Container, Grid, Skeleton, Stack, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { api } from '../api/client';
import { GameCard } from '../components/GameCard';
import { mergeGames } from '../data/builtinGames';
import { useAsync } from '../hooks/useAsync';
import { useAuth } from '../state/AuthContext';
import { useSnackbar } from '../state/SnackbarContext';
import type { Game } from '../types';

export function HomePage() {
  const { user } = useAuth();
  const { notify } = useSnackbar();
  const { data, loading, setData } = useAsync(async () => mergeGames((await api.games()).items), []);
  const portalContent = useAsync(async () => {
    const [notices, banners, events] = await Promise.all([api.notices(), api.banners(), api.events()]);
    return { notices: notices.items, banners: banners.items, events: events.items };
  }, []);
  const games = data ?? mergeGames();
  const featuredEvent = portalContent.data?.events.find((event) => event.status === 'active') ?? portalContent.data?.events[0];
  const favorite = async (game: Game) => {
    const next = !game.favorite;
    setData(games.map((item) => item.id === game.id ? { ...item, favorite: next } : item));
    try { await api.toggleFavorite(game.id, next); notify(next ? '즐겨찾기에 추가했습니다.' : '즐겨찾기에서 제거했습니다.', 'success'); }
    catch (cause) { setData(games); notify(cause instanceof Error ? cause.message : '즐겨찾기를 변경하지 못했습니다.', 'error'); }
  };
  return (
    <>
      <Box sx={{ borderBottom: 1, borderColor: 'divider', background: 'linear-gradient(110deg,rgba(29,93,126,.34),rgba(7,16,29,.5) 55%,rgba(59,118,83,.15))' }}>
        <Container maxWidth="xl" sx={{ py: { xs: 6, md: 9 } }}>
          <Grid container spacing={4} alignItems="center"><Grid size={{ xs: 12, md: 8 }}>
            <Chip icon={<WhatshotRounded />} label="오늘의 플레이" color="primary" variant="outlined" />
            <Typography variant="h1" mt={2}>안녕하세요, {user?.display_name || user?.username}님.<br />오늘은 어떤 게임을 해볼까요?</Typography>
            <Typography color="text.secondary" mt={2} sx={{ maxWidth: 740, fontSize: '1.08rem' }}>짧은 휴식에 집중력을 리셋하고, 동료들과 건강한 기록 경쟁을 즐겨 보세요.</Typography>
            <Stack direction="row" spacing={1.5} mt={4}><Button component={RouterLink} to="/games" variant="contained" size="large" startIcon={<PlayCircleRounded />}>게임 찾기</Button><Button component={RouterLink} to="/rankings" variant="outlined" size="large" startIcon={<EmojiEventsRounded />}>이번 주 랭킹</Button></Stack>
          </Grid><Grid size={{ xs: 12, md: 4 }}><Card sx={{ bgcolor: alpha('#111d2c', .68), backdropFilter: 'blur(12px)' }}><CardContent sx={{ p: 3 }}><Typography color="text.secondary">나의 포털 레벨</Typography><Stack direction="row" alignItems="baseline" spacing={1} mt={1}><Typography sx={{ fontSize: '3.2rem', fontWeight: 900, color: 'secondary.main' }}>{user?.level ?? 1}</Typography><Typography color="text.secondary">LEVEL</Typography></Stack><Box sx={{ height: 9, bgcolor: alpha('#fff', .08), borderRadius: 9, mt: 2, overflow: 'hidden' }}><Box sx={{ width: `${Math.min(100, (user?.xp ?? 35) % 100)}%`, height: '100%', bgcolor: 'secondary.main' }} /></Box><Typography variant="body2" color="text.secondary" mt={1}>{user?.xp ?? 35} XP · 다음 레벨까지 플레이해 보세요</Typography></CardContent></Card></Grid></Grid>
        </Container>
      </Box>
      <Container maxWidth="xl" sx={{ py: 6 }}>
        <Stack direction="row" justifyContent="space-between" alignItems="end" mb={2.5}><Box><Typography variant="h2">지금 인기 있는 게임</Typography><Typography color="text.secondary" mt={.6}>바로 시작할 수 있는 igame 기본 게임입니다.</Typography></Box><Button component={RouterLink} to="/games" endIcon={<ArrowForwardRounded />}>전체 보기</Button></Stack>
        <Grid container spacing={2.5}>{loading ? Array.from({ length: 5 }, (_, index) => <Grid key={index} size={{ xs: 12, sm: 6, lg: 2.4 }}><Skeleton variant="rounded" height={390} /></Grid>) : games.slice(0, 5).map((game) => <Grid key={game.id} size={{ xs: 12, sm: 6, md: 4, lg: 2.4 }}><GameCard game={game} onFavorite={(item) => void favorite(item)} /></Grid>)}</Grid>
        {(portalContent.data?.banners.length || portalContent.data?.notices.length) ? <Grid container spacing={2.5} mt={4}>{portalContent.data.banners.slice(0, 1).map((banner) => <Grid key={banner.id} size={{ xs: 12, md: 7 }}><Card sx={{ overflow: 'hidden', height: '100%' }}><Box component="img" src={banner.image_url} alt="" sx={{ width: '100%', maxHeight: 260, objectFit: 'cover', display: 'block' }} /><CardContent sx={{ p: 3 }}><Typography variant="h3">{banner.title}</Typography>{banner.link_url && <Button href={banner.link_url} sx={{ mt: 2 }} endIcon={<ArrowForwardRounded />}>자세히 보기</Button>}</CardContent></Card></Grid>)}{portalContent.data.notices.length > 0 && <Grid size={{ xs: 12, md: portalContent.data.banners.length ? 5 : 12 }}><Card sx={{ height: '100%' }}><CardContent sx={{ p: 3 }}><Typography variant="h3">공지사항</Typography><Stack spacing={1.5} mt={2}>{portalContent.data.notices.slice(0, 4).map((notice) => <Box key={notice.id} sx={{ p: 1.5, bgcolor: 'action.hover', borderRadius: 2 }}><Stack direction="row" spacing={1} alignItems="center">{notice.pinned && <Chip label="중요" size="small" color="primary" />}<Typography fontWeight={800}>{notice.title}</Typography></Stack><Typography color="text.secondary" variant="body2" mt={.5} sx={{ whiteSpace: 'pre-wrap' }}>{notice.content}</Typography></Box>)}</Stack></CardContent></Card></Grid>}</Grid> : null}
        <Grid container spacing={2.5} mt={4}><Grid size={{ xs: 12, md: 8 }}><Card><CardContent sx={{ p: 3 }}><Typography variant="h3">{featuredEvent ? featuredEvent.name : '사내 이벤트'}</Typography><Typography color="text.secondary" mt={1}>{featuredEvent?.description || '현재 진행 중인 이벤트가 없습니다. 관리자가 새 이벤트를 열면 이곳에서 바로 확인할 수 있습니다.'}</Typography>{featuredEvent && <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mt={3}><Chip label={featuredEvent.status === 'active' ? '진행 중' : '예정'} color={featuredEvent.status === 'active' ? 'success' : 'default'} />{featuredEvent.ends_at && <Chip label={`${new Date(featuredEvent.ends_at).toLocaleDateString('ko-KR')} 종료`} color="warning" />}{featuredEvent.event_type && <Chip label={featuredEvent.event_type} />}</Stack>}<Button component={RouterLink} to="/events" sx={{ mt: 3 }} variant="outlined">이벤트 확인</Button></CardContent></Card></Grid><Grid size={{ xs: 12, md: 4 }}><Card sx={{ height: '100%' }}><CardContent sx={{ p: 3 }}><Typography variant="h3">플레이 가이드</Typography><Typography color="text.secondary" mt={1}>게임 시작 후 생성되는 안전한 세션으로 기록이 검증됩니다. 브라우저에서 새로고침해도 현재 메뉴로 돌아옵니다.</Typography></CardContent></Card></Grid></Grid>
      </Container>
    </>
  );
}
