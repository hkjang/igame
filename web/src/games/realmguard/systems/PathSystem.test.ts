import { describe, expect, it } from "vitest";
import {
  closestPointOnPaths,
  normalizedPathProgress,
  pathLength,
} from "./PathSystem";

describe("PathSystem", () => {
  it("compares progress by travelled distance rather than waypoint count", () => {
    const shortSegments = [
      { x: 0, y: 0 }, { x: 20, y: 0 }, { x: 40, y: 0 },
      { x: 60, y: 0 }, { x: 100, y: 0 },
    ];
    const longSegments = [{ x: 0, y: 0 }, { x: 100, y: 0 }];
    expect(pathLength(shortSegments)).toBe(100);
    expect(normalizedPathProgress(shortSegments, 3, { x: 50, y: 0 })).toBe(.5);
    expect(normalizedPathProgress(longSegments, 1, { x: 50, y: 0 })).toBe(.5);
    expect(normalizedPathProgress(longSegments, 1, { x: 50, y: -20 }, 20)).toBe(.5);
  });

  it("projects a defense unit onto the closest segment across all lanes", () => {
    const result = closestPointOnPaths(
      [
        [{ x: 0, y: 0 }, { x: 100, y: 0 }],
        [{ x: 0, y: 100 }, { x: 100, y: 100 }],
      ],
      { x: 45, y: 82 },
    );
    expect(result.laneIndex).toBe(1);
    expect(result.point).toEqual({ x: 45, y: 100 });
    expect(result.distance).toBe(18);
  });
});
