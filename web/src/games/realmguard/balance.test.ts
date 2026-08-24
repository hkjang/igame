import { describe, expect, it } from 'vitest';
import { calculateTowerStats, effectiveDamage } from './systems/CombatMath';
import { BALANCE, ENEMIES, TOWERS } from './content';

const regular = ENEMIES.filter((enemy) => !enemy.traits.includes('boss'));
const costToLevel3 = (base: number) => base + BALANCE.towerUpgradeCost[1] + BALANCE.towerUpgradeCost[2];

/** Damage a shot actually lands, averaged over the regular enemy roster. */
function landedDamage(amount: number, type: string): number {
  const total = regular.reduce(
    (sum, enemy) => sum + effectiveDamage(amount, type as never, enemy.armor, new Set(enemy.traits)),
    0,
  );
  return total / regular.length;
}

function damagePerGold(towerIndex: number, branchId?: string): number {
  const tower = TOWERS[towerIndex];
  const level = branchId ? 3 : 1;
  const stats = calculateTowerStats(tower, level, branchId);
  const dps = landedDamage(stats.damage, tower.damageType) / stats.fireRate;
  return dps / (branchId ? costToLevel3(tower.cost) : tower.cost);
}

const branches = TOWERS.flatMap((tower, index) =>
  tower.branches.map((branch) => ({
    tower: tower.id,
    branch: branch.id,
    towerIndex: index,
    perGold: damagePerGold(index, branch.id),
  })),
);

describe('RealmGuard tower balance', () => {
  // shield_line shipped with slow 0.68 against the tower's own 0.52 default.
  // Since the value multiplies enemy speed, the branch sold as "강력한 지상 저지"
  // made enemies move faster than leaving the tower un-upgraded.
  it('never gives a slowing branch a weaker slow than the tower it upgrades', () => {
    TOWERS.forEach((tower, index) => {
      const base = calculateTowerStats(tower, 1).slow;
      if (base <= 0) return;
      for (const branch of tower.branches) {
        const upgraded = calculateTowerStats(tower, 3, branch.id).slow;
        expect(
          upgraded,
          `${tower.id}/${branch.id} slows to ${upgraded} where the base tower already slows to ${base}`,
        ).toBeLessThanOrEqual(base);
      }
      expect(index).toBeGreaterThanOrEqual(0);
    });
  });

  it('never makes a fully upgraded tower less gold-efficient than its own base', () => {
    for (const entry of branches) {
      const base = damagePerGold(entry.towerIndex);
      const tower = TOWERS[entry.towerIndex];
      // A branch may trade damage for control, but only where it actually buys
      // control: shield_line gave up damage and weakened the slow as well.
      const buysControl =
        calculateTowerStats(tower, 3, entry.branch).slow < calculateTowerStats(tower, 1).slow ||
        calculateTowerStats(tower, 3, entry.branch).splash > calculateTowerStats(tower, 1).splash;
      if (buysControl) continue;
      expect(
        entry.perGold,
        `${entry.tower}/${entry.branch} is worth less per gold at level 3 (${entry.perGold.toFixed(3)}) than at level 1 (${base.toFixed(3)})`,
      ).toBeGreaterThanOrEqual(base);
    }
  });

  it('keeps every damage branch within reach of the best one', () => {
    // ember_core once led by 200% over the weakest branch, so no other choice
    // was defensible. A spread this wide means the roster has one answer.
    const best = Math.max(...branches.map((entry) => entry.perGold));
    const damageBranches = branches.filter((entry) => {
      const tower = TOWERS[entry.towerIndex];
      return calculateTowerStats(tower, 3, entry.branch).slow >= calculateTowerStats(tower, 1).slow;
    });
    for (const entry of damageBranches) {
      expect(
        entry.perGold,
        `${entry.tower}/${entry.branch} at ${entry.perGold.toFixed(3)} per gold against a best of ${best.toFixed(3)}`,
      ).toBeGreaterThan(best * 0.65);
    }
  });

  it('leaves no branch strictly dominated inside its own tower', () => {
    for (const tower of TOWERS) {
      if (tower.branches.length < 2) continue;
      const stats = tower.branches.map((branch) => ({
        id: branch.id,
        ...calculateTowerStats(tower, 3, branch.id),
      }));
      for (const candidate of stats) {
        const dominated = stats.some(
          (other) =>
            other.id !== candidate.id &&
            other.damage / other.fireRate >= candidate.damage / candidate.fireRate &&
            other.range >= candidate.range &&
            other.splash >= candidate.splash &&
            other.pierce >= candidate.pierce &&
            other.slow <= candidate.slow &&
            (other.damage / other.fireRate > candidate.damage / candidate.fireRate ||
              other.range > candidate.range ||
              other.splash > candidate.splash),
        );
        expect(dominated, `${tower.id}/${candidate.id} is beaten on every axis by its sibling branch`).toBe(false);
      }
    }
  });
});
