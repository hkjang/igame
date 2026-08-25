import { api } from '../../api/client';
import { DEFAULT_REALMGUARD_CONFIG } from './content';
import type { EnemyArchetype, HeroDefinition, RealmGuardConfig, RealmProgress, RealmResult, RealmStage, RealmWave, TowerDefinition } from './types';

type UnknownRecord = Record<string, unknown>;

export class RealmGuardConfigError extends Error {
  constructor(message: string) { super(message); this.name = 'RealmGuardConfigError'; }
}

export interface RealmGuardServerResult {
  result: RealmResult & { id?: string; breakdown?: Record<string, number> };
  progress?: unknown;
}

export interface NormalizedRealmGuardCompletion {
  result: Partial<RealmResult> & Pick<RealmResult, 'score' | 'stars'>;
  progress?: RealmProgress;
}

export interface RealmGuardRankingEntry {
  rank: number;
  display_name: string;
  score: number;
  stars?: number;
  stage_id?: string;
  difficulty?: string;
  mode?: string;
  department?: string;
  hero_id?: string;
}

export interface RealmGuardRankingFilters {
  group: 'stage' | 'department' | 'hero';
  metric?: 'score' | 'stars';
  period: 'daily' | 'weekly' | 'season' | 'all_time';
  mode: 'campaign' | 'endless';
  difficulty: 'casual' | 'normal' | 'veteran';
  stage_id?: string;
  hero_id?: string;
}

const record = (value: unknown): UnknownRecord => value && typeof value === 'object' && !Array.isArray(value) ? value as UnknownRecord : {};
const array = (value: unknown): unknown[] => Array.isArray(value) ? value : [];
const number = (value: unknown, fallback: number) => Number.isFinite(Number(value)) ? Number(value) : fallback;
const string = (value: unknown, fallback: string) => typeof value === 'string' && value ? value : fallback;
const color = (value: unknown, fallback: number) => Math.max(0, Math.min(0xffffff, Math.round(number(value, fallback))));

function normalizeWave(value: unknown, fallback: RealmWave): RealmWave {
  const raw = record(value);
  const rawEntries = array(raw.entries).length ? array(raw.entries) : array(raw.enemies);
  const entries = rawEntries.map((entryValue) => {
    const entry = record(entryValue);
    return {
      enemy: string(entry.enemy ?? entry.enemy_id, fallback.entries[0]?.enemy ?? 'mireling'),
      count: Math.max(1, number(entry.count, 1)),
      interval: Math.max(.15, number(entry.interval, number(entry.interval_ms, 800) / 1000)),
      delay: Math.max(0, number(entry.delay, number(entry.delay_ms, 0) / 1000)),
      pathIndex: Math.max(0, Math.floor(number(entry.path_index ?? entry.pathIndex, 0))),
      parallel: Boolean(entry.parallel),
      modifiers: array(entry.modifiers).filter((item): item is string => typeof item === 'string'),
    };
  });
  const bossId = string(raw.boss_id, '');
  if (bossId) entries.push({ enemy: bossId, count: 1, interval: 1.5, delay: 2, pathIndex: 0, parallel: false, modifiers: [] });
  return {
    id: string(raw.id, fallback.id), label: string(raw.label, fallback.label),
    entries: entries.length ? entries : fallback.entries, reward: Math.max(0, number(raw.reward, fallback.reward)),
  };
}

