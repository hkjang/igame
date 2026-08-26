import { useEffect, useMemo, useState } from "react";
import AddRounded from "@mui/icons-material/AddRounded";
import BugReportRounded from "@mui/icons-material/BugReportRounded";
import CheckCircleRounded from "@mui/icons-material/CheckCircleRounded";
import PreviewRounded from "@mui/icons-material/PreviewRounded";
import PublishRounded from "@mui/icons-material/PublishRounded";
import SaveRounded from "@mui/icons-material/SaveRounded";
import ScienceRounded from "@mui/icons-material/ScienceRounded";
import SchoolRounded from "@mui/icons-material/SchoolRounded";
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
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  Grid,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";
import { Link as RouterLink, useSearchParams } from "react-router-dom";
import { ErrorPanel } from "../../../components/ErrorPanel";
import { UnsavedChangesDialog } from "../../../components/UnsavedChangesDialog";
import { useUnsavedGuard } from "../../../hooks/useUnsavedGuard";
import {
  DEFENSE_PACKS,
  DEFENSE_SLUGS,
  isDefenseSlug,
} from "../../../games/defense/content";
import {
  defenseStudioAPI,
  reviewDefenseVersion,
} from "../../../games/defense/api";
import type {
  DefenseSection,
  DefenseSlug,
  DefenseVersion,
} from "../../../games/defense/types";
import { useAsync } from "../../../hooks/useAsync";
import { useAuth } from "../../../state/AuthContext";
import { useSnackbar } from "../../../state/SnackbarContext";
import { JsonSubField } from "../JsonSubField";
import {
  defenseReportMetrics,
  formatDefenseReportMetric,
} from "./report";
import {
  defenseVersionsForSlug,
  mergeCreatedDefenseVersion,
  normalizeDefenseStudioView,
  type DefenseStudioView,
} from "./navigation";

const sections: Array<{ id: DefenseSection; label: string }> = [
  { id: "stages", label: "Stage" },
  { id: "waves", label: "Wave" },
  { id: "towers", label: "Tower" },
  { id: "enemies", label: "Enemy" },
  { id: "bosses", label: "Boss" },
  { id: "heroes", label: "Hero / Agent" },
  { id: "skills", label: "Skill" },
  { id: "events", label: "Event" },
  { id: "education", label: "Quiz / Education" },
  { id: "balance", label: "Balance" },
  { id: "campaigns", label: "Campaign" },
  { id: "resource_rules", label: "AI Resource Rules" },
  { id: "model_profiles", label: "AI Model Profiles" },
];
const editableStatuses = new Set<DefenseVersion["status"]>([
  "draft",
  "testing",
]);

function parseJSON(value: string) {
  try {
    return { value: JSON.parse(value) as unknown, error: "" };
  } catch (cause) {
    return {
      value: undefined,
      error:
        cause instanceof Error
          ? cause.message
          : "JSON 형식이 올바르지 않습니다.",
    };
  }
}

