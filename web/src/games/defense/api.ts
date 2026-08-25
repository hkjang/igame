import { api } from "../../api/client";
import type {
  EnemyArchetype,
  HeroDefinition,
  RealmBalance,
  RealmDifficulty,
  RealmStage,
  RealmWave,
  SkillDefinition,
  TowerDefinition,
} from "../realmguard/types";
import { DEFENSE_PACKS, DEFENSE_SERIES_VERSION } from "./content";
import type {
  AIModelProfile,
  AIResourceRules,
  DefenseConfigEnvelope,
  DefenseContentPack,
  DefenseEducationEvent,
  DefenseLearningBreakdown,
  DefenseLearningReport,
  DefenseProgress,
  DefenseRankingEntry,
  DefenseSection,
  DefenseServerResult,
  DefenseSlug,
  DefenseVersion,
} from "./types";

type UnknownRecord = Record<string, unknown>;

const asRecord = (value: unknown): UnknownRecord =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as UnknownRecord)
    : {};
const asArray = (value: unknown): unknown[] =>
  Array.isArray(value) ? value : [];
const asString = (value: unknown, fallback = "") =>
  typeof value === "string" && value ? value : fallback;
const asNumber = (value: unknown, fallback = 0) =>
  Number.isFinite(Number(value)) ? Number(value) : fallback;
const asBoolean = (value: unknown, fallback = false) =>
  typeof value === "boolean" ? value : fallback;
const colorAt = (index: number) =>
  [
    0x65d6ff, 0x72e0a6, 0xffc866, 0xb694ff, 0xff7c91, 0x67e8db, 0xf49b67,
    0x91a7ff,
  ][index % 8];
const DEFENSE_WAVE_MODIFIERS = new Set([
  "armored",
  "swift",
  "flying",
  "magic_resist",
  "stealth",
  "berserk",
  "immune_stun",
]);

function usesStrictWaveRoutingSchema(schemaVersion: string) {
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(schemaVersion);
  if (!match) return false;
  const major = Number(match[1]);
  const minor = Number(match[2]);
  return major > 0 || minor >= 4;
}

export class DefenseConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "DefenseConfigError";
  }
}

function normalizeVersion(value: unknown): DefenseVersion {
  const raw = asRecord(value);
  return {
    id: asString(raw.id),
    version_no: asNumber(raw.version_no),
    label: asString(raw.label, `v${asString(raw.content_version, DEFENSE_SERIES_VERSION)}`),
    status: asString(raw.status, "published") as DefenseVersion["status"],
    content_version: asString(raw.content_version, DEFENSE_SERIES_VERSION),
    policy_version: asString(raw.policy_version),
    asset_version: asString(raw.asset_version, "procedural-defense-2"),
    checksum: asString(raw.checksum),
    notes: asString(raw.notes),
    created_at: asString(raw.created_at),
    updated_at: asString(raw.updated_at),
    created_by: asString(raw.created_by),
    review_comment: asString(raw.review_comment),
    source_version_id: asString(raw.source_version_id) || undefined,
  };
}

function normalizeEnemy(
  value: unknown,
  index: number,
  boss: boolean,
): EnemyArchetype {
  const raw = asRecord(value);
  const traits = asArray(raw.traits).filter(
    (item): item is EnemyArchetype["traits"][number] =>
      typeof item === "string",
  );
  if (boss && !traits.includes("boss")) traits.push("boss");
  return {
    id: asString(raw.id, `${boss ? "boss" : "enemy"}-${index + 1}`),
    name: asString(raw.name, `${boss ? "Boss" : "Enemy"} ${index + 1}`),
    color: Math.max(
      0,
      Math.min(0xffffff, asNumber(raw.color, colorAt(index + (boss ? 4 : 0)))),
    ),
    hp: asNumber(raw.hp, boss ? 2400 : 80),
    speed: asNumber(raw.speed, boss ? 24 : 50),
    armor: asNumber(raw.armor),
    reward: asNumber(raw.reward ?? raw.bounty, boss ? 240 : 10),
    lifeDamage: asNumber(
      raw.health_damage ?? raw.life_damage ?? raw.lifeDamage,
      boss ? 10 : 1,
    ),
    radius: asNumber(raw.radius, boss ? 34 : 12 + (index % 6)),
    traits,
    threatType: asString(raw.threat_type ?? raw.threatType),
    resourceEffect: Object.fromEntries(
      Object.entries(asRecord(raw.resource_effect)).map(([key, effect]) => [
        key,
        asNumber(effect),
      ]),
    ),
  };
}