function normalizeStage(value: unknown, index: number, globalWaves: unknown[]): RealmStage {
  const raw = record(value);
  const fallback = DEFAULT_REALMGUARD_CONFIG.stages[index] ?? DEFAULT_REALMGUARD_CONFIG.stages.at(-1)!;
  const stageId = string(raw.id, fallback.id);
  const embedded = array(raw.waves);
  const referenced = globalWaves.filter((wave) => string(record(wave).stage_id, '') === stageId);
  const sourceWaves = embedded.length ? embedded : referenced;
  const rawPaths = array(raw.paths).map((lane) => array(lane).map((point) => ({ x: number(record(point).x, 0), y: number(record(point).y, 0) }))).filter((lane) => lane.length >= 2);
  const path = array(raw.path).map((point) => ({ x: number(record(point).x, 0), y: number(record(point).y, 0) }));
  const paths = rawPaths.length ? rawPaths : path.length >= 2 ? [path] : fallback.paths?.length ? fallback.paths : [fallback.path];
  const spots = array(raw.tower_spots ?? raw.towerSpots).map((spot, spotIndex) => ({ id: string(record(spot).id, `${stageId}-spot-${spotIndex + 1}`), x: number(record(spot).x, 0), y: number(record(spot).y, 0) }));
  return {
    ...fallback,
    id: stageId,
    number: number(raw.number, fallback.number),
    name: string(raw.name, fallback.name),
    subtitle: string(raw.subtitle ?? raw.description, fallback.subtitle),
    mode: raw.mode === 'campaign' || raw.mode === 'endless' ? raw.mode : fallback.mode,
    theme: ['verdant', 'ember', 'frost', 'void'].includes(String(raw.theme)) ? raw.theme as RealmStage['theme'] : fallback.theme,
    path: paths[0],
    paths,
    towerSpots: spots.length >= 4 ? spots : fallback.towerSpots,
    waves: sourceWaves.length ? sourceWaves.map((wave, waveIndex) => normalizeWave(wave, fallback.waves[waveIndex % fallback.waves.length])) : fallback.waves,
    startingGold: number(raw.starting_gold ?? raw.startingGold, fallback.startingGold),
    lives: number(raw.lives ?? raw.base_lives, fallback.lives),
    version: string(raw.version ?? raw.stage_version, fallback.version),
    gimmick: ['ember_vents', 'winter_blessing', 'time_surge'].includes(String(raw.gimmick)) ? raw.gimmick as RealmStage['gimmick'] : fallback.gimmick,
  };
}

function normalizeTower(value: unknown, index: number): TowerDefinition {
  const raw = record(value);
  const fallback = DEFAULT_REALMGUARD_CONFIG.towers.find((tower) => tower.id === raw.id) ?? DEFAULT_REALMGUARD_CONFIG.towers[index % DEFAULT_REALMGUARD_CONFIG.towers.length];
  const branches = array(raw.branches).map((branchValue) => {
    const branch = record(branchValue);
    return {
      id: string(branch.id, 'branch'), name: string(branch.name, '분기'), description: string(branch.description, ''),
      damageMultiplier: number(branch.damage_multiplier ?? branch.damageMultiplier, 1), rangeMultiplier: number(branch.range_multiplier ?? branch.rangeMultiplier, 1),
      rateMultiplier: number(branch.rate_multiplier ?? branch.rateMultiplier, 1), splash: number(branch.splash, 0), slow: number(branch.slow, 0), pierce: number(branch.pierce, 0),
    };
  });
  return {
    ...fallback,
    id: string(raw.id, fallback.id), name: string(raw.name, fallback.name), role: string(raw.role, fallback.role), color: color(raw.color, fallback.color),
    cost: number(raw.cost, fallback.cost), damage: number(raw.damage, fallback.damage), range: number(raw.range, fallback.range),
    fireRate: number(raw.fire_rate ?? raw.fireRate, raw.attack_ms ? number(raw.attack_ms, 1000) / 1000 : fallback.fireRate),
    projectileSpeed: number(raw.projectile_speed ?? raw.projectileSpeed, fallback.projectileSpeed),
    damageType: ['physical', 'arcane', 'siege', 'frost'].includes(String(raw.damage_type ?? raw.damageType)) ? (raw.damage_type ?? raw.damageType) as TowerDefinition['damageType'] : fallback.damageType,
    branches: branches.length === 2 ? branches : fallback.branches,
  };
}

function normalizeEnemy(value: unknown, index: number): EnemyArchetype {
  const raw = record(value);
  const fallback = DEFAULT_REALMGUARD_CONFIG.enemies.find((enemy) => enemy.id === raw.id) ?? DEFAULT_REALMGUARD_CONFIG.enemies[index % DEFAULT_REALMGUARD_CONFIG.enemies.length];
  return {
    ...fallback, id: string(raw.id, fallback.id), name: string(raw.name, fallback.name), color: color(raw.color, fallback.color), hp: number(raw.hp, fallback.hp),
    speed: number(raw.speed, fallback.speed), armor: number(raw.armor, fallback.armor), reward: number(raw.reward ?? raw.bounty, fallback.reward),
    lifeDamage: number(raw.life_damage ?? raw.lifeDamage, fallback.lifeDamage), radius: number(raw.radius, fallback.radius),
    traits: Array.isArray(raw.traits) ? array(raw.traits).filter((trait): trait is EnemyArchetype['traits'][number] => typeof trait === 'string') : fallback.traits,
  };
}