function ReportDashboard({
  data,
  education,
}: {
  data: Record<string, unknown> | undefined;
  education: boolean;
}) {
  if (!data) return <CircularProgress sx={{ mt: 3 }} />;
  const summary =
    data.summary && typeof data.summary === "object"
      ? (data.summary as Record<string, unknown>)
      : data;
  const metrics = defenseReportMetrics(education);
  const list = (key: string) =>
    Array.isArray(data[key])
      ? (data[key] as Array<Record<string, unknown>>)
      : [];
  const rows = [
    ...list("weak_topics"),
    ...list("topics"),
    ...list("questions"),
    ...list("departments"),
  ].slice(0, 18);
  const metricValue = (key: string) => {
    const aliases: Record<string, string[]> = {
      participants: ["participants", "unique_users"],
      plays: ["plays", "runs"],
      average_score: ["average_score", "avg_score"],
      average_game_score: ["average_game_score", "avg_game_score"],
      average_play_time_ms: [
        "average_play_time_ms",
        "average_duration_ms",
        "avg_play_time_ms",
      ],
    };
    for (const candidate of aliases[key] ?? [key]) {
      if (summary[candidate] !== undefined) return summary[candidate];
    }
    if (key === "department_count" && Array.isArray(data.departments))
      return data.departments.length;
    if (key === "average_score" && Array.isArray(data.topics)) {
      const scores = data.topics
        .map((item) => Number((item as Record<string, unknown>).score))
        .filter(Number.isFinite);
      return scores.length
        ? scores.reduce((sum, value) => sum + value, 0) / scores.length
        : 0;
    }
    return 0;
  };
  return (
    <Box data-testid="defense-report-dashboard" mt={2}>
      <Grid container spacing={1.2}>
        {metrics.map(([label, key]) => (
          <Grid key={key} size={{ xs: 6, md: 4, xl: 2 }}>
            <Card variant="outlined">
              <CardContent sx={{ p: 1.5 }}>
                <Typography variant="body2" color="text.secondary">
                  {label}
                </Typography>
                <Typography variant="h3" mt={0.5}>
                  {formatDefenseReportMetric(key, metricValue(key))}
                </Typography>
              </CardContent>
            </Card>
          </Grid>
        ))}
      </Grid>
      {rows.length > 0 && (
        <Box className="admin-scrollbar" sx={{ mt: 2, overflowX: "auto" }}>
          <Box
            component="table"
            sx={{
              width: "100%",
              borderCollapse: "collapse",
              "& th,& td": {
                textAlign: "left",
                p: 1.1,
                borderBottom: 1,
                borderColor: "divider",
              },
            }}
          >
            <thead>
              <tr>
                <th>구분</th>
                <th>주제 / 조직 / 문항</th>
                <th>정답·완료</th>
                <th>점수·개선</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row, index) => (
                <tr
                  key={`${row.topic ?? row.department ?? row.question_id ?? index}`}
                >
                  <td>
                    {String(
                      row.type ??
                        (row.department
                          ? "부서"
                          : row.question_id
                            ? "문항"
                            : "주제"),
                    )}
                  </td>
                  <td>
                    {String(
                      row.topic ??
                        row.department ??
                        row.question ??
                        row.question_id ??
                        "-",
                    )}
                  </td>
                  <td>
                    {String(
                      row.correct ?? row.completed ?? row.participants ?? 0,
                    )}{" "}
                    / {String(row.total ?? row.attempts ?? 0)}
                  </td>
                  <td>
                    {String(row.score ?? row.accuracy ?? row.improvement ?? 0)}
                  </td>
                </tr>
              ))}
            </tbody>
          </Box>
        </Box>
      )}
      <Box component="details" sx={{ mt: 2 }}>
        <Box
          component="summary"
          sx={{ cursor: "pointer", color: "text.secondary", fontWeight: 750 }}
        >
          진단용 원본 JSON
        </Box>
        <Box
          component="pre"
          className="admin-scrollbar"
          tabIndex={0}
          sx={{
            mt: 1,
            p: 2,
            maxHeight: 360,
            overflow: "auto",
            bgcolor: "surface.code",
            borderRadius: 2,
            fontSize: ".9rem",
            whiteSpace: "pre-wrap",
          }}
        >
          {JSON.stringify(data, null, 2)}
        </Box>
      </Box>
    </Box>
  );
}

