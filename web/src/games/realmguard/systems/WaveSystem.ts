import type { WaveEntry } from '../types';

export interface SpawnPlan {
  enemy: string;
  at: number;
  pathIndex: number;
  modifiers: string[];
}

/** Expands sequential and parallel wave groups into a deterministic, time-sorted spawn plan. */
export function expandWave(entries: WaveEntry[], cycle = 0): SpawnPlan[] {
  const plan: SpawnPlan[] = [];
  let cursor = 0;
  for (const entry of entries) {
    const delay = Math.max(0, entry.delay ?? 0) * 1000;
    const start = entry.parallel ? delay : cursor + delay;
    const count = Math.max(1, entry.count + cycle * 2);
    const interval = Math.max(150, entry.interval * 1000);
    for (let index = 0; index < count; index += 1) plan.push({ enemy: entry.enemy, at: start + index * interval, pathIndex: Math.max(0, Math.floor(entry.pathIndex ?? 0)), modifiers: [...(entry.modifiers ?? [])] });
    if (!entry.parallel) cursor = start + count * interval;
  }
  return plan.sort((a, b) => a.at - b.at || a.pathIndex - b.pathIndex || a.enemy.localeCompare(b.enemy));
}

export function canCompleteWave(completed: boolean, waveActive: boolean, pendingSpawns: number, activeEnemies: number): boolean {
  return !completed && waveActive && pendingSpawns === 0 && activeEnemies === 0;
}
