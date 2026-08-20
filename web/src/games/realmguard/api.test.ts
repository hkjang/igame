import { describe, expect, it } from 'vitest';
import { DEFAULT_REALMGUARD_CONFIG } from './content';
import { normalizeRealmGuardCompletion, normalizeRealmGuardConfig, normalizeRealmGuardProgress, normalizeRealmGuardVersion, realmGuardRankingURL, resultPayload } from './api';

describe('RealmGuard API adapter', () => {
  it('runs a canonical published snake_case snapshot directly', () => {
    const payload = {
      version: { id: '11111111-1111-4111-8111-111111111111', content_version: '2', balance_version: '3', asset_version: 'procedural-2' },
      stages: DEFAULT_REALMGUARD_CONFIG.stages.map((stage, index) => ({
        ...stage, starting_gold: stage.startingGold, tower_spots: stage.towerSpots,
        ...(index === 0 ? {
          paths: [stage.path, stage.path.map((point) => ({ x: point.x, y: point.y + 24 }))],
          waves: stage.waves.map((wave, waveIndex) => waveIndex === 0 ? { ...wave, entries: wave.entries.map((entry, entryIndex) => entryIndex === 0 ? { ...entry, path_index: 1, parallel: true } : entry) } : wave),
        } : {}),
      })),
      towers: DEFAULT_REALMGUARD_CONFIG.towers,
      enemies: DEFAULT_REALMGUARD_CONFIG.enemies.filter((enemy) => !enemy.traits.includes('boss')),
      bosses: DEFAULT_REALMGUARD_CONFIG.enemies.filter((enemy) => enemy.traits.includes('boss')),
      heroes: DEFAULT_REALMGUARD_CONFIG.heroes,
      skills: DEFAULT_REALMGUARD_CONFIG.skills,
      balance: {
        difficulties: {
          casual: { enemy_hp: .82, enemy_speed: .92, gold: 1.18, score: .8, difficulty_bonus: 0 },
          normal: { enemy_hp: 1, enemy_speed: 1, gold: 1, score: 1, difficulty_bonus: 5000 },
          veteran: { enemy_hp: 1.38, enemy_speed: 1.12, gold: .9, score: 1.5, difficulty_bonus: 10000 },
        },
        tower_upgrade_cost: [0, 70, 120], hero_level_xp: [0, 8, 20], endless_ramp: .085,
        endless_wave_bonus: 1000, sell_refund_rate: .72, clear_time_target_ms: 900000, clear_time_bonus_divisor: 100,
      },
    };
    const config = normalizeRealmGuardConfig(payload);
    expect(config.contentVersion).toBe('2');
    expect(config.versionId).toBe('11111111-1111-4111-8111-111111111111');
    expect(config.stages).toHaveLength(11);
    expect(config.stages[0].path.length).toBeGreaterThan(2);
    expect(config.stages[0].paths).toHaveLength(2);
    expect(config.stages[0].waves[0].entries[0]).toMatchObject({ pathIndex: 1, parallel: true });
    expect(config.stages[0].towerSpots).toHaveLength(8);
    expect(config.stages[0].waves.length).toBeGreaterThanOrEqual(8);
    expect(config.balance.difficulties.normal.enemyHp).toBe(1);
    expect(config.balance.difficulties.veteran.enemySpeed).toBe(1.12);
    expect(config.balance.difficulties.casual.gold).toBe(1.18);
    expect(config.balance.towerUpgradeCost).toEqual([0, 70, 120]);
    expect(config.balance.difficultyBonus).toEqual({ casual: 0, normal: 5000, veteran: 10000 });
    expect(config.balance.parTimeSeconds).toBe(900);
    expect(config.balance.clearTimeBonusPerSecond).toBe(10);
    expect(config.balance.endlessWaveBonus).toBe(1000);
    expect(config.balance.sellRefundRate).toBe(.72);
  });

  it('maps authoritative result fields without client score or stars', () => {
    const payload = resultPayload({
      stage_id: 'stage-1', mode: 'campaign', difficulty: 'normal', duration_ms: 120000, lives: 18, gold: 90,
      earned_gold: 400, spent_gold: 310, sold_gold: 0, kills: 42, waves: 9, waves_completed: 9, escaped: 2, spawned: 44,
      defeated_by_enemy: { mireling: 42 }, escaped_by_enemy: { mireling: 2 }, spawned_by_enemy: { mireling: 44 },
      hero_id: 'aerin', hero_level: 3, content_version: '2', stage_version: '4', balance_version: '3', asset_version: 'procedural-2',
      victory: true, score: 0, stars: 0,
    });
    expect(payload).toMatchObject({ remaining_lives: 18, remaining_gold: 90, waves_completed: 9, victory: true, defeated_by_enemy: { mireling: 42 } });
    expect(payload).not.toHaveProperty('score');
    expect(payload).not.toHaveProperty('stars');
  });

  it('rejects incomplete published content instead of silently using defaults', () => {
    expect(() => normalizeRealmGuardConfig({ stages: [], towers: [] })).toThrow(/게시된 RealmGuard 설정/);
  });

  it('accepts renamed stages when campaign numbering and references stay valid', () => {
    const stages = DEFAULT_REALMGUARD_CONFIG.stages.map((stage) => ({
      ...stage,
      id: stage.mode === 'campaign' ? `realm-${stage.number}` : 'infinite-veil',
    }));
    const config = normalizeRealmGuardConfig({
      version: { id: '22222222-2222-4222-8222-222222222222', content_version: 'renamed' },
      stages,
      towers: DEFAULT_REALMGUARD_CONFIG.towers,
      enemies: DEFAULT_REALMGUARD_CONFIG.enemies.filter((enemy) => !enemy.traits.includes('boss')),
      bosses: DEFAULT_REALMGUARD_CONFIG.enemies.filter((enemy) => enemy.traits.includes('boss')),
      heroes: DEFAULT_REALMGUARD_CONFIG.heroes,
      skills: DEFAULT_REALMGUARD_CONFIG.skills,
      balance: DEFAULT_REALMGUARD_CONFIG.balance,
    });
    expect(config.stages.filter((stage) => stage.mode === 'campaign').map((stage) => stage.id)).toEqual(stages.slice(0, 10).map((stage) => stage.id));
    expect(config.stages.find((stage) => stage.mode === 'endless')?.id).toBe('infinite-veil');
  });

  it('builds RealmGuard leaderboard filters for stage, period and hero', () => {
    expect(realmGuardRankingURL({ group: 'hero', period: 'season', mode: 'endless', difficulty: 'veteran', stage_id: 'endless-rift', hero_id: 'nyra' }))
      .toBe('/api/v1/realmguard/rankings?group=hero&metric=score&period=season&mode=endless&difficulty=veteran&stage_id=endless-rift&hero_id=nyra');
    expect(realmGuardRankingURL({ group: 'department', metric: 'stars', period: 'weekly', mode: 'campaign', difficulty: 'normal' })).toContain('metric=stars');
  });

  it('unwraps the RealmGuard version envelope', () => {
    expect(normalizeRealmGuardVersion({ version: { label: 'v0.2.0', content_version: '7', stage_version: '5', balance_version: '3', asset_version: 'procedural-2' } })).toEqual({ label: 'v0.2.0', content_version: '7', stage_version: '5', balance_version: '3', asset_version: 'procedural-2' });
  });

  it('normalizes aggregate progress, account hero levels and unlocks', () => {
    const progress = normalizeRealmGuardProgress({
      items: [{ stage_id: 'stage-10', difficulty: 'normal', completed: true, stars: 2 }],
      stages: [{ stage_id: 'stage-1', stars: 3, best_score: 42000, difficulties: ['casual', 'normal'] }],
      heroes: [{ hero_id: 'aerin', unlocked: true, level: 4, xp: 91 }, { hero_id: 'nyra', unlocked: false, level: 1, xp: 0 }],
      skills: [{ skill_id: 'meteor', unlocked: true, level: 2 }, { skill_id: 'freeze', unlocked: false, level: 1 }],
      hero_levels: { aerin: 4, nyra: 1 }, loadout: { hero_id: 'aerin', skill_ids: ['meteor'] },
      total_stars: 18, unlocked_stage: 11, campaign_completed: true,
    });
    expect(progress.stages[0]).toEqual({ stage_id: 'stage-1', stars: 3, best_score: 42000, difficulties: ['casual', 'normal'] });
    expect(progress.hero_levels.aerin).toBe(4);
    expect(progress.heroes.nyra.unlocked).toBe(false);
    expect(progress.skills.freeze.unlocked).toBe(false);
    expect(progress.campaign_completed).toBe(true);
  });

  it('normalizes progress returned by authoritative completion for immediate unlocks', () => {
    const completion = normalizeRealmGuardCompletion({
      result: { score: 48200, stars: 3, verified: true },
      progress: {
        stages: [{ stage_id: 'stage-1', stars: 3, best_score: 48200, difficulties: ['normal'] }],
        heroes: [{ hero_id: 'aerin', unlocked: true, level: 2, xp: 12 }, { hero_id: 'brann', unlocked: true, level: 1, xp: 0 }],
        skills: [{ skill_id: 'meteor', unlocked: true, level: 1 }, { skill_id: 'reinforcement', unlocked: true, level: 1 }],
        hero_levels: { aerin: 2, brann: 1 },
        loadout: { hero_id: 'aerin', skill_ids: ['meteor'] },
        total_stars: 3,
        unlocked_stage: 2,
      },
    });
    expect(completion.result).toMatchObject({ score: 48200, stars: 3, verified: true });
    expect(completion.progress).toMatchObject({
      unlocked_stage: 2,
      heroes: { brann: { unlocked: true } },
      skills: { reinforcement: { unlocked: true } },
    });
  });
});
