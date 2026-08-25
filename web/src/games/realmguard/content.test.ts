import { describe, expect, it } from 'vitest';
import { advanceFixedSimulation, calculateLocalResult, calculateStartingGold, DEFAULT_REALMGUARD_CONFIG, simulationCooldownReady } from './content';

describe('RealmGuard content', () => {
  it('ships a complete campaign and endless mode', () => {
    expect(DEFAULT_REALMGUARD_CONFIG.stages.filter((stage) => stage.mode === 'campaign')).toHaveLength(10);
    expect(DEFAULT_REALMGUARD_CONFIG.stages.some((stage) => stage.mode === 'endless')).toBe(true);
    expect(DEFAULT_REALMGUARD_CONFIG.stages.every((stage) => stage.towerSpots.length >= 8 && stage.waves.length >= 8 && stage.waves.length <= 15)).toBe(true);
  });

  it('contains the designed combat roster and branches', () => {
    expect(DEFAULT_REALMGUARD_CONFIG.enemies.filter((enemy) => enemy.traits.includes('boss'))).toHaveLength(2);
    expect(DEFAULT_REALMGUARD_CONFIG.enemies.length).toBeGreaterThanOrEqual(12);
    expect(DEFAULT_REALMGUARD_CONFIG.towers).toHaveLength(4);
    expect(DEFAULT_REALMGUARD_CONFIG.towers.flatMap((tower) => tower.branches)).toHaveLength(8);
    expect(DEFAULT_REALMGUARD_CONFIG.heroes).toHaveLength(3);
    expect(DEFAULT_REALMGUARD_CONFIG.heroes.every((hero) => hero.skill1 && hero.skill2 && hero.ultimate && hero.respawnSeconds > 0)).toBe(true);
    expect(DEFAULT_REALMGUARD_CONFIG.heroes.map((hero) => hero.unlockStage)).toEqual([1, 3, 6]);
    expect(DEFAULT_REALMGUARD_CONFIG.towers.some((tower) => tower.role.includes('병사 소환'))).toBe(true);
    expect(DEFAULT_REALMGUARD_CONFIG.skills.map((skill) => skill.id)).toEqual(['meteor', 'reinforcement', 'freeze']);
  });

  it('awards campaign stars from remaining lives', () => {
    expect(calculateLocalResult({ victory: true, lives: 18, kills: 40, waves: 6, gold: 100, difficulty: 'normal', mode: 'campaign' }).stars).toBe(3);
    expect(calculateLocalResult({ victory: true, lives: 17, kills: 40, waves: 6, gold: 100, difficulty: 'normal', mode: 'campaign' }).stars).toBe(2);
    expect(calculateLocalResult({ victory: true, lives: 9, kills: 40, waves: 6, gold: 100, difficulty: 'normal', mode: 'campaign' }).stars).toBe(1);
    expect(calculateLocalResult({ victory: false, lives: 0, kills: 40, waves: 6, gold: 100, difficulty: 'normal', mode: 'campaign' }).stars).toBe(0);
  });

  it('matches server starting gold and score formulas for every difficulty and endless waves', () => {
    const stage = DEFAULT_REALMGUARD_CONFIG.stages[0];
    expect(calculateStartingGold(stage, DEFAULT_REALMGUARD_CONFIG.balance, 'casual')).toBe(Math.round(stage.startingGold * 1.18));
    expect(calculateStartingGold(stage, DEFAULT_REALMGUARD_CONFIG.balance, 'normal')).toBe(stage.startingGold);
    expect(calculateStartingGold(stage, DEFAULT_REALMGUARD_CONFIG.balance, 'veteran')).toBe(Math.round(stage.startingGold * .9));
    expect(calculateLocalResult({ victory: false, lives: 20, kills: 0, waves: 7, gold: 100, duration_ms: 60_000, difficulty: 'normal', mode: 'endless' }).score)
      .toBe(20_000 + 1_000 + 7_000 + 5_000);
  });

  it('advances every simulation cooldown exactly twice as fast at 2x without changing wall time', () => {
    const one = Array.from({ length: 4 }).reduce<{ remainder: number; elapsed: number }>((clock) => {
      const next = advanceFixedSimulation(clock.remainder, 250, 1);
      return { remainder: next.remainder, elapsed: clock.elapsed + next.elapsed };
    }, { remainder: 0, elapsed: 0 });
    const two = Array.from({ length: 4 }).reduce<{ remainder: number; elapsed: number }>((clock) => {
      const next = advanceFixedSimulation(clock.remainder, 250, 2);
      return { remainder: next.remainder, elapsed: clock.elapsed + next.elapsed };
    }, { remainder: 0, elapsed: 0 });
    expect(one.elapsed).toBe(1_000);
    expect(two.elapsed).toBe(2_000);
    expect(simulationCooldownReady(one.elapsed, 0, 1_500)).toBe(false);
    expect(simulationCooldownReady(two.elapsed, 0, 1_500)).toBe(true);
  });
});
