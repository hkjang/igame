import { useMemo, useState } from 'react';
import CampaignRounded from '@mui/icons-material/CampaignRounded';
import PushPinRounded from '@mui/icons-material/PushPinRounded';
import SearchRounded from '@mui/icons-material/SearchRounded';
import { Box, Card, CardContent, Chip, Container, InputAdornment, Stack, TextField, Typography } from '@mui/material';
import { EmptyState } from '../components/EmptyState';
import { PageHeader } from '../components/PageHeader';
import { visuallyHidden } from '../components/RouteChrome';
import { api } from '../api/client';
import { ErrorPanel } from '../components/ErrorPanel';
import { LoadingScreen } from '../components/LoadingScreen';
import { useAsync } from '../hooks/useAsync';

/**
 * The full announcement archive.
 *
 * The home page shows the four newest and nothing else, which meant an
 * announcement was unreachable for anyone who was away when it went up.
 */
export function NoticesPage() {
  const result = useAsync(() => api.notices(), []);
  const [search, setSearch] = useState('');
  const notices = useMemo(() => {
    const items = result.data?.items ?? [];
    const term = search.trim().toLocaleLowerCase('ko');
    if (!term) return items;
    return items.filter((notice) => `${notice.title} ${notice.content}`.toLocaleLowerCase('ko').includes(term));
  }, [result.data, search]);

  return (
    <Container maxWidth="md" sx={{ py: { xs: 4, md: 6 } }}>
      <PageHeader icon={<CampaignRounded />} title="공지사항" description="서비스 안내와 운영 공지를 모두 확인할 수 있습니다." />

      <TextField
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        placeholder="제목이나 내용 검색"
        aria-label="공지사항 검색"
        sx={{ mt: 4, maxWidth: 520 }}
        slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded /></InputAdornment> } }}
      />

      <Box mt={3}>
        {result.loading ? <LoadingScreen label="공지사항을 불러오는 중…" />
          : result.error ? <ErrorPanel error={result.error} retry={() => void result.reload()} />
          : notices.length === 0 ? (
            <EmptyState
              icon={search ? <SearchRounded /> : <CampaignRounded />}
              title={search ? '검색 결과가 없습니다' : '등록된 공지가 없습니다'}
              description={search ? '다른 검색어를 사용해 보세요.' : '관리자가 공지를 올리면 이곳에 바로 표시됩니다.'}
            />
          ) : (
            <Stack spacing={2}>
              {notices.map((notice) => (
                <Card key={notice.id} component="article">
                  <CardContent sx={{ p: 3 }}>
                    <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                      {notice.pinned && <Chip size="small" color="primary" icon={<PushPinRounded />} label="중요" />}
                      <Typography variant="h3" component="h2">{notice.title}</Typography>
                    </Stack>
                    {notice.published_at && (
                      <Typography variant="body2" color="text.secondary" mt={0.8} component="time" dateTime={notice.published_at}>
                        {new Date(notice.published_at).toLocaleString('ko-KR')}
                      </Typography>
                    )}
                    <Typography mt={1.5} sx={{ whiteSpace: 'pre-wrap' }}>{notice.content}</Typography>
                  </CardContent>
                </Card>
              ))}
            </Stack>
          )}
      </Box>

      {!result.loading && !result.error && (
        <Box role="status" aria-live="polite" sx={visuallyHidden}>
          {`공지 ${notices.length}건`}
        </Box>
      )}
    </Container>
  );
}

export default NoticesPage;