function normalizeTower(value: unknown, index: number): TowerDefinition {
  const raw = asRecord(value);
  const id = asString(raw.id, `tower-${index + 1}`);
  const name = asString(raw.name, `Tower ${index + 1}`);
  const rawBranches = asArray(raw.branches);
  const branches =
    rawBranches.length === 2
      ? rawBranches.map((value, branchIndex) => {
          const branch = asRecord(value);
          return {
            id: asString(branch.id, `${id}-branch-${branchIndex + 1}`),
            name: asString(branch.name, `${name} ${branchIndex + 1}`),
            description: asString(
              branch.description,
              "전문 방어 능력을 강화합니다.",
            ),
            damageMultiplier:
              branch.damage_multiplier === undefined
                ? undefined
                : asNumber(branch.damage_multiplier),
            rangeMultiplier:
              branch.range_multiplier === undefined
                ? undefined
                : asNumber(branch.range_multiplier),
            rateMultiplier:
              branch.rate_multiplier === undefined
                ? undefined
                : asNumber(branch.rate_multiplier),
            splash: asNumber(branch.splash),
            slow: asNumber(branch.slow),
            pierce: asNumber(branch.pierce),
          };
        })
      : [
          {
            id: `${id}-precision`,
            name: `${name} 정밀화`,
            description: "단일 위협 대응력 강화",
            damageMultiplier: 1.7,
            rangeMultiplier: 1.15,
          },
          {
            id: `${id}-network`,
            name: `${name} 연계망`,
            description: "연계 속도와 범위 강화",
            rateMultiplier: 0.66,
            splash: 58,
          },
        ];
  const damageType = asString(raw.damage_type ?? raw.damageType);
  return {
    id,
    name,
    role: asString(raw.role, "공통 방어"),
    color: Math.max(0, Math.min(0xffffff, asNumber(raw.color, colorAt(index)))),
    cost: asNumber(raw.cost, 80),
    damage: asNumber(raw.damage, 20),
    range: asNumber(raw.range, 135),
    fireRate: asNumber(raw.fire_rate ?? raw.fireRate, 0.7),
    projectileSpeed: asNumber(
      raw.projectile_speed ?? raw.projectileSpeed,
      380,
    ),
    damageType: ["physical", "magic", "true"].includes(damageType)
      ? (damageType as TowerDefinition["damageType"])
      : "physical",
    branches,
    effectiveAgainst: asArray(
      raw.effective_against ?? raw.effectiveAgainst,
    ).filter((item): item is string => typeof item === "string"),
    effectiveMultiplier: asNumber(
      raw.effective_multiplier ?? raw.effectiveMultiplier,
      1.5,
    ),
  };
}

function normalizeHero(value: unknown, index: number): HeroDefinition {
  const raw = asRecord(value);
  const role = asString(raw.role, "Guardian");
  return {
    id: asString(raw.id, `hero-${index + 1}`),
    name: asString(raw.name, `Guardian ${index + 1}`),
    title: asString(raw.title, role),
    color: Math.max(
      0,
      Math.min(0xffffff, asNumber(raw.color, colorAt(index + 2))),
    ),
    hp: asNumber(raw.hp ?? raw.base_hp, 500 + index * 100),
    damage: asNumber(raw.damage ?? raw.base_damage, 32 + index * 6),
    range: asNumber(raw.range, index % 2 ? 55 : 120),
    speed: asNumber(raw.speed, 130),
    respawnSeconds: asNumber(raw.respawn_seconds, 9 + index),
    skill1: asString(raw.skill1, `${role} 분석`),
    skill2: asString(raw.skill2, `${role} 대응`),
    ultimate: asString(raw.ultimate, `${role} 총력전`),
    unlockStage: Math.max(1, Math.floor(asNumber(raw.unlock_stage, 1))),
  };
}

function normalizeSkills(
  values: unknown[],
  fallback: DefenseContentPack,
): SkillDefinition[] {
  const canonical = [
    { id: "meteor", effect: "area_damage" },
    { id: "reinforcement", effect: "reinforcement" },
    { id: "freeze", effect: "freeze" },
  ];
  return canonical.map(({ id, effect }) => {
    const raw = asRecord(
      values.find((value) => asString(asRecord(value).effect) === effect),
    );
    const local = fallback.config.skills.find((skill) => skill.id === id)!;
    return {
      id,
      name: asString(raw.name, local.name),
      description: asString(raw.description ?? raw.effect, local.description),
      cooldown: asNumber(raw.cooldown, local.cooldown),
      color: asString(raw.color, local.color),
    };
  });
}

function normalizeBalance(
  value: unknown,
  fallback: RealmBalance,
): RealmBalance {
  const raw = asRecord(value);
  const difficulties = asRecord(raw.difficulties);
  const difficulty = (key: RealmDifficulty) => {
    const current = asRecord(difficulties[key]);
    return {
      enemyHp: asNumber(current.enemy_hp),
      enemySpeed: asNumber(current.enemy_speed),
      gold: asNumber(current.gold),
      score: asNumber(current.score),
    };
  };
  const bonus = (key: RealmDifficulty) =>
    asNumber(asRecord(difficulties[key]).difficulty_bonus);
  const upgradeCosts = asArray(raw.tower_upgrade_cost).map(Number);
  return {
    difficulties: {
      casual: difficulty("casual"),
      normal: difficulty("normal"),
      veteran: difficulty("veteran"),
    },
    towerUpgradeCost: upgradeCosts,
    heroLevelXp: fallback.heroLevelXp,
    endlessRamp: asNumber(raw.endless_ramp, fallback.endlessRamp),
    endlessWaveBonus: asNumber(
      raw.endless_wave_bonus,
      fallback.endlessWaveBonus,
    ),
    sellRefundRate: asNumber(raw.sell_refund_rate),
    difficultyBonus: {
      casual: bonus("casual"),
      normal: bonus("normal"),
      veteran: bonus("veteran"),
    },
    clearTimeBonusPerSecond: Math.max(
      1,
      Math.round(1000 / asNumber(raw.clear_time_bonus_divisor)),
    ),
    parTimeSeconds: Math.max(
      1,
      Math.round(asNumber(raw.clear_time_target_ms) / 1000),
    ),
  };
}

