import {
  Alert,
  Box,
  Button,
  Container,
  Stack,
  Typography,
} from "@mui/material";
import ArrowBackRounded from "@mui/icons-material/ArrowBackRounded";
import { lazy, Suspense } from "react";
import { Link as RouterLink, useParams } from "react-router-dom";
import { ErrorPanel } from "../components/ErrorPanel";
import { LoadingScreen } from "../components/LoadingScreen";
import { defenseStudioAPI } from "../games/defense/api";
import { isDefenseSlug } from "../games/defense/content";
import { useAsync } from "../hooks/useAsync";
import { useAuth } from "../state/AuthContext";

// Loaded on demand like the play route does: this module pulls in the Defense
// engine and, through it, Phaser.
const DefenseSeriesGame = lazy(() => import("../games/defense/DefenseSeriesGame"));

export function DefensePreviewPage() {
  const { user } = useAuth();
  const { slug = "", id = "" } = useParams();
  const validSlug = isDefenseSlug(slug) ? slug : undefined;
  const preview = useAsync(async () => {
    if (!validSlug || !id)
      throw new Error("미리보기 대상이 올바르지 않습니다.");
    return defenseStudioAPI.preview(validSlug, id);
  }, [id, validSlug]);
  if (preview.loading)
    return (
      <Container sx={{ py: 8 }}>
        <Typography>Defense 미리보기를 준비하는 중…</Typography>
      </Container>
    );
  if (preview.error || !preview.data || !validSlug)
    return (
      <Container sx={{ py: 8 }}>
        <ErrorPanel
          error={preview.error ?? new Error("미리보기를 불러오지 못했습니다.")}
          retry={() => void preview.reload()}
        />
      </Container>
    );
  const roles = [user?.role, ...(user?.roles ?? [])];
  const managerOnly =
    roles.includes("manager") &&
    !roles.some((role) => role === "admin" || role === "operator");
  return (
    <Container maxWidth="xl" sx={{ py: 3 }}>
      <Stack
        direction={{ xs: "column", sm: "row" }}
        justifyContent="space-between"
        gap={2}
        mb={2}
      >
        <Box>
          <Typography variant="h2">Defense Content Preview</Typography>
          <Typography color="text.secondary">
            {preview.data.envelope.version.label} · 저장되지 않는 관리자 연습
            환경
          </Typography>
        </Box>
        <Button
          component={RouterLink}
          to={
            managerOnly
              ? "/reviews"
              : `/admin/defense?game=${validSlug}&version=${id}`
          }
          startIcon={<ArrowBackRounded />}
        >
          {managerOnly ? "승인함으로" : "Content Studio로"}
        </Button>
      </Stack>
      <Alert severity="warning" sx={{ mb: 2 }}>
        미리보기에서는 세션·교육 답안·결과·진행도·랭킹을 저장하지 않습니다.
      </Alert>
      <Suspense fallback={<LoadingScreen label="게임 엔진을 불러오는 중…" />}>
        <DefenseSeriesGame
          preview={{
            slug: validSlug,
            pack: preview.data.pack,
            label: preview.data.envelope.version.label,
          }}
          onStart={async () => true}
          onFinish={async () => undefined}
        />
      </Suspense>
    </Container>
  );
}

export default DefensePreviewPage;
