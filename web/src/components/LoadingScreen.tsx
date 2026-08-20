import { CircularProgress, Stack, Typography } from '@mui/material';

export function LoadingScreen({ label = '불러오는 중…' }: { label?: string }) {
  return <Stack role="status" alignItems="center" justifyContent="center" spacing={2} sx={{ minHeight: 280 }}><CircularProgress /><Typography color="text.secondary">{label}</Typography></Stack>;
}