function normalizeWave(
  value: unknown,
  index: number,
  strictWaveRouting = false,
): RealmWave {
  const raw = asRecord(value);
  return {
    id: asString(raw.id, `wave-${index + 1}`),
    label: asString(raw.label, `${asNumber(raw.number, index + 1)} 웨이브`),
    reward: asNumber(raw.reward, 30),
    entries: asArray(raw.entries)
      .map((entryValue) => {
        const entry = asRecord(entryValue);
        return {
          enemy: asString(entry.enemy ?? entry.enemy_id),
          count: Math.floor(asNumber(entry.count, 1)),
          interval: asNumber(
            entry.interval,
            asNumber(entry.interval_ms, 700) / 1000,
          ),
          delay: Math.max(
            0,
            strictWaveRouting
              ? asNumber(entry.delay)
              : asNumber(entry.delay, asNumber(entry.delay_ms) / 1000),
          ),
          pathIndex: strictWaveRouting
            ? Math.floor(asNumber(entry.path_index))
            : Math.max(
                0,
                Math.floor(asNumber(entry.path_index ?? entry.pathIndex)),
              ),
          parallel: asBoolean(entry.parallel),
          modifiers: asArray(entry.modifiers).filter(
            (item): item is string => typeof item === "string",
          ),
        };
      })
      .filter((entry) => entry.enemy),
  };
}

function normalizeEvents(content: UnknownRecord): DefenseEducationEvent[] {
  const questions = new Map(
    asArray(content.education).map((value) => {
      const raw = asRecord(value);
      return [asString(raw.id), raw] as const;
    }),
  );
  return asArray(content.events)
    .map((value, index) => {
      const raw = asRecord(value);
      const question = questions.get(asString(raw.education_id)) ?? raw;
      return {
        id: asString(raw.id, `event-${index + 1}`),
        stage_id: asString(raw.stage_id, "stage-1"),
        trigger: asString(raw.trigger, "battle_start").replace("-", "_"),
        topic: asString(question.topic, "general"),
        question: asString(
          question.question,
          "이 상황에서 가장 안전한 대응을 선택하세요.",
        ),
        answers: asArray(question.answers).map((answerValue, answerIndex) => {
          const answer = asRecord(answerValue);
          return {
            id: asString(answer.id, `answer-${answerIndex + 1}`),
            text: asString(answer.text, `선택 ${answerIndex + 1}`),
          };
        }),
        reward: asRecord(raw.reward) as DefenseEducationEvent["reward"],
        penalty: asRecord(raw.penalty) as DefenseEducationEvent["penalty"],
      };
    })
    .filter((event) => event.id && event.answers.length >= 2);
}

