import { useCallback, useEffect, useRef, useState } from "react";
import AutoAwesomeRounded from "@mui/icons-material/AutoAwesomeRounded";
import BoltRounded from "@mui/icons-material/BoltRounded";
import FastForwardRounded from "@mui/icons-material/FastForwardRounded";
import FlagRounded from "@mui/icons-material/FlagRounded";
import HealthAndSafetyRounded from "@mui/icons-material/HealthAndSafetyRounded";
import PauseRounded from "@mui/icons-material/PauseRounded";
import PlayArrowRounded from "@mui/icons-material/PlayArrowRounded";
import PsychologyRounded from "@mui/icons-material/PsychologyRounded";
import RestartAltRounded from "@mui/icons-material/RestartAltRounded";
import SchoolRounded from "@mui/icons-material/SchoolRounded";
import ShieldRounded from "@mui/icons-material/ShieldRounded";
import {
  Alert,
  Box,
  Button,
  Card,
  CardActionArea,
  CardContent,
  Chip,
  CircularProgress,
  Container,
  Divider,
  Grid,
  LinearProgress,
  MenuItem,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useParams } from "react-router-dom";
import { ErrorPanel } from "../../components/ErrorPanel";
import { useAsync } from "../../hooks/useAsync";
import type { BuiltinGameProps } from "../types";
import type {
  BattleHUD,
  RealmCommand,
  RealmDifficulty,
  RealmResult,
  RealmSceneController,
} from "../realmguard/types";
import { HeroSelectCard } from "../realmguard/HeroSelectCard";
import { StageRoster } from "../realmguard/StageRoster";
import { DEFENSE_PACKS, isDefenseSlug } from "./content";
import {
  defenseAPI,
  normalizeDefenseLearningBreakdown,
  normalizeDefenseServerResult,
} from "./api";
import { mountDefenseCore } from "./core/DefenseCore";
import { StageMapPreview } from "./StageMapPreview";
import {
  createDefenseUUID,
  DEFENSE_OPTIONAL_TELEMETRY_LIMIT,
  defenseAttestationPayload,
  defenseEducationTrigger,
  isAllowedDefenseTelemetry,
  isRequiredDefenseTelemetry,
  openDefensePromptBeforeTelemetry,
  retryDefenseTelemetry,
  shouldPauseDefensePrompt,
} from "./telemetry";
import {
  aiDepletionDisposition,
  aiEscapedResourceCosts,
  aiResourcePercent,
  applyAIResourceCosts,
  applyAIResourceDeltas,
  buildAIResourceState,
  defenseTelemetryUsesAIResourceState,
  initialAIResources,
  isAIResourceDepleted,
} from "./resource";
import { isDefenseHeroUnlocked, resolveDefenseProgress } from "./progress";
import type {
  AIModelProfile,
  AIResources,
  DefenseAnswerSubmission,
  DefenseContentPack,
  DefenseEducationEvent,
  DefenseLearningBreakdown,
  DefenseProgress,
  DefenseServerResult,
  DefenseSlug,
} from "./types";
import { optionLabel } from "../../labels";
import { GameSurface } from "../GameSurface";
import { BATTLE_SCROLL_MARGIN, useBattleInView } from "../useBattleInView";

const INITIAL_HUD: BattleHUD = {
  status: "ready",
  gold: 0,
  lives: 20,
  wave: 0,
  totalWaves: 0,
  kills: 0,
  heroLevel: 1,
  heroHp: 0,
  heroMaxHp: 0,
  heroAlive: true,
  heroRespawn: 0,
  nextWaveIn: 10,
  skillCooldowns: {},
  speed: 1,
};
const INITIAL_AI: AIResources = {
  compute: 1000,
  token: 1000,
  trust: 100,
  latency: 100,
};
const difficultyLabels: Record<RealmDifficulty, string> = {
  casual: "캐주얼",
  normal: "노멀",
  veteran: "베테랑",
};

interface AnswerFeedback {
  correct?: boolean;
  topic: string;
  score?: number;
  explanation: string;
  sending?: boolean;
  error?: string;
}

interface DefenseSeriesGameProps extends BuiltinGameProps {
  preview?: { slug: DefenseSlug; pack: DefenseContentPack; label: string };
}

function practiceProgress(pack: DefenseContentPack): DefenseProgress {
  return {
    items: pack.config.stages.flatMap((stage) =>
      (["casual", "normal", "veteran"] as RealmDifficulty[]).map(
        (difficulty) => ({
          stage_id: stage.id,
          difficulty,
          unlocked: true,
          completed: false,
          best_score: 0,
          best_learning_score: 0,
          attempts: 0,
          completions: 0,
          total_playtime_ms: 0,
        }),
      ),
    ),
    summary: {
      completed_stages: 0,
      total_stars: 0,
      total_playtime_ms: 0,
      campaign_complete: false,
    },
  };
}

function LearningPanel({
  breakdown,
  score,
}: {
  breakdown: DefenseLearningBreakdown[];
  score: number;
}) {
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" spacing={1} alignItems="center">
          <SchoolRounded color="secondary" />
          <Typography variant="h3">Learning Score</Typography>
          <Chip color="secondary" label={`${score}점`} />
        </Stack>
        <Grid container spacing={1} mt={1}>
          {breakdown.length ? (
            breakdown.map((item) => (
              <Grid key={item.topic} size={{ xs: 12, sm: 6 }}>
                <Paper variant="outlined" sx={{ p: 1.2 }}>
                  <Stack direction="row" justifyContent="space-between">
                    <Typography fontWeight={800}>{item.topic}</Typography>
                    <Typography color="secondary.main" fontWeight={900}>
                      {item.score}
                    </Typography>
                  </Stack>
                  <LinearProgress
                    variant="determinate"
                    value={item.score}
                    color="secondary"
                    sx={{ mt: 1 }}
                  />
                  <Typography variant="body2" color="text.secondary" mt={0.6}>
                    {item.correct}/{item.total} 올바른 판단
                  </Typography>
                </Paper>
              </Grid>
            ))
          ) : (
            <Grid size={12}>
              <Typography color="text.secondary">
                전투 중 교육 상황에 답하면 주제별 이해도가 표시됩니다.
              </Typography>
            </Grid>
          )}
        </Grid>
      </CardContent>
    </Card>
  );
}

