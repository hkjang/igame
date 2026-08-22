import { lazy, Suspense, type ComponentType } from 'react';
import ArrowBackRounded from '@mui/icons-material/ArrowBackRounded';
import FullscreenRounded from '@mui/icons-material/FullscreenRounded';
import LeaderboardRounded from '@mui/icons-material/LeaderboardRounded';
import { Alert, Box, Button, Chip, Container, Stack, Typography } from '@mui/material';
import { Link as RouterLink, useParams } from 'react-router-dom';
import { api } from '../api/client';
import { ErrorPanel } from '../components/ErrorPanel';
import { LoadingScreen } from '../components/LoadingScreen';
import { Game2048 } from '../games/Game2048';
import { MemoryGame } from '../games/MemoryGame';
import { ReactionGame } from '../games/ReactionGame';
import { SnakeGame } from '../games/SnakeGame';
import { TypingGame } from '../games/TypingGame';
import { isDefenseSlug } from '../games/defense/content';
import type { BuiltinGameProps } from '../games/types';
import { useGameRuntime } from '../games/useGameRuntime';
import { useAsync } from '../hooks/useAsync';
import { gameRankingHref } from './gameLinks';

const RealmGuardGame = lazy(() => import('../games/realmguard/RealmGuardGame'));
const DefenseSeriesGame = lazy(() => import('../games/defense/DefenseSeriesGame'));

const runners: Partial<Record<string, ComponentType<BuiltinGameProps>>> = {
  '2048': Game2048, snake: SnakeGame, memory: MemoryGame, reaction: ReactionGame, typing: TypingGame,
  realmguard: RealmGuardGame,
  'office-guardians': DefenseSeriesGame,
  'cyber-fortress': DefenseSeriesGame,
  'ai-nexus-defense': DefenseSeriesGame,
};

export function GamePlayPage() {
  const { slug = '' } = useParams();
  const remote = useAsync(() => api.game(slug), [slug]);
  const game = remote.data;
  const runtime = useGameRuntime(game?.id ?? slug, isDefenseSlug(slug) ? `/api/v1/defense/${slug}/results` : '/api/v1/realmguard/results');
  if (remote.loading) return <LoadingScreen label="게임을 준비하는 중…" />;
  if (remote.error) return <Container sx={{ py: 8 }}><ErrorPanel error={remote.error} retry={() => void remote.reload()} /><Button component={RouterLink} to="/games" sx={{ mt: 2 }}>게임 목록으로</Button></Container>;
  if (!game) return <Container sx={{ py: 8 }}><Alert severity="error">게임을 찾을 수 없습니다.</Alert><Button component={RouterLink} to="/games" sx={{ mt: 2 }}>게임 목록으로</Button></Container>;
  const Runner = runners[slug];
  const gameURL = (() => { try { const url = new URL(game.game_url ?? '', window.location.origin); return ['http:', 'https:'].includes(url.protocol) ? url.href : ''; } catch { return ''; } })();
  return <Container maxWidth="xl" sx={{ py: 3 }}><Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'center' }} gap={2} mb={2}><Stack direction="row" spacing={1.5} alignItems="center"><Button component={RouterLink} to="/games" startIcon={<ArrowBackRounded />}>나가기</Button><Box><Typography variant="h2">{game.name}</Typography><Stack direction="row" spacing={1} mt={.5}><Chip size="small" label={game.category} /><Chip size="small" variant="outlined" color={runtime.online ? 'success' : 'warning'} label={runtime.online ? '기록 모드' : '연습 모드'} /></Stack></Box></Stack><Stack direction="row" spacing={1}><Button component={RouterLink} to={gameRankingHref(slug, game.id)} startIcon={<LeaderboardRounded />}>랭킹</Button><Button startIcon={<FullscreenRounded />} onClick={() => { void document.documentElement.requestFullscreen().catch(() => undefined); }}>전체 화면</Button></Stack></Stack>
    <Box sx={{ minHeight: slug === 'realmguard' || isDefenseSlug(slug) ? 0 : 600, border: 1, borderColor: 'divider', borderRadius: 3, overflow: 'hidden', bgcolor: '#0b1725', display: 'grid', placeItems: 'center', p: Runner && slug !== 'realmguard' && !isDefenseSlug(slug) ? { xs: 2, md: 4 } : 0 }}>{Runner ? <Suspense fallback={<LoadingScreen label="게임 엔진을 불러오는 중…" />}><Runner onStart={runtime.start} onFinish={runtime.finish} onTelemetry={runtime.telemetry} onAuthoritativeComplete={runtime.completeAuthoritatively} onAuthoritativeRequest={runtime.requestAuthoritatively} isRecording={runtime.isRecording} online={runtime.online} /></Suspense> : !gameURL ? <Alert severity="error">안전한 게임 실행 URL이 등록되지 않았습니다.</Alert> : game.game_type === 'external' ? <Stack alignItems="center" spacing={2} p={4}><Typography variant="h3">새 창에서 {game.name}을 실행합니다</Typography><Typography color="text.secondary">외부 게임도 igame SDK를 사용하면 동일한 세션과 랭킹을 이용할 수 있습니다.</Typography><Button component="a" href={gameURL} target="_blank" rel="noopener noreferrer" variant="contained">게임 열기</Button></Stack> : <Box component="iframe" title={`${game.name} 게임`} src={gameURL} sandbox="allow-scripts allow-forms allow-same-origin allow-pointer-lock" allow="fullscreen; gamepad" sx={{ width: '100%', height: 720, border: 0 }} />}</Box>
    <Typography variant="body2" color="text.secondary" mt={2}>게임 세션과 점수는 Game SDK를 통해 서버에서 검증됩니다. 비정상 요청은 랭킹에 반영되지 않을 수 있습니다.</Typography>
  </Container>;
}
