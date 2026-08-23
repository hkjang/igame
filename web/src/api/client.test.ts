import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, onSessionExpired, streamAI } from './client';

afterEach(() => vi.restoreAllMocks());

describe('api client', () => {
  it('normalizes version response casing', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ Version: '1.2.3', Commit: 'abcdef', BuildDate: '2026-08-20T00:00:00Z' }), { status: 200, headers: { 'content-type': 'application/json' } }));
    await expect(api.version()).resolves.toMatchObject({ version: '1.2.3', commit: 'abcdef' });
  });

  it('preserves the status and code of a structured backend error', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'unauthorized', message: 'authentication required' } }), { status: 401, headers: { 'content-type': 'application/json' } }));
    await expect(api.me()).rejects.toMatchObject({ status: 401, code: 'unauthorized', message: '로그인이 필요합니다.' });
  });

  it('translates the English API message for a user-facing code', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'invalid_credentials', message: 'invalid username or password' } }), { status: 401, headers: { 'content-type': 'application/json' } }));
    await expect(api.login('admin', 'nope')).rejects.toMatchObject({ code: 'invalid_credentials', message: '아이디 또는 비밀번호가 올바르지 않습니다.' });
  });

  it('keeps the detailed server message for codes it does not translate', async () => {
    const detail = 'stage id must be 1-32 ASCII characters';
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'content_validation_failed', message: detail } }), { status: 422, headers: { 'content-type': 'application/json' } }));
    await expect(api.request('/api/v1/admin/defense/cyber-fortress/versions', { method: 'POST', body: '{}' })).rejects.toMatchObject({ message: detail });
  });

  it('adds the Retry-After wait to a throttled sign-in', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'too_many_attempts', message: 'too many failed sign-in attempts; try again later' } }), { status: 429, headers: { 'content-type': 'application/json', 'retry-after': '900' } }));
    await expect(api.login('admin', 'nope')).rejects.toMatchObject({ message: '로그인 시도가 너무 많습니다. 약 15분 후에 다시 시도할 수 있습니다.' });
  });

  it('reports an expired session once, and not for sign-in itself', async () => {
    const expired = vi.fn();
    onSessionExpired(expired);
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'unauthorized', message: 'authentication required' } }), { status: 401, headers: { 'content-type': 'application/json' } }));
    await expect(api.games()).rejects.toBeInstanceOf(ApiError);
    expect(expired).toHaveBeenCalledTimes(1);
    await expect(api.login('admin', 'nope')).rejects.toBeInstanceOf(ApiError);
    expect(expired).toHaveBeenCalledTimes(1);
    onSessionExpired(() => undefined);
  });

  it('preserves application data fields when the caller requests the full envelope', async () => {
    const response = { version: { id: 'v1', checksum: 'abc' }, section: 'stages', data: [{ id: 'stage-1' }] };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(response), { status: 200, headers: { 'content-type': 'application/json' } }));
    await expect(api.requestEnvelope<typeof response>('/api/v1/admin/realmguard/drafts/stages')).resolves.toEqual(response);
  });

  it('parses OpenAI-compatible SSE deltas incrementally', async () => {
    const stream = 'data: {"choices":[{"delta":{"content":"안녕"}}]}\n\ndata: {"choices":[{"delta":{"content":"하세요"}}]}\n\ndata: [DONE]\n\n';
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(stream, { status: 200, headers: { 'content-type': 'text/event-stream' } }));
    let output = '';
    await streamAI('/api/v1/ai/chat/completions', { messages: [] }, { onToken: (token) => { output += token; } });
    expect(output).toBe('안녕하세요');
  });
});

describe('adminList paging', () => {
  function capture() {
    const spy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ items: [], total: 214 }), { status: 200, headers: { 'content-type': 'application/json' } }),
    );
    return spy;
  }

  it('omits the query string entirely when no paging is requested', async () => {
    const spy = capture();
    await api.adminList('categories');
    expect(spy.mock.calls[0][0]).toBe('/api/v1/admin/categories');
  });

  it('sends limit, offset and the search term', async () => {
    const spy = capture();
    await api.adminList('audit', { limit: 50, offset: 100, q: 'auth.login' });
    expect(spy.mock.calls[0][0]).toBe('/api/v1/admin/audit?limit=50&offset=100&q=auth.login');
  });

  it('leaves out an empty search and a zero offset', async () => {
    const spy = capture();
    await api.adminList('users', { limit: 25, offset: 0, q: '' });
    expect(spy.mock.calls[0][0]).toBe('/api/v1/admin/users?limit=25');
  });

  it('surfaces the unpaged total so the console can page past the first result', async () => {
    capture();
    await expect(api.adminList('users', { limit: 50 })).resolves.toMatchObject({ total: 214 });
  });
});
