import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { normalizeRealmGuardConfig } from "../api";
import type { RealmDifficulty } from "../types";
import { kernelDigest, projectKernelConfig, withLoadout } from "./config";

/**
 * The browser normalizes published content and the Go verifier projects the
 * same content independently. Both read the committed snapshot of what
 * `/api/v1/realmguard/config` actually serves for the canonical seed, so if
 * either side starts reading a field differently the digests stop matching
 * here, instead of in production where every submitted battle would be refused.
 */
const TESTDATA = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../../internal/api/testdata",
);
const CONFIG_PATH = resolve(TESTDATA, "realmguard_published_config.json");
const FIXTURE_PATH = resolve(TESTDATA, "realmguard_projection.json");

function publishedConfig() {
  return normalizeRealmGuardConfig(
    JSON.parse(readFileSync(CONFIG_PATH, "utf8")) as unknown,
  );
}

describe("kernel projection", () => {
  const normalized = publishedConfig();
  // skill_ids is the loadout, and it is the dimension that was missing: skills
  // unlock with progress, so the first stage a fresh account can play is fought
  // with one skill while the content declares three. The server projected all
  // three, the digests disagreed, and every battle came back as a content
  // mismatch. The single-skill case below is that battle.
  const cases: Array<{ stage_id: string; difficulty: RealmDifficulty; hero_id: string; skill_ids: string[] }> = [
    { stage_id: "stage-1", difficulty: "normal", hero_id: "aerin", skill_ids: ["meteor"] },
    { stage_id: "stage-1", difficulty: "veteran", hero_id: "aerin", skill_ids: ["meteor", "reinforcement", "freeze"] },
    { stage_id: "stage-4", difficulty: "casual", hero_id: "brann", skill_ids: ["meteor", "reinforcement"] },
    { stage_id: "stage-5", difficulty: "veteran", hero_id: "nyra", skill_ids: ["freeze", "meteor"] },
    { stage_id: "stage-7", difficulty: "normal", hero_id: "brann", skill_ids: ["reinforcement"] },
    { stage_id: "stage-9", difficulty: "veteran", hero_id: "aerin", skill_ids: ["meteor", "reinforcement", "freeze"] },
    { stage_id: "stage-10", difficulty: "casual", hero_id: "nyra", skill_ids: ["freeze"] },
    { stage_id: "endless-rift", difficulty: "veteran", hero_id: "nyra", skill_ids: ["meteor", "freeze"] },
  ];

  const digested = cases.map((item) => {
    const stage = normalized.stages.find((entry) => entry.id === item.stage_id)!;
    return {
      ...item,
      digest: kernelDigest(
        projectKernelConfig(
          withLoadout(normalized, item.skill_ids),
          stage,
          item.difficulty,
          item.hero_id,
        ),
      ),
    };
  });

  it("agrees with the Go projection of the same published content", () => {
    const serialized = `${JSON.stringify({ cases: digested }, null, 2)}\n`;
    if (process.env.UPDATE_KERNEL_VECTORS === "1") {
      mkdirSync(dirname(FIXTURE_PATH), { recursive: true });
      writeFileSync(FIXTURE_PATH, serialized);
    }
    expect(readFileSync(FIXTURE_PATH, "utf8")).toBe(serialized);
  });

  it("covers every difficulty, hero and stage shape a battle can start from", () => {
    expect(new Set(digested.map((item) => item.difficulty)).size).toBe(3);
    expect(new Set(digested.map((item) => item.hero_id)).size).toBe(3);
    expect(digested.some((item) => item.stage_id === "endless-rift")).toBe(true);
    // A loadout of one, of two and of three: the sizes a campaign actually
    // passes through, and the ones the projection has to agree on.
    expect(new Set(digested.map((item) => item.skill_ids.length))).toEqual(new Set([1, 2, 3]));
    expect(new Set(digested.map((item) => item.digest)).size).toBe(digested.length);
  });

  it("reads the canonical published snapshot, not the offline fallback", () => {
    expect(normalized.contentVersion).toBe("0.3.1");
    expect(normalized.stages).toHaveLength(11);
    expect(normalized.stages.every((stage) => stage.waves.length >= 8)).toBe(true);
  });
});
