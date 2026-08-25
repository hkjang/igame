import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../api/client";
import {
  defenseAPI,
  defenseStudioAPI,
  DefenseConfigError,
  normalizeDefenseConfig,
  normalizeDefenseLearningBreakdown,
  normalizeDefenseLearningReport,
} from "./api";

function envelope() {
  return {
    game: {
      slug: "cyber-fortress",
      name: "Cyber Fortress",
      education_enabled: true,
    },
    version: {
      id: "11111111-1111-4111-8111-111111111111",
      version_no: 1,
      label: "v0.3.0",
      status: "published",
      content_version: "0.3.0",
      policy_version: "security-policy-1",
      asset_version: "procedural-1",
      checksum: "abc",
    },
    content: {
      schema_version: "0.3.0",
      stages: [
        {
          id: "stage-1",
          number: 1,
          name: "Scenario 1",
          mode: "campaign",
          theme: "void",
          map_style: "cyber-vault",
          starting_health: 20,
          starting_resource: 250,
          version: "1.0.0",
          path: [
            { x: -30, y: 220 },
            { x: 640, y: 220 },
            { x: 1310, y: 420 },
          ],
          tower_spots: [
            { id: "a", x: 120, y: 100 },
            { id: "b", x: 320, y: 340 },
            { id: "c", x: 700, y: 160 },
            { id: "d", x: 980, y: 500 },
          ],
        },
      ],
      waves: [
        {
          id: "wave-1",
          stage_id: "stage-1",
          number: 1,
          reward: 30,
          entries: [{ enemy: "threat-1", count: 4, interval: 0.7 }],
        },
      ],
      towers: [
        {
          id: "control-1",
          name: "WAF",
          role: "web defense",
          cost: 80,
          damage: 20,
          range: 130,
          fire_rate: 0.7,
          projectile_speed: 380,
          damage_type: "magic",
          effective_against: ["web_attack"],
          effective_multiplier: 1.8,
          branches: [
            { id: "precision", name: "Precision", description: "single" },
            { id: "network", name: "Network", description: "area" },
          ],
        },
      ],
      enemies: [
        {
          id: "threat-1",
          name: "SQL Injection",
          hp: 100,
          speed: 45,
          armor: 0.1,
          reward: 10,
          health_damage: 1,
          radius: 14,
          traits: [],
          threat_type: "web_attack",
        },
      ],
      bosses: [],
      heroes: [
        {
          id: "hero-1",
          name: "Analyst",
          title: "Security Analyst",
          role: "ranged",
          hp: 500,
          damage: 30,
          range: 110,
          speed: 130,
          respawn_seconds: 9,
          unlock_stage: 1,
          skill1: "Observe",
          skill2: "Contain",
          ultimate: "Recover",
        },
      ],
      skills: [
        {
          id: "s1",
          name: "Isolate",
          description: "Isolate threats",
          effect: "area_damage",
          cooldown: 20,
        },
        {
          id: "s2",
          name: "Team",
          description: "Call the team",
          effect: "reinforcement",
          cooldown: 28,
        },
        {
          id: "s3",
          name: "Block",
          description: "Block threats",
          effect: "freeze",
          cooldown: 36,
        },
      ],
      events: [
        {
          id: "event-1",
          stage_id: "stage-1",
          trigger: "battle-start",
          education_id: "question-1",
          reward: { resource: 10 },
        },
      ],
      education: [
        {
          id: "question-1",
          topic: "response",
          question: "가장 안전한 대응은?",
          answers: [
            { id: "a", text: "A" },
            { id: "b", text: "B" },
          ],
        },
      ],
      balance: {
        difficulties: {
          casual: {
            difficulty_bonus: 0,
            enemy_hp: 0.82,
            enemy_speed: 0.92,
            gold: 1.18,
            score: 0.8,
          },
          normal: {
            difficulty_bonus: 5000,
            enemy_hp: 1,
            enemy_speed: 1,
            gold: 1,
            score: 1,
          },
          veteran: {
            difficulty_bonus: 10000,
            enemy_hp: 1.38,
            enemy_speed: 1.12,
            gold: 0.9,
            score: 1.5,
          },
        },
        clear_time_target_ms: 900000,
        clear_time_bonus_divisor: 100,
        health_score_factor: 1000,
        resource_score_factor: 10,
        wave_score_factor: 100,
        min_wave_duration_ms: 100,
        duration_tolerance_ms: 5000,
        tower_upgrade_cost: [0, 70, 120],
        sell_refund_rate: 0.65,
      },
      campaigns: [],
    },
  };
}

function currentEnvelope() {
  const raw = envelope();
  raw.version.label = "v0.4.0";
  raw.version.content_version = "0.4.0";
  raw.version.asset_version = "procedural-defense-2";
  raw.content.schema_version = "0.4.0";
  return raw;
}

