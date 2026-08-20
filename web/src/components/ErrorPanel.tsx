import ReplayRounded from '@mui/icons-material/ReplayRounded';
import { Alert, AlertTitle, Button } from '@mui/material';

export function ErrorPanel({ error, retry }: { error: Error; retry?: () => void }) {
  return (
    <Alert severity="error" variant="outlined" action={retry ? <Button color="inherit" startIcon={<ReplayRounded />} onClick={retry}>다시 시도</Button> : undefined}>
      <AlertTitle>요청을 완료하지 못했습니다</AlertTitle>{error.message}
    </Alert>
  );
}