function ResourceHUD({
  pack,
  values,
}: {
  pack: DefenseContentPack;
  values: AIResources;
}) {
  const limits = initialAIResources(pack);
  const rows: Array<[keyof AIResources, string, string, number, number]> = [
    [
      "compute",
      "Compute",
      "#b694ff",
      aiResourcePercent(pack, "compute", values.compute),
      values.compute,
    ],
    [
      "token",
      "Token",
      "#65d6ff",
      aiResourcePercent(pack, "token", values.token),
      values.token,
    ],
    [
      "trust",
      "Trust",
      "#72e0a6",
      aiResourcePercent(pack, "trust", values.trust),
      values.trust,
    ],
    [
      "latency",
      "Latency pressure",
      "#ff9a76",
      100 - aiResourcePercent(pack, "latency", values.latency),
      limits.latency - values.latency,
    ],
  ];
  return (
    <Paper
      aria-label="AI 자원 상태"
      data-testid="defense-ai-resource-hud"
      sx={{
        position: "absolute",
        left: 10,
        top: 68,
        width: { xs: "min(230px, calc(100% - 20px))", sm: 230 },
        p: 1.2,
        bgcolor: "rgba(7,16,29,.94)",
        zIndex: 4,
      }}
    >
      <Typography fontWeight={900} mb={0.8}>
        AI Nexus 자원
      </Typography>
      {rows.map(([key, label, color, progress, display]) => (
        <Box
          key={key}
          data-testid={`defense-ai-resource-${key}`}
          data-remaining={values[key]}
          data-start={limits[key]}
          mb={0.7}
        >
          <Stack direction="row" justifyContent="space-between">
            <Typography variant="body2">{label}</Typography>
            <Typography variant="body2" fontWeight={800}>
              {display}
            </Typography>
          </Stack>
          <LinearProgress
            variant="determinate"
            value={progress}
            sx={{ mt: 0.25, "& .MuiLinearProgress-bar": { bgcolor: color } }}
          />
        </Box>
      ))}
    </Paper>
  );
}

function ModelProfiles({ pack }: { pack: DefenseContentPack }) {
  if (!pack.modelProfiles?.length) return null;
  return (
    <Card variant="outlined">
      <CardContent>
        <Typography variant="h3">Model Tower 전략</Typography>
        <Typography variant="body2" color="text.secondary" mt={0.5}>
          정확도가 높은 모델은 더 많은 Compute·Token과 Latency 여유를
          소비합니다.
        </Typography>
        <Stack spacing={0.8} mt={1.2}>
          {pack.modelProfiles.map((profile) => (
            <Paper
              key={profile.id}
              data-testid={`defense-profile-${profile.id}`}
              variant="outlined"
              sx={{ p: 1 }}
            >
              <Stack direction="row" justifyContent="space-between" gap={1}>
                <Box>
                  <Typography fontWeight={850}>{profile.name}</Typography>
                  <Typography variant="body2" color="text.secondary">
                    정확도 {profile.accuracy}% · 피해 ×
                    {profile.damage_multiplier}
                  </Typography>
                </Box>
                <Typography
                  variant="body2"
                  textAlign="right"
                  color="primary.main"
                >
                  C {profile.compute_cost}
                  <br />T {profile.token_cost} · L {profile.latency_cost}
                </Typography>
              </Stack>
            </Paper>
          ))}
        </Stack>
      </CardContent>
    </Card>
  );
}

function DefenseLeaderboard({ slug }: { slug: DefenseSlug }) {
  const [period, setPeriod] = useState("weekly");
  const [group, setGroup] = useState("individual");
  const result = useAsync(
    () => defenseAPI.rankings(slug, period, group),
    [group, period, slug],
  );
  const forbidden =
    result.error &&
    "status" in result.error &&
    Number((result.error as Error & { status?: number }).status) === 403;
  return (
    <Card variant="outlined" sx={{ mt: 2 }}>
      <CardContent>
        <Stack
          direction={{ xs: "column", sm: "row" }}
          justifyContent="space-between"
          gap={1}
        >
          <Box>
            <Typography variant="h3">Defense 랭킹</Typography>
            <Typography color="text.secondary">
              개인·부서·팀 경쟁 기록
            </Typography>
          </Box>
          <Stack direction="row" spacing={1}>
            <TextField
              select
              size="small"
              label="기간"
              value={period}
              onChange={(event) => setPeriod(event.target.value)}
              sx={{ minWidth: 108 }}
            >
              <MenuItem value="daily">오늘</MenuItem>
              <MenuItem value="weekly">주간</MenuItem>
              <MenuItem value="monthly">월간</MenuItem>
              <MenuItem value="season">시즌</MenuItem>
              <MenuItem value="all_time">전체</MenuItem>
            </TextField>
            <TextField
              select
              size="small"
              label="그룹"
              value={group}
              onChange={(event) => setGroup(event.target.value)}
              sx={{ minWidth: 112 }}
            >
              <MenuItem value="individual">개인</MenuItem>
              <MenuItem value="department">부서</MenuItem>
              <MenuItem value="team">팀</MenuItem>
            </TextField>
          </Stack>
        </Stack>
        {result.loading && <LinearProgress sx={{ mt: 1 }} />}
        {result.error && (
          <Alert severity="warning" sx={{ mt: 1 }}>
            {forbidden
              ? "관리자가 설정한 개인정보 공개 범위에 따라 이 랭킹은 볼 수 없습니다."
              : result.error.message}
          </Alert>
        )}
        <Grid container spacing={1} mt={0.5}>
          {result.data?.items.slice(0, 6).map((entry) => (
            <Grid
              key={`${entry.rank}-${entry.display_name}`}
              size={{ xs: 12, sm: 6 }}
            >
              <Stack direction="row" spacing={1} alignItems="center">
                <Chip size="small" label={`#${entry.rank}`} />
                <Box flex={1}>
                  <Typography fontWeight={800}>{entry.display_name}</Typography>
                  <Typography variant="body2" color="text.secondary">
                    {entry.department || entry.team || entry.stage_id}
                  </Typography>
                </Box>
                <Typography fontWeight={900}>
                  {entry.score.toLocaleString()}
                </Typography>
              </Stack>
            </Grid>
          ))}
          {!result.loading &&
            !result.error &&
            result.data?.items.length === 0 && (
              <Grid size={12}>
                <Typography color="text.secondary">
                  아직 등록된 기록이 없습니다.
                </Typography>
              </Grid>
            )}
        </Grid>
      </CardContent>
    </Card>
  );
}

