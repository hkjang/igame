import { useEffect, useRef, useState } from 'react';
import SaveRounded from '@mui/icons-material/SaveRounded';
import DarkModeRounded from '@mui/icons-material/DarkModeRounded';
import LightModeRounded from '@mui/icons-material/LightModeRounded';
import SettingsBrightnessRounded from '@mui/icons-material/SettingsBrightnessRounded';
import { Alert, Box, Button, Card, CardContent, Divider, FormControlLabel, Slider, Stack, Switch, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import {
  applyPreferences,
  FONT_SCALE_MAX,
  FONT_SCALE_MIN,
  FONT_SCALE_STEP,
  loadPreferences,
  savePreferences,
  type Preferences,
} from '../state/preferences';
import { useSnackbar } from '../state/SnackbarContext';
import { useThemeMode } from '../state/ThemeModeContext';

export function PreferencesPage() {
  const { notify } = useSnackbar();
  const { preference, mode, setPreference } = useThemeMode();
  const stored = useRef<Preferences>(loadPreferences());
  const [draft, setDraft] = useState<Preferences>(stored.current);
  const dirty = draft.fontScale !== stored.current.fontScale || draft.motion !== stored.current.motion;

  // Appearance is owned by the theme provider and applies on selection, so the
  // draft below must never write a stale theme back over it.
  useEffect(() => {
    applyPreferences({ ...draft, theme: preference });
  }, [draft, preference]);
  // Choosing a text size means seeing it, so the draft applies straight away;
  // leaving without saving puts the stored size and motion setting back.
  useEffect(() => () => applyPreferences({ ...stored.current, theme: loadPreferences().theme }), []);

  const save = () => {
    if (!savePreferences({ ...draft, theme: preference })) {
      notify('브라우저가 저장을 허용하지 않아 이번 세션에만 적용됩니다.', 'warning');
      return;
    }
    stored.current = { ...draft, theme: preference };
    applyPreferences({ ...draft, theme: preference });
    notify('개인화 설정을 저장했습니다.', 'success');
  };

  return (
    <Card>
      <CardContent sx={{ p: 3, maxWidth: 700 }}>
        <Typography variant="h3">가시성 및 움직임</Typography>
        <Typography color="text.secondary" mt={1}>
          기본 16px에서 편한 크기로 조절할 수 있습니다. 조절하면 화면에 바로 적용되며, 저장해야 다음 방문에도 유지됩니다.
        </Typography>
        <Stack spacing={3} mt={4}>
          <Box>
            <Typography gutterBottom id="theme-mode-label">화면 모드</Typography>
            <ToggleButtonGroup
              exclusive
              value={preference}
              onChange={(_, next) => next && setPreference(next)}
              aria-labelledby="theme-mode-label"
            >
              <ToggleButton value="system"><SettingsBrightnessRounded sx={{ mr: 1 }} />시스템 설정</ToggleButton>
              <ToggleButton value="light"><LightModeRounded sx={{ mr: 1 }} />밝게</ToggleButton>
              <ToggleButton value="dark"><DarkModeRounded sx={{ mr: 1 }} />어둡게</ToggleButton>
            </ToggleButtonGroup>
            <Typography variant="body2" color="text.secondary" mt={1}>
              {preference === 'system'
                ? `운영체제 설정을 따릅니다. 지금은 ${mode === 'light' ? '밝은' : '어두운'} 화면입니다.`
                : '화면 모드는 선택 즉시 적용되고 저장됩니다.'}
            </Typography>
          </Box>
          <Divider />
          <Box>
            <Typography gutterBottom id="font-scale-label">글자 크기 · {draft.fontScale}%</Typography>
            <Slider
              value={draft.fontScale}
              onChange={(_, value) => setDraft((current) => ({ ...current, fontScale: value as number }))}
              min={FONT_SCALE_MIN}
              max={FONT_SCALE_MAX}
              step={FONT_SCALE_STEP}
              marks
              aria-labelledby="font-scale-label"
              valueLabelDisplay="auto"
              getAriaValueText={(value) => `${value} 퍼센트`}
            />
          </Box>
          <FormControlLabel
            control={<Switch checked={draft.motion} onChange={(event) => setDraft((current) => ({ ...current, motion: event.target.checked }))} />}
            label="화면 전환 애니메이션 사용"
          />
          {dirty && <Alert severity="info" role="status">미리 적용된 상태입니다. 저장하지 않고 나가면 이전 설정으로 되돌아갑니다.</Alert>}
          <Button variant="contained" startIcon={<SaveRounded />} onClick={save} disabled={!dirty} sx={{ alignSelf: 'flex-start' }}>
            설정 저장
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}
