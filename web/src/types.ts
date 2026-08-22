export interface User {
  id: string;
  username: string;
  display_name: string;
  nickname?: string;
  email?: string;
  department?: string;
  team?: string;
  avatar_url?: string;
  ranking_opt_out?: boolean;
  role?: string;
  roles: string[];
  level?: number;
  xp?: number;
}

export interface PublicConfig {
  name: string;
  display_name?: string;
  version: string;
  oidc_enabled: boolean;
  oidc_login_url: string;
  bootstrap_login_enabled?: boolean;
  approval_enabled?: boolean;
  ai_enabled?: boolean;
}

export interface VersionInfo {
  version: string;
  commit?: string;
  buildDate?: string;
}

export type GameStatus = 'draft' | 'active' | 'maintenance' | 'disabled';

export interface Game {
  id: string;
  slug: string;
  name: string;
  description: string;
  category: string;
  category_name?: string;
  tags: string[];
  game_url?: string;
  game_type: 'builtin' | 'embedded' | 'iframe' | 'external';
  status: GameStatus;
  ranking: boolean;
  achievement: boolean;
  ranking_enabled?: boolean;
  achievement_enabled?: boolean;
  version: string;
  developer?: string;
  accent?: string;
  icon?: string;
  thumbnail?: string;
  banner?: string;
  plays?: number;
  favorite?: boolean;
}

export interface RankingEntry {
  rank: number;
  user_id?: string;
  display_name: string;
  name?: string;
  department?: string;
  team?: string;
  members?: number;
  score: number;
  game_name?: string;
}

export interface ApiList<T> { items: T[]; total?: number }

export interface PersonalKey {
  id: string;
  name: string;
  prefix: string;
  permissions: string[];
  created_at: string;
  last_used_at?: string;
  expires_at?: string;
  revoked_at?: string;
  status: 'active' | 'revoked';
}