describe("Defense result normalization", () => {
  it("normalizes current object maps, legacy scores, and arrays to render-safe topic rows", () => {
    expect(
      normalizeDefenseLearningBreakdown({
        llm_quality: { correct: 1, total: 2, score: 50 },
        ai_security: 75,
      }),
    ).toEqual([
      { topic: "ai_security", correct: 0, total: 0, score: 75 },
      { topic: "llm_quality", correct: 1, total: 2, score: 50 },
    ]);
    expect(
      normalizeDefenseLearningBreakdown([
        { topic: "governance", correct: 3, total: 2, score: 120 },
      ]),
    ).toEqual([
      { topic: "governance", correct: 2, total: 2, score: 100 },
    ]);
  });
});

describe("Defense canonical config adapter", () => {
  afterEach(() => vi.restoreAllMocks());
  it("preserves server ids and joins global waves and sanitized education events", () => {
    const normalized = normalizeDefenseConfig(
      envelope(),
      "cyber-fortress",
    ).pack;
    expect(normalized.config.versionId).toBe(
      "11111111-1111-4111-8111-111111111111",
    );
    expect(normalized.config.stages[0].waves[0].entries[0].enemy).toBe(
      "threat-1",
    );
    expect(normalized.config.stages[0].path).toEqual([
      { x: -30, y: 220 },
      { x: 640, y: 220 },
      { x: 1310, y: 420 },
    ]);
    expect(normalized.config.stages[0].mapStyle).toBe("cyber-vault");
    expect(normalized.config.towers[0]).toMatchObject({
      effectiveAgainst: ["web_attack"],
      effectiveMultiplier: 1.8,
    });
    expect(normalized.events[0]).toMatchObject({
      id: "event-1",
      question: "가장 안전한 대응은?",
    });
    expect(normalized.events[0].answers).toEqual(
      expect.arrayContaining([{ id: "a", text: "A" }]),
    );
    expect(normalized.events[0]).not.toHaveProperty("correct_answer_id");
    expect(normalized.config.heroes[0].unlockStage).toBe(1);
  });

  it("rejects missing server geometry instead of borrowing practice geometry", () => {
    const raw = envelope();
    delete (raw.content.stages[0] as { path?: unknown }).path;
    expect(() => normalizeDefenseConfig(raw, "cyber-fortress")).toThrow(
      DefenseConfigError,
    );
  });

  it("rejects wave lanes outside the selected stage geometry", () => {
    for (const pathIndex of [-1, 1]) {
      const raw = currentEnvelope();
      (
        raw.content.waves[0].entries[0] as
          (typeof raw.content.waves)[0]["entries"][0] & { path_index?: number }
      ).path_index = pathIndex;
      expect(() => normalizeDefenseConfig(raw, "cyber-fortress")).toThrow(
        DefenseConfigError,
      );
    }
  });

  it("preserves valid wave routing and rejects unsafe delay, parallel, and modifier fields", () => {
    const valid = currentEnvelope();
    const validEntry = valid.content.waves[0].entries[0] as unknown as Record<
      string,
      unknown
    >;
    validEntry.delay = 1.5;
    validEntry.path_index = 0;
    validEntry.parallel = true;
    validEntry.modifiers = ["armored", "swift", "immune_stun"];
    expect(
      normalizeDefenseConfig(valid, "cyber-fortress").pack.config.stages[0]
        .waves[0].entries[0],
    ).toMatchObject({
      delay: 1.5,
      pathIndex: 0,
      parallel: true,
      modifiers: ["armored", "swift", "immune_stun"],
    });

    const invalidMutations: Array<(entry: Record<string, unknown>) => void> = [
      (entry) => {
        entry.delay = -0.01;
      },
      (entry) => {
        entry.delay = 3600.01;
      },
      (entry) => {
        entry.delay = "1";
      },
      (entry) => {
        entry.path_index = 0.5;
      },
      (entry) => {
        entry.parallel = "true";
      },
      (entry) => {
        entry.modifiers = ["unknown"];
      },
      (entry) => {
        entry.modifiers = ["armored", "armored"];
      },
      (entry) => {
        entry.modifiers = "swift";
      },
    ];
    for (const mutate of invalidMutations) {
      const raw = currentEnvelope();
      mutate(
        raw.content.waves[0].entries[0] as unknown as Record<string, unknown>,
      );
      expect(() => normalizeDefenseConfig(raw, "cyber-fortress")).toThrow(
        DefenseConfigError,
      );
    }
  });

  it("keeps already-published 0.3 custom packs playable through legacy wave normalization", () => {
    const legacy = envelope();
    const entry = legacy.content.waves[0].entries[0] as unknown as Record<
      string,
      unknown
    >;
    entry.delay = -2;
    entry.path_index = -1;
    entry.parallel = "true";
    entry.modifiers = ["regenerating", "healer"];
    expect(
      normalizeDefenseConfig(legacy, "cyber-fortress").pack.config.stages[0]
        .waves[0].entries[0],
    ).toMatchObject({
      delay: 0,
      pathIndex: 0,
      parallel: false,
      modifiers: ["regenerating", "healer"],
    });
  });

  it("matches the 0.4 server contract for nulls and ignores legacy aliases", () => {
    const current = currentEnvelope();
    const entry = current.content.waves[0].entries[0] as unknown as Record<
      string,
      unknown
    >;
    entry.delay = null;
    entry.delay_ms = 5000;
    entry.path_index = null;
    entry.pathIndex = 3;
    entry.parallel = null;
    entry.modifiers = null;
    expect(
      normalizeDefenseConfig(current, "cyber-fortress").pack.config.stages[0]
        .waves[0].entries[0],
    ).toMatchObject({ delay: 0, pathIndex: 0, parallel: false, modifiers: [] });
  });

  it("preserves true damage and rejects non-canonical damage enums", () => {
    const trueDamage = envelope();
    trueDamage.content.towers[0].damage_type = "true";
    expect(
      normalizeDefenseConfig(trueDamage, "cyber-fortress").pack.config.towers[0]
        .damageType,
    ).toBe("true");
    const legacy = envelope();
    legacy.content.towers[0].damage_type = "special";
    expect(() => normalizeDefenseConfig(legacy, "cyber-fortress")).toThrow(
      DefenseConfigError,
    );
  });

  it("rejects missing tower branches and dangling wave references", () => {
    const noBranches = envelope();
    noBranches.content.towers[0].branches = [];
    expect(() => normalizeDefenseConfig(noBranches, "cyber-fortress")).toThrow(
      DefenseConfigError,
    );
    const dangling = envelope();
    dangling.content.waves[0].entries[0].enemy = "missing";
    expect(() => normalizeDefenseConfig(dangling, "cyber-fortress")).toThrow(
      DefenseConfigError,
    );
  });

  it("rejects a missing authoritative hero unlock stage", () => {
    const raw = envelope();
    delete (raw.content.heroes[0] as { unlock_stage?: number }).unlock_stage;
    expect(() => normalizeDefenseConfig(raw, "cyber-fortress")).toThrow(
      DefenseConfigError,
    );
  });

  it("rejects a missing authoritative difficulty multiplier instead of using practice balance", () => {
    const raw = envelope();
    delete (raw.content.balance.difficulties.casual as { gold?: number }).gold;
    expect(() => normalizeDefenseConfig(raw, "cyber-fortress")).toThrow(
      DefenseConfigError,
    );
  });

  it("rejects finite but unsafe runtime values above server bounds", () => {
    const mutations: Array<(raw: ReturnType<typeof envelope>) => void> = [
      (raw) => {
        raw.content.towers[0].damage = 1e308;
      },
      (raw) => {
        (
          raw.content.towers[0].branches[0] as (typeof raw.content.towers)[0]["branches"][0] & {
            splash?: number;
          }
        ).splash = 1e308;
      },
      (raw) => {
        raw.content.heroes[0].hp = 1e308;
      },
      (raw) => {
        raw.content.balance.difficulties.veteran.enemy_hp = 1e308;
      },
    ];
    for (const mutate of mutations) {
      const raw = envelope();
      mutate(raw);
      expect(() => normalizeDefenseConfig(raw, "cyber-fortress")).toThrow(
        DefenseConfigError,
      );
    }
  });

  it("normalizes learning campaign objects instead of collapsing the game envelope", () => {
    const report = normalizeDefenseLearningReport({
      game: { id: "g1", slug: "cyber-fortress", name: "Cyber Fortress" },
      overall_score: 80,
      topics: [{ topic: "phishing", correct: 4, total: 5, score: 80 }],
      completed_campaigns: [
        {
          campaign_id: "campaign-1",
          completed: true,
          completed_stages: 10,
          required_stages: 10,
          learning_score: 80,
        },
      ],
    });
    expect(report.completed_campaigns).toEqual([
      expect.objectContaining({ campaign_id: "campaign-1", completed: true }),
    ]);
  });

  it("extracts the version field from the public version envelope", async () => {
    const version = envelope().version;
    vi.spyOn(api, "requestEnvelope").mockResolvedValue({ version });
    await expect(defenseAPI.version("cyber-fortress")).resolves.toEqual(
      version,
    );
  });

  it("sends policy and rollback source when creating a managed draft", async () => {
    const version = envelope().version;
    const request = vi
      .spyOn(api, "request")
      .mockResolvedValue({ version });
    await defenseStudioAPI.createVersion("cyber-fortress", {
      label: "정책 갱신",
      policy_version: "security-policy-2",
      source_version_id: version.id,
    });
    expect(request).toHaveBeenCalledWith(
      "/api/v1/admin/defense/cyber-fortress/versions",
      expect.objectContaining({
        method: "POST",
        body: expect.any(String),
      }),
    );
    const body = JSON.parse(
      (request.mock.calls[0][1] as { body: string }).body,
    ) as Record<string, unknown>;
    expect(body).toMatchObject({
      policy_version: "security-policy-2",
      source_version_id: version.id,
    });
  });
});