export function DefenseSeriesGame({
  onStart,
  onTelemetry,
  onAuthoritativeComplete,
  onAuthoritativeRequest,
  isRecording,
  preview,
}: DefenseSeriesGameProps) {
  const params = useParams();
  const routeSlug = params.slug ?? preview?.slug ?? "";
  const slug = isDefenseSlug(routeSlug) ? routeSlug : preview?.slug;
  const [practice, setPractice] = useState(Boolean(preview));
  const resource = useAsync(async () => {
    if (!slug) throw new Error("지원하지 않는 Defense 게임입니다.");
    if (preview)
      return {
        pack: preview.pack,
        progress: practiceProgress(preview.pack),
        rankings: [],
        learning: undefined,
        label: preview.label,
      };
    const current = await defenseAPI.config(slug);
    const [progress, rankings, learning] = await Promise.allSettled([
      defenseAPI.progress(slug),
      defenseAPI.rankings(slug),
      current.pack.educationEnabled
        ? defenseAPI.learning(slug)
        : Promise.resolve(undefined),
    ]);
    return {
      pack: current.pack,
      progress:
        progress.status === "fulfilled"
          ? progress.value
          : {
              items: [],
              summary: {
                completed_stages: 0,
                total_stars: 0,
                total_playtime_ms: 0,
                campaign_complete: false,
              },
            },
      rankings: rankings.status === "fulfilled" ? rankings.value.items : [],
      learning: learning.status === "fulfilled" ? learning.value : undefined,
      label: current.envelope.version.label,
    };
  }, [preview, slug]);
  const [practicePack, setPracticePack] = useState<DefenseContentPack>();
  const [progressOverride, setProgressOverride] = useState<DefenseProgress>();
  const pack = practicePack ?? resource.data?.pack;
  const progress = practicePack
    ? practiceProgress(practicePack)
    : resolveDefenseProgress(resource.data?.progress, progressOverride);
  const canvasRef = useRef<HTMLDivElement>(null);
  const controller = useRef<RealmSceneController | undefined>(undefined);
  const telemetryQueue = useRef<Promise<void>>(Promise.resolve());
  const telemetryFailure = useRef<Error | undefined>(undefined);
  const telemetrySequence = useRef(0);
  const optionalTelemetryCount = useRef(0);
  const battleId = useRef("");
  const recordingRef = useRef(false);
  const hudRef = useRef(INITIAL_HUD);
  const activeEventRef = useRef<DefenseEducationEvent | undefined>(undefined);
  const deferredAIDefeatRef = useRef(false);
  const answeredEventsRef = useRef<Set<string>>(new Set());
  const answersRef = useRef<DefenseAnswerSubmission[]>([]);
  const learningRef = useRef(
    new Map<string, { correct: number; total: number; score: number }>(),
  );
  const aiRef = useRef<AIResources>({ ...INITIAL_AI });
  const lastEscapedByEnemy = useRef<Record<string, number>>({});
  const [phase, setPhase] = useState<"select" | "battle" | "result">("select");
  const battleRef = useBattleInView<HTMLDivElement>(phase !== "select");
  const [stageId, setStageId] = useState("stage-1");
  const [difficulty, setDifficulty] = useState<RealmDifficulty>("normal");
  const [heroId, setHeroId] = useState("");
  const [hud, setHUD] = useState(INITIAL_HUD);
  const [battleKey, setBattleKey] = useState(0);
  const [activeEvent, setActiveEvent] = useState<DefenseEducationEvent>();
  const [feedback, setFeedback] = useState<AnswerFeedback>();
  const [answeredEvents, setAnsweredEvents] = useState<Set<string>>(new Set());
  const [aiResources, setAIResources] = useState<AIResources>({
    ...INITIAL_AI,
  });
  const [localResult, setLocalResult] = useState<RealmResult>();
  const [serverResult, setServerResult] = useState<DefenseServerResult>();
  const [resultError, setResultError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [starting, setStarting] = useState(false);
  const [recording, setRecording] = useState(false);
  const [learningBreakdown, setLearningBreakdown] = useState<
    DefenseLearningBreakdown[]
  >([]);
  const [learningScore, setLearningScore] = useState(0);
  const [eventPaused, setEventPaused] = useState(false);

  useEffect(() => {
    if (!pack) return;
    const nextStage =
      pack.config.stages.find((item) => item.id === stageId) ??
      pack.config.stages[0];
    if (nextStage?.id !== stageId)
      setStageId(nextStage?.id ?? "stage-1");
    const selectedHero = pack.config.heroes.find((item) => item.id === heroId);
    if (!selectedHero || !isDefenseHeroUnlocked(selectedHero, nextStage))
      setHeroId(
        pack.config.heroes.find((item) =>
          isDefenseHeroUnlocked(item, nextStage),
        )?.id ?? "",
      );
  }, [heroId, pack, stageId]);
  useEffect(() => {
    setProgressOverride(undefined);
  }, [pack?.config.versionId, slug]);

  const stage = pack?.config.stages.find((item) => item.id === stageId);
  const hero = pack?.config.heroes.find((item) => item.id === heroId);
  useEffect(() => {
    hudRef.current = hud;
  }, [hud]);
  useEffect(() => {
    activeEventRef.current = activeEvent;
  }, [activeEvent]);
  useEffect(() => {
    answeredEventsRef.current = answeredEvents;
  }, [answeredEvents]);

  const queueTelemetry = useCallback(
    (event: string, data: Record<string, unknown> = {}) => {
      if (!recordingRef.current || !onTelemetry) return Promise.resolve();
      if (!isAllowedDefenseTelemetry(event)) return Promise.resolve();
      if (!isRequiredDefenseTelemetry(event)) {
        if (optionalTelemetryCount.current >= DEFENSE_OPTIONAL_TELEMETRY_LIMIT)
          return Promise.resolve();
        optionalTelemetryCount.current += 1;
      }
      const payload = defenseAttestationPayload(
        battleId.current,
        ++telemetrySequence.current,
        data,
      );
      const pending = telemetryQueue.current.then(async () => {
        if (telemetryFailure.current) throw telemetryFailure.current;
        try {
          await retryDefenseTelemetry(() => onTelemetry(event, payload));
        } catch (cause) {
          const error =
            cause instanceof Error
              ? cause
              : new Error("Defense 전투 검증 로그를 전송하지 못했습니다.");
          telemetryFailure.current = error;
          throw error;
        }
      });
      telemetryQueue.current = pending.catch(() => undefined);
      return pending;
    },
    [onTelemetry],
  );

  const flushTelemetry = useCallback(async () => {
    await telemetryQueue.current;
    if (telemetryFailure.current) throw telemetryFailure.current;
  }, []);

  const openTriggeredEvent = useCallback(
    (trigger: string) => {
      if (!pack?.educationEnabled || activeEventRef.current) return false;
      const event = pack.events.find(
        (item) =>
          item.stage_id === stageId &&
          item.trigger.replace("-", "_") === trigger &&
          !answeredEventsRef.current.has(item.id),
      );
      if (!event) return false;
      activeEventRef.current = event;
      const scene = controller.current;
      if (
        shouldPauseDefensePrompt(
          true,
          false,
          Boolean(scene),
          hudRef.current.status,
        )
      ) {
        scene!.command({ type: "toggle-pause" });
        setEventPaused(true);
      }
      setFeedback(undefined);
      setActiveEvent(event);
      return true;
    },
    [pack, stageId],
  );

  useEffect(() => {
    const scene = controller.current;
    if (
      phase === "battle" &&
      shouldPauseDefensePrompt(
        Boolean(activeEvent),
        eventPaused,
        Boolean(scene),
        hudRef.current.status,
      )
    ) {
      scene!.command({ type: "toggle-pause" });
      setEventPaused(true);
    }
  }, [activeEvent, battleKey, eventPaused, phase]);

  const updateAILoad = useCallback(
    (event: string, data: Record<string, unknown>) => {
      if (slug !== "ai-nexus-defense") return;
      const rules = pack?.resourceRules;
      if (!rules || !defenseTelemetryUsesAIResourceState(event)) return;
      const current = aiRef.current;
      let next = current;
      if (event === "defense.wave.start")
        next = applyAIResourceCosts(pack, current, {
          compute: rules.wave_compute_cost,
          token: rules.wave_token_cost,
        });
      if (event === "defense.tower.build") {
        const profile = pack.modelProfiles?.find(
          (item) => item.id === data.profile_id && item.tower_id === data.tower,
        );
        if (profile)
          next = applyAIResourceCosts(pack, current, {
            compute: profile.compute_cost,
            token: profile.token_cost,
            latency: profile.latency_cost,
          });
      }
      if (
        event === "defense.wave.complete" ||
        event === "defense.battle.complete"
      ) {
        const histogram =
          data.escaped_by_enemy && typeof data.escaped_by_enemy === "object"
            ? (data.escaped_by_enemy as Record<string, number>)
            : {};
        const escapedCosts = aiEscapedResourceCosts(
          pack,
          histogram,
          lastEscapedByEnemy.current,
        );
        lastEscapedByEnemy.current = escapedCosts.cumulative;
        next = applyAIResourceCosts(pack, next, escapedCosts.costs);
      }
      aiRef.current = next;
      setAIResources(next);
      return next;
    },
    [pack, slug],
  );

  const coreTelemetry = useCallback(
    async (event: string, data: Record<string, unknown> = {}) => {
      const resources = updateAILoad(event, data);
      const terminalWaveStart = Boolean(
        event === "defense.wave.start" &&
          resources &&
          isAIResourceDepleted(resources),
      );
      const scheduled = openDefensePromptBeforeTelemetry(
        defenseEducationTrigger(event, data, terminalWaveStart),
        openTriggeredEvent,
        () =>
          queueTelemetry(event, {
            ...data,
            ...(resources && pack
              ? { resource_state: buildAIResourceState(pack, resources) }
              : {}),
          }),
      );
      if (resources) {
        const disposition = aiDepletionDisposition(
          resources,
          scheduled.opened || Boolean(activeEventRef.current),
        );
        if (disposition === "defer") deferredAIDefeatRef.current = true;
        if (disposition === "defeat")
          controller.current?.command({ type: "force-defeat" });
      }
      await scheduled.pending;
    },
    [openTriggeredEvent, pack, queueTelemetry, updateAILoad],
  );

  const currentBreakdown = useCallback(() => {
    const values = [...learningRef.current.entries()].map(([topic, value]) => ({
      topic,
      ...value,
    }));
    const score = values.length
      ? Math.round(
          values.reduce((sum, item) => sum + item.score, 0) / values.length,
        )
      : 0;
    setLearningBreakdown(values);
    setLearningScore(score);
    return { values, score };
  }, []);

  const submitResult = useCallback(
    async (battle: RealmResult) => {
      if (
        !pack ||
        practice ||
        preview ||
        !recordingRef.current ||
        !onAuthoritativeComplete
      ) {
        const learning = currentBreakdown();
        setServerResult({
          result: {
            score: battle.score,
            stars: battle.stars,
            verified: false,
            learning_score: learning.score,
            learning_breakdown: learning.values,
          },
        });
        setPhase("result");
        return;
      }
      setSubmitting(true);
      setResultError("");
      setPhase("result");
      try {
        const response = normalizeDefenseServerResult(
          await onAuthoritativeComplete({
            stage_id: battle.stage_id,
            difficulty: battle.difficulty,
            duration_ms: battle.duration_ms,
            remaining_health: battle.lives,
            remaining_resource: battle.gold,
            kills: battle.kills,
            escaped: battle.escaped,
            spawned: battle.spawned,
            waves_completed: battle.waves_completed,
            victory: battle.victory,
            defeated_by_enemy: battle.defeated_by_enemy,
            escaped_by_enemy: battle.escaped_by_enemy,
            spawned_by_enemy: battle.spawned_by_enemy,
            content_version: pack.config.contentVersion,
            policy_version: pack.policyVersion,
            answers: answersRef.current,
            battle: {
              earned_resource: battle.earned_gold,
              spent_resource: battle.spent_gold,
              recovered_resource: battle.sold_gold,
              hero_id: battle.hero_id,
              hero_level: battle.hero_level,
            },
            ...(slug === "ai-nexus-defense"
              ? { resource_state: buildAIResourceState(pack, aiRef.current) }
              : {}),
          }),
        );
        setServerResult(response);
        if (response.progress) setProgressOverride(response.progress);
        const breakdown = normalizeDefenseLearningBreakdown(
          response.result.learning_breakdown,
        );
        setLearningBreakdown(breakdown);
        setLearningScore(response.result.learning_score);
      } catch (cause) {
        setResultError(
          cause instanceof Error
            ? cause.message
            : "Defense 결과를 검증하지 못했습니다.",
        );
      } finally {
        setSubmitting(false);
      }
    },
    [currentBreakdown, onAuthoritativeComplete, pack, practice, preview, slug],
  );

  useEffect(() => {
    if (phase !== "battle" || !canvasRef.current || !pack || !stage || !hero)
      return;
    const mounted = mountDefenseCore(canvasRef.current, {
      slug: pack.slug,
      config: pack.config,
      stage,
      difficulty,
      hero,
      policyVersion: pack.policyVersion,
      onHUD: setHUD,
      onTelemetry: coreTelemetry,
      onComplete: (result) => {
        setLocalResult(result);
        void submitResult(result);
      },
      onCompleteError: (result, error) => {
        setLocalResult(result);
        setResultError(error.message);
        setPhase("result");
      },
    });
    controller.current = mounted;
    return () => {
      mounted.destroy();
      controller.current = undefined;
    };
  }, [
    battleKey,
    coreTelemetry,
    difficulty,
    hero,
    pack,
    phase,
    stage,
    submitResult,
  ]);

  const startBattle = async () => {
    if (!pack || !stage || !hero) return;
    setStarting(true);
    try {
      if (!practice && !preview) {
        const allowed = await onStart({
          defense_content_version_id: pack.config.versionId,
          stage_id: stage.id,
          difficulty,
        });
        if (!allowed) {
          await resource.reload();
          return;
        }
      }
      const initialAI = initialAIResources(pack);
      answersRef.current = [];
      learningRef.current = new Map();
      aiRef.current = initialAI;
      lastEscapedByEnemy.current = {};
      deferredAIDefeatRef.current = false;
      const activeRecording = !practice && !preview && Boolean(isRecording?.());
      recordingRef.current = activeRecording;
      setRecording(activeRecording);
      telemetryQueue.current = Promise.resolve();
      telemetryFailure.current = undefined;
      telemetrySequence.current = 0;
      optionalTelemetryCount.current = 0;
      battleId.current = createDefenseUUID();
      setAnswersAndReset(initialAI);
      setHUD({
        ...INITIAL_HUD,
        totalWaves: stage.waves.length,
        lives: stage.lives,
        gold: stage.startingGold,
      });
      setLocalResult(undefined);
      setServerResult(undefined);
      setResultError("");
      setBattleKey((value) => value + 1);
      setPhase("battle");
    } finally {
      setStarting(false);
    }
  };

  const setAnswersAndReset = (initialAI: AIResources = { ...INITIAL_AI }) => {
    const empty = new Set<string>();
    answeredEventsRef.current = empty;
    activeEventRef.current = undefined;
    deferredAIDefeatRef.current = false;
    setAnsweredEvents(empty);
    setLearningBreakdown([]);
    setLearningScore(0);
    setAIResources(initialAI);
    setActiveEvent(undefined);
    setFeedback(undefined);
    setEventPaused(false);
  };

  const answerEvent = async (answerId: string) => {
    if (!activeEvent || feedback?.sending || feedback?.score !== undefined)
      return;
    if (
      practice ||
      preview ||
      !recordingRef.current ||
      !onAuthoritativeRequest
    ) {
      answersRef.current.push({
        event_id: activeEvent.id,
        answer_id: answerId,
      });
      setFeedback({
        topic: activeEvent.topic,
        explanation: "연습 모드에서는 정답과 교육 점수를 판정하지 않습니다.",
      });
      return;
    }
    setFeedback({
      topic: activeEvent.topic,
      explanation: "서버 정책으로 판단을 확인하는 중입니다.",
      sending: true,
    });
    try {
      await flushTelemetry();
      const raw = (await onAuthoritativeRequest(
        `/api/v1/defense/${pack!.slug}/education/events/${encodeURIComponent(activeEvent.id)}/answer`,
        { answer_id: answerId },
      )) as {
        answer?: {
          correct?: boolean;
          topic?: string;
          score?: number;
          explanation?: string;
          effect?: {
            resource_delta?: number;
            trust_delta?: number;
            latency_headroom_delta?: number;
          };
        };
      };
      const answer =
        raw?.answer ??
        (raw as unknown as {
          correct?: boolean;
          topic?: string;
          score?: number;
          explanation?: string;
          effect?: {
            resource_delta?: number;
            trust_delta?: number;
            latency_headroom_delta?: number;
          };
        });
      if (
        typeof answer.correct !== "boolean" ||
        !Number.isFinite(Number(answer.score))
      )
        throw new Error("서버의 교육 판정 응답이 올바르지 않습니다.");
      const topic = answer.topic || activeEvent.topic;
      const prior = learningRef.current.get(topic) ?? {
        correct: 0,
        total: 0,
        score: 0,
      };
      const next = {
        correct: prior.correct + (answer.correct ? 1 : 0),
        total: prior.total + 1,
        score: Math.round(
          (prior.score * prior.total + Number(answer.score)) /
            (prior.total + 1),
        ),
      };
      learningRef.current.set(topic, next);
      currentBreakdown();
      answersRef.current.push({
        event_id: activeEvent.id,
        answer_id: answerId,
      });
      setAnsweredEvents((current) => {
        const next = new Set(current).add(activeEvent.id);
        answeredEventsRef.current = next;
        return next;
      });
      const resourceDelta = Number(answer.effect?.resource_delta ?? 0);
      controller.current?.command({ type: "adjust-economy", resourceDelta });
      let educationAIState: AIResources | undefined;
      if (slug === "ai-nexus-defense") {
        const current = aiRef.current;
        const updated = applyAIResourceDeltas(pack!, current, {
          trust: Number(answer.effect?.trust_delta ?? 0),
          latency: Number(answer.effect?.latency_headroom_delta ?? 0),
        });
        aiRef.current = updated;
        educationAIState = updated;
        setAIResources(updated);
      }
      const applyPending = queueTelemetry("defense.education.apply", {
        event_id: activeEvent.id,
        resource_delta: resourceDelta,
        trust_delta: Number(answer.effect?.trust_delta ?? 0),
        latency_headroom_delta: Number(
          answer.effect?.latency_headroom_delta ?? 0,
        ),
        ...(educationAIState
          ? { resource_state: buildAIResourceState(pack!, educationAIState) }
          : {}),
      });
      if (educationAIState && isAIResourceDepleted(educationAIState))
        deferredAIDefeatRef.current = true;
      await applyPending;
      setFeedback({
        correct: answer.correct,
        topic,
        score: Number(answer.score),
        explanation:
          answer.explanation ||
          (answer.correct
            ? "정책에 맞는 안전한 판단입니다."
            : "정책 기준을 다시 확인해 보세요."),
      });
    } catch (cause) {
      setFeedback({
        topic: activeEvent.topic,
        explanation: "",
        error:
          cause instanceof Error
            ? cause.message
            : "교육 답안을 저장하지 못했습니다.",
      });
    }
  };

  const continueBattle = () => {
    if (
      !activeEvent ||
      (!practice &&
        !preview &&
        recordingRef.current &&
        feedback?.score === undefined)
    )
      return;
    const shouldDefeat =
      slug === "ai-nexus-defense" &&
      deferredAIDefeatRef.current &&
      isAIResourceDepleted(aiRef.current);
    activeEventRef.current = undefined;
    deferredAIDefeatRef.current = false;
    setActiveEvent(undefined);
    setFeedback(undefined);
    setEventPaused(false);
    if (shouldDefeat) {
      controller.current?.command({ type: "force-defeat" });
    } else if (eventPaused && controller.current) {
      controller.current.command({ type: "toggle-pause" });
    }
  };

  if (!slug)
    return (
      <Alert severity="error">지원하지 않는 Defense Series 게임입니다.</Alert>
    );
  if (resource.loading && !practicePack)
    return (
      <Stack
        minHeight={560}
        alignItems="center"
        justifyContent="center"
        spacing={2}
      >
        <CircularProgress />
        <Typography>게시된 Defense 콘텐츠를 준비하는 중…</Typography>
      </Stack>
    );
  if ((resource.error || !resource.data) && !practicePack)
    return (
      <Container sx={{ py: 4 }}>
        <ErrorPanel
          error={
            resource.error ?? new Error("Defense 콘텐츠를 불러오지 못했습니다.")
          }
          retry={() => void resource.reload()}
        />
        <Alert severity="warning" sx={{ mt: 2 }}>
          내장 연습은 서버 세션·교육 판정·결과·진행도·랭킹을 사용하지 않습니다.
        </Alert>
        <Button
          sx={{ mt: 2 }}
          variant="outlined"
          onClick={() => {
            setPractice(true);
            setPracticePack(DEFENSE_PACKS[slug]);
          }}
        >
          내장 연습 모드 시작
        </Button>
      </Container>
    );
  if (!pack || !progress) return null;

  if (phase === "select")
    return (
      <GameSurface>
      <Box
        data-testid="defense-game-shell"
        sx={{ width: "100%", maxWidth: 1280, mx: "auto", p: { xs: 1, md: 2 } }}
      >
        <Paper
          sx={{
            minHeight: 650,
            overflow: "auto",
            p: { xs: 2, md: 3 },
            background: `radial-gradient(circle at 80% 5%,${pack.presentation.primary}33,transparent 35%),linear-gradient(145deg,#0a2230,#101627 60%,#241b38)`,
          }}
        >
          <Stack
            direction={{ xs: "column", md: "row" }}
            justifyContent="space-between"
            gap={2}
          >
            <Box>
              <Typography
                variant="overline"
                color="primary.main"
                fontWeight={900}
              >
                {pack.presentation.eyebrow}
              </Typography>
              <Typography
                component="h1"
                variant="h1"
                sx={{ fontSize: { xs: "2.2rem", md: "3.4rem" } }}
              >
                {pack.presentation.name}
              </Typography>
              <Typography
                color="text.secondary"
                sx={{ mt: 1, maxWidth: 760, fontSize: "1.05rem" }}
              >
                {pack.presentation.story}
              </Typography>
            </Box>
            <Stack
              direction="row"
              flexWrap="wrap"
              useFlexGap
              spacing={1}
              alignContent="flex-start"
            >
              <Chip
                label={resource.data?.label ?? `v${pack.config.contentVersion}`}
                color="primary"
                variant="outlined"
              />
              <Chip
                icon={<ShieldRounded />}
                label={`${progress.summary.completed_stages}/${pack.config.stages.length} 완료`}
              />
              <Chip
                label={`${progress.summary.total_stars} ★`}
                color="warning"
              />
            </Stack>
          </Stack>
          {(practice || preview) && (
            <Alert severity="warning" sx={{ mt: 2 }}>
              {preview
                ? `Content Studio 미리보기 · ${preview.label}`
                : "내장 연습 모드"}{" "}
              · 세션, 교육 판정, 결과, 진행도와 랭킹을 저장하지 않습니다.
            </Alert>
          )}
          <Grid container spacing={2.5} mt={0.5}>
            <Grid size={{ xs: 12, lg: 8 }}>
              <Typography variant="h3" mt={2} mb={1}>
                캠페인
              </Typography>
              <Grid data-testid="defense-stage-select" container spacing={1}>
                {pack.config.stages.map((item) => {
                  const saved = progress.items.find(
                    (value) =>
                      value.stage_id === item.id &&
                      value.difficulty === difficulty,
                  );
                  const unlocked =
                    practice ||
                    preview ||
                    item.number === 1 ||
                    Boolean(saved?.unlocked);
                  return (
                    <Grid key={item.id} size={{ xs: 12, sm: 6, md: 4 }}>
                      <Card
                        data-testid={`defense-stage-${item.id}`}
                        data-unlocked={unlocked}
                        variant="outlined"
                        sx={{
                          height: "100%",
                          borderColor:
                            item.id === stageId
                              ? pack.presentation.primary
                              : undefined,
                        }}
                      >
                        <CardActionArea
                          disabled={!unlocked}
                          onClick={() => setStageId(item.id)}
                          sx={{
                            minHeight: 224,
                            p: 1.4,
                            opacity: unlocked ? 1 : 0.45,
                          }}
                        >
                          <StageMapPreview
                            stage={item}
                            style={{ height: 104, marginBottom: 10 }}
                          />
                          <Stack direction="row" justifyContent="space-between" alignItems="center">
                            <Typography variant="body2" color="primary.main">
                              STAGE {item.number}
                            </Typography>
                            <Chip
                              size="small"
                              label={`${item.paths?.length ?? 1} LANE`}
                              variant="outlined"
                            />
                          </Stack>
                          <Typography fontWeight={850} mt={0.5}>
                            {item.name}
                          </Typography>
                          <Typography
                            variant="body2"
                            color="text.secondary"
                            mt={0.5}
                          >
                            {saved?.completed
                              ? `${saved.best_score.toLocaleString()}점`
                              : unlocked
                                ? `${item.waves.length} waves`
                                : "잠김"}
                          </Typography>
                        </CardActionArea>
                      </Card>
                    </Grid>
                  );
                })}
              </Grid>
              {stage && (
                <StageRoster
                  stage={stage}
                  enemies={pack.config.enemies}
                  game={slug}
                  noun={pack.presentation.enemyName}
                />
              )}
              {!practice && !preview && <DefenseLeaderboard slug={slug} />}
            </Grid>
            <Grid size={{ xs: 12, lg: 4 }}>
              <Stack spacing={2} mt={{ lg: 2 }}>
                <Box>
                  <Typography variant="h3">난이도</Typography>
                  <TextField
                    select
                    fullWidth
                    value={difficulty}
                    onChange={(event) =>
                      setDifficulty(event.target.value as RealmDifficulty)
                    }
                    sx={{ mt: 1 }}
                  >
                    {(Object.keys(difficultyLabels) as RealmDifficulty[]).map(
                      (value) => (
                        <MenuItem key={value} value={value}>
                          {difficultyLabels[value]}
                        </MenuItem>
                      ),
                    )}
                  </TextField>
                </Box>
                <Box>
                  <Typography variant="h3">
                    {pack.presentation.heroName}
                  </Typography>
                  <Stack spacing={1} mt={1}>
                    {pack.config.heroes.map((item) => {
                      const unlocked = isDefenseHeroUnlocked(item, stage);
                      return (
                        <HeroSelectCard
                          key={item.id}
                          hero={item}
                          game={slug}
                          selected={heroId === item.id}
                          unlocked={unlocked}
                          level={1}
                          unlockLabel={`Stage ${item.unlockStage ?? 1} 해금`}
                          onSelect={setHeroId}
                          testId={`defense-hero-${item.id}`}
                        />
                      );
                    })}
                  </Stack>
                </Box>
                {slug === "ai-nexus-defense" && <ModelProfiles pack={pack} />}
                {pack.educationEnabled && (
                  <LearningPanel
                    breakdown={resource.data?.learning?.topics ?? []}
                    score={resource.data?.learning?.overall_score ?? 0}
                  />
                )}
                <Button
                  data-testid="defense-start"
                  variant="contained"
                  size="large"
                  disabled={
                    starting ||
                    !stage ||
                    !hero ||
                    !isDefenseHeroUnlocked(hero, stage)
                  }
                  onClick={() => void startBattle()}
                  startIcon={
                    starting ? (
                      <CircularProgress size={20} />
                    ) : (
                      <ShieldRounded />
                    )
                  }
                  sx={{ minHeight: 58, fontSize: "1.05rem" }}
                >
                  방어 시작
                </Button>
              </Stack>
            </Grid>
          </Grid>
        </Paper>
      </Box>
      </GameSurface>
    );

  const command = (value: RealmCommand) => controller.current?.command(value);
  const buildOptions = pack.config.towers.reduce<
    Array<{
      tower: (typeof pack.config.towers)[number];
      profile?: AIModelProfile;
    }>
  >((options, tower) => {
    const profiles =
      pack.modelProfiles?.filter((profile) => profile.tower_id === tower.id) ??
      [];
    options.push(
      ...(profiles.length
        ? profiles.map((profile) => ({ tower, profile }))
        : [{ tower }]),
    );
    return options;
  }, []);
  return (
    <GameSurface>
    <Box
      ref={battleRef}
      data-testid="defense-game-shell"
      data-battle-status={hud.status}
      data-ai-depleted={
        slug === "ai-nexus-defense" && isAIResourceDepleted(aiResources)
      }
      data-education-open={Boolean(activeEvent)}
      data-event-paused={eventPaused}
      sx={{
        width: "100%",
        maxWidth: 1280,
        mx: "auto",
        position: "relative",
        aspectRatio: "16 / 9",
        minHeight: { xs: 620, md: 0 },
        bgcolor: "#07101d",
        overflow: "hidden",
        scrollMarginTop: `${BATTLE_SCROLL_MARGIN}px`,
      }}
    >
      <Box
        ref={canvasRef}
        data-testid="defense-canvas"
        aria-label={`${pack.presentation.name} 전장`}
        sx={{
          position: "absolute",
          inset: 0,
          touchAction: "none",
          "& canvas": {
            display: "block",
            width: "100%",
            height: "100%",
            objectFit: "contain",
            touchAction: "none",
          },
        }}
      />
      {phase === "battle" && (
        <>
          <Stack
            data-testid="defense-battle-hud"
            direction="row"
            spacing={1}
            sx={{
              position: "absolute",
              top: 10,
              left: 10,
              right: 10,
              zIndex: 3,
              pointerEvents: "auto",
              overflowX: "auto",
              overflowY: "hidden",
              touchAction: "pan-x",
              pb: 0.5,
              scrollbarWidth: "thin",
              "& .MuiChip-root, & .MuiButton-root": { flexShrink: 0 },
            }}
          >
            <Chip
              icon={<HealthAndSafetyRounded />}
              label={`${pack.presentation.healthName} ${hud.lives}`}
              color={hud.lives <= 5 ? "error" : "default"}
            />
            <Chip label={`${pack.presentation.resourceName} ${hud.gold}`} />
            <Chip label={`웨이브 ${hud.wave}/${hud.totalWaves}`} />
            <Chip label={`${hud.kills} 차단`} />
            <Chip
              data-testid="defense-battle-status"
              label={`상태 ${optionLabel(hud.status)}`}
            />
            <Box flex={1} />
            <Button
              title={
                hud.heroAlive
                  ? "클릭한 뒤 전장에서 이동 지점을 선택하세요."
                  : `${hud.heroRespawn}초 후 복귀`
              }
              disabled={!hud.heroAlive}
              variant="contained"
              color="secondary"
              onClick={() => command({ type: "move-hero" })}
              startIcon={<PsychologyRounded />}
              sx={{ minWidth: 190 }}
            >
              {hero?.name ?? pack.presentation.heroName} Lv.{hud.heroLevel} ·{" "}
              {hud.heroHp}/{hud.heroMaxHp} HP
            </Button>
            <Button
              variant="contained"
              onClick={() =>
                command({ type: "speed", value: hud.speed === 1 ? 2 : 1 })
              }
              startIcon={<FastForwardRounded />}
            >
              {hud.speed}×
            </Button>
            <Button
              variant="contained"
              onClick={() => command({ type: "toggle-pause" })}
              startIcon={
                hud.status === "paused" ? (
                  <PlayArrowRounded />
                ) : (
                  <PauseRounded />
                )
              }
            >
              {hud.status === "paused" ? "계속" : "일시정지"}
            </Button>
          </Stack>
          {slug === "ai-nexus-defense" && (
            <ResourceHUD pack={pack} values={aiResources} />
          )}
          <Stack
            data-testid="defense-skill-stack"
            spacing={1}
            sx={{
              position: "absolute",
              right: 10,
              top:
                slug === "ai-nexus-defense" ? { xs: 280, sm: 68 } : 68,
              zIndex: 3,
            }}
          >
            {pack.config.skills.map((skill) => (
              <Button
                key={skill.id}
                disabled={(hud.skillCooldowns[skill.id] ?? 0) > 0}
                variant="contained"
                onClick={() => command({ type: "skill", skill: skill.id })}
                startIcon={
                  skill.id === "reinforcement" ? (
                    <PsychologyRounded />
                  ) : (
                    <BoltRounded />
                  )
                }
                sx={{
                  minWidth: 142,
                  bgcolor: skill.color,
                  color: "#07101d",
                  fontWeight: 900,
                }}
              >
                {hud.skillCooldowns[skill.id]
                  ? `${hud.skillCooldowns[skill.id]}초`
                  : skill.name}
              </Button>
            ))}
          </Stack>
          <Paper
            data-testid="defense-command-panel"
            sx={{
              position: "absolute",
              left: 10,
              right: 10,
              bottom: 10,
              zIndex: 3,
              p: 1.2,
              bgcolor: "rgba(7,16,29,.94)",
            }}
          >
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              className="admin-scrollbar"
              sx={{ overflowX: "auto" }}
            >
              <Button
                variant="contained"
                color="warning"
                onClick={() => command({ type: "start-wave" })}
                startIcon={<FlagRounded />}
                sx={{ minWidth: 170 }}
              >
                다음 웨이브 {hud.nextWaveIn ? `(${hud.nextWaveIn}초)` : ""}
              </Button>
              <Divider orientation="vertical" flexItem />
              {hud.selectedSpot ? (
                hud.selectedTower ? (
                  <>
                    <Typography sx={{ minWidth: 120 }} fontWeight={850}>
                      {pack.modelProfiles?.find(
                        (profile) => profile.id === hud.selectedTower?.profile,
                      )?.name ??
                        pack.config.towers.find(
                          (tower) => tower.id === hud.selectedTower?.type,
                        )?.name}{" "}
                      L{hud.selectedTower.level}
                    </Typography>
                    {hud.selectedTower.level < 2 && (
                      <Button
                        variant="outlined"
                        onClick={() => command({ type: "upgrade" })}
                      >
                        강화
                      </Button>
                    )}
                    {hud.selectedTower.level === 2 &&
                      pack.config.towers
                        .find((tower) => tower.id === hud.selectedTower?.type)
                        ?.branches.map((branch) => (
                          <Button
                            key={branch.id}
                            variant="outlined"
                            onClick={() =>
                              command({ type: "upgrade", branch: branch.id })
                            }
                          >
                            {branch.name}
                          </Button>
                        ))}
                    <Button
                      color="error"
                      onClick={() => command({ type: "sell" })}
                    >
                      판매
                    </Button>
                  </>
                ) : (
                  buildOptions.map(({ tower, profile }) => (
                    <Button
                      key={`${tower.id}-${profile?.id ?? "base"}`}
                      data-testid={
                        profile ? `defense-profile-${profile.id}` : undefined
                      }
                      title={`효과적 대응: ${tower.effectiveAgainst?.join(", ") || "범용"}${profile ? ` · 정확도 ${profile.accuracy}% · 피해 ×${profile.damage_multiplier}` : ""}`}
                      variant="contained"
                      disabled={
                        hud.gold < tower.cost ||
                        Boolean(
                          profile &&
                          (aiResources.compute < profile.compute_cost ||
                            aiResources.token < profile.token_cost ||
                            aiResources.latency < profile.latency_cost),
                        )
                      }
                      onClick={() =>
                        command({
                          type: "build",
                          tower: tower.id,
                          profile: profile?.id,
                        })
                      }
                      sx={{ minWidth: profile ? 190 : 150 }}
                    >
                      {profile
                        ? `${profile.name} · C${profile.compute_cost}/T${profile.token_cost}/L${profile.latency_cost}`
                        : `${tower.name} ${tower.cost} · ${tower.effectiveAgainst?.join("/")}`}
                    </Button>
                  ))
                )
              ) : (
                <Typography color="text.secondary">
                  건설 지점을 선택해 {pack.presentation.towerName}을 배치하세요.
                </Typography>
              )}
            </Stack>
          </Paper>
          {!recording && !practice && !preview && (
            <Alert
              severity="warning"
              sx={{
                position: "absolute",
                left: 10,
                top:
                  slug === "ai-nexus-defense"
                    ? { xs: 430, sm: 292 }
                    : 68,
                zIndex: 3,
              }}
            >
              세션이 없어 연습 결과로 처리됩니다.
            </Alert>
          )}
        </>
      )}
      {activeEvent && (
        <Box
          data-testid="defense-choice-event"
          sx={{
            position: "absolute",
            inset: 0,
            zIndex: 10,
            bgcolor: "rgba(3,8,17,.86)",
            display: "grid",
            placeItems: "center",
            p: 2,
          }}
        >
          <Card
            sx={{
              width: "min(680px,96%)",
              border: 1,
              borderColor: "secondary.main",
            }}
          >
            <CardContent sx={{ p: { xs: 2.5, md: 4 } }}>
              <Stack spacing={2}>
                <Stack direction="row" spacing={1} alignItems="center">
                  <AutoAwesomeRounded color="secondary" />
                  <Typography
                    variant="overline"
                    color="secondary.main"
                    fontWeight={900}
                  >
                    {activeEvent.topic} · HUMAN IN THE LOOP
                  </Typography>
                </Stack>
                <Typography
                  variant="h2"
                  sx={{ fontSize: { xs: "1.5rem", md: "2rem" } }}
                >
                  {activeEvent.question}
                </Typography>
                {activeEvent.answers.map((answer) => (
                  <Button
                    data-testid="defense-answer"
                    data-answer-id={answer.id}
                    key={answer.id}
                    variant="outlined"
                    disabled={Boolean(
                      feedback?.sending ||
                      feedback?.score !== undefined ||
                      (feedback && recordingRef.current && !feedback.error),
                    )}
                    onClick={() => void answerEvent(answer.id)}
                    sx={{
                      minHeight: 54,
                      justifyContent: "flex-start",
                      textAlign: "left",
                      fontSize: "1rem",
                    }}
                  >
                    {answer.text}
                  </Button>
                ))}
                {feedback && (
                  <Alert
                    data-testid="defense-choice-feedback"
                    severity={
                      feedback.error
                        ? "error"
                        : feedback.correct === false
                          ? "warning"
                          : feedback.correct
                            ? "success"
                            : "info"
                    }
                  >
                    {feedback.error || feedback.explanation}
                    {feedback.score !== undefined && ` · ${feedback.score}점`}
                    {feedback.error && (
                      <Button
                        size="small"
                        onClick={() => setFeedback(undefined)}
                      >
                        다시 시도
                      </Button>
                    )}
                  </Alert>
                )}
                <Button
                  data-testid="defense-choice-continue"
                  variant="contained"
                  disabled={
                    !feedback ||
                    Boolean(feedback.sending || feedback.error) ||
                    (recordingRef.current && feedback.score === undefined)
                  }
                  onClick={continueBattle}
                >
                  전투 계속
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Box>
      )}
      {phase === "result" && (
        <Box
          data-testid="defense-result"
          sx={{
            position: "absolute",
            inset: 0,
            zIndex: 9,
            bgcolor: "rgba(3,8,17,.9)",
            display: "grid",
            placeItems: "center",
            p: 2,
            overflowY: "auto",
          }}
        >
          <Card sx={{ width: "min(760px,96%)" }}>
            <CardContent sx={{ p: { xs: 2.5, md: 4 } }}>
              <Stack spacing={2}>
                {submitting ? (
                  <>
                    <CircularProgress sx={{ alignSelf: "center" }} />
                    <Typography variant="h3" textAlign="center">
                      서버에서 전투와 교육 결과를 검증하는 중…
                    </Typography>
                  </>
                ) : (
                  <>
                    <Typography variant="h2" textAlign="center">
                      {localResult?.victory ? "방어 성공" : "방어선 붕괴"}
                    </Typography>
                    {serverResult && (
                      <>
                        <Typography
                          textAlign="center"
                          sx={{
                            fontSize: "3rem",
                            fontWeight: 950,
                            color: "warning.main",
                          }}
                        >
                          {serverResult.result.score.toLocaleString()}
                        </Typography>
                        <Typography
                          textAlign="center"
                          aria-label={`${serverResult.result.stars}개 별`}
                          sx={{ fontSize: "2rem", color: "warning.main" }}
                        >
                          {"★".repeat(serverResult.result.stars)}
                          {"☆".repeat(3 - serverResult.result.stars)}
                        </Typography>
                        <Alert
                          severity={
                            serverResult.result.verified ? "success" : "warning"
                          }
                        >
                          {serverResult.result.verified
                            ? "서버 검증을 완료해 진행도와 전용 랭킹에 반영했습니다."
                            : "연습 결과이며 서버에 저장하지 않았습니다."}
                        </Alert>
                        {pack.educationEnabled && (
                          <LearningPanel
                            breakdown={learningBreakdown}
                            score={learningScore}
                          />
                        )}
                        {slug === "ai-nexus-defense" && (
                          <Grid container spacing={1}>
                            {Object.entries(aiResources).map(([key, value]) => (
                              <Grid key={key} size={3}>
                                <Paper
                                  variant="outlined"
                                  sx={{ p: 1, textAlign: "center" }}
                                >
                                  <Typography
                                    variant="body2"
                                    color="text.secondary"
                                  >
                                    {key}
                                  </Typography>
                                  <Typography fontWeight={900}>
                                    {value}
                                  </Typography>
                                </Paper>
                              </Grid>
                            ))}
                          </Grid>
                        )}
                      </>
                    )}
                    {resultError && (
                      <Alert severity="error">
                        {resultError}
                        {localResult && (
                          <Button
                            size="small"
                            onClick={() => void submitResult(localResult)}
                          >
                            다시 제출
                          </Button>
                        )}
                      </Alert>
                    )}
                    <Stack direction="row" justifyContent="center" spacing={1}>
                      <Button
                        startIcon={<RestartAltRounded />}
                        variant="outlined"
                        onClick={() => void startBattle()}
                      >
                        다시 도전
                      </Button>
                      <Button
                        variant="contained"
                        onClick={() => {
                          setPhase("select");
                          setAnswersAndReset();
                        }}
                      >
                        스테이지 선택
                      </Button>
                    </Stack>
                  </>
                )}
              </Stack>
            </CardContent>
          </Card>
        </Box>
      )}
    </Box>
    </GameSurface>
  );
}

export default DefenseSeriesGame;
