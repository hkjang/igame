import CodeRounded from '@mui/icons-material/CodeRounded';
import DescriptionRounded from '@mui/icons-material/DescriptionRounded';
import KeyRounded from '@mui/icons-material/KeyRounded';
import DownloadRounded from '@mui/icons-material/DownloadRounded';
import { Alert, Box, Button, Card, CardContent, Container, Grid, Stack, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { DeveloperWorkflow } from '../components/DeveloperWorkflow';

const sample = `import { createGameHub } from '@igame/gamehub-js';

const game = createGameHub({ gameId: 'my-game' });
await game.init();
await game.start();
await game.submitScore({ score: 3250 });
await game.finish({ score: 3250, duration: 185 });`;

export function DeveloperPage() {
  return <Container maxWidth="lg" sx={{ py: 6 }}><Typography variant="h1" sx={{ fontSize: { xs: '2.2rem', md: '3.2rem' } }}>Game Developer</Typography><Typography color="text.secondary" mt={1}>게임 로직에 집중하세요. 인증, 세션, 점수, 랭킹은 Game SDK가 연결합니다.</Typography><Alert severity="info" sx={{ mt: 3 }}>신규 게임 공개는 관리자 승인 정책이 켜져 있을 때 검토 후 반영됩니다.</Alert><Grid container spacing={3} mt={1}><Grid size={{ xs: 12, md: 8 }}><Card><CardContent sx={{ p: 3 }}><Stack direction="row" spacing={1.5} alignItems="center"><CodeRounded color="primary" /><Typography variant="h3">빠른 시작</Typography></Stack><Box component="pre" className="admin-scrollbar" sx={{ mt: 2, p: 2.5, overflowX: 'auto', bgcolor: '#050b12', borderRadius: 2, color: '#c7eaff', fontSize: '.9rem', lineHeight: 1.7 }}><code>{sample}</code></Box><Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.2} mt={2}><Button component="a" href="/sdk/gamehub-js.js" download startIcon={<DownloadRounded />} variant="contained">SDK ESM 다운로드</Button><Button component="a" href="/sdk/gamehub-js.d.ts" download startIcon={<DownloadRounded />} variant="outlined">TypeScript 타입</Button></Stack></CardContent></Card></Grid><Grid size={{ xs: 12, md: 4 }}><Stack spacing={2}><Card><CardContent><DescriptionRounded color="primary" /><Typography variant="h4" mt={1}>SDK API</Typography><Typography color="text.secondary" mt={1}>init · start · pause · submitScore · unlockAchievement · finish · telemetry</Typography></CardContent></Card><Card><CardContent><KeyRounded color="primary" /><Typography variant="h4" mt={1}>개발 키</Typography><Typography color="text.secondary" mt={1}>개인별 최소 권한 키를 만들고 언제든 회전할 수 있습니다.</Typography><Button component={RouterLink} to="/profile/keys" sx={{ mt: 2 }} variant="outlined">키 관리</Button></CardContent></Card></Stack></Grid></Grid><DeveloperWorkflow /></Container>;
}
