import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, streamAI } from './client';

afterEach(() => vi.restoreAllMocks());

describe('api client', () => {
  it('normalizes version response casing', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ Version: '1.2.3', Commit: 'abcdef', BuildDate: '2026-08-20T00:00:00Z' }), { status: 200, headers: { 'content-type': 'application/json' } }));
    await expect(api.version()).resolves.toMatchObject({ version: '1.2.3', commit: 'abcdef' });
  });

  it('preserves structured backend errors', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'unauthorized', message: '로그인이 필요합니다.' } }), { status: 401, headers: { 'content-type': 'application/json' } }));
    await expect(api.me()).rejects.toMatchObject({ status: 401, code: 'unauthorized', message: '로그인이 필요합니다.' });
  });

  it('parses OpenAI-compatible SSE deltas incrementally', async () => {
    const stream = 'data: {"choices":[{"delta":{"content":"안녕"}}]}\n\ndata: {"choices":[{"delta":{"content":"하세요"}}]}\n\ndata: [DONE]\n\n';
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(stream, { status: 200, headers: { 'content-type': 'text/event-stream' } }));
    let output = '';
    await streamAI('/api/v1/ai/chat/completions', { messages: [] }, { onToken: (token) => { output += token; } });
    expect(output).toBe('안녕하세요');
  });
});
