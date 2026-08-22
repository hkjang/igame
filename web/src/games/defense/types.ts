import type {
  RealmDifficulty,
  RealmGuardConfig,
  RealmResult,
} from "../realmguard/types";

export type DefenseSlug =
  "office-guardians" | "cyber-fortress" | "ai-nexus-defense";
export type DefenseSection =
  | "stages"
  | "waves"
  | "towers"
  | "enemies"
  | "bosses"
  | "heroes"
  | "skills"
  | "events"
  | "education"
  | "balance"
  | "campaigns"
  | "resource_rules"
  | "model_profiles";

export interface DefenseAnswerOption {
  id: string;
  text: string;
}

export interface DefenseEducationEvent {
  id: string;
  stage_id: string;
  trigger: string;
  topic: string;
  question: string;
  answers: DefenseAnswerOption[];
  reward?: { resource?: number; learning?: number };
  penalty?: { resource?: number; learning?: number; threat?: string };
}

export interface DefenseQuestion extends DefenseEducationEvent {
  policy_reference?: string;
}

export interface DefensePresentation {
  name: string;
  shortName: string;
  eyebrow: string;
  description: string;
  story: string;
  primary: string;
  secondary: string;
  resourceName: string;
  healthName: string;
  heroName: string;
  towerName: string;
  enemyName: string;
}

export interface AIResources {
  compute: number;
  token: number;
  trust: number;
  latency: number;
}

export interface AIModelProfile {
  id: "small" | "medium" | "large" | "reasoning" | "vision";
  name: string;
  tower_id: string;
  compute_cost: number;
  token_cost: number;
  latency_cost: number;
  accuracy: number;
  damage_multiplier: number;
}

export interface AIResourceRules {
  compute_start: number;
  token_start: number;
  trust_start: number;
  latency_max: number;
  wave_compute_cost: number;
  wave_token_cost: number;
  escaped_trust_cost: number;
  escaped_latency_cost: number;
}

export interface DefenseContentPack {
  slug: DefenseSlug;
  presentation: DefensePresentation;
  config: RealmGuardConfig;
  events: DefenseEducationEvent[];
  education: DefenseQuestion[];
  policyVersion: string;
  educationEnabled: boolean;
  modelProfiles?: AIModelProfile[];
  resourceRules?: AIResourceRules;
}

export interface DefenseVersion {
  id: string;
  version_no: number;
  label: string;
  status:
    | "draft"
    | "testing"
    | "pending_approval"
    | "approved"
    | "published"
    | "archived";
  content_version: string;
  policy_version: string;
  asset_version: string;
  checksum: string;
  notes?: string;
  created_at?: string;
  updated_at?: string;
  created_by?: string;
  review_comment?: string;
  source_version_id?: string;
}

export interface DefenseConfigEnvelope {
  game: { slug: DefenseSlug; name: string; education_enabled: boolean };
  version: DefenseVersion;
  content: Record<string, unknown>;
}

export interface DefenseStageProgress {
  stage_id: string;
  difficulty: RealmDifficulty;
  unlocked: boolean;
  completed: boolean;
  best_score: number;
  best_learning_score: number;
  attempts: number;
  completions: number;
  total_playtime_ms: number;
}

export interface DefenseProgress {
  version?: DefenseVersion;
  items: DefenseStageProgress[];
  summary: {
    completed_stages: number;
    total_stars: number;
    total_playtime_ms: number;
    campaign_complete: boolean;
  };
}

export interface DefenseAnswerSubmission {
  event_id: string;
  answer_id: string;
}

export interface DefenseLearningBreakdown {
  topic: string;
  correct: number;
  total: number;
  score: number;
}

export interface DefenseServerResult {
  result: {
    id?: string;
    score: number;
    stars: number;
    verified: boolean;
    score_breakdown?: Record<string, number>;
    learning_score: number;
    learning_breakdown?:
      | DefenseLearningBreakdown[]
      | Record<
          string,
          | number
          | Omit<DefenseLearningBreakdown, "topic">
        >;
  };
  progress?: DefenseProgress;
}

export interface DefenseRankingEntry {
  rank: number;
  display_name: string;
  department?: string;
  team?: string;
  score: number;
  learning_score?: number;
  stage_id?: string;
  difficulty?: string;
}

export interface DefenseLearningReport {
  game: { id?: string; slug: DefenseSlug; name: string };
  overall_score: number;
  topics: DefenseLearningBreakdown[];
  completed_campaigns: Array<{
    campaign_id: string;
    completed: boolean;
    completed_stages: number;
    required_stages: number;
    learning_score: number;
    completed_at?: string;
    updated_at?: string;
  }>;
}

export interface DefenseLocalCompletion {
  battle: RealmResult;
  answers: DefenseAnswerSubmission[];
  learningScore: number;
  learningBreakdown: DefenseLearningBreakdown[];
  aiResources?: AIResources;
}
