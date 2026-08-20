import { describe, expect, it } from 'vitest';
import { DEFAULT_REALMGUARD_CONFIG } from '../content';
import { effectiveDamage, movementMultiplier } from './CombatMath';
import { calculateResult, calculateStartingGold } from './RewardSystem';
import { targetComparator } from './TargetSystem';
import { canCompleteWave, expandWave } from './WaveSystem';

describe('RealmGuard deterministic systems', () => {
  it('expands two lanes and parallel spawn groups deterministically', () => {
    const plan = expandWave([{ enemy: 'mireling', count: 2, interval: 1, pathIndex: 0 }, { enemy: 'glintfox', count: 2, interval: .5, delay: .25, pathIndex: 1, parallel: true }]);
    expect(plan.map((item) => [item.enemy, item.at, item.pathIndex])).toEqual([['mireling', 0, 0], ['glintfox', 250, 1], ['glintfox', 750, 1], ['mireling', 1000, 0]]);
  });

  it('never completes a wave after a defeat has already completed the battle', () => {
    expect(canCompleteWave(true, true, 0, 0)).toBe(false);
    expect(canCompleteWave(false, true, 0, 0)).toBe(true);
  });

  it('orders first, last and closest targets', () => {
    const origin = { x: 0, y: 0 }; const a = { pathProgress: 1, hp: 20, x: 30, y: 0 }; const b = { pathProgress: 3, hp: 50, x: 80, y: 0 };
    expect(targetComparator('first', origin, a, b)).toBeGreaterThan(0);
    expect(targetComparator('last', origin, a, b)).toBeLessThan(0);
    expect(targetComparator('closest', origin, a, b)).toBeLessThan(0);
  });

  it('applies advanced enemy modifiers', () => {
    expect(effectiveDamage(100, 'arcane', 0, new Set(['magic_resist']))).toBe(52);
    expect(movementMultiplier(new Set(['berserk']), .3, false, 1, false)).toBe(1.5);
    expect(movementMultiplier(new Set(['immune_stun']), 1, true, .2, false)).toBe(1);
  });

  it('mirrors server economy and scoring', () => {
    const balance = DEFAULT_REALMGUARD_CONFIG.balance; const stage = DEFAULT_REALMGUARD_CONFIG.stages[0];
    expect(calculateStartingGold(stage, balance, 'casual')).toBe(Math.round(stage.startingGold * 1.18));
    expect(calculateResult({ victory: false, lives: 20, waves: 7, gold: 100, difficulty: 'normal', mode: 'endless' }, balance).score).toBe(33_000);
  });
});
