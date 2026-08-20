import type { ApiList, Game, PersonalKey, PublicConfig, RankingEntry, User, VersionInfo } from '../types';

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code = 'request_failed',
    readonly details?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

type Envelope<T> = T | { data: T } | { item: T } | { items: T[]; total?: number } | { user: T } | { game: T };

function csrfToken(): string | undefined {
  const part = document.cookie.split('; ').find((value) => value.startsWith('igame_csrf='));
  return part ? decodeURIComponent(part.split('=').slice(1).join('=')) : undefined;
}

function unwrap<T>(body: Envelope<T>): T {
  if (body && typeof body === 'object') {
    if ('data' in body) return body.data;
    if ('user' in body) return body.user;
    if ('item' in body) return body.item;
    if ('game' in body) return body.game;
  }
  return body as T;
}

function normalizeUser(user: User): User {
  const role = user.role;
  return { ...user, display_name: user.display_name || user.username, roles: user.roles?.length ? user.roles : role ? [role] : [] };
}

function normalizeGame(game: Game): Game {
  return {
    ...game,
    slug: game.slug || game.id,
    category: game.category || game.category_name || '기타',
    tags: game.tags ?? [],
    ranking: game.ranking ?? game.ranking_enabled ?? false,
    achievement: game.achievement ?? game.achievement_enabled ?? false,
    game_type: game.game_type === 'embedded' && ['2048', 'snake', 'memory', 'reaction', 'typing'].includes(game.slug || game.id) ? 'builtin' : game.game_type,
  };
}

function normalizeKey(key: PersonalKey): PersonalKey {
  return { ...key, permissions: key.permissions ?? [], status: key.revoked_at ? 'revoked' : (key.status ?? 'active') };
}

function normalizeRanking(entry: RankingEntry): RankingEntry {
  return { ...entry, display_name: entry.display_name || entry.name || entry.team || entry.department || '익명' };
}

interface KeyMutationResponse { secret?: string; key?: string; api_key?: PersonalKey; item?: PersonalKey }
interface KeyListResponse extends ApiList<PersonalKey> { available_permissions?: string[] }

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  const csrf = csrfToken();
  if (csrf) headers.set('X-CSRF-Token', csrf);
  let response: Response;
  try {
    response = await fetch(path, { ...init, headers, credentials: 'include' });
  } catch (cause) {
    throw new ApiError('서버에 연결할 수 없습니다. 네트워크와 서비스 상태를 확인해 주세요.', 0, 'network_error', cause);
  }
  const contentType = response.headers.get('content-type') ?? '';
  const body = response.status === 204
    ? undefined
    : contentType.includes('application/json')
      ? await response.json().catch(() => undefined)
      : await response.text().catch(() => undefined);
  if (!response.ok) {
    const apiError = body && typeof body === 'object' && 'error' in body
      ? (body as { error?: { message?: string; code?: string } }).error
      : undefined;
    const message = apiError?.message || (typeof body === 'string' && body) || `요청을 처리하지 못했습니다. (${response.status})`;
    throw new ApiError(message, response.status, apiError?.code, body);
  }
  return unwrap<T>(body as Envelope<T>);
}

async function list<T>(path: string): Promise<ApiList<T>> {
  const body = await request<ApiList<T> | T[]>(path);
  return Array.isArray(body) ? { items: body } : body;
}

export interface StreamOptions {
  signal?: AbortSignal;
  onToken: (text: string) => void;
  onEvent?: (event: string, data: unknown) => void;
}