function normalizeHero(value: unknown, index: number): HeroDefinition {
  const raw = record(value);
  const fallback = DEFAULT_REALMGUARD_CONFIG.heroes.find((hero) => hero.id === raw.id) ?? DEFAULT_REALMGUARD_CONFIG.heroes[index % DEFAULT_REALMGUARD_CONFIG.heroes.length];
  return {
    ...fallback, id: string(raw.id, fallback.id), name: string(raw.name, fallback.name), title: string(raw.title ?? raw.role, fallback.title), color: color(raw.color, fallback.color),
    hp: number(raw.hp ?? raw.base_hp, fallback.hp), damage: number(raw.damage ?? raw.base_damage, fallback.damage), range: number(raw.range, fallback.range),
    speed: number(raw.speed, fallback.speed), respawnSeconds: number(raw.respawn_seconds, fallback.respawnSeconds),
    skill1: string(raw.skill1, fallback.skill1), skill2: string(raw.skill2, fallback.skill2), ultimate: string(raw.ultimate, fallback.ultimate),
    unlockStage: Math.max(1, Math.floor(number(raw.unlock_stage ?? raw.unlockStage, fallback.unlockStage ?? 1))),
  };
}

export function normalizeRealmGuardConfig(payload: unknown): RealmGuardConfig {
  const raw = record(payload);
  const version = record(raw.version);
  const versionId = string(version.id, '');
  const rawBases = array(raw.base_towers).length ? array(raw.base_towers) : array(raw.towers);
  const rawAdvanced = array(raw.advanced_towers);
  const rawTowers = rawBases.map((baseValue) => {
    const base = record(baseValue);
    const inline = array(base.branches);
    const derived = rawAdvanced.filter((value) => string(record(value).evolves_from, '') === string(base.id, ''));
    return { ...base, branches: inline.length ? inline : derived };
  });
  const stageSource = array(raw.stages);
  const globalWaves = array(raw.waves);
  const rawEnemies = [...array(raw.enemies), ...array(raw.bosses)];
  const rawHeroes = array(raw.heroes);
  const rawSkills = array(raw.skills);
  const expectedTowerIds = new Set(DEFAULT_REALMGUARD_CONFIG.towers.map((tower) => tower.id));
  const expectedEnemyIds = new Set(DEFAULT_REALMGUARD_CONFIG.enemies.map((enemy) => enemy.id));
  const expectedHeroIds = new Set(DEFAULT_REALMGUARD_CONFIG.heroes.map((hero) => hero.id));
  const expectedSkillIds = new Set(['meteor', 'reinforcement', 'freeze']);
  const enemyIds = new Set(rawEnemies.map((value) => string(record(value).id, '')));
  const branchIds = rawTowers.flatMap((value) => array(record(value).branches).map((branch) => string(record(branch).id, '')));
  const difficultyData = record(record(raw.balance).difficulties);
  const stageIds = stageSource.map((value) => string(record(value).id, ''));
  const campaignNumbers = stageSource
    .filter((value) => record(value).mode === 'campaign')
    .map((value) => Math.floor(number(record(value).number, 0)))
    .sort((left, right) => left - right);
  const stageModesValid = stageSource.every((value) => ['campaign', 'endless'].includes(String(record(value).mode)));
  const campaignNumbersContiguous = campaignNumbers.length >= 10 && campaignNumbers.every((value, index) => value === index + 1);
  const exactlyOneEndless = stageSource.filter((value) => record(value).mode === 'endless').length === 1;
  const coreValid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(versionId)
    && stageSource.length >= 11 && stageIds.every(Boolean) && new Set(stageIds).size === stageIds.length
    && stageModesValid && campaignNumbersContiguous && exactlyOneEndless
    && stageSource.every((value) => {
      const stage = record(value); const stageId = string(stage.id, '');
      const waves = array(stage.waves).length ? array(stage.waves) : globalWaves.filter((wave) => string(record(wave).stage_id, '') === stageId);
      const paths = array(stage.paths).length ? array(stage.paths).map((lane) => array(lane)) : [array(stage.path)];
      const spots = array(stage.tower_spots ?? stage.towerSpots);
      const spotIds = spots.map((spot) => string(record(spot).id, ''));
      const waveReferencesValid = waves.every((waveValue) => {
        const wave = record(waveValue);
        const entries = array(wave.entries).length ? array(wave.entries) : array(wave.enemies);
        const references = entries.map((entry) => string(record(entry).enemy ?? record(entry).enemy_id, '')).filter(Boolean);
        const boss = string(wave.boss_id, ''); if (boss) references.push(boss);
        return references.length > 0 && references.every((id) => enemyIds.has(id)) && entries.every((entry) => {
          const pathIndex = Math.floor(number(record(entry).path_index ?? record(entry).pathIndex, 0));
          return pathIndex >= 0 && pathIndex < paths.length;
        });
      });
      return paths.length >= 1 && paths.every((path) => path.length >= 2 && path.every((point) => Number.isFinite(Number(record(point).x)) && Number.isFinite(Number(record(point).y))))
        && spots.length >= 4 && spotIds.every(Boolean) && new Set(spotIds).size === spotIds.length
        && waves.length >= 8 && waves.length <= 15 && waveReferencesValid;
    })
    && rawTowers.length >= 4 && [...expectedTowerIds].every((id) => rawTowers.some((value) => string(record(value).id, '') === id && array(record(value).branches).length === 2))
    && branchIds.every(Boolean) && new Set(branchIds).size === branchIds.length
    && rawEnemies.length >= 12 && [...expectedEnemyIds].every((id) => rawEnemies.some((value) => string(record(value).id, '') === id))
    && rawHeroes.length >= 3 && [...expectedHeroIds].every((id) => rawHeroes.some((value) => string(record(value).id, '') === id))
    && rawSkills.length === 3 && [...expectedSkillIds].every((id) => rawSkills.some((value) => string(record(value).id, '') === id))
    && ['casual', 'normal', 'veteran'].every((key) => Object.keys(record(difficultyData[key])).length > 0);
  if (!coreValid) throw new RealmGuardConfigError('게시된 RealmGuard 설정이 실행 스키마와 맞지 않습니다. 관리자 Designer에서 검증 후 다시 게시해 주세요.');
  const towers = rawTowers.map(normalizeTower);
  const enemies = rawEnemies.map(normalizeEnemy);
  const heroes = rawHeroes.map(normalizeHero);
  const stages = [...stageSource].sort((a, b) => number(record(a).number, 0) - number(record(b).number, 0)).map((stage, index) => normalizeStage(stage, index, globalWaves));
  const rawBalance = record(raw.balance);
  const rawDifficulties = record(rawBalance.difficulties);
  const normalizeDifficulty = (key: 'casual' | 'normal' | 'veteran') => {
    const source = record(rawDifficulties[key]);
    const fallback = DEFAULT_REALMGUARD_CONFIG.balance.difficulties[key];
    return {
      enemyHp: number(source.enemy_hp ?? source.enemyHp, fallback.enemyHp), enemySpeed: number(source.enemy_speed ?? source.enemySpeed, fallback.enemySpeed),
      gold: number(source.gold, fallback.gold), score: number(source.score, fallback.score),
    };
  };
  const numericArray = (value: unknown, fallback: number[]) => Array.isArray(value) && value.every((item) => Number.isFinite(Number(item))) ? value.map(Number) : fallback;
  const normalizedBalance: RealmGuardConfig['balance'] = {
    difficulties: { casual: normalizeDifficulty('casual'), normal: normalizeDifficulty('normal'), veteran: normalizeDifficulty('veteran') },
    towerUpgradeCost: numericArray(rawBalance.tower_upgrade_cost ?? rawBalance.towerUpgradeCost, DEFAULT_REALMGUARD_CONFIG.balance.towerUpgradeCost),
    heroLevelXp: numericArray(rawBalance.hero_level_xp ?? rawBalance.heroLevelXp, DEFAULT_REALMGUARD_CONFIG.balance.heroLevelXp),
    endlessRamp: number(rawBalance.endless_ramp ?? rawBalance.endlessRamp, DEFAULT_REALMGUARD_CONFIG.balance.endlessRamp),
    endlessWaveBonus: number(rawBalance.endless_wave_bonus ?? rawBalance.endlessWaveBonus, DEFAULT_REALMGUARD_CONFIG.balance.endlessWaveBonus),
    sellRefundRate: number(rawBalance.sell_refund_rate ?? rawBalance.sellRefundRate, DEFAULT_REALMGUARD_CONFIG.balance.sellRefundRate),
    difficultyBonus: Object.fromEntries((['casual', 'normal', 'veteran'] as const).map((key) => [key, number(record(rawDifficulties[key]).difficulty_bonus ?? record(rawBalance.difficulty_bonus ?? rawBalance.difficultyBonus)[key], DEFAULT_REALMGUARD_CONFIG.balance.difficultyBonus[key])])) as RealmGuardConfig['balance']['difficultyBonus'],
    clearTimeBonusPerSecond: number(rawBalance.clear_time_bonus_per_second ?? rawBalance.clearTimeBonusPerSecond, rawBalance.clear_time_bonus_divisor ? 1000 / number(rawBalance.clear_time_bonus_divisor, 100) : DEFAULT_REALMGUARD_CONFIG.balance.clearTimeBonusPerSecond),
    parTimeSeconds: number(rawBalance.par_time_seconds ?? rawBalance.parTimeSeconds, rawBalance.clear_time_target_ms ? number(rawBalance.clear_time_target_ms, 900000) / 1000 : DEFAULT_REALMGUARD_CONFIG.balance.parTimeSeconds),
  };
  return {
    ...DEFAULT_REALMGUARD_CONFIG,
    versionId,
    contentVersion: string(version.content_version ?? raw.content_version, DEFAULT_REALMGUARD_CONFIG.contentVersion),
    balanceVersion: string(version.balance_version ?? raw.balance_version, DEFAULT_REALMGUARD_CONFIG.balanceVersion),
    assetVersion: string(version.asset_version ?? raw.asset_version, DEFAULT_REALMGUARD_CONFIG.assetVersion),
    stages,
    towers,
    enemies,
    heroes,
    skills: rawSkills.map((value, index) => { const source = record(value); const fallback = DEFAULT_REALMGUARD_CONFIG.skills.find((skill) => skill.id === source.id) ?? DEFAULT_REALMGUARD_CONFIG.skills[index % DEFAULT_REALMGUARD_CONFIG.skills.length]; return { ...fallback, ...source, id: string(source.id, fallback.id), name: string(source.name, fallback.name), description: string(source.description, fallback.description), cooldown: number(source.cooldown ?? source.cooldown_seconds, fallback.cooldown), color: string(source.color, fallback.color) }; }),
    balance: normalizedBalance,
  };
}

