import { describe, expect, it } from "vitest";
import {
  defenseReportMetrics,
  formatDefenseReportMetric,
} from "./report";

describe("Defense operations report formatting", () => {
  it("uses gameplay score outside education reports", () => {
    expect(defenseReportMetrics(false)).toContainEqual([
      "평균 게임 점수",
      "average_game_score",
    ]);
    expect(defenseReportMetrics(true)).toContainEqual([
      "평균 학습 점수",
      "average_score",
    ]);
  });

  it("renders retry improvement as a score delta, not a percentage", () => {
    expect(formatDefenseReportMetric("improvement", 7.25)).toBe("7.3점");
    expect(formatDefenseReportMetric("retry_rate", 31.2)).toBe("31.2%");
  });
});
