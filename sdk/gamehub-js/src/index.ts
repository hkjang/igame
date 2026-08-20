export type GameHubEvent =
  | 'game.init'
  | 'game.start'
  | 'game.pause'
  | 'game.resume'
  | 'game.finish'
  | 'score.submit'
  | 'achievement.unlock'
  | string;

export interface GameHubConfig {
  gameId: string;
  baseUrl?: string;
  accessToken?: string;
  fetch?: typeof globalThis.fetch;
  metadata?: Record<string, unknown>;
  onError?: (error: GameHubError) => void;
}

export interface GameUser {
  id: string;
  username: string;
  display_name?: string;
  department?: string;
  roles?: string[];
  [key: string]: unknown;
}

export interface GameSession {
  id: string;
  session_token?: string;
  game_id?: string;
  started_at?: string;
  status?: string;
  [key: string]: unknown;
}

export interface ScoreInput {
  score: number;
  metadata?: Record<string, unknown>;
  proof?: string | Record<string, unknown>;
}

export interface FinishInput extends ScoreInput {
  duration?: number;
  result?: Record<string, unknown>;
}

export interface LeaderboardOptions {
  period?: 'daily' | 'weekly' | 'monthly' | 'season' | 'all';
  limit?: number;
  scope?: 'individual' | 'team' | 'department';
}

export interface TelemetryInput {
  event: GameHubEvent;
  payload?: Record<string, unknown>;
  occurredAt?: string;
}

interface ApiEnvelope<T> {
  data?: T;
  user?: T;
  session?: T;
  event?: T;
  item?: T;
  items?: T;
  error?: { code?: string; message?: string };
}

export class GameHubError extends Error {
  readonly status?: number;
  readonly code?: string;
  readonly details?: unknown;

  constructor(message: string, options: { status?: number; code?: string; details?: unknown } = {}) {
    super(message);
    this.name = 'GameHubError';
    this.status = options.status;
    this.code = options.code;
    this.details = options.details;
  }
}

function unwrap<T>(body: ApiEnvelope<T> | T): T {
  if (body && typeof body === 'object') {
    const envelope = body as ApiEnvelope<T>;
    if (envelope.data !== undefined) return envelope.data;
    if (envelope.session !== undefined) return envelope.session;
    if (envelope.user !== undefined) return envelope.user;
    if (envelope.event !== undefined) return envelope.event;
    if (envelope.item !== undefined) return envelope.item;
    if (envelope.items !== undefined) return envelope.items;
  }
  return body as T;
}

function getCookie(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined;
  const pair = document.cookie.split('; ').find((value) => value.startsWith(`${name}=`));
  return pair ? decodeURIComponent(pair.slice(name.length + 1)) : undefined;
}

/**
 * Creates one runtime client per game instance. The SDK never stores an AI or
 * service API key in the browser; authentication is delegated to the portal.
 */
export class GameHubClient {
  readonly gameId: string;
  private readonly baseUrl: string;
  private readonly accessToken?: string;
  private readonly fetcher: typeof globalThis.fetch;
  private readonly metadata: Record<string, unknown>;
  private readonly onError?: (error: GameHubError) => void;
  private currentSession?: GameSession;
  private scoreSubmitted = false;

  constructor(config: GameHubConfig) {
    if (!config.gameId.trim()) throw new GameHubError('gameId is required', { code: 'invalid_config' });
    this.gameId = config.gameId;
    this.baseUrl = (config.baseUrl ?? '').replace(/\/$/, '');
    this.accessToken = config.accessToken;
    this.fetcher = config.fetch ?? globalThis.fetch.bind(globalThis);
    this.metadata = config.metadata ?? {};
    this.onError = config.onError;
  }

  get session(): Readonly<GameSession> | undefined {
    return this.currentSession;
  }

  async init(): Promise<{ user: GameUser; gameId: string }> {
    const user = await this.getUser();
    return { user, gameId: this.gameId };
  }

  async getUser(): Promise<GameUser> {
    return this.request<GameUser>('/api/v1/me');
  }

  async start(metadata: Record<string, unknown> = {}): Promise<GameSession> {
    const session = await this.request<GameSession>(
      `/api/v1/games/${encodeURIComponent(this.gameId)}/sessions`,
      { method: 'POST', body: JSON.stringify({ metadata: { ...this.metadata, ...metadata } }) },
    );
    this.currentSession = session;
    this.scoreSubmitted = false;
    return session;
  }

  async pause(): Promise<void> {
    await this.telemetry({ event: 'game.pause' });
  }

  async resume(): Promise<void> {
    await this.telemetry({ event: 'game.resume' });
  }

  async submitScore(input: ScoreInput | number): Promise<unknown> {
    const value = typeof input === 'number' ? { score: input } : input;
    const sessionId = this.requireSession();
    const response = await this.request('/api/v1/scores', {
      method: 'POST',
      body: JSON.stringify({
        session_id: sessionId,
        session_token: this.currentSession?.session_token,
        game_id: this.gameId,
        score: value.score,
        metadata: value.metadata ?? {},
        proof: typeof value.proof === 'string' ? value.proof : value.proof ? JSON.stringify(value.proof) : undefined,
      }),
    });
    this.scoreSubmitted = true;
    return response;
  }

