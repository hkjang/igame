import type { TargetingMode } from "../types";

export interface TargetSnapshot {
  pathProgress: number;
  hp: number;
  x: number;
  y: number;
}

/** sqrt of the squared sum, not `Math.hypot`, so Go replays bit-identically. */
function distance(dx: number, dy: number) {
  return Math.sqrt(dx * dx + dy * dy);
}

export function targetComparator(
  mode: TargetingMode,
  origin: { x: number; y: number },
  a: TargetSnapshot,
  b: TargetSnapshot,
) {
  if (mode === "last") return a.pathProgress - b.pathProgress;
  if (mode === "strong") return b.hp - a.hp;
  if (mode === "weak") return a.hp - b.hp;
  if (mode === "closest")
    return (
      distance(a.x - origin.x, a.y - origin.y) -
      distance(b.x - origin.x, b.y - origin.y)
    );
  return b.pathProgress - a.pathProgress;
}