function DefenseQuickEditor({
  data,
  onChange,
}: {
  data: unknown;
  onChange: (value: unknown) => void;
}) {
  const [index, setIndex] = useState(0);
  const list = Array.isArray(data) ? data : undefined;
  const current = (list?.[Math.min(index, Math.max(0, list.length - 1))] ??
    data) as Record<string, unknown> | undefined;
  if (!current || typeof current !== "object")
    return (
      <Alert severity="info" sx={{ mt: 2 }}>
        편집할 구조화 데이터가 없습니다.
      </Alert>
    );
  const scalarKeys = Object.keys(current).filter((key) =>
    [
      "id",
      "name",
      "label",
      "question",
      "topic",
      "trigger",
      "stage_id",
      "education_id",
      "role",
      "mode",
      "theme",
      "version",
      "starting_health",
      "starting_resource",
      "reward",
      "cost",
      "damage",
      "range",
      "fire_rate",
      "projectile_speed",
      "effective_multiplier",
      "hp",
      "speed",
      "armor",
      "health_damage",
      "threat_type",
      "cooldown",
      "compute_start",
      "token_start",
      "trust_start",
      "latency_max",
      "wave_compute_cost",
      "wave_token_cost",
      "escaped_trust_cost",
      "escaped_latency_cost",
      "compute_cost",
      "token_cost",
      "latency_cost",
      "accuracy",
      "damage_multiplier",
    ].includes(key),
  );
  const structuredKeys = Object.keys(current).filter(
    (key) =>
      [
        "answers",
        "entries",
        "branches",
        "effective_against",
        "resource_effect",
        "tower_spots",
        "path",
        "paths",
        "reward",
        "penalty",
        "effects",
        "stage_ids",
      ].includes(key) && typeof current[key] === "object",
  );
  const update = (key: string, value: unknown) => {
    const next = { ...current, [key]: value };
    if (list) {
      const copy = [...list];
      copy[Math.min(index, copy.length - 1)] = next;
      onChange(copy);
    } else onChange(next);
  };
  return (
    <Box data-testid="defense-quick-editor" mt={2}>
      <Divider sx={{ mb: 2 }} />
      <Typography variant="h3">구조화 빠른 편집</Typography>
      {list && (
        <TextField
          select
          fullWidth
          label="항목"
          value={Math.min(index, Math.max(0, list.length - 1))}
          onChange={(event) => setIndex(Number(event.target.value))}
          sx={{ mt: 1.5 }}
        >
          {list.map((value, itemIndex) => {
            const item = value as Record<string, unknown>;
            return (
              <MenuItem key={String(item.id ?? itemIndex)} value={itemIndex}>
                {String(
                  item.name ??
                    item.question ??
                    item.id ??
                    `항목 ${itemIndex + 1}`,
                )}
              </MenuItem>
            );
          })}
        </TextField>
      )}
      <Stack spacing={1.2} mt={1.5}>
        {scalarKeys.map((key) => (
          <TextField
            key={key}
            label={key}
            value={String(current[key] ?? "")}
            multiline={key === "question"}
            onChange={(event) =>
              update(
                key,
                typeof current[key] === "number"
                  ? Number(event.target.value)
                  : event.target.value,
              )
            }
          />
        ))}
        {structuredKeys.map((key) => (
          <JsonSubField
            key={key}
            resetKey={`${index}:${key}`}
            initial={current[key] ?? []}
            label={`${key} JSON`}
            helperText={`${Array.isArray(current[key]) ? current[key].length : 0}개 · JSON으로 편집`}
            onChange={(value) => update(key, value)}
          />
        ))}
      </Stack>
      <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1} mt={2}>
        <Chip
          label={
            list
              ? `${list.length}개 항목`
              : `${Object.keys(current).length}개 설정`
          }
        />
        <Chip label={`${scalarKeys.length}개 주요 필드`} color="primary" />
        <Chip
          label={`${structuredKeys.length}개 구조 필드`}
          color="secondary"
        />
      </Stack>
    </Box>
  );
}