  async submitResult(result: FinishInput): Promise<unknown> {
    await this.submitScore(result);
    return this.finish(result);
  }

  async unlockAchievement(code: string, metadata: Record<string, unknown> = {}): Promise<unknown> {
    return this.request('/api/v1/me/achievements', {
      method: 'POST',
      body: JSON.stringify({ code, game_id: this.gameId, session_id: this.currentSession?.id, session_token: this.currentSession?.session_token, metadata }),
    });
  }

  async getLeaderboard(options: LeaderboardOptions = {}): Promise<unknown> {
    const query = new URLSearchParams({ game_id: this.gameId });
    if (options.period) query.set('period', options.period);
    if (options.limit) query.set('limit', String(options.limit));
    if (options.scope) query.set('group', options.scope);
    return this.request(`/api/v1/rankings?${query.toString()}`);
  }

  async getEvent(eventId?: string): Promise<unknown> {
    const path = eventId ? `/api/v1/events/${encodeURIComponent(eventId)}` : '/api/v1/events';
    return this.request(path);
  }

  async finish(input: Partial<FinishInput> = {}): Promise<unknown> {
    const sessionId = this.requireSession();
    // Score submission atomically validates and closes the backend session.
    // A following finish() stays idempotent from the caller's perspective.
    if (this.scoreSubmitted) {
      this.currentSession = undefined;
      this.scoreSubmitted = false;
      return { finished: true, submitted: true };
    }
    const response = await this.request(`/api/v1/sessions/${encodeURIComponent(sessionId)}/finish`, {
      method: 'POST',
      body: JSON.stringify({
        game_id: this.gameId,
        session_token: this.currentSession?.session_token,
        score: input.score,
        duration_ms: input.duration === undefined ? undefined : Math.round(input.duration * 1000),
        result: input.result ?? input.metadata ?? {},
      }),
    });
    this.currentSession = undefined;
    this.scoreSubmitted = false;
    return response;
  }

  async telemetry(input: TelemetryInput): Promise<unknown> {
    return this.request('/api/v1/telemetry', {
      method: 'POST',
      body: JSON.stringify({
        game_id: this.gameId,
        session_id: this.currentSession?.id,
        session_token: this.currentSession?.session_token,
        event: input.event,
        data: input.payload ?? {},
        occurred_at: input.occurredAt ?? new Date().toISOString(),
      }),
    });
  }

  private requireSession(): string {
    if (!this.currentSession?.id) {
      throw new GameHubError('Call start() before submitting a score or result.', { code: 'session_required' });
    }
    return this.currentSession.id;
  }

  private async request<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
    if (this.accessToken) headers.set('Authorization', `Bearer ${this.accessToken}`);
    const csrf = getCookie('igame_csrf');
    if (csrf) headers.set('X-CSRF-Token', csrf);

    try {
      const response = await this.fetcher(`${this.baseUrl}${path}`, {
        ...init,
        headers,
        credentials: init.credentials ?? 'include',
      });
      const contentType = response.headers.get('content-type') ?? '';
      const body = contentType.includes('application/json') ? await response.json() : await response.text();
      if (!response.ok) {
        const envelope = body as ApiEnvelope<T>;
        throw new GameHubError(envelope?.error?.message ?? `Request failed (${response.status})`, {
          status: response.status,
          code: envelope?.error?.code,
          details: body,
        });
      }
      if (response.status === 204) return undefined as T;
      return unwrap<T>(body as ApiEnvelope<T> | T);
    } catch (cause) {
      const error = cause instanceof GameHubError
        ? cause
        : new GameHubError(cause instanceof Error ? cause.message : 'Network request failed', {
            code: 'network_error',
            details: cause,
          });
      this.onError?.(error);
      throw error;
    }
  }
}

export function createGameHub(config: GameHubConfig): GameHubClient {
  return new GameHubClient(config);
}

let defaultClient: GameHubClient | undefined;

/** Compatibility facade for small games that prefer GameHub.init/start style calls. */
export const GameHub = {
  init(config: GameHubConfig) {
    defaultClient = createGameHub(config);
    return defaultClient.init();
  },
  client(): GameHubClient {
    if (!defaultClient) throw new GameHubError('Call GameHub.init() first.', { code: 'not_initialized' });
    return defaultClient;
  },
  start(metadata?: Record<string, unknown>) { return this.client().start(metadata); },
  pause() { return this.client().pause(); },
  resume() { return this.client().resume(); },
  submitScore(input: ScoreInput | number) { return this.client().submitScore(input); },
  submitResult(input: FinishInput) { return this.client().submitResult(input); },
  unlockAchievement(code: string, metadata?: Record<string, unknown>) {
    return this.client().unlockAchievement(code, metadata);
  },
  getLeaderboard(options?: LeaderboardOptions) { return this.client().getLeaderboard(options); },
  getEvent(eventId?: string) { return this.client().getEvent(eventId); },
  getUser() { return this.client().getUser(); },
  finish(input?: Partial<FinishInput>) { return this.client().finish(input); },
  telemetry(input: TelemetryInput) { return this.client().telemetry(input); },
};

export default GameHub;