export function normalizeDefenseConfig(
  payload: unknown,
  slug: DefenseSlug,
): { pack: DefenseContentPack; envelope: DefenseConfigEnvelope } {
  const raw = asRecord(payload);
  const game = asRecord(raw.game);
  const content = asRecord(raw.content);
  const version = normalizeVersion(raw.version);
  const strictWaveRouting = usesStrictWaveRoutingSchema(
    asString(content.schema_version),
  );
  if (
    asString(game.slug) !== slug ||
    !version.id ||
    !asArray(content.stages).length ||
    !asArray(content.waves).length
  ) {
    throw new DefenseConfigError(
      "게시된 Defense 콘텐츠가 실행 스키마와 맞지 않습니다. Content Studio에서 검증 후 다시 게시해 주세요.",
    );
  }
  const fallback = DEFENSE_PACKS[slug];
  const globalWaves = asArray(content.waves);
  const rawTowers = asArray(content.towers);
  const rawHeroes = asArray(content.heroes);
  const rawSkills = asArray(content.skills);
  const rawEnemies = asArray(content.enemies);
  const rawBosses = asArray(content.bosses);
  const enemies = rawEnemies.map((value, index) =>
    normalizeEnemy(value, index, false),
  );
  const bosses = rawBosses.map((value, index) =>
    normalizeEnemy(value, index, true),
  );
  const knownEnemyIds = new Set([...enemies, ...bosses].map((item) => item.id));
  const threatTypes = new Set(
    [...enemies, ...bosses].map((item) => item.threatType).filter(Boolean),
  );
  const towers = rawTowers.map(normalizeTower);
  const heroes = rawHeroes.map(normalizeHero);
  const modelProfiles = asArray(content.model_profiles).map((value) => {
    const profile = asRecord(value);
    return {
      id: asString(profile.id),
      name: asString(profile.name),
      tower_id: asString(profile.tower_id),
      compute_cost: asNumber(profile.compute_cost),
      token_cost: asNumber(profile.token_cost),
      latency_cost: asNumber(profile.latency_cost),
      accuracy: asNumber(profile.accuracy),
      damage_multiplier: asNumber(profile.damage_multiplier),
    } as AIModelProfile;
  });
  const resourceRaw = asRecord(content.resource_rules);
  const resourceRules = Object.keys(resourceRaw).length
    ? ({
        compute_start: asNumber(resourceRaw.compute_start),
        token_start: asNumber(resourceRaw.token_start),
        trust_start: asNumber(resourceRaw.trust_start),
        latency_max: asNumber(resourceRaw.latency_max),
        wave_compute_cost: asNumber(resourceRaw.wave_compute_cost),
        wave_token_cost: asNumber(resourceRaw.wave_token_cost),
        escaped_trust_cost: asNumber(resourceRaw.escaped_trust_cost),
        escaped_latency_cost: asNumber(resourceRaw.escaped_latency_cost),
      } satisfies AIResourceRules)
    : undefined;
  const stages = asArray(content.stages).map((value, index): RealmStage => {
    const stage = asRecord(value);
    const id = asString(stage.id, `stage-${index + 1}`);
    const waves = globalWaves
      .filter((wave) => asString(asRecord(wave).stage_id) === id)
      .map((wave, waveIndex) =>
        normalizeWave(wave, waveIndex, strictWaveRouting),
      );
    const rawPaths = asArray(stage.paths).map((lane) =>
      asArray(lane).map((point) => ({
        x: asNumber(asRecord(point).x),
        y: asNumber(asRecord(point).y),
      })),
    );
    const rawPath = asArray(stage.path).map((point) => ({
      x: asNumber(asRecord(point).x),
      y: asNumber(asRecord(point).y),
    }));
    const paths = rawPaths.length ? rawPaths : [rawPath];
    const towerSpots = asArray(stage.tower_spots ?? stage.towerSpots).map(
      (spotValue, spotIndex) => {
        const spot = asRecord(spotValue);
        return {
          id: asString(spot.id, `${id}-spot-${spotIndex + 1}`),
          x: asNumber(spot.x),
          y: asNumber(spot.y),
        };
      },
    );
    return {
      id,
      number: Math.max(1, Math.floor(asNumber(stage.number, index + 1))),
      name: asString(stage.name),
      subtitle: asString(
        stage.subtitle ?? stage.description,
        `${asString(stage.name)} 방어 시나리오`,
      ),
      mode: stage.mode === "endless" ? "endless" : "campaign",
      theme: ["verdant", "ember", "frost", "void"].includes(
        asString(stage.theme),
      )
        ? (asString(stage.theme) as RealmStage["theme"])
        : "void",
      path: paths[0],
      paths,
      towerSpots,
      waves,
      startingGold: asNumber(
        stage.starting_resource ?? stage.starting_gold,
        250,
      ),
      lives: asNumber(stage.starting_health ?? stage.lives, 20),
      version: asString(stage.version),
      mapStyle: asString(stage.map_style ?? stage.mapStyle) || undefined,
      gimmick: ["time_surge", "ember_vents", "winter_blessing"].includes(
        asString(stage.gimmick),
      )
        ? (asString(stage.gimmick) as RealmStage["gimmick"])
        : undefined,
    };
  });
  const finiteNumber = (value: unknown) =>
    typeof value === "number" && Number.isFinite(value);
  const finiteIn = (value: unknown, minimum: number, maximum: number) =>
    finiteNumber(value) &&
    Number(value) >= minimum &&
    Number(value) <= maximum;
  const integerIn = (value: unknown, minimum: number, maximum: number) =>
    finiteIn(value, minimum, maximum) && Number.isInteger(Number(value));
  const optionalFiniteIn = (
    value: unknown,
    minimum: number,
    maximum: number,
  ) => value === undefined || finiteIn(value, minimum, maximum);
  const rawBalance = asRecord(content.balance);
  const rawDifficulties = asRecord(rawBalance.difficulties);
  const balanceSchemaValid =
    (["casual", "normal", "veteran"] as RealmDifficulty[]).every((key) => {
      const value = asRecord(rawDifficulties[key]);
      return (
        integerIn(value.difficulty_bonus, 0, 1_000_000_000_000) &&
        ["enemy_hp", "enemy_speed", "gold", "score"].every(
          (field) => finiteIn(value[field], Number.MIN_VALUE, 100),
        )
      );
    }) &&
    integerIn(rawBalance.clear_time_target_ms, 1, 86_400_000) &&
    integerIn(rawBalance.clear_time_bonus_divisor, 1, 1_000_000_000) &&
    integerIn(rawBalance.health_score_factor, 0, 1_000_000_000) &&
    integerIn(rawBalance.resource_score_factor, 0, 1_000_000_000) &&
    integerIn(rawBalance.wave_score_factor, 0, 1_000_000_000) &&
    integerIn(rawBalance.min_wave_duration_ms, 100, 3_600_000) &&
    integerIn(rawBalance.duration_tolerance_ms, 0, 60_000) &&
    finiteIn(rawBalance.sell_refund_rate, 0, 1) &&
    asArray(rawBalance.tower_upgrade_cost).length >= 3 &&
    asArray(rawBalance.tower_upgrade_cost).length <= 10 &&
    asArray(rawBalance.tower_upgrade_cost).every(
      (cost, index) =>
        integerIn(cost, 0, 1_000_000_000) &&
        (index === 0 || Number(cost) > 0),
    );
  const geometryValid = stages.every(
    (stage) =>
      (stage.paths ?? [stage.path]).length > 0 &&
      (stage.paths ?? [stage.path]).length <= 4 &&
      (stage.paths ?? [stage.path]).every(
        (path) =>
          path.length >= 2 &&
          path.length <= 64 &&
          path.every(
            (point) =>
              point.x >= -100 &&
              point.x <= 1380 &&
              point.y >= -100 &&
              point.y <= 820,
          ),
      ) &&
      stage.towerSpots.length >= 4 &&
      stage.towerSpots.length <= 32 &&
      stage.towerSpots.every(
        (spot) =>
          spot.x >= 0 &&
          spot.x <= 1280 &&
          spot.y >= 0 &&
          spot.y <= 720,
      ) &&
      new Set(stage.towerSpots.map((spot) => spot.id)).size ===
        stage.towerSpots.length,
  );
  const stageSchemaValid = asArray(content.stages).every((value) => {
    const rawStage = asRecord(value);
    return (
      ["verdant", "ember", "frost", "void"].includes(
        asString(rawStage.theme),
      ) &&
      asString(rawStage.mode) === "campaign" &&
      integerIn(rawStage.number, 1, stages.length) &&
      integerIn(rawStage.starting_health, 1, 1_000_000) &&
      integerIn(rawStage.starting_resource, 0, 1_000_000_000)
    );
  });
  const towerSchemaValid = rawTowers.every((value) => {
    const rawTower = asRecord(value);
    const refs = asArray(rawTower.effective_against);
    const branches = asArray(rawTower.branches);
    return (
      branches.length === 2 &&
      branches.every((branchValue) => {
        const branch = asRecord(branchValue);
        return Boolean(
          asString(branch.id) &&
            asString(branch.name) &&
            asString(branch.description) &&
            optionalFiniteIn(branch.damage_multiplier, 0.01, 10) &&
            optionalFiniteIn(branch.range_multiplier, 0.01, 10) &&
            optionalFiniteIn(branch.rate_multiplier, 0.01, 10) &&
            optionalFiniteIn(branch.splash, 0, 2000) &&
            optionalFiniteIn(branch.slow, 0, 1) &&
            optionalFiniteIn(branch.pierce, 0, 1000),
        );
      }) &&
      refs.length > 0 &&
      refs.every((item) => typeof item === "string" && threatTypes.has(item)) &&
      finiteIn(rawTower.effective_multiplier, 1.000001, 10) &&
      ["physical", "magic", "true"].includes(
        asString(rawTower.damage_type),
      ) &&
      integerIn(rawTower.cost, 1, 1_000_000_000) &&
      finiteIn(rawTower.damage, 0.01, 1_000_000_000) &&
      finiteIn(rawTower.range, 1, 5000) &&
      finiteIn(rawTower.fire_rate, 0.01, 3600) &&
      finiteIn(rawTower.projectile_speed, 1, 100_000)
    );
  });
  const waveSchemaValid = globalWaves.every((value) => {
    const wave = asRecord(value);
    const entries = asArray(wave.entries);
    return (
      integerIn(wave.number, 1, 100) &&
      integerIn(wave.reward, 0, 1_000_000_000) &&
      entries.length >= 1 &&
      entries.length <= 8 &&
      entries.every((entryValue) => {
        const entry = asRecord(entryValue);
        const pathIndex = strictWaveRouting
          ? entry.path_index
          : entry.path_index ?? entry.pathIndex;
        const modifiers = asArray(entry.modifiers);
        const delayValid =
          entry.delay == null || finiteIn(entry.delay, 0, 3600);
        return (
          integerIn(entry.count, 1, 500) &&
          finiteIn(entry.interval, 0.05, 3600) &&
          (!strictWaveRouting ||
            (delayValid &&
              (pathIndex == null || integerIn(pathIndex, 0, 3)) &&
              (entry.parallel == null ||
                typeof entry.parallel === "boolean") &&
              (entry.modifiers == null ||
                Array.isArray(entry.modifiers)) &&
              modifiers.length <= 8 &&
              modifiers.every(
                (modifier) =>
                  typeof modifier === "string" &&
                  DEFENSE_WAVE_MODIFIERS.has(modifier),
              ) &&
              new Set(modifiers).size === modifiers.length))
        );
      }) &&
      entries.reduce<number>(
        (sum, entryValue) => sum + Number(asRecord(entryValue).count),
        0,
      ) <= 2000
    );
  });
  const heroSchemaValid = rawHeroes.every((value) => {
    const item = asRecord(value);
    return (
      asString(item.title) &&
      asString(item.skill1) &&
      asString(item.skill2) &&
      asString(item.ultimate) &&
      finiteIn(item.hp, 1, 1_000_000_000_000) &&
      finiteIn(item.damage, 0.01, 1_000_000_000) &&
      finiteIn(item.range, 1, 5000) &&
      finiteIn(item.speed, 0.01, 10_000) &&
      finiteIn(item.respawn_seconds, 0.01, 3600) &&
      finiteNumber(item.unlock_stage) &&
      Number.isInteger(Number(item.unlock_stage)) &&
      Number(item.unlock_stage) >= 1 &&
      Number(item.unlock_stage) <= stages.length
    );
  });
  const skillSchemaValid =
    rawSkills.length === 3 &&
    new Set(rawSkills.map((value) => asString(asRecord(value).effect))).size ===
      3 &&
    ["area_damage", "reinforcement", "freeze"].every((effect) =>
      rawSkills.some((value) => {
        const rawSkill = asRecord(value);
        return (
          asString(rawSkill.effect) === effect &&
          asString(rawSkill.id) &&
          asString(rawSkill.name) &&
          asString(rawSkill.description) &&
          finiteIn(rawSkill.cooldown, 0.01, 3600)
        );
      }),
    );
  const aiRulesSchemaValid =
    slug !== "ai-nexus-defense" ||
    (resourceRules &&
      ["compute_start", "token_start", "trust_start", "latency_max"].every(
        (key) => integerIn(resourceRaw[key], 1, 1_000_000),
      ) &&
      integerIn(
        resourceRaw.wave_compute_cost,
        0,
        Number(resourceRaw.compute_start) - 1,
      ) &&
      integerIn(
        resourceRaw.wave_token_cost,
        0,
        Number(resourceRaw.token_start) - 1,
      ) &&
      integerIn(
        resourceRaw.escaped_trust_cost,
        0,
        Number(resourceRaw.trust_start),
      ) &&
      integerIn(
        resourceRaw.escaped_latency_cost,
        0,
        Number(resourceRaw.latency_max),
      ) &&
      (() => {
        const limits = asRecord(rawBalance.resource_state_limits);
        const factors = asRecord(rawBalance.ai_resource_score_factors);
        const expected: Record<string, unknown> = {
          compute: resourceRaw.compute_start,
          token: resourceRaw.token_start,
          trust: resourceRaw.trust_start,
          latency: resourceRaw.latency_max,
        };
        return (
          Object.keys(limits).length === 4 &&
          Object.keys(factors).length === 4 &&
          Object.entries(expected).every(
            ([key, value]) =>
              limits[key] === value && integerIn(factors[key], 0, 1_000_000),
          )
        );
      })());
  const enemySchemaValid = [...rawEnemies, ...rawBosses].every((value) => {
    const item = asRecord(value);
    const effect = asRecord(item.resource_effect);
    return (
      asString(item.threat_type) &&
      finiteIn(item.hp, Number.MIN_VALUE, 1_000_000_000_000) &&
      finiteIn(item.speed, Number.MIN_VALUE, 10_000) &&
      finiteIn(item.armor, 0, 1) &&
      integerIn(item.reward, 0, 1_000_000_000) &&
      integerIn(item.health_damage, 1, 1_000_000) &&
      finiteIn(item.radius, Number.MIN_VALUE, 200) &&
      (slug !== "ai-nexus-defense"
        ? Object.keys(effect).length === 0
        : Object.keys(effect).length > 0 &&
          Object.entries(effect).every(
            ([key, cost]) =>
              ["compute", "token", "trust", "latency"].includes(key) &&
              integerIn(cost, 0, 1_000_000),
          ))
    );
  });
  const expectedProfileIDs = [
    "small",
    "medium",
    "large",
    "reasoning",
    "vision",
  ];
  const modelProfilesSchemaValid =
    slug !== "ai-nexus-defense" ||
    (modelProfiles.length === expectedProfileIDs.length &&
      expectedProfileIDs.every((id) =>
        modelProfiles.some((profile) => profile.id === id),
      ) &&
      new Set(modelProfiles.map((profile) => profile.id)).size ===
        expectedProfileIDs.length &&
      modelProfiles.every(
        (profile) =>
          profile.name &&
          towers.some((tower) => tower.id === profile.tower_id) &&
          integerIn(
            profile.compute_cost,
            1,
            Number(resourceRules?.compute_start),
          ) &&
          integerIn(
            profile.token_cost,
            1,
            Number(resourceRules?.token_start),
          ) &&
          integerIn(
            profile.latency_cost,
            0,
            Number(resourceRules?.latency_max),
          ) &&
          integerIn(profile.accuracy, 1, 100) &&
          finiteIn(profile.damage_multiplier, Number.MIN_VALUE, 10),
      ));
  if (
    !geometryValid ||
    !stageSchemaValid ||
    !towerSchemaValid ||
    !waveSchemaValid ||
    !heroSchemaValid ||
    !skillSchemaValid ||
    !balanceSchemaValid ||
    !aiRulesSchemaValid ||
    !enemySchemaValid ||
    !modelProfilesSchemaValid ||
    !stages.every(
      (stage) =>
        stage.name &&
        stage.version &&
        stage.waves.length > 0 &&
        stage.waves.every(
          (wave) =>
            wave.entries.length > 0 &&
            wave.entries.every(
              (entry) =>
                knownEnemyIds.has(entry.enemy) &&
                (!strictWaveRouting ||
                  ((entry.pathIndex ?? 0) >= 0 &&
                    (entry.pathIndex ?? 0) < (stage.paths?.length ?? 1))),
            ),
        ),
    ) ||
    !towers.length ||
    !heroes.length ||
    (slug === "ai-nexus-defense" && !resourceRules)
  ) {
    throw new DefenseConfigError(
      "Defense 콘텐츠의 웨이브 또는 유닛 참조가 올바르지 않습니다.",
    );
  }
  const pack: DefenseContentPack = {
    ...fallback,
    educationEnabled: asBoolean(game.education_enabled),
    policyVersion: version.policy_version,
    config: {
      versionId: version.id,
      contentVersion: version.content_version,
      balanceVersion: version.checksum || version.content_version,
      assetVersion: version.asset_version,
      stages,
      enemies: [...enemies, ...bosses],
      towers: towers.map((tower) => ({
        ...tower,
        profiles: modelProfiles
          .filter((item) => item.tower_id === tower.id)
          .map((profile) => ({
            id: profile.id,
            name: profile.name,
            damageMultiplier: profile.damage_multiplier,
          })),
      })),
      heroes,
      skills: normalizeSkills(rawSkills, fallback),
      balance: normalizeBalance(content.balance, fallback.config.balance),
    },
    events: normalizeEvents(content),
    education: [],
    modelProfiles: slug === "ai-nexus-defense" ? modelProfiles : undefined,
    resourceRules: slug === "ai-nexus-defense" ? resourceRules : undefined,
  };
  return {
    pack,
    envelope: {
      game: {
        slug,
        name: asString(game.name, fallback.presentation.name),
        education_enabled: pack.educationEnabled,
      },
      version,
      content,
    },
  };
}