export async function getRealmGuardConfig() {
  return normalizeRealmGuardConfig(await api.request<unknown>('/api/v1/realmguard/config'));
}

export function normalizeRealmGuardVersion(value: unknown): { label: string; content_version: string; stage_version: string; balance_version: string; asset_version: string } {
  const outer = record(value);
  const payload = Object.keys(record(outer.version)).length ? record(outer.version) : outer;
  return {
    label: string(payload.label ?? payload.version, 'v0.2.0'), content_version: string(payload.content_version, ''),
    stage_version: string(payload.stage_version, ''), balance_version: string(payload.balance_version, ''), asset_version: string(payload.asset_version, ''),
  };
}

export async function getRealmGuardVersion() {
  return normalizeRealmGuardVersion(await api.request<unknown>('/api/v1/realmguard/version'));
}

export function normalizeRealmGuardProgress(value: unknown): RealmProgress {
  const payload = record(value);
  const items = array(payload.items).map((value) => record(value));
  const heroLevels: Record<string, number> = Object.fromEntries(Object.entries(record(payload.hero_levels)).map(([id, level]) => [id, number(level, 1)]));
  const heroes: RealmProgress['heroes'] = {};
  for (const value of array(payload.heroes)) { const hero = record(value); const id = string(hero.hero_id, ''); if (id) { heroes[id] = { unlocked: Boolean(hero.unlocked), level: number(hero.level, 1), xp: number(hero.xp, 0) }; heroLevels[id] = heroes[id].level; } }
  const skills: RealmProgress['skills'] = {};
  for (const value of array(payload.skills)) { const skill = record(value); const id = string(skill.skill_id, ''); if (id) skills[id] = { unlocked: Boolean(skill.unlocked), level: number(skill.level, 1) }; }
  const loadout = record(payload.loadout);
  const stageAggregates = array(payload.stages).length ? array(payload.stages).map(record) : items;
  return {
    total_stars: number(payload.total_stars, items.reduce((sum, item) => sum + number(item.stars, 0), 0)),
    unlocked_stage: Math.max(1, number(payload.unlocked_stage ?? payload.highest_stage, 1)), hero_levels: heroLevels, heroes, skills,
    loadout: { hero_id: string(loadout.hero_id, 'aerin'), skill_ids: array(loadout.skill_ids).filter((value): value is string => typeof value === 'string'), settings: record(loadout.settings) },
    campaign_completed: Boolean(payload.campaign_completed ?? payload.campaign_complete) || items.some((item) => string(item.stage_id, '') === 'stage-10' && Boolean(item.completed)),
    stages: stageAggregates.map((item) => ({ stage_id: string(item.stage_id, ''), stars: number(item.stars, 0), best_score: number(item.best_score, 0), difficulties: Array.isArray(item.difficulties) ? array(item.difficulties).filter((value): value is RealmProgress['stages'][number]['difficulties'][number] => ['casual', 'normal', 'veteran'].includes(String(value))) : typeof item.difficulty === 'string' ? [item.difficulty as RealmProgress['stages'][number]['difficulties'][number]] : [] })),
  };
}

