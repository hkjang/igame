import { useState } from 'react';
import SaveRounded from '@mui/icons-material/SaveRounded';
import { Box, Button, Card, CardContent, FormControlLabel, Slider, Stack, Switch, Typography } from '@mui/material';
import { useSnackbar } from '../state/SnackbarContext';

export function PreferencesPage() {
  const { notify } = useSnackbar();
  const [fontScale, setFontScale] = useState(Number(localStorage.getItem('igame-font-scale') ?? 100));
  const [motion, setMotion] = useState(localStorage.getItem('igame-motion') !== 'off');
  const save = () => {
    localStorage.setItem('igame-font-scale', String(fontScale)); localStorage.setItem('igame-motion', motion ? 'on' : 'off');
    document.documentElement.style.fontSize = `${16 * fontScale / 100}px`; document.documentElement.dataset.motion = motion ? 'on' : 'off'; notify('개인화 설정을 저장했습니다.', 'success');
  };
  return <Card><CardContent sx={{ p: 3, maxWidth: 700 }}><Typography variant="h3">가시성 및 움직임</Typography><Typography color="text.secondary" mt={1}>기본 16px에서 편한 크기로 조절할 수 있습니다.</Typography><Stack spacing={3} mt={4}><Box><Typography gutterBottom>글자 크기 · {fontScale}%</Typography><Slider value={fontScale} onChange={(_, value) => setFontScale(value as number)} min={100} max={125} step={5} marks aria-label="글자 크기" /></Box><FormControlLabel control={<Switch checked={motion} onChange={(event) => setMotion(event.target.checked)} />} label="화면 전환 애니메이션 사용" /><Button variant="contained" startIcon={<SaveRounded />} onClick={save} sx={{ alignSelf: 'flex-start' }}>설정 저장</Button></Stack></CardContent></Card>;
}