export async function getDefenseConfig(slug: DefenseSlug) {
  const raw = await api.requestEnvelope<DefenseConfigEnvelope>(
    `/api/v1/defense/${encodeURIComponent(slug)}/config`,
  );
  return normalizeDefenseConfig(raw, slug);
}

export function normalizeDefenseLearningReport(
  value: unknown,
): DefenseLearningReport {
  const raw = asRecord(value);
  const game = asRecord(raw.game);
  const slug = asString(game.slug);
  if (
    !["office-guardians", "cyber-fortress", "ai-nexus-defense"].includes(slug)
  )
    throw new Error("서버가 유효한 Defense 학습 보고서를 반환하지 않았습니다.");
  return {
    game: {
      id: asString(game.id) || undefined,
      slug: slug as DefenseSlug,
      name: asString(game.name),
    },
    overall_score: asNumber(raw.overall_score),
    topics: asArray(raw.topics).map((value) => {
      const topic = asRecord(value);
      return {
        topic: asString(topic.topic),
        correct: asNumber(topic.correct),
        total: asNumber(topic.total),
        score: asNumber(topic.score),
      };
    }),
    completed_campaigns: asArray(raw.completed_campaigns).map((value) => {
      const campaign = asRecord(value);
      return {
        campaign_id: asString(campaign.campaign_id),
        completed: asBoolean(campaign.completed),
        completed_stages: asNumber(campaign.completed_stages),
        required_stages: asNumber(campaign.required_stages),
        learning_score: asNumber(campaign.learning_score),
        completed_at: asString(campaign.completed_at) || undefined,
        updated_at: asString(campaign.updated_at) || undefined,
      };
    }),
  };
}