/** Reads both SSE and newline-delimited streaming AI responses. */
export async function streamAI(
  path: string,
  payload: Record<string, unknown>,
  { signal, onToken, onEvent }: StreamOptions,
): Promise<void> {
  const csrf = csrfToken();
  const response = await fetch(path, {
    method: 'POST', credentials: 'include', signal,
    headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream', ...(csrf ? { 'X-CSRF-Token': csrf } : {}) },
    body: JSON.stringify({ ...payload, stream: true }),
  }).catch((cause) => { throw new ApiError('AI 서버에 연결할 수 없습니다.', 0, 'network_error', cause); });
  if (!response.ok || !response.body) {
    throw new ApiError(`AI 응답을 시작하지 못했습니다. (${response.status})`, response.status, 'stream_failed');
  }
  const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = '';
  let eventName = '';
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += value;
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';
    for (const raw of lines) {
      const line = raw.trim();
      if (!line || line.startsWith(':')) continue;
      if (line.startsWith('event:')) { eventName = line.slice(6).trim(); continue; }
      const content = line.startsWith('data:') ? line.slice(5).trim() : line;
      if (content === '[DONE]') continue;
      try {
        const data = JSON.parse(content) as { token?: string; text?: string; output_text?: string; event?: string; data?: unknown; choices?: Array<{ text?: string; delta?: { content?: string } }> };
        const token = data.token ?? data.text ?? data.output_text ?? data.choices?.[0]?.delta?.content ?? data.choices?.[0]?.text;
        if (token) onToken(token);
        if (data.event || eventName) onEvent?.(data.event ?? eventName, data.data ?? data);
        eventName = '';
      } catch { onToken(content); }
    }
  }
  if (buffer.trim()) onToken(buffer.trim().replace(/^data:\s*/, ''));
}

