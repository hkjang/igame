import { describe, expect, it } from "vitest";
import { DEFENSE_PACKS, DEFENSE_SLUGS } from "./content";
import {
  DEFENSE_MAP_GEOMETRY_COUNT,
  defenseMapLayout,
  defenseMapTowerSpots,
  type DefenseMapLayout,
} from "./maps";

const PATH_BOUNDS = {
  minimumX: -100,
  maximumX: 1380,
  minimumY: -100,
  maximumY: 820,
};
const SPOT_BOUNDS = {
  minimumX: 0,
  maximumX: 1280,
  minimumY: 0,
  maximumY: 720,
};

function geometryFingerprint(layout: DefenseMapLayout): string {
  const paths = layout.paths
    .map((path) => path.map(({ x, y }) => `${x},${y}`).join(";"))
    .join("|");
  const spots = layout.towerSpots
    .map(({ x, y }) => `${x},${y}`)
    .join(";");
  return `${paths}::${spots}`;
}

function pointKey(point: { x: number; y: number }): string {
  return `${point.x}:${point.y}`;
}

function distance(a: { x: number; y: number }, b: { x: number; y: number }) {
  return Math.hypot(a.x - b.x, a.y - b.y);
}

function segmentDistance(
  point: { x: number; y: number },
  start: { x: number; y: number },
  end: { x: number; y: number },
) {
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  const lengthSquared = dx * dx + dy * dy;
  const ratio = lengthSquared
    ? Math.max(0, Math.min(1, ((point.x - start.x) * dx + (point.y - start.y) * dy) / lengthSquared))
    : 0;
  return distance(point, { x: start.x + dx * ratio, y: start.y + dy * ratio });
}

function nearestLaneDistance(layout: DefenseMapLayout, point: { x: number; y: number }) {
  return Math.min(
    ...layout.paths.flatMap((path) =>
      path.slice(1).map((end, index) => segmentDistance(point, path[index], end)),
    ),
  );
}

describe("Defense built-in map layouts", () => {
  it("ships at least eight genuinely distinct geometry definitions", () => {
    expect(DEFENSE_MAP_GEOMETRY_COUNT).toBeGreaterThanOrEqual(8);
  });

  for (const slug of DEFENSE_SLUGS) {
    describe(slug, () => {
      const stageCount = DEFENSE_PACKS[slug].config.stages.length;
      const layouts = Array.from({ length: stageCount }, (_, index) =>
        defenseMapLayout(slug, index),
      );

      it("uses unique geometry for every built-in stage", () => {
        expect(stageCount).toBeGreaterThanOrEqual(8);
        expect(new Set(layouts.map(geometryFingerprint))).toHaveLength(
          stageCount,
        );
      });

      it("keeps every path point inside the accepted battlefield bounds", () => {
        for (const layout of layouts) {
          expect(layout.paths.length).toBeGreaterThan(0);
          for (const path of layout.paths) {
            expect(path.length).toBeGreaterThanOrEqual(2);
            for (const point of path) {
              expect(Number.isFinite(point.x)).toBe(true);
              expect(Number.isFinite(point.y)).toBe(true);
              expect(point.x).toBeGreaterThanOrEqual(PATH_BOUNDS.minimumX);
              expect(point.x).toBeLessThanOrEqual(PATH_BOUNDS.maximumX);
              expect(point.y).toBeGreaterThanOrEqual(PATH_BOUNDS.minimumY);
              expect(point.y).toBeLessThanOrEqual(PATH_BOUNDS.maximumY);
            }
          }
        }
      });

      it("provides bounded, unique and identifiable tower spots", () => {
        layouts.forEach((layout, index) => {
          expect(layout.towerSpots.length).toBeGreaterThanOrEqual(8);
          expect(new Set(layout.towerSpots.map(pointKey))).toHaveLength(
            layout.towerSpots.length,
          );

          const stageId = `stage-${index + 1}`;
          const spots = defenseMapTowerSpots(slug, index, stageId);
          expect(spots).toHaveLength(layout.towerSpots.length);
          expect(new Set(spots.map((spot) => spot.id))).toHaveLength(
            spots.length,
          );

          for (const [spotIndex, spot] of spots.entries()) {
            expect(spot.id).toBe(`${stageId}-spot-${spotIndex + 1}`);
            expect(Number.isFinite(spot.x)).toBe(true);
            expect(Number.isFinite(spot.y)).toBe(true);
            expect(spot.x).toBeGreaterThanOrEqual(SPOT_BOUNDS.minimumX);
            expect(spot.x).toBeLessThanOrEqual(SPOT_BOUNDS.maximumX);
            expect(spot.y).toBeGreaterThanOrEqual(SPOT_BOUNDS.minimumY);
            expect(spot.y).toBeLessThanOrEqual(SPOT_BOUNDS.maximumY);
            expect(
              nearestLaneDistance(layout, spot),
              `stage ${index + 1} spot ${spotIndex + 1} path clearance`,
            ).toBeGreaterThanOrEqual(40);
            expect(
              nearestLaneDistance(layout, spot),
              `stage ${index + 1} spot ${spotIndex + 1} path reach`,
            ).toBeLessThanOrEqual(190);
            for (const other of spots.slice(spotIndex + 1))
              expect(distance(spot, other)).toBeGreaterThanOrEqual(60);
          }
        });
      });

      it("includes at least one multi-lane battlefield", () => {
        expect(layouts.some((layout) => layout.paths.length > 1)).toBe(true);
      });
    });
  }
});