export const defenseAPI = {
  config: getDefenseConfig,
  version: async (slug: DefenseSlug) =>
    (
      await api.requestEnvelope<{ version: DefenseVersion }>(
        `/api/v1/defense/${encodeURIComponent(slug)}/version`,
      )
    ).version,
  progress: (slug: DefenseSlug) =>
    api.request<DefenseProgress>(
      `/api/v1/defense/${encodeURIComponent(slug)}/progress`,
    ),
  rankings: (
    slug: DefenseSlug,
    period = "weekly",
    group = "individual",
    limit = 10,
  ) =>
    api.requestEnvelope<{
      items: DefenseRankingEntry[];
      period: string;
      group: string;
      version: DefenseVersion;
    }>(
      `/api/v1/defense/${encodeURIComponent(slug)}/rankings?${new URLSearchParams({ period, group, limit: String(limit) })}`,
    ),
  learning: async (slug: DefenseSlug) =>
    normalizeDefenseLearningReport(
      await api.requestEnvelope<DefenseLearningReport>(
        `/api/v1/defense/${encodeURIComponent(slug)}/learning`,
      ),
    ),
};

export interface DefenseStudioVersionResponse {
  version: DefenseVersion;
}

export const defenseStudioAPI = {
  versions: (slug: DefenseSlug) =>
    api.request<{ items: DefenseVersion[] }>(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/versions`,
    ),
  createVersion: (
    slug: DefenseSlug,
    input: {
      label?: string;
      notes?: string;
      asset_version?: string;
      source_version_id?: string;
      policy_version?: string;
    },
  ) =>
    api.request<DefenseStudioVersionResponse>(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/versions`,
      {
        method: "POST",
        body: JSON.stringify({
          ...input,
          source_version_id: input.source_version_id || undefined,
          policy_version: input.policy_version || undefined,
        }),
      },
    ),
  section: (slug: DefenseSlug, section: DefenseSection, versionId: string) =>
    api.requestEnvelope<{
      version: DefenseVersion;
      section: DefenseSection;
      data: unknown;
    }>(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/drafts/${section}?version_id=${encodeURIComponent(versionId)}`,
    ),
  saveSection: (
    slug: DefenseSlug,
    section: DefenseSection,
    versionId: string,
    checksum: string,
    data: unknown,
  ) =>
    api.requestEnvelope<{
      version: DefenseVersion;
      section: DefenseSection;
      data: unknown;
    }>(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/drafts/${section}?version_id=${encodeURIComponent(versionId)}`,
      {
        method: "PUT",
        headers: { "If-Match": `"${checksum}"` },
        body: JSON.stringify({ data }),
      },
    ),
  testVersion: (slug: DefenseSlug, id: string) =>
    api.request<{
      version: DefenseVersion;
      validation: Record<string, unknown>;
    }>(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/versions/${encodeURIComponent(id)}/test`,
      { method: "POST" },
    ),
  publishVersion: (slug: DefenseSlug, id: string) =>
    api.request<
      DefenseStudioVersionResponse & {
        published?: boolean;
        approval_required?: boolean;
      }
    >(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/versions/${encodeURIComponent(id)}/publish`,
      { method: "POST", body: "{}" },
    ),
  telemetry: (slug: DefenseSlug, days = 30) =>
    api.requestEnvelope<Record<string, unknown>>(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/telemetry?days=${days}`,
    ),
  learningReport: (slug: DefenseSlug) =>
    api.requestEnvelope<Record<string, unknown>>(
      `/api/v1/admin/defense/${encodeURIComponent(slug)}/learning-report`,
    ),
  preview: async (slug: DefenseSlug, id: string) =>
    normalizeDefenseConfig(
      await api.requestEnvelope<DefenseConfigEnvelope>(
        `/api/v1/defense/${encodeURIComponent(slug)}/versions/${encodeURIComponent(id)}/preview`,
      ),
      slug,
    ),
};

export function pendingDefenseVersions() {
  return api.request<{
    items: Array<
      DefenseVersion & { game_slug: DefenseSlug; game_name: string }
    >;
  }>("/api/v1/defense/versions/pending");
}

export function reviewDefenseVersion(
  id: string,
  decision: "approved" | "rejected",
  comment: string,
) {
  return api.request<{ version: DefenseVersion; decision: string }>(
    `/api/v1/defense/versions/${encodeURIComponent(id)}/review`,
    { method: "POST", body: JSON.stringify({ decision, comment }) },
  );
}

export function normalizeDefenseServerResult(
  value: unknown,
): DefenseServerResult {
  const raw = asRecord(value);
  const result = asRecord(raw.result);
  if (
    !Number.isFinite(Number(result.score)) ||
    !Number.isFinite(Number(result.stars))
  )
    throw new Error("서버가 유효한 Defense 결과를 반환하지 않았습니다.");
  return value as DefenseServerResult;
}

export function normalizeDefenseLearningBreakdown(
  value: unknown,
): DefenseLearningBreakdown[] {
  const entries: Array<[string, unknown]> = Array.isArray(value)
    ? value.map((item) => {
        const raw = asRecord(item);
        return [asString(raw.topic), raw];
      })
    : Object.entries(asRecord(value));
  return entries
    .flatMap(([topic, value]) => {
      if (!topic) return [];
      const raw = asRecord(value);
      const total = Math.max(0, Math.floor(asNumber(raw.total)));
      return [
        {
          topic,
          correct: Math.min(
            total,
            Math.max(0, Math.floor(asNumber(raw.correct))),
          ),
          total,
          score: Math.max(
            0,
            Math.min(
              100,
              asNumber(Object.keys(raw).length ? raw.score : value),
            ),
          ),
        },
      ];
    })
    .sort((left, right) => left.topic.localeCompare(right.topic));
}
