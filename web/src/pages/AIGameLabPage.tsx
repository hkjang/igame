import { type FormEvent, useRef, useState } from 'react';
import AutoAwesomeRounded from '@mui/icons-material/AutoAwesomeRounded';
import SendRounded from '@mui/icons-material/SendRounded';
import StopCircleRounded from '@mui/icons-material/StopCircleRounded';
import { Alert, Box, Button, Card, CardContent, CircularProgress, Container, Stack, TextField, Typography } from '@mui/material';
import { streamAI } from '../api/client';
import { useAuth } from '../state/AuthContext';

interface Message { role: 'user' | 'assistant'; content: string }

export function AIGameLabPage() {
  const { config } = useAuth();
  const [messages, setMessages] = useState<Message[]>([{ role: 'assistant', content: '낡은 우주 정거장의 비상등이 켜졌습니다. 당신의 앞에는 잠긴 관제실과 미지의 신호가 있습니다. 무엇을 할까요?' }]);
  const [prompt, setPrompt] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [draft, setDraft] = useState('');
  const [error, setError] = useState('');
  const abort = useRef<AbortController | undefined>(undefined);
  if (config.ai_enabled === false) return <Container maxWidth="md" sx={{ py: 6 }}><Alert severity="info">AI Game Lab이 비활성화되어 있습니다. 서비스 관리자가 AI Runtime을 설정하면 사용할 수 있습니다.</Alert></Container>;
  const submit = async (event: FormEvent) => {
    event.preventDefault(); const input = prompt.trim(); if (!input || streaming) return;
    const history = [...messages, { role: 'user' as const, content: input }];
    setMessages(history); setPrompt(''); setDraft(''); setError(''); setStreaming(true);
    const controller = new AbortController(); abort.current = controller; let accumulated = '';
    try {
      await streamAI('/api/v1/ai/chat/completions', {
        messages: [{ role: 'system', content: '당신은 한국어로 진행하는 사내 친화적 SF 텍스트 어드벤처 게임 마스터입니다. 선택 결과를 간결하게 묘사하고 마지막에 2~3개 선택지를 제시하세요.' }, ...history],
        max_tokens: 4096,
      }, { signal: controller.signal, onToken: (token) => { accumulated += token; setDraft(accumulated); } });
      if (accumulated) setMessages((current) => [...current, { role: 'assistant', content: accumulated }]);
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'AI 응답을 받지 못했습니다.');
    } finally { setDraft(''); setStreaming(false); abort.current = undefined; }
  };
  return <Container maxWidth="md" sx={{ py: { xs: 4, md: 6 } }}><Stack direction="row" spacing={1.5} alignItems="center"><Box sx={{ width: 52, height: 52, display: 'grid', placeItems: 'center', borderRadius: 2.5, bgcolor: 'rgba(175,140,255,.16)', color: '#c3a9ff' }}><AutoAwesomeRounded fontSize="large" /></Box><Box><Typography variant="h1" sx={{ fontSize: { xs: '2.1rem', md: '3rem' } }}>AI Game Lab</Typography><Typography color="text.secondary">스트리밍 AI Dungeon 데모</Typography></Box></Stack><Alert severity="info" sx={{ mt: 3 }}>AI 요청은 igame 서버를 거쳐 전송되며 API Key는 브라우저에 노출되지 않습니다.</Alert><Card sx={{ mt: 3 }}><CardContent className="admin-scrollbar" aria-live="polite" sx={{ p: { xs: 2, md: 3 }, height: 'min(58vh,620px)', overflowY: 'auto' }}><Stack spacing={2}>{messages.map((message, index) => <Box key={index} sx={{ alignSelf: message.role === 'user' ? 'flex-end' : 'flex-start', maxWidth: '86%', px: 2, py: 1.5, borderRadius: 2.5, whiteSpace: 'pre-wrap', bgcolor: message.role === 'user' ? 'primary.dark' : 'action.hover', color: message.role === 'user' ? '#fff' : 'text.primary' }}><Typography>{message.content}</Typography></Box>)}{streaming && <Box sx={{ alignSelf: 'flex-start', maxWidth: '86%', px: 2, py: 1.5, borderRadius: 2.5, bgcolor: 'action.hover', whiteSpace: 'pre-wrap' }}>{draft ? <Typography>{draft}</Typography> : <Stack direction="row" spacing={1} alignItems="center"><CircularProgress size={20} /><Typography color="text.secondary">이야기를 만드는 중…</Typography></Stack>}</Box>}</Stack></CardContent></Card>{error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}<Stack component="form" onSubmit={(event) => void submit(event)} direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mt={2}><TextField value={prompt} onChange={(event) => setPrompt(event.target.value)} label="다음 행동" placeholder="예: 신호의 발신지를 추적한다" disabled={streaming} inputProps={{ maxLength: 2000 }} />{streaming ? <Button type="button" color="warning" variant="outlined" startIcon={<StopCircleRounded />} onClick={() => abort.current?.abort()}>중단</Button> : <Button type="submit" variant="contained" startIcon={<SendRounded />} disabled={!prompt.trim()}>전송</Button>}</Stack></Container>;
}
