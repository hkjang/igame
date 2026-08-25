import type { Point } from "../types";

function distance(a: Point, b: Point) {
  return Math.hypot(b.x - a.x, b.y - a.y);
}

export function pathLength(path: Point[]) {
  let total = 0;
  for (let index = 1; index < path.length; index += 1)
    total += distance(path[index - 1], path[index]);
  return total;
}

/** Returns comparable 0..1 progress even when lanes use different waypoint counts. */
export function normalizedPathProgress(
  path: Point[],
  nextPointIndex: number,
  current: Point,
  displayYOffset = 0,
) {
  const total = pathLength(path);
  if (total <= 0 || path.length < 2) return 0;
  if (nextPointIndex >= path.length) return 1;
  const next = Math.max(1, nextPointIndex);
  let travelled = 0;
  for (let index = 1; index < next; index += 1)
    travelled += distance(path[index - 1], path[index]);
  const pathSpaceCurrent = {
    x: current.x,
    y: current.y + displayYOffset,
  };
  travelled += Math.min(
    distance(path[next - 1], pathSpaceCurrent),
    distance(path[next - 1], path[next]),
  );
  return Math.max(0, Math.min(1, travelled / total));
}

function closestPointOnSegment(point: Point, start: Point, end: Point) {
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  const lengthSquared = dx * dx + dy * dy;
  if (lengthSquared === 0) return { ...start };
  const projection = Math.max(
    0,
    Math.min(
      1,
      ((point.x - start.x) * dx + (point.y - start.y) * dy) / lengthSquared,
    ),
  );
  return { x: start.x + dx * projection, y: start.y + dy * projection };
}

/** Finds a true segment projection across every lane, not just a waypoint. */
export function closestPointOnPaths(paths: Point[][], point: Point) {
  let closest = paths[0]?.[0] ? { ...paths[0][0] } : { ...point };
  let bestDistance = Number.POSITIVE_INFINITY;
  let laneIndex = 0;
  for (let pathIndex = 0; pathIndex < paths.length; pathIndex += 1) {
    const path = paths[pathIndex];
    for (let index = 1; index < path.length; index += 1) {
      const candidate = closestPointOnSegment(point, path[index - 1], path[index]);
      const candidateDistance = distance(point, candidate);
      if (candidateDistance < bestDistance) {
        closest = candidate;
        bestDistance = candidateDistance;
        laneIndex = pathIndex;
      }
    }
  }
  return { point: closest, laneIndex, distance: bestDistance };
}