/**
 * Normalizes the atomic RealmGuard completion response. Keeping this adapter
 * beside progress normalization makes the just-unlocked campaign state usable
 * immediately, without waiting for a page refresh.
 */
export function normalizeRealmGuardCompletion(value: unknown): NormalizedRealmGuardCompletion {
  const payload = record(value);
  const rawResult = record(payload.result);
  const score = Number(rawResult.score);
  const stars = Number(rawResult.stars);
  if (!Number.isFinite(score) || !Number.isFinite(stars)) throw new Error('서버가 유효한 점수와 별을 반환하지 않았습니다.');
  const rawProgress = record(payload.progress);
  return {
    result: { ...(rawResult as Partial<RealmResult>), score, stars },
    progress: Object.keys(rawProgress).length ? normalizeRealmGuardProgress(rawProgress) : undefined,
  };
}

export async function getRealmGuardProgress(): Promise<RealmProgress> {
  return normalizeRealmGuardProgress(await api.request<unknown>('/api/v1/realmguard/progress'));
}

export function saveRealmGuardLoadout(input: { hero_id: string; skill_ids: string[] }) {
  return api.request<void>('/api/v1/realmguard/progress', { method: 'PUT', body: JSON.stringify(input) });
}

export function resultPayload(stats: RealmResult): Record<string, unknown> {
  return {
    stage_id: stats.stage_id, mode: stats.mode, difficulty: stats.difficulty, duration_ms: stats.duration_ms,
    remaining_lives: stats.lives, remaining_gold: stats.gold, earned_gold: stats.earned_gold, spent_gold: stats.spent_gold,
    sold_gold: stats.sold_gold, kills: stats.kills, waves_completed: stats.waves_completed, escaped: stats.escaped, spawned: stats.spawned,
    defeated_by_enemy: stats.defeated_by_enemy, escaped_by_enemy: stats.escaped_by_enemy, spawned_by_enemy: stats.spawned_by_enemy,
    hero_id: stats.hero_id, hero_level: stats.hero_level, content_version: stats.content_version,
    stage_version: stats.stage_version, balance_version: stats.balance_version, asset_version: stats.asset_version,
    victory: stats.victory, ledger: stats.ledger,
  };
}

export function realmGuardRankingURL(filters: RealmGuardRankingFilters) {
  const query = new URLSearchParams({ group: filters.group, metric: filters.group === 'department' ? filters.metric ?? 'score' : 'score', period: filters.period, mode: filters.mode, difficulty: filters.difficulty });
  if (filters.stage_id) query.set('stage_id', filters.stage_id);
  if (filters.hero_id) query.set('hero_id', filters.hero_id);
  return `/api/v1/realmguard/rankings?${query.toString()}`;
}

export async function getRealmGuardRankings(filters: RealmGuardRankingFilters) {
  const payload = await api.request<{ items?: unknown[] }>(realmGuardRankingURL(filters));
  return array(payload.items).map((value, index): RealmGuardRankingEntry => {
    const item = record(value);
    return {
      rank: number(item.rank, index + 1), display_name: string(item.display_name ?? item.name, '익명'), score: number(item.score, 0),
      stars: number(item.stars, 0), stage_id: string(item.stage_id, ''), difficulty: string(item.difficulty, ''), mode: string(item.mode, ''),
      department: string(item.department, ''), hero_id: string(item.hero_id, ''),
    };
  });
}