export const api = {
  request,
  publicConfig: () => request<PublicConfig>('/api/v1/public/config'),
  version: async (): Promise<VersionInfo> => {
    const raw = await request<Record<string, string>>('/api/v1/version');
    const commit = raw.commit ?? raw.Commit;
    const buildDate = raw.build_date ?? raw.buildDate ?? raw.BuildDate;
    return {
      version: raw.version ?? raw.Version ?? 'dev',
      commit: commit && commit.toLowerCase() !== 'unknown' ? commit : undefined,
      buildDate: buildDate && buildDate.toLowerCase() !== 'unknown' && !Number.isNaN(new Date(buildDate).valueOf()) ? buildDate : undefined,
    };
  },
  me: async () => normalizeUser(await request<User>('/api/v1/me')),
  login: async (username: string, password: string) => normalizeUser(await request<User>('/api/v1/auth/login', {
    method: 'POST', body: JSON.stringify({ username, password }),
  })),
  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST' }),
  games: async () => { const result = await list<Game>('/api/v1/games'); return { ...result, items: result.items.map(normalizeGame) }; },
  game: async (id: string) => normalizeGame(await request<Game>(`/api/v1/games/${encodeURIComponent(id)}`)),
  rankings: async (gameId = '', period = 'weekly', scope = 'individual') => {
    const result = await list<RankingEntry>(`/api/v1/rankings?${new URLSearchParams({ game_id: gameId, period: period === 'all' ? 'all_time' : period, group: scope })}`);
    return { ...result, items: result.items.map(normalizeRanking) };
  },
  joinEvent: (eventId: string) => request<{ joined: boolean; event_id: string }>(`/api/v1/events/${encodeURIComponent(eventId)}/join`, { method: 'POST' }),
  events: () => list<{ id: string; name: string; description?: string; status?: string; starts_at?: string; ends_at?: string; event_type?: string; participant_count?: number; joined?: boolean }>('/api/v1/events'),
  workflowReviews: () => list<{ id: string; requester_username: string; action: string; resource_type: string; resource_id?: string; payload?: Record<string, unknown>; status: string; comment?: string; created_at: string }>('/api/v1/workflow/reviews'),
  reviewWorkflow: (id: string, decision: 'approved' | 'rejected', comment: string) => request<{ id: string; status: string }>(`/api/v1/workflow/requests/${encodeURIComponent(id)}/review`, { method: 'POST', body: JSON.stringify({ decision, comment }) }),
  submitWorkflow: (input: { action: 'create' | 'update'; resource_type: 'game'; resource_id?: string; payload: Record<string, unknown> }) => request<{ status?: string; resource_id?: string; approval_required?: boolean; request?: { id: string; status: string } }>('/api/v1/workflow/requests', { method: 'POST', body: JSON.stringify(input) }),
  myWorkflowRequests: () => list<{ id: string; action: string; resource_type: string; resource_id?: string; status: string; comment?: string; created_at: string; reviewed_at?: string; applied_at?: string }>('/api/v1/workflow/requests'),
  notices: () => list<{ id: string; title: string; content: string; pinned?: boolean; published_at?: string }>('/api/v1/notices'),
  banners: () => list<{ id: string; title: string; image_url: string; link_url?: string; starts_at?: string; ends_at?: string; sort_order?: number }>('/api/v1/banners'),
  toggleFavorite: (gameId: string, favorite: boolean) => request<void>(`/api/v1/games/${encodeURIComponent(gameId)}/favorite`, {
    method: favorite ? 'POST' : 'DELETE',
  }),
  updateProfile: async (input: Partial<User> & { ranking_opt_out?: boolean }) => normalizeUser(await request<User>('/api/v1/me', {
    method: 'PATCH', body: JSON.stringify(input),
  })),
  playHistory: () => list<{ id: string; game_name: string; game_slug: string; status: string; started_at: string; duration_ms?: number; score?: number; verified?: boolean }>('/api/v1/me/history'),
  myAchievements: () => list<{ id?: string; code: string; name: string; description?: string; unlocked_at?: string }>('/api/v1/me/achievements'),
  personalKeys: async () => { const result = await request<KeyListResponse>('/api/v1/me/api-keys'); return { ...result, items: (result.items ?? []).map(normalizeKey) }; },
  rotatePersonalKey: async (id: string) => { const result = await request<KeyMutationResponse>(`/api/v1/me/api-keys/${encodeURIComponent(id)}/rotate`, { method: 'POST' }); return { secret: result.secret ?? result.key ?? '', api_key: result.api_key ?? result.item }; },
  createPersonalKey: async (input: { name: string; permissions: string[]; expires_at?: string }) => {
    const result = await request<KeyMutationResponse>('/api/v1/me/api-keys', { method: 'POST', body: JSON.stringify(input) });
    return { secret: result.secret ?? result.key ?? '', api_key: result.api_key ?? result.item };
  },
  revokePersonalKey: (id: string) => request<void>(`/api/v1/me/api-keys/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  updatePersonalKey: (id: string, input: { name: string; permissions: string[]; expires_at?: string }) => request<void>(`/api/v1/me/api-keys/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }),
  changePassword: (current_password: string, new_password: string) => request<void>('/api/v1/me/password', { method: 'PUT', body: JSON.stringify({ current_password, new_password }) }),
  adminList: <T>(resource: string) => list<T>(`/api/v1/admin/${resource}`),
  adminCreate: <T>(resource: string, input: unknown) => request<T>(`/api/v1/admin/${resource}`, { method: 'POST', body: JSON.stringify(input) }),
  adminUpdate: <T>(resource: string, id: string, input: unknown) => request<T>(`/api/v1/admin/${resource}/${encodeURIComponent(id)}`, { method: resource === 'users' ? 'PATCH' : 'PUT', body: JSON.stringify(input) }),
  adminDelete: (resource: string, id: string) => request<void>(`/api/v1/admin/${resource}/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  adminSettings: () => request<{ settings: Record<string, Record<string, unknown>>; updated_at?: Record<string, string> }>('/api/v1/admin/settings'),
  saveAdminSetting: (key: string, value: unknown) => request<Record<string, unknown>>(`/api/v1/admin/settings/${encodeURIComponent(key)}`, { method: 'PUT', body: JSON.stringify(['oidc', 'ai'].includes(key) ? value : { value }) }),
};
