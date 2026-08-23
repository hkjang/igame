import CalendarMonthRounded from '@mui/icons-material/CalendarMonthRounded';
import GroupsRounded from '@mui/icons-material/GroupsRounded';
import { Alert, Box, Button, Card, CardActions, CardContent, Chip, Container, Grid, LinearProgress, Stack, Typography } from '@mui/material';
import { api } from '../api/client';
import { ErrorPanel } from '../components/ErrorPanel';
import { useAsync } from '../hooks/useAsync';
import { useSnackbar } from '../state/SnackbarContext';

interface EventItem { id: string; name: string; description?: string; status?: string; starts_at?: string; ends_at?: string; participants?: number; participant_count?: number; progress?: number; type?: string; event_type?: string; joined?: boolean }

export function EventsPage() {
  const { notify } = useSnackbar();
  const { data, loading, error, setData, reload } = useAsync(() => api.request<{ items: EventItem[] }>('/api/v1/events'), []);
  const events = data?.items ?? [];
  const join = async (event: EventItem) => {
    try { await api.joinEvent(event.id); setData({ items: events.map((item) => item.id === event.id ? { ...item, joined: true, participant_count: (item.participant_count ?? 0) + (item.joined ? 0 : 1) } : item) }); notify('이벤트에 참여했습니다.', 'success'); }
    catch (cause) { notify(cause instanceof Error ? cause.message : '이벤트에 참여하지 못했습니다.', 'error'); }
  };
  return <Container maxWidth="xl" sx={{ py: { xs: 4, md: 6 } }}><Typography variant="h1" sx={{ fontSize: { xs: '2.2rem', md: '3.2rem' } }}>시즌 & 이벤트</Typography><Typography color="text.secondary" mt={1}>동료들과 함께 참여하고 특별한 배지를 획득하세요.</Typography>
    {loading && <LinearProgress sx={{ mt: 4 }} />}{error && <Box mt={4}><ErrorPanel error={error} retry={() => void reload()} /></Box>}
    {!loading && !error && events.length === 0 && <Alert severity="info" sx={{ mt: 4 }}>현재 진행 중인 이벤트가 없습니다. 다음 이벤트를 기다려 주세요.</Alert>}
    <Grid container spacing={3} mt={1}>{events.map((event) => <Grid key={event.id} size={{ xs: 12, md: 6, lg: 4 }}><Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}><Box sx={(theme) => ({ height: 9, background: `linear-gradient(90deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})` })} /><CardContent sx={{ p: 3, flex: 1 }}><Stack direction="row" justifyContent="space-between"><Chip label={event.status === 'active' ? '진행 중' : event.status ?? '예정'} color={event.status === 'active' ? 'success' : 'default'} /><Chip label={event.event_type ?? event.type ?? '이벤트'} variant="outlined" /></Stack><Typography variant="h3" mt={2}>{event.name}</Typography><Typography color="text.secondary" mt={1}>{event.description}</Typography><Stack spacing={1.2} mt={3}><Stack direction="row" spacing={1}><CalendarMonthRounded color="primary" /><Typography>{event.starts_at ? new Date(event.starts_at).toLocaleDateString('ko-KR') : '일정 미정'} ~ {event.ends_at ? new Date(event.ends_at).toLocaleDateString('ko-KR') : ''}</Typography></Stack><Stack direction="row" spacing={1}><GroupsRounded color="primary" /><Typography>{event.participant_count ?? event.participants ?? 0}명 참여</Typography></Stack></Stack>{typeof event.progress === 'number' && <LinearProgress variant="determinate" value={event.progress} sx={{ mt: 3, height: 8, borderRadius: 5 }} />}</CardContent><CardActions sx={{ p: 3, pt: 0 }}><Button fullWidth variant={event.joined ? 'outlined' : 'contained'} disabled={event.joined || event.status !== 'active'} onClick={() => void join(event)}>{event.joined ? '참여 중' : event.status === 'active' ? '이벤트 참여' : '참여 대기'}</Button></CardActions></Card></Grid>)}</Grid>
  </Container>;
}
