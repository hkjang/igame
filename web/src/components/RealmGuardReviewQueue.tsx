import { useState } from "react";
import CastleRounded from "@mui/icons-material/CastleRounded";
import CheckCircleRounded from "@mui/icons-material/CheckCircleRounded";
import CloseRounded from "@mui/icons-material/CloseRounded";
import OpenInNewRounded from "@mui/icons-material/OpenInNewRounded";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  LinearProgress,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import { useAsync } from "../hooks/useAsync";
import { useAuth } from "../state/AuthContext";
import { useSnackbar } from "../state/SnackbarContext";
import {
  pendingRealmGuardVersions,
  reviewRealmGuardVersion,
  type RealmGuardVersionRecord,
} from "../pages/admin/realmguard/api";

export function RealmGuardReviewQueue({ enabled }: { enabled: boolean }) {
  const { user } = useAuth();
  const { notify } = useSnackbar();
  const canReview = [user?.role, ...(user?.roles ?? [])].some(
    (role) => role && ["manager", "admin"].includes(role),
  );
  const resource = useAsync(
    () =>
      enabled && canReview
        ? pendingRealmGuardVersions()
        : Promise.resolve({ items: [] }),
    [canReview, enabled],
  );
  const [target, setTarget] = useState<RealmGuardVersionRecord>();
  const [decision, setDecision] = useState<"approved" | "rejected">("approved");
  const [comment, setComment] = useState("");
  const [busy, setBusy] = useState(false);
  if (!enabled || !canReview) return null;
  const open = (
    version: RealmGuardVersionRecord,
    next: "approved" | "rejected",
  ) => {
    setTarget(version);
    setDecision(next);
    setComment("");
  };
  const submit = async () => {
    if (!target) return;
    setBusy(true);
    try {
      await reviewRealmGuardVersion(target.id, decision, comment.trim());
      setTarget(undefined);
      await resource.reload();
      notify(
        decision === "approved"
          ? "RealmGuard 게시 버전을 승인했습니다."
          : "RealmGuard 게시 버전을 반려했습니다.",
        "success",
      );
    } catch (cause) {
      notify(
        cause instanceof Error
          ? cause.message
          : "검토 결과를 저장하지 못했습니다.",
        "error",
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <Box mt={5}>
      <Divider sx={{ mb: 4 }} />
      <Stack direction="row" spacing={1.3} alignItems="center" mb={2}>
        <CastleRounded color="primary" />
        <Box>
          <Typography variant="h3">RealmGuard 게시 승인</Typography>
          <Typography color="text.secondary">
            테스트를 통과한 콘텐츠 스냅샷과 변경 영역을 확인합니다.
          </Typography>
        </Box>
      </Stack>
      {resource.loading && <LinearProgress />}
      {resource.error && (
        <Alert severity="error">{resource.error.message}</Alert>
      )}
      <Stack spacing={1.5}>
        {resource.data?.items.map((version) => (
          <Card key={version.id}>
            <CardContent sx={{ p: 2.5 }}>
              <Stack
                direction={{ xs: "column", md: "row" }}
                gap={2}
                alignItems={{ md: "center" }}
              >
                <Box flex={1}>
                  <Stack
                    direction="row"
                    flexWrap="wrap"
                    useFlexGap
                    spacing={1}
                    alignItems="center"
                  >
                    <Chip label="게시 승인 대기" color="warning" size="small" />
                    <Typography variant="h4">{version.label}</Typography>
                  </Stack>
                  <Typography color="text.secondary" mt={1}>
                    요청자{" "}
                    {version.creator?.display_name ||
                      version.creator?.username ||
                      version.created_by ||
                      "알 수 없음"}
                    {version.creator?.team ? ` · ${version.creator.team}` : ""}
                  </Typography>
                  <Stack
                    direction="row"
                    flexWrap="wrap"
                    useFlexGap
                    spacing={0.7}
                    mt={1.2}
                  >
                    {version.changed_sections?.map((section) => (
                      <Chip
                        key={section}
                        size="small"
                        variant="outlined"
                        label={section}
                      />
                    ))}
                    {!version.changed_sections?.length && (
                      <Typography variant="body2" color="text.secondary">
                        변경 영역 정보 없음
                      </Typography>
                    )}
                  </Stack>
                  <Typography variant="body2" mt={1}>
                    {version.notes || "릴리스 노트 없음"}
                  </Typography>
                </Box>
                <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}>
                  <Button
                    component={RouterLink}
                    to={`/realmguard/preview/${version.id}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    startIcon={<OpenInNewRounded />}
                  >
                    연습 미리보기
                  </Button>
                  <Button
                    color="error"
                    variant="outlined"
                    startIcon={<CloseRounded />}
                    onClick={() => open(version, "rejected")}
                  >
                    반려
                  </Button>
                  <Button
                    color="success"
                    variant="contained"
                    startIcon={<CheckCircleRounded />}
                    onClick={() => open(version, "approved")}
                  >
                    승인
                  </Button>
                </Stack>
              </Stack>
            </CardContent>
          </Card>
        ))}
        {!resource.loading &&
          !resource.error &&
          resource.data?.items.length === 0 && (
            <Alert severity="info">
              검토를 기다리는 RealmGuard 게시 버전이 없습니다.
            </Alert>
          )}
      </Stack>
      <Dialog
        open={Boolean(target)}
        onClose={() => !busy && setTarget(undefined)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>
          {decision === "approved"
            ? "RealmGuard 게시 승인"
            : "RealmGuard 게시 반려"}
        </DialogTitle>
        <DialogContent>
          <Typography color="text.secondary" mb={2}>
            {target?.label} · 변경 영역{" "}
            {target?.changed_sections?.join(", ") || "정보 없음"}
          </Typography>
          <Alert severity="warning" sx={{ mb: 2 }}>
            미리보기에서 실제 전투 구성을 확인한 뒤 결정하세요. 요청자·승인자
            분리 정책과 팀 범위는 서버가 검증합니다.
          </Alert>
          <TextField
            label="검토 의견"
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            multiline
            minRows={4}
            required={decision === "rejected"}
            helperText={
              decision === "rejected" ? "반려 사유는 필수입니다." : "선택 사항"
            }
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setTarget(undefined)}>취소</Button>
          <Button
            variant="contained"
            color={decision === "approved" ? "success" : "error"}
            disabled={busy || (decision === "rejected" && !comment.trim())}
            onClick={() => void submit()}
          >
            {decision === "approved" ? "승인" : "반려"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
