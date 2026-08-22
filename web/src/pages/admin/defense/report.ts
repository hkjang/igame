export function defenseReportMetrics(education: boolean) {
  return education
    ? [
        ["참여자", "participants"],
        ["완료율", "completion_rate"],
        ["평균 학습 점수", "average_score"],
        ["재도전율", "retry_rate"],
        ["재도전 개선", "improvement"],
        ["평균 플레이", "average_play_time_ms"],
      ]
    : [
        ["플레이어", "participants"],
        ["게임 실행", "plays"],
        ["완료율", "completion_rate"],
        ["평균 게임 점수", "average_game_score"],
        ["부서 참여", "department_count"],
        ["평균 플레이", "average_play_time_ms"],
      ];
}

export function formatDefenseReportMetric(key: string, value: unknown) {
  if (key === "improvement") return `${Number(value ?? 0).toFixed(1)}점`;
  if (key.includes("rate")) return `${Number(value ?? 0).toFixed(1)}%`;
  if (key.includes("time"))
    return `${Math.round(Number(value ?? 0) / 1000)}초`;
  return Number.isFinite(Number(value))
    ? Number(value).toLocaleString()
    : String(value ?? 0);
}