export function DefenseContentStudioPage() {
  const { user, config } = useAuth();
  const { notify } = useSnackbar();
  const [params, setParams] = useSearchParams();
  const initialSlug = params.get("game") ?? "";
  const initialSection = params.get("section") ?? "";
  const [slug, setSlug] = useState<DefenseSlug>(
    isDefenseSlug(initialSlug) ? initialSlug : "office-guardians",
  );
  const [section, setSection] = useState<DefenseSection>(
    sections.some((item) => item.id === initialSection)
      ? (initialSection as DefenseSection)
      : "stages",
  );
  const [versionId, setVersionId] = useState(params.get("version") ?? "");
  const [view, setView] = useState<DefenseStudioView>(
    normalizeDefenseStudioView(params.get("view")),
  );
  const [editor, setEditor] = useState("[]");
  const [loadedEditor, setLoadedEditor] = useState("");
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [reviewComment, setReviewComment] = useState("");
  const [draft, setDraft] = useState({
    label: "",
    notes: "",
    asset_version: "",
    source_version_id: "",
    policy_version: "",
  });
  const isAdmin = [user?.role, ...(user?.roles ?? [])].includes("admin");
  const versions = useAsync(async () => {
    const response = await defenseStudioAPI.versions(slug);
    return { ...response, slug };
  }, [slug]);
  const versionItems = defenseVersionsForSlug(versions.data, slug);
  const hasActiveVersion = versionItems.some((item) => item.id === versionId);
  const sectionData = useAsync(
    () =>
      versionId && view === "editor" && hasActiveVersion
        ? defenseStudioAPI.section(slug, section, versionId)
        : Promise.resolve(undefined),
    [hasActiveVersion, section, slug, versionId, view],
  );
  const telemetry = useAsync(
    () =>
      view === "telemetry"
        ? defenseStudioAPI.telemetry(slug)
        : Promise.resolve(undefined),
    [slug, view],
  );
  const report = useAsync(
    () =>
      view === "report"
        ? defenseStudioAPI.learningReport(slug)
        : Promise.resolve(undefined),
    [slug, view],
  );
  const parsed = useMemo(() => parseJSON(editor), [editor]);
  const selected = versionItems.find((item) => item.id === versionId);
  const editorReady = Boolean(
    selected && sectionData.data?.version.id === selected.id,
  );
  const educationReportEnabled = Boolean(
    report.data?.education_enabled ??
    (Array.isArray(report.data?.topics) && report.data.topics.length > 0),
  );

  useEffect(() => {
    if (versions.data?.slug !== slug) {
      if (versionId) setVersionId("");
      return;
    }
    const items = versions.data.items;
    if (!items.length) {
      setVersionId("");
      return;
    }
    if (!items.some((item) => item.id === versionId))
      setVersionId(
        items.find((item) => editableStatuses.has(item.status))?.id ??
          items[0].id,
      );
  }, [slug, versionId, versions.data]);
  useEffect(() => {
    if (sectionData.data && sectionData.data.version.id === selected?.id) {
      const text = JSON.stringify(sectionData.data.data, null, 2);
      setEditor(text);
      setLoadedEditor(text);
    }
  }, [sectionData.data, selected?.id]);
  // Switching game, version or section reloads the editor, so an unsaved draft
  // has to be confirmed away rather than silently replaced.
  const dirty = Boolean(loadedEditor) && editor !== loadedEditor;
  const { guard, askingToDiscard, discard, keepEditing } = useUnsavedGuard(dirty);
  useEffect(() => {
    const next = new URLSearchParams({ game: slug, section });
    if (versionId) next.set("version", versionId);
    next.set("view", view);
    setParams(next, { replace: true });
  }, [section, setParams, slug, versionId, view]);
  useEffect(() => {
    if (
      slug !== "ai-nexus-defense" &&
      (section === "resource_rules" || section === "model_profiles")
    )
      setSection("stages");
  }, [section, slug]);

  const mutate = async (label: string, action: () => Promise<unknown>) => {
    setSaving(true);
    try {
      await action();
      notify(label, "success");
      await Promise.all([versions.reload(), sectionData.reload()]);
    } catch (cause) {
      notify(
        cause instanceof Error ? cause.message : "요청을 처리하지 못했습니다.",
        "error",
      );
    } finally {
      setSaving(false);
    }
  };
  const save = () =>
    mutate(`${section} Draft를 저장했습니다.`, async () => {
      if (!versionId || parsed.error || parsed.value === undefined)
        throw new Error("저장할 유효한 JSON이 없습니다.");
      const checksum = sectionData.data?.version.checksum;
      if (!checksum)
        throw new Error(
          "편집 버전 checksum이 없습니다. Draft를 새로고침해 주세요.",
        );
      await defenseStudioAPI.saveSection(
        slug,
        section,
        versionId,
        checksum,
        parsed.value,
      );
      setLoadedEditor(editor);
    });
  const create = () =>
    mutate("새 Defense Draft를 만들었습니다.", async () => {
      const response = await defenseStudioAPI.createVersion(slug, draft);
      versions.setData((current) => ({
        ...mergeCreatedDefenseVersion(
          current?.slug === slug ? current : undefined,
          response.version,
        ),
        slug,
      }));
      setVersionId(response.version.id);
      setCreateOpen(false);
      setDraft({
        label: "",
        notes: "",
        asset_version: "",
        source_version_id: "",
        policy_version: "",
      });
    });

  return (
    <Container data-testid="defense-studio" maxWidth="xl" sx={{ py: 4 }}>
      <Stack
        direction={{ xs: "column", lg: "row" }}
        justifyContent="space-between"
        gap={2}
      >
        <Box>
          <Typography
            component="h1"
            variant="h1"
            sx={{ fontSize: { xs: "2.1rem", md: "3rem" } }}
          >
            Defense Content Studio
          </Typography>
          <Typography
            color="text.secondary"
            mt={1}
            sx={{ fontSize: "1.05rem" }}
          >
            세 게임의 엔진 규칙은 공유하고 콘텐츠·교육 정책·밸런스는 독립된
            버전으로 검토하고 게시합니다.
          </Typography>
        </Box>
        <Stack direction={{ xs: "column", sm: "row" }} spacing={1}>
          <TextField
            data-testid="defense-game-select"
            select
            label="게임"
            value={slug}
            onChange={(event) => guard(() => {
              versions.setData(undefined);
              setSlug(event.target.value as DefenseSlug);
              setVersionId("");
            })}
            sx={{ minWidth: 220 }}
          >
            {DEFENSE_SLUGS.map((value) => (
              <MenuItem key={value} value={value}>
                {DEFENSE_PACKS[value].presentation.name}
              </MenuItem>
            ))}
          </TextField>
          <FormControl sx={{ minWidth: 230 }}>
            <InputLabel>편집 버전</InputLabel>
            <Select
              label="편집 버전"
              value={versionId}
              onChange={(event) => guard(() => setVersionId(event.target.value))}
            >
              {versionItems.map((item) => (
                <MenuItem key={item.id} value={item.id}>
                  {item.label} · {item.status}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Button
            variant="contained"
            startIcon={<AddRounded />}
            onClick={() => setCreateOpen(true)}
          >
            새 Draft
          </Button>
        </Stack>
      </Stack>
      <Alert severity="info" sx={{ mt: 2 }}>
        미리보기는 연습 전용이며 세션·답안·결과·진행도·랭킹을 저장하지 않습니다.
        게시된 콘텐츠만 기록 모드에서 사용됩니다.
      </Alert>
      <Tabs
        value={view}
        onChange={(_, value: typeof view) => guard(() => setView(value))}
        variant="scrollable"
        scrollButtons="auto"
        sx={{ mt: 2, borderBottom: 1, borderColor: "divider" }}
      >
        <Tab value="editor" label="Content Editor" />
        <Tab value="versions" label="Versions & Approval" />
        <Tab value="telemetry" label="Telemetry" />
        <Tab value="report" label="Education Report" />
      </Tabs>
      {versions.loading && <LinearProgress />}
      <Box mt={3}>
        {versions.error ? (
          <ErrorPanel
            error={versions.error}
            retry={() => void versions.reload()}
          />
        ) : view === "editor" ? (
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 4 }}>
              <Card>
                <CardContent>
                  <Typography variant="h3">콘텐츠 영역</Typography>
                  <TextField
                    select
                    fullWidth
                    label="콘텐츠 영역"
                    value={section}
                    onChange={(event) => {
                      const next = event.target.value as DefenseSection;
                      guard(() => setSection(next));
                    }}
                    sx={{ mt: 2 }}
                  >
                    {sections
                      .filter(
                        (item) =>
                          slug === "ai-nexus-defense" ||
                          !["resource_rules", "model_profiles"].includes(
                            item.id,
                          ),
                      )
                      .map((item) => (
                        <MenuItem key={item.id} value={item.id}>
                          {item.label}
                        </MenuItem>
                      ))}
                  </TextField>
                  <Divider sx={{ my: 2 }} />
                  <Typography fontWeight={850}>
                    {DEFENSE_PACKS[slug].presentation.name}
                  </Typography>
                  <Typography color="text.secondary" mt={0.7}>
                    선택한 section의 전체 JSON을 편집합니다. 저장 시 checksum을
                    비교하여 다른 관리자의 변경을 덮어쓰지 않습니다.
                  </Typography>
                  {selected && (
                    <Stack
                      direction="row"
                      flexWrap="wrap"
                      useFlexGap
                      spacing={1}
                      mt={2}
                    >
                      <Chip
                        label={selected.status}
                        color={
                          selected.status === "published"
                            ? "success"
                            : "default"
                        }
                      />
                      <Chip label={selected.content_version} />
                      <Chip label={selected.policy_version || "정책 없음"} />
                    </Stack>
                  )}
                  {!parsed.error && (
                    <DefenseQuickEditor
                      // Remount when the document changes: the field holds its
                      // own text while it is being typed, and without this it
                      // would keep showing the previous game's JSON.
                      key={`${slug}:${section}:${versionId}`}
                      data={parsed.value}
                      onChange={(value) =>
                        setEditor(JSON.stringify(value, null, 2))
                      }
                    />
                  )}
                  <Button
                    component={RouterLink}
                    to={
                      versionId ? `/defense/${slug}/preview/${versionId}` : "#"
                    }
                    disabled={!versionId}
                    startIcon={<PreviewRounded />}
                    variant="outlined"
                    sx={{ mt: 2 }}
                    fullWidth
                  >
                    미리보기
                  </Button>
                </CardContent>
              </Card>
            </Grid>
            <Grid size={{ xs: 12, md: 8 }}>
              <Card>
                <CardContent>
                  <Stack
                    direction={{ xs: "column", sm: "row" }}
                    justifyContent="space-between"
                    gap={1}
                  >
                    <Box>
                      <Typography variant="h3">{section} JSON</Typography>
                      <Typography color="text.secondary">
                        고급 데이터 편집기 · 서버 전체 검증 후 게시 가능
                      </Typography>
                    </Box>
                    <Button
                      variant="contained"
                      startIcon={
                        saving ? (
                          <CircularProgress size={18} />
                        ) : (
                          <SaveRounded />
                        )
                      }
                      disabled={
                        saving ||
                        Boolean(parsed.error) ||
                        !editableStatuses.has(selected?.status ?? "archived")
                      }
                      onClick={() => void save()}
                    >
                      Draft 저장
                    </Button>
                  </Stack>
                  {sectionData.error && (
                    <Alert severity="error" sx={{ mt: 2 }}>
                      {sectionData.error.message}
                    </Alert>
                  )}
                  {!sectionData.error &&
                    (sectionData.loading || !editorReady ? (
                      <CircularProgress sx={{ mt: 3 }} />
                    ) : (
                      <>
                        <TextField
                          data-testid="defense-section-editor"
                          value={editor}
                          onChange={(event) => setEditor(event.target.value)}
                          multiline
                          fullWidth
                          minRows={22}
                          maxRows={34}
                          aria-label={`${section} JSON 편집기`}
                          inputProps={{ spellCheck: false }}
                          sx={{
                            mt: 2,
                            "& textarea": {
                              fontFamily:
                                "ui-monospace, SFMono-Regular, Consolas, monospace",
                              fontSize: "1rem",
                              lineHeight: 1.55,
                            },
                          }}
                        />
                        {parsed.error && (
                          <Alert severity="error" sx={{ mt: 1 }}>
                            {parsed.error}
                          </Alert>
                        )}
                      </>
                    ))}
                </CardContent>
              </Card>
            </Grid>
          </Grid>
        ) : view === "versions" ? (
          <Card>
            <CardContent>
              <Stack
                direction={{ xs: "column", md: "row" }}
                justifyContent="space-between"
                gap={2}
              >
                <Box>
                  <Typography variant="h3">버전 검증과 게시</Typography>
                  <Typography color="text.secondary" mt={0.5}>
                    {config.approval_enabled
                      ? "Draft → Test → 팀장 승인 → 게시 흐름을 따릅니다."
                      : "승인 절차가 꺼져 있어 테스트한 버전을 즉시 게시합니다."}
                  </Typography>
                </Box>
                <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}>
                  <Button
                    startIcon={<ScienceRounded />}
                    disabled={
                      !selected ||
                      saving ||
                      !editableStatuses.has(selected.status)
                    }
                    onClick={() =>
                      void mutate("전체 콘텐츠 검증을 완료했습니다.", () =>
                        defenseStudioAPI.testVersion(slug, selected!.id),
                      )
                    }
                  >
                    테스트
                  </Button>
                  <Button
                    startIcon={<PublishRounded />}
                    variant="contained"
                    disabled={
                      !selected ||
                      saving ||
                      !["testing", "approved"].includes(selected.status)
                    }
                    onClick={() =>
                      void mutate(
                        selected?.status === "approved" ||
                          !config.approval_enabled
                          ? "게시했습니다."
                          : "승인 요청을 처리했습니다.",
                        () =>
                          defenseStudioAPI.publishVersion(slug, selected!.id),
                      )
                    }
                  >
                    {selected?.status === "approved"
                      ? "게시"
                      : config.approval_enabled
                        ? "승인 요청"
                        : "즉시 게시"}
                  </Button>
                  {config.approval_enabled &&
                    isAdmin &&
                    selected?.status === "pending_approval" && (
                      <>
                        <Button
                          color="success"
                          startIcon={<CheckCircleRounded />}
                          onClick={() =>
                            void mutate("승인했습니다.", () =>
                              reviewDefenseVersion(
                                selected.id,
                                "approved",
                                reviewComment,
                              ),
                            )
                          }
                        >
                          승인
                        </Button>
                        <Button
                          color="error"
                          onClick={() =>
                            void mutate("반려했습니다.", () =>
                              reviewDefenseVersion(
                                selected.id,
                                "rejected",
                                reviewComment,
                              ),
                            )
                          }
                        >
                          반려
                        </Button>
                      </>
                    )}
                  <Button
                    component={RouterLink}
                    to={
                      versionId ? `/defense/${slug}/preview/${versionId}` : "#"
                    }
                    disabled={!versionId}
                    startIcon={<PreviewRounded />}
                  >
                    미리보기
                  </Button>
                </Stack>
              </Stack>
              {config.approval_enabled && (
                <TextField
                  fullWidth
                  label="검토 의견"
                  value={reviewComment}
                  onChange={(event) => setReviewComment(event.target.value)}
                  sx={{ mt: 2 }}
                />
              )}
              <Grid container spacing={1.2} mt={1}>
                {versionItems.map((item) => (
                  <Grid key={item.id} size={{ xs: 12, md: 6 }}>
                    <Card
                      variant="outlined"
                      sx={{
                        borderColor:
                          item.id === versionId ? "primary.main" : undefined,
                      }}
                    >
                      <CardActionArea onClick={() => setVersionId(item.id)}>
                        <CardContent>
                          <Stack direction="row" justifyContent="space-between">
                            <Typography fontWeight={900}>
                              {item.label}
                            </Typography>
                            <Chip size="small" label={item.status} />
                          </Stack>
                          <Typography
                            variant="body2"
                            color="text.secondary"
                            mt={1}
                          >
                            {item.content_version} · {item.policy_version}
                          </Typography>
                          <Typography variant="body2" mt={0.5}>
                            {item.notes || "릴리스 노트 없음"}
                          </Typography>
                          {item.source_version_id && (
                            <Typography
                              variant="body2"
                              color="text.secondary"
                              mt={0.5}
                            >
                              복제 원본 · {item.source_version_id}
                            </Typography>
                          )}
                        </CardContent>
                      </CardActionArea>
                    </Card>
                  </Grid>
                ))}
              </Grid>
            </CardContent>
          </Card>
        ) : view === "telemetry" ? (
          <Card>
            <CardContent>
              <Stack direction="row" spacing={1}>
                <BugReportRounded color="primary" />
                <Typography variant="h3">최근 운영 Telemetry</Typography>
              </Stack>
              <Typography color="text.secondary" mt={1}>
                {slug === "office-guardians"
                  ? "조직별 플레이와 방어 성과를 확인합니다."
                  : "게임·교육 이벤트의 참여와 완료 추이를 확인합니다."}
              </Typography>
              {telemetry.error ? (
                <ErrorPanel
                  error={telemetry.error}
                  retry={() => void telemetry.reload()}
                />
              ) : (
                <ReportDashboard data={telemetry.data} education={false} />
              )}
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardContent>
              <Stack direction="row" spacing={1}>
                <SchoolRounded color="secondary" />
                <Typography variant="h3">교육 효과 Report</Typography>
              </Stack>
              <Typography color="text.secondary" mt={1}>
                {educationReportEnabled
                  ? "게시된 교육 콘텐츠의 참여·완료율·문제별 정답률·취약 주제·부서별 개선을 확인합니다."
                  : "현재 게시 버전에는 교육 평가가 없어 조직 플레이 통계를 표시합니다."}
              </Typography>
              {report.error ? (
                <ErrorPanel
                  error={report.error}
                  retry={() => void report.reload()}
                />
              ) : (
                <ReportDashboard
                  data={report.data}
                  education={educationReportEnabled}
                />
              )}
            </CardContent>
          </Card>
        )}
      </Box>
      <Dialog
        open={createOpen}
        onClose={() => !saving && setCreateOpen(false)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>
          새 {DEFENSE_PACKS[slug].presentation.name} Draft
        </DialogTitle>
        <DialogContent>
          <Stack spacing={2} mt={1}>
            <TextField
              data-testid="defense-rollback-source"
              select
              label="복제할 기준 버전"
              value={draft.source_version_id}
              onChange={(event) =>
                setDraft({ ...draft, source_version_id: event.target.value })
              }
              helperText="과거 버전을 선택하면 안전한 롤백 Draft가 생성되며 테스트·승인 흐름을 다시 거칩니다."
            >
              <MenuItem value="">현재 게시 버전</MenuItem>
              {versionItems.map((item) => (
                <MenuItem key={item.id} value={item.id}>
                  {item.label} · {item.status} · #{item.version_no}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              data-testid="defense-policy-version"
              label="Policy Version"
              value={draft.policy_version}
              onChange={(event) =>
                setDraft({ ...draft, policy_version: event.target.value })
              }
              helperText="비워 두면 기준 버전의 정책 버전을 상속합니다."
            />
            <TextField
              label="버전 라벨"
              value={draft.label}
              onChange={(event) =>
                setDraft({ ...draft, label: event.target.value })
              }
            />
            <TextField
              label="릴리스 노트"
              multiline
              minRows={4}
              value={draft.notes}
              onChange={(event) =>
                setDraft({ ...draft, notes: event.target.value })
              }
            />
            <TextField
              label="Asset Version"
              value={draft.asset_version}
              onChange={(event) =>
                setDraft({ ...draft, asset_version: event.target.value })
              }
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateOpen(false)}>취소</Button>
          <Button
            variant="contained"
            disabled={saving}
            onClick={() => void create()}
          >
            Draft 만들기
          </Button>
        </DialogActions>
      </Dialog>
      <UnsavedChangesDialog open={askingToDiscard} onKeepEditing={keepEditing} onDiscard={discard} />
    </Container>
  );
}

export default DefenseContentStudioPage;
