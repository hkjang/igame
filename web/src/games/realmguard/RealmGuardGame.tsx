import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import BoltRounded from '@mui/icons-material/BoltRounded';
import CastleRounded from '@mui/icons-material/CastleRounded';
import FastForwardRounded from '@mui/icons-material/FastForwardRounded';
import FavoriteRounded from '@mui/icons-material/FavoriteRounded';
import FlagRounded from '@mui/icons-material/FlagRounded';
import LocalFireDepartmentRounded from '@mui/icons-material/LocalFireDepartmentRounded';
import PauseRounded from '@mui/icons-material/PauseRounded';
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded';
import RestartAltRounded from '@mui/icons-material/RestartAltRounded';
import ShieldRounded from '@mui/icons-material/ShieldRounded';
import StarsRounded from '@mui/icons-material/StarsRounded';
import { Alert, Box, Button, Card, CardActionArea, CardContent, Chip, CircularProgress, Divider, Grid, LinearProgress, MenuItem, Paper, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { useLocation } from 'react-router-dom';
import { ErrorPanel } from '../../components/ErrorPanel';
import { useAsync } from '../../hooks/useAsync';
import { REALMGUARD_RANKING_ANCHOR } from '../../pages/gameLinks';
import type { BuiltinGameProps } from '../types';
import { getRealmGuardConfig, getRealmGuardProgress, getRealmGuardRankings, getRealmGuardVersion, normalizeRealmGuardCompletion, resultPayload, saveRealmGuardLoadout } from './api';
import type { RealmGuardRankingFilters } from './api';
import { createRealmGuardUUID, isRequiredRealmGuardTelemetry, REALMGUARD_OPTIONAL_TELEMETRY_LIMIT, realmGuardEventPayload, retryRealmGuardTelemetry } from './telemetry';
import type { BattleHUD, RealmDifficulty, RealmGuardConfig, RealmProgress, RealmResult, RealmSceneController, RealmStage, TargetingMode } from './types';
import { HeroSelectCard } from './HeroSelectCard';
import { withLoadout } from './kernel/config';
import { GameSurface } from '../GameSurface';
import { BATTLE_SCROLL_MARGIN, useBattleInView } from '../useBattleInView';
import { StageRoster } from './StageRoster';

const EMPTY_PROGRESS: RealmProgress = { total_stars: 0, unlocked_stage: 1, hero_levels: { aerin: 1 }, heroes: { aerin: { unlocked: true, level: 1, xp: 0 } }, skills: { meteor: { unlocked: true, level: 1 } }, loadout: { hero_id: 'aerin', skill_ids: ['meteor'] }, campaign_completed: false, stages: [] };
const initialHUD: BattleHUD = {
  status: 'ready', gold: 0, lives: 20, wave: 0, totalWaves: 0, kills: 0, heroLevel: 1,
  heroHp: 0, heroMaxHp: 0, heroAlive: true, heroRespawn: 0, nextWaveIn: 10, skillCooldowns: {}, speed: 1,
};
const difficultyLabels: Record<RealmDifficulty, { label: string; description: string }> = {
  casual: { label: '캐주얼', description: '부담 없이 이야기와 전술을 익힙니다.' },
  normal: { label: '노멀', description: '권장 밸런스와 표준 랭킹 규칙입니다.' },
  veteran: { label: '베테랑', description: '강한 적과 높은 점수 보너스에 도전합니다.' },
};
const targetingLabels: Record<TargetingMode, string> = { first: '선두', last: '후미', strong: '강한 적', weak: '약한 적', closest: '가까운 적' };
const portraitBattleMedia = '@media (max-width:600px) and (orientation:portrait)';

function RealmGuardLeaderboard({ config }: { config: RealmGuardConfig }) {
  const [group, setGroup] = useState<RealmGuardRankingFilters['group']>('stage');
  const [metric, setMetric] = useState<NonNullable<RealmGuardRankingFilters['metric']>>('score');
  const [period, setPeriod] = useState<RealmGuardRankingFilters['period']>('weekly');
  const [mode, setMode] = useState<RealmGuardRankingFilters['mode']>('campaign');
  const [difficulty, setDifficulty] = useState<RealmGuardRankingFilters['difficulty']>('normal');
  const [stageId, setStageId] = useState('stage-1');
  const [heroId, setHeroId] = useState('aerin');
  useEffect(() => {
    const first = config.stages.find((stage) => stage.mode === mode);
    if (first && !config.stages.some((stage) => stage.id === stageId && stage.mode === mode)) setStageId(first.id);
  }, [config.stages, mode, stageId]);
  useEffect(() => { if (group !== 'department' && metric !== 'score') setMetric('score'); }, [group, metric]);
  const result = useAsync(() => getRealmGuardRankings({ group, metric, period, mode, difficulty, stage_id: group === 'stage' ? stageId : undefined, hero_id: group === 'hero' ? heroId : undefined }), [difficulty, group, heroId, metric, mode, period, stageId]);
  return <Card id={REALMGUARD_RANKING_ANCHOR} variant="outlined" sx={{ mt: 2, scrollMarginTop: 96 }}><CardContent sx={{ p: 2.5 }}><Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" gap={1}><Box><Typography variant="h3">RealmGuard 랭킹</Typography><Typography color="text.secondary">스테이지·부서·영웅별 수호 기록</Typography></Box><Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}><TextField select size="small" label="기간" value={period} onChange={(event) => setPeriod(event.target.value as typeof period)} sx={{ minWidth: 110 }}><MenuItem value="daily">오늘</MenuItem><MenuItem value="weekly">주간</MenuItem><MenuItem value="season">시즌</MenuItem><MenuItem value="all_time">전체</MenuItem></TextField><TextField select size="small" label="그룹" value={group} onChange={(event) => setGroup(event.target.value as typeof group)} sx={{ minWidth: 120 }}><MenuItem value="stage">스테이지</MenuItem><MenuItem value="department">부서</MenuItem><MenuItem value="hero">영웅</MenuItem></TextField>{group === 'department' && <TextField select size="small" label="집계" value={metric} onChange={(event) => setMetric(event.target.value as typeof metric)} sx={{ minWidth: 105 }}><MenuItem value="score">점수</MenuItem><MenuItem value="stars">별</MenuItem></TextField>}<TextField select size="small" label="모드" value={mode} onChange={(event) => setMode(event.target.value as typeof mode)} sx={{ minWidth: 120 }}><MenuItem value="campaign">캠페인</MenuItem><MenuItem value="endless">Endless</MenuItem></TextField><TextField select size="small" label="난이도" value={difficulty} onChange={(event) => setDifficulty(event.target.value as typeof difficulty)} sx={{ minWidth: 120 }}>{(Object.keys(difficultyLabels) as RealmDifficulty[]).map((value) => <MenuItem key={value} value={value}>{difficultyLabels[value].label}</MenuItem>)}</TextField>{group === 'stage' && <TextField select size="small" label="스테이지" value={stageId} onChange={(event) => setStageId(event.target.value)} sx={{ minWidth: 150 }}>{config.stages.filter((stage) => stage.mode === mode).map((stage) => <MenuItem key={stage.id} value={stage.id}>{stage.name}</MenuItem>)}</TextField>}{group === 'hero' && <TextField select size="small" label="영웅" value={heroId} onChange={(event) => setHeroId(event.target.value)} sx={{ minWidth: 130 }}>{config.heroes.map((hero) => <MenuItem key={hero.id} value={hero.id}>{hero.name}</MenuItem>)}</TextField>}</Stack></Stack>{result.loading && <LinearProgress sx={{ mt: 2 }} />}{result.error && <Alert severity="warning" sx={{ mt: 2 }}>랭킹을 불러오지 못했습니다. {result.error.message}</Alert>}<Grid container spacing={1} mt={1}>{result.data?.slice(0, 10).map((entry) => <Grid key={`${entry.rank}-${entry.display_name}`} size={{ xs: 12, sm: 6, lg: 4 }}><Paper variant="outlined" sx={{ p: 1.3 }}><Stack direction="row" alignItems="center" spacing={1}><Chip size="small" label={`#${entry.rank}`} color={entry.rank <= 3 ? 'warning' : 'default'} /><Box flex={1}><Typography fontWeight={800}>{entry.display_name}</Typography><Typography variant="body2" color="text.secondary">{entry.department || entry.hero_id || entry.stage_id}</Typography></Box><Typography fontWeight={900}>{(metric === 'stars' ? entry.stars ?? 0 : entry.score).toLocaleString()}{metric === 'stars' ? ' ★' : ''}</Typography></Stack></Paper></Grid>)}{!result.loading && !result.error && result.data?.length === 0 && <Grid size={12}><Typography color="text.secondary">아직 등록된 기록이 없습니다.</Typography></Grid>}</Grid></CardContent></Card>;
}

interface RealmGuardGameProps extends BuiltinGameProps {
  preview?: { config: RealmGuardConfig; label: string };
}

function previewProgress(config: RealmGuardConfig): RealmProgress {
  return {
    ...EMPTY_PROGRESS, total_stars: 30, unlocked_stage: Number.MAX_SAFE_INTEGER, campaign_completed: true,
    hero_levels: Object.fromEntries(config.heroes.map((hero) => [hero.id, 1])),
    heroes: Object.fromEntries(config.heroes.map((hero) => [hero.id, { unlocked: true, level: 1, xp: 0 }])),
    skills: Object.fromEntries(config.skills.map((skill) => [skill.id, { unlocked: true, level: 1 }])),
    loadout: { hero_id: config.heroes[0]?.id ?? 'aerin', skill_ids: config.skills.slice(0, 3).map((skill) => skill.id) },
  };
}

export function RealmGuardGame({ onStart, onTelemetry, onAuthoritativeComplete, isRecording, preview }: RealmGuardGameProps) {
  const location = useLocation();
  const resource = useAsync(async () => {
    if (preview) return { config: preview.config, progress: previewProgress(preview.config), progressAvailable: true, version: { label: preview.label, content_version: preview.config.contentVersion, stage_version: '', balance_version: preview.config.balanceVersion, asset_version: preview.config.assetVersion } };
    const config = await getRealmGuardConfig();
    const [progressResult, versionResult] = await Promise.allSettled([getRealmGuardProgress(), getRealmGuardVersion()]);
    return {
      config,
      progress: progressResult.status === 'fulfilled' ? progressResult.value : EMPTY_PROGRESS,
      progressAvailable: progressResult.status === 'fulfilled',
      version: versionResult.status === 'fulfilled' ? versionResult.value : { label: `v${config.contentVersion}`, content_version: config.contentVersion, stage_version: '', balance_version: config.balanceVersion, asset_version: config.assetVersion },
    };
  }, [preview]);
  const canvasRef = useRef<HTMLDivElement>(null);
  const controller = useRef<RealmSceneController | undefined>(undefined);
  const telemetryQueue = useRef<Promise<void>>(Promise.resolve());
  const telemetryFailure = useRef<Error | undefined>(undefined);
  const telemetrySequence = useRef(0);
  const optionalTelemetryCount = useRef(0);
  const battleEventId = useRef('');
  const [phase, setPhase] = useState<'select' | 'battle' | 'result'>('select');
  const battleRef = useBattleInView<HTMLDivElement>(phase !== 'select');
  const [stageId, setStageId] = useState('stage-1');
  const [difficulty, setDifficulty] = useState<RealmDifficulty>('normal');
  const [heroId, setHeroId] = useState('aerin');
  const [skillIds, setSkillIds] = useState<string[]>(['meteor']);
  const [recording, setRecording] = useState(false);
  const [hud, setHUD] = useState(initialHUD);
  const [battleKey, setBattleKey] = useState(0);
  const [starting, setStarting] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [localResult, setLocalResult] = useState<RealmResult>();
  const [finalResult, setFinalResult] = useState<RealmResult>();
  const [resultError, setResultError] = useState('');
  const [resultRetryable, setResultRetryable] = useState(true);
  const [aimHint, setAimHint] = useState('');
  const setResourceData = resource.setData;
  const config = resource.data?.config;
  const progress = resource.data?.progress ?? EMPTY_PROGRESS;
  const stage = config?.stages.find((item) => item.id === stageId);
  const hero = config?.heroes.find((item) => item.id === heroId);
  const battleConfig = useMemo(() => config ? withLoadout(config, skillIds) : undefined, [config, skillIds]);
  const queueTelemetry = useCallback((event: string, data?: Record<string, unknown>) => {
    if (!recording || !onTelemetry) return Promise.resolve();
    if (!isRequiredRealmGuardTelemetry(event)) {
      if (optionalTelemetryCount.current >= REALMGUARD_OPTIONAL_TELEMETRY_LIMIT) return Promise.resolve();
      optionalTelemetryCount.current += 1;
    }
    const payload = realmGuardEventPayload(battleEventId.current, ++telemetrySequence.current, data);
    const pending = telemetryQueue.current.then(async () => {
      if (telemetryFailure.current) throw telemetryFailure.current;
      try { await retryRealmGuardTelemetry(() => onTelemetry(event, payload)); }
      catch (cause) {
        const error = cause instanceof Error ? cause : new Error('전투 검증 로그를 전송하지 못했습니다.');
        telemetryFailure.current = error;
        throw error;
      }
    });
    telemetryQueue.current = pending.catch(() => undefined);
    return pending;
  }, [onTelemetry, recording]);

  useEffect(() => {
    if (location.hash !== `#${REALMGUARD_RANKING_ANCHOR}`) return;
    setPhase('select');
    const timer = window.setTimeout(() => document.getElementById(REALMGUARD_RANKING_ANCHOR)?.scrollIntoView?.({ behavior: 'smooth', block: 'start' }), 0);
    return () => window.clearTimeout(timer);
  }, [location.hash, location.key, resource.data?.config]);

  useEffect(() => {
    if (resource.data?.config && !resource.data.config.stages.some((item) => item.id === stageId)) {
      setStageId(resource.data.config.stages.find((item) => item.mode === 'campaign')?.id ?? resource.data.config.stages[0]?.id ?? '');
    }
    const loadoutHero = resource.data?.progress.loadout.hero_id;
    if (loadoutHero && resource.data?.progress.heroes[loadoutHero]?.unlocked) setHeroId(loadoutHero);
    if (resource.data?.config) {
      const available = resource.data.config.skills.filter((skill) => resource.data?.progress.skills[skill.id]?.unlocked);
      const saved = resource.data.progress.loadout.skill_ids.filter((id) => available.some((skill) => skill.id === id)).slice(0, 3);
      setSkillIds(saved.length ? saved : available.slice(0, 1).map((skill) => skill.id));
    }
  }, [resource.data?.config, resource.data?.progress, stageId]);

  const submitAuthoritativeResult = useCallback(async (result: RealmResult) => {
    if (!recording) { setFinalResult({ ...result, verified: false }); setPhase('result'); return; }
    if (!onAuthoritativeComplete) { setResultError('서버 결과 제출 기능을 사용할 수 없습니다. 기록은 저장되지 않았습니다.'); setPhase('result'); return; }
    setSubmitting(true); setResultRetryable(true); setResultError(''); setPhase('result');
    try {
      const response = await onAuthoritativeComplete(resultPayload(result));
      const completion = normalizeRealmGuardCompletion(response);
      const authoritative = completion.result;
      setFinalResult({ ...result, ...authoritative, score: Number(authoritative.score), stars: Number(authoritative.stars), verified: authoritative.verified !== false });
      const updatedProgress = completion.progress ?? await getRealmGuardProgress().catch(() => undefined);
      if (updatedProgress) setResourceData((current) => current ? { ...current, progress: updatedProgress, progressAvailable: true } : current);
    } catch (cause) { setResultError(cause instanceof Error ? cause.message : '결과를 제출하지 못했습니다.'); }
    finally { setSubmitting(false); }
  }, [onAuthoritativeComplete, recording, setResourceData]);

  const handleComplete = useCallback((result: RealmResult) => {
    setLocalResult(result);
    void submitAuthoritativeResult(result);
  }, [submitAuthoritativeResult]);

  const handleCompleteError = useCallback((result: RealmResult, error: Error) => {
    setLocalResult(result); setFinalResult(undefined); setSubmitting(false); setResultRetryable(false); setPhase('result');
    setResultError(`필수 전투 검증 로그를 전송하지 못해 이 결과를 저장할 수 없습니다. 새 전투로 다시 도전해 주세요. (${error.message})`);
  }, []);

  useEffect(() => {
    if (phase !== 'battle' || !canvasRef.current || !battleConfig || !stage || !hero) return;
    let cancelled = false;
    let mounted: RealmSceneController | undefined;
    void import('./RealmGuardScene').then(({ mountRealmGuard }) => {
      if (cancelled || !canvasRef.current) return;
      mounted = mountRealmGuard(canvasRef.current, {
        config: battleConfig, stage, difficulty, hero, accountHeroLevel: progress.hero_levels[hero.id] ?? 1,
        onHUD: setHUD,
        onTelemetry: queueTelemetry,
        onComplete: handleComplete,
        onCompleteError: handleCompleteError,
      });
      controller.current = mounted;
    });
    return () => { cancelled = true; mounted?.destroy(); if (controller.current === mounted) controller.current = undefined; };
  }, [battleConfig, battleKey, difficulty, handleComplete, handleCompleteError, hero, phase, progress.hero_levels, queueTelemetry, stage]);

  const selectedTower = config?.towers.find((tower) => tower.id === hud.selectedTower?.type);
  const stageProgress = (id: string) => progress.stages.filter((item) => item.stage_id === id).reduce((best, item) => item.stars > best.stars ? item : best, { stage_id: id, stars: 0, best_score: 0, difficulties: [] });
  const unlocked = (candidate: RealmStage) => candidate.mode === 'endless' ? progress.campaign_completed : candidate.number <= Math.max(1, progress.unlocked_stage);

  const startBattle = async () => {
    if (!config || !stage || !hero) return;
    setStarting(true);
    try {
      const allowed = preview ? true : await onStart({
        game: 'realmguard', stage_id: stage.id, mode: stage.mode, difficulty, hero_id: hero.id,
        realmguard_version_id: config.versionId,
        content_version: config.contentVersion, balance_version: config.balanceVersion, stage_version: stage.version, asset_version: config.assetVersion,
      });
      if (!allowed) { await resource.reload(); return; }
      const activeRecording = !preview && (isRecording?.() ?? false);
      setRecording(activeRecording);
      telemetryQueue.current = Promise.resolve();
      telemetryFailure.current = undefined;
      telemetrySequence.current = 0;
      optionalTelemetryCount.current = 0;
      battleEventId.current = createRealmGuardUUID();
      if (activeRecording) await saveRealmGuardLoadout({ hero_id: hero.id, skill_ids: skillIds.filter((id) => progress.skills[id]?.unlocked).slice(0, 3) }).catch(() => undefined);
      setHUD({ ...initialHUD, totalWaves: stage.mode === 'endless' ? 0 : stage.waves.length });
      setLocalResult(undefined); setFinalResult(undefined); setResultError(''); setResultRetryable(true); setAimHint(''); setBattleKey((value) => value + 1); setPhase('battle');
    } finally { setStarting(false); }
  };

  const retryBattle = async () => { await startBattle(); };

  if (resource.loading) return <Stack minHeight={520} alignItems="center" justifyContent="center" spacing={2}><CircularProgress /><Typography>RealmGuard 세계를 구성하는 중…</Typography></Stack>;
  if (resource.error || !resource.data || !config) return <Box sx={{ width: '100%', p: 3 }}><ErrorPanel error={resource.error ?? new Error('RealmGuard 설정을 불러오지 못했습니다.')} retry={() => void resource.reload()} /></Box>;

  if (phase === 'select') return <Box sx={{ width: '100%', maxWidth: 1280, mx: 'auto', p: { xs: 1, md: 2 } }}>
    <GameSurface>
    <Paper sx={{ position: 'relative', minHeight: { xs: 620, lg: 660 }, p: { xs: 2, md: 3 }, background: 'radial-gradient(circle at 78% 10%,rgba(111,79,174,.26),transparent 32%),linear-gradient(145deg,#102b2c,#101627 62%,#261b36)' }}>
      <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" gap={2}><Box><Stack direction="row" spacing={1} alignItems="center"><CastleRounded sx={{ fontSize: 42, color: '#7fe0c1' }} /><Typography variant="h1" sx={{ fontSize: { xs: '2rem', md: '3rem' } }}>RealmGuard</Typography></Stack><Typography color="text.secondary" mt={1}>장막 너머의 Realm을 지키는 독자 IP 전략 타워디펜스</Typography></Box><Stack direction="row" spacing={1} alignItems="center"><Chip label={resource.data.version.label} color="primary" variant="outlined" /><Chip icon={<StarsRounded />} label={`${progress.total_stars} 별`} /></Stack></Stack>
      {!resource.data.progressAvailable && <Alert severity="warning" sx={{ mt: 2 }}>진행도를 불러오지 못해 1 스테이지만 표시합니다. 플레이 세션이 연습 모드라면 진행도와 랭킹은 저장되지 않습니다.</Alert>}
      {preview && <Alert severity="warning" sx={{ mt: 2 }}>Designer 미리보기 · {preview.label} · 연습 전용이며 결과, 진행도, 별과 랭킹을 저장하지 않습니다.</Alert>}
      <Grid container spacing={2} mt={1}><Grid size={{ xs: 12, lg: 8 }}><Typography variant="h3" mb={1.5}>캠페인 지도</Typography><Grid container spacing={1.2}>{config.stages.map((item) => { const available = unlocked(item); const saved = stageProgress(item.id); return <Grid key={item.id} size={{ xs: 6, sm: 4, md: 3 }}><Card variant={item.id === stageId ? 'elevation' : 'outlined'} sx={{ height: '100%', borderColor: item.id === stageId ? 'primary.main' : undefined }}><CardActionArea disabled={!available} onClick={() => setStageId(item.id)} sx={{ height: '100%', minHeight: 105, p: 1.5, opacity: available ? 1 : .46 }}><Typography variant="body2" color="primary.main">{item.mode === 'endless' ? '∞ ENDLESS' : `STAGE ${item.number}`}</Typography><Typography fontWeight={850} mt={.4}>{item.name}</Typography><Typography aria-label={`${saved.stars}개 별`} color="warning.main" mt={1}>{'★'.repeat(saved.stars)}{'☆'.repeat(3 - saved.stars)}</Typography></CardActionArea></Card></Grid>; })}</Grid>{stage && <StageRoster stage={stage} enemies={config.enemies} />}</Grid><Grid size={{ xs: 12, lg: 4 }}><Stack spacing={2}><Box><Typography variant="h3">난이도</Typography><TextField select fullWidth value={difficulty} onChange={(event) => setDifficulty(event.target.value as RealmDifficulty)} sx={{ mt: 1.2 }}>{(Object.keys(difficultyLabels) as RealmDifficulty[]).map((key) => <MenuItem key={key} value={key}><Box><Typography fontWeight={800}>{difficultyLabels[key].label}</Typography><Typography variant="body2" color="text.secondary">{difficultyLabels[key].description}</Typography></Box></MenuItem>)}</TextField></Box><Box><Typography variant="h3">출전 영웅</Typography><Stack spacing={1} mt={1.2}>{config.heroes.map((item) => { const heroUnlocked = progress.heroes[item.id]?.unlocked ?? false; return <HeroSelectCard key={item.id} hero={item} game="realmguard" selected={heroId === item.id} unlocked={heroUnlocked} level={progress.hero_levels[item.id] ?? 1} unlockLabel={`Stage ${item.unlockStage ?? 1} 해금`} onSelect={setHeroId} />; })}</Stack></Box><Box><Typography variant="h3">액티브 스킬 <Typography component="span" color="text.secondary">({skillIds.length}/3)</Typography></Typography><Stack direction="row" flexWrap="wrap" useFlexGap spacing={1} mt={1}>{config.skills.map((skill) => { const available = progress.skills[skill.id]?.unlocked ?? false; const selected = skillIds.includes(skill.id); return <Tooltip key={skill.id} title={available ? skill.description : '캠페인을 진행하면 잠금 해제됩니다.'}><span><Button disabled={!available || (!selected && skillIds.length >= 3)} variant={selected ? 'contained' : 'outlined'} onClick={() => setSkillIds((current) => current.includes(skill.id) ? current.length > 1 ? current.filter((id) => id !== skill.id) : current : [...current, skill.id].slice(0, 3))}>{available ? skill.name : '잠김'}</Button></span></Tooltip>; })}</Stack></Box><Button variant="contained" size="large" startIcon={starting ? <CircularProgress size={20} /> : <ShieldRounded />} disabled={starting || !stage || !hero || skillIds.length === 0 || !progress.heroes[hero.id]?.unlocked || !unlocked(stage)} onClick={() => void startBattle()} sx={{ minHeight: 56 }}>수호전 시작</Button></Stack></Grid></Grid>
      <RealmGuardLeaderboard config={config} />
    </Paper>
    </GameSurface>
  </Box>;

  const command = (value: Parameters<RealmSceneController['command']>[0]) => controller.current?.command(value);
  return <GameSurface><Box ref={battleRef} data-testid="realmguard-battle-shell" sx={{ width: '100%', maxWidth: 1280, mx: 'auto', position: 'relative', aspectRatio: '16 / 9', bgcolor: '#09131f', overflow: 'hidden', userSelect: 'none', scrollMarginTop: `${BATTLE_SCROLL_MARGIN}px`, [portraitBattleMedia]: { aspectRatio: 'auto', minHeight: `min(760px, calc(100dvh - ${BATTLE_SCROLL_MARGIN + 8}px))` } }}>
    <Box ref={canvasRef} aria-label="RealmGuard 전장" sx={{ position: 'absolute', inset: 0, touchAction: 'none', '& canvas': { display: 'block', maxWidth: '100%', maxHeight: '100%', touchAction: 'none' }, [portraitBattleMedia]: { top: 132, bottom: 270 } }} />
    {phase === 'battle' && <>
      <Stack data-testid="realmguard-battle-hud" direction="row" spacing={1} sx={{ position: 'absolute', top: 10, left: 10, right: 10, zIndex: 4, pointerEvents: 'auto', overflowX: 'auto', overflowY: 'hidden', touchAction: 'pan-x', pb: .5, scrollbarWidth: 'thin', '& .MuiChip-root, & .MuiButton-root, & > span': { flexShrink: 0 }, [portraitBattleMedia]: { top: 8, left: 8, right: 8 } }}><Chip icon={<FavoriteRounded />} label={`${hud.lives} 생명`} color={hud.lives <= 5 ? 'error' : 'default'} sx={{ fontWeight: 800 }} /><Chip label={`${hud.gold} 골드`} sx={{ fontWeight: 800 }} /><Chip label={`파동 ${hud.wave}/${hud.totalWaves || '∞'}`} sx={{ fontWeight: 800 }} /><Chip label={`${hud.kills} 처치`} /><Box flex={1} /><Tooltip title={hud.heroAlive ? '영웅을 이동할 지점을 선택합니다.' : `${hud.heroRespawn}초 후 부활`}><span><Button disabled={!hud.heroAlive} variant="contained" color="secondary" onClick={() => { command({ type: 'move-hero' }); setAimHint('영웅이 이동할 지점을 선택하세요.'); }} sx={{ minHeight: 44, minWidth: 190 }}>{hero?.name ?? '영웅'} Lv.{hud.heroLevel} · {hud.heroHp}/{hud.heroMaxHp} HP</Button></span></Tooltip><Button variant="contained" onClick={() => command({ type: 'speed', value: hud.speed === 1 ? 2 : 1 })} startIcon={<FastForwardRounded />} sx={{ minHeight: 44 }}>{hud.speed}×</Button><Button variant="contained" onClick={() => command({ type: 'toggle-pause' })} startIcon={hud.status === 'paused' ? <PlayArrowRounded /> : <PauseRounded />} sx={{ minHeight: 44 }}>{hud.status === 'paused' ? '계속' : '일시정지'}</Button></Stack>
      <Stack data-testid="realmguard-skill-panel" spacing={1} sx={{ position: 'absolute', right: 10, top: 68, zIndex: 4, [portraitBattleMedia]: { top: 'auto', right: 8, bottom: 108 } }}>{battleConfig?.skills.map((skill) => { const cooldown = hud.skillCooldowns[skill.id] ?? 0; return <Tooltip key={skill.id} title={skill.description} placement="left"><span><Button disabled={cooldown > 0 || hud.status === 'paused'} variant="contained" onClick={() => { command({ type: 'skill', skill: skill.id }); setAimHint(skill.id === 'freeze' ? '' : `${skill.name}을 사용할 지점을 선택하세요.`); }} sx={{ minWidth: 128, minHeight: 48, bgcolor: skill.color, color: '#07101d', fontWeight: 900, '&:hover': { bgcolor: skill.color, filter: 'brightness(1.08)' } }} startIcon={skill.id === 'meteor' ? <LocalFireDepartmentRounded /> : <BoltRounded />}>{cooldown ? `${cooldown}초` : skill.name}</Button></span></Tooltip>; })}</Stack>
      {aimHint && <Alert data-testid="realmguard-aim-hint" severity="info" sx={{ position: 'absolute', top: 72, left: '50%', zIndex: 5, transform: 'translateX(-50%)', py: .3, [portraitBattleMedia]: { top: 142, width: 'calc(100% - 16px)', maxWidth: 390 } }} onClose={() => setAimHint('')}>{aimHint}</Alert>}
      <Paper data-testid="realmguard-command-panel" sx={{ position: 'absolute', left: 10, bottom: 10, right: 10, zIndex: 4, p: 1.2, bgcolor: 'rgba(7,16,29,.92)', backdropFilter: 'blur(8px)', [portraitBattleMedia]: { left: 8, right: 8, bottom: 8, p: 1 } }}><Stack direction="row" spacing={1} alignItems="center" sx={{ overflowX: 'auto', overflowY: 'hidden', touchAction: 'pan-x', pb: .5, scrollbarWidth: 'thin', '& .MuiButton-root': { flexShrink: 0, whiteSpace: 'nowrap' }, '& .MuiInputBase-root, & .MuiTypography-root, & .MuiDivider-root, & > span': { flexShrink: 0 } }}><Button variant="contained" color="warning" disabled={hud.status === 'paused' || hud.nextWaveIn === 0} startIcon={<FlagRounded />} onClick={() => command({ type: 'start-wave' })} sx={{ minWidth: 190, minHeight: 48 }}>{hud.nextWaveIn > 0 ? `조기 호출 +${hud.nextWaveIn * 3}G (${hud.nextWaveIn}초)` : '파동 진행 중'}</Button><Divider orientation="vertical" flexItem />{hud.selectedSpot ? hud.selectedTower && selectedTower ? <><Typography fontWeight={850} sx={{ minWidth: 120 }}>{selectedTower.name} L{hud.selectedTower.level}</Typography>{hud.selectedTower.level < 2 && <Button variant="outlined" disabled={hud.gold < config.balance.towerUpgradeCost[1]} onClick={() => command({ type: 'upgrade' })} sx={{ minHeight: 44 }}>L2 강화 {config.balance.towerUpgradeCost[1]}G</Button>}{hud.selectedTower.level === 2 && selectedTower.branches.map((branch) => <Tooltip key={branch.id} title={branch.description}><Button variant="outlined" disabled={hud.gold < config.balance.towerUpgradeCost[2]} onClick={() => command({ type: 'upgrade', branch: branch.id })} sx={{ minHeight: 44 }}>{branch.name} {config.balance.towerUpgradeCost[2]}G</Button></Tooltip>)}<TextField select size="small" label="공격 우선순위" value={hud.selectedTower.targeting} onChange={(event) => command({ type: 'targeting', mode: event.target.value as TargetingMode })} sx={{ minWidth: 145 }}>{(Object.keys(targetingLabels) as TargetingMode[]).map((mode) => <MenuItem key={mode} value={mode}>{targetingLabels[mode]}</MenuItem>)}</TextField><Button color="error" onClick={() => command({ type: 'sell' })} sx={{ minHeight: 44 }}>판매</Button></> : config.towers.map((tower) => <Tooltip key={tower.id} title={tower.role}><span><Button variant="contained" disabled={hud.gold < tower.cost} onClick={() => command({ type: 'build', tower: tower.id })} sx={{ minHeight: 48 }}>{tower.name} {tower.cost}G</Button></span></Tooltip>) : <Typography color="text.secondary">빛나는 건설 지점을 선택해 타워를 배치하세요.</Typography>}</Stack></Paper>
      {!recording && <Alert data-testid="realmguard-practice-warning" severity="warning" sx={{ position: 'absolute', left: 10, top: 68, zIndex: 3, maxWidth: 390, [portraitBattleMedia]: { left: 8, right: 8, top: 68, maxWidth: 'none' } }}>연습 모드 · 진행도와 랭킹은 저장되지 않습니다.</Alert>}
    </>}
    {phase === 'result' && <Box sx={{ position: 'absolute', inset: 0, bgcolor: 'rgba(3,8,16,.86)', display: 'grid', placeItems: 'center', p: 2 }}><Card sx={{ width: 'min(620px,92%)' }}><CardContent sx={{ p: { xs: 2.5, md: 4 } }}><Stack alignItems="center" spacing={2}>{submitting ? <><CircularProgress /><Typography variant="h3">서버에서 전투 결과를 검증하는 중…</Typography><LinearProgress sx={{ width: '100%' }} /></> : <><Typography variant="h2">{localResult?.victory ? 'Realm 수호 성공' : '방어선 붕괴'}</Typography>{finalResult && <><Typography sx={{ fontSize: '3rem', fontWeight: 950, color: 'warning.main' }}>{finalResult.score.toLocaleString()}</Typography><Typography aria-label={`${finalResult.stars}개 별`} sx={{ fontSize: '2rem', color: 'warning.main' }}>{'★'.repeat(finalResult.stars)}{'☆'.repeat(3 - finalResult.stars)}</Typography><Grid container spacing={1} width="100%">{[['남은 생명', finalResult.lives], ['처치', finalResult.kills], ['완료 파동', finalResult.waves_completed], ['전투 영웅 레벨', finalResult.hero_level]].map(([label, value]) => <Grid key={String(label)} size={3}><Paper variant="outlined" sx={{ p: 1.2, textAlign: 'center' }}><Typography variant="body2" color="text.secondary">{label}</Typography><Typography fontWeight={900}>{value}</Typography></Paper></Grid>)}</Grid>{finalResult.verified ? <Alert severity="success" sx={{ width: '100%' }}>서버 검증이 완료되어 진행도와 랭킹에 반영되었습니다.</Alert> : <Alert severity="warning" sx={{ width: '100%' }}>연습 결과입니다. 진행도와 랭킹에는 저장되지 않았습니다.</Alert>}</>}{resultError && <Alert severity="error" sx={{ width: '100%' }}>{resultError}{resultRetryable && <Button size="small" onClick={() => localResult && void submitAuthoritativeResult(localResult)}>다시 제출</Button>}</Alert>}<Stack direction="row" spacing={1}><Button startIcon={<RestartAltRounded />} variant="outlined" onClick={() => void retryBattle()}>다시 도전</Button><Button variant="contained" onClick={() => setPhase('select')}>스테이지 선택</Button></Stack></>}</Stack></CardContent></Card></Box>}
  </Box></GameSurface>;
}

export default RealmGuardGame;
