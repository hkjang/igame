import SearchOffRounded from '@mui/icons-material/SearchOffRounded';
import { Button, Container, Stack, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';

export function NotFoundPage() {
  return <Container maxWidth="sm"><Stack minHeight="70vh" alignItems="center" justifyContent="center" textAlign="center"><SearchOffRounded sx={{ fontSize: 82, color: 'text.secondary' }} /><Typography variant="h1" mt={2}>404</Typography><Typography variant="h3" mt={1}>페이지를 찾을 수 없습니다</Typography><Typography color="text.secondary" mt={1}>주소가 바뀌었거나 접근할 수 없는 메뉴입니다.</Typography><Button component={RouterLink} to="/" variant="contained" sx={{ mt: 3 }}>홈으로 돌아가기</Button></Stack></Container>;
}
