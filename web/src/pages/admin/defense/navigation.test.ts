import { describe, expect, it } from "vitest";
import {
  defenseVersionsForSlug,
  mergeCreatedDefenseVersion,
  normalizeDefenseStudioView,
} from "./navigation";

describe("Defense Content Studio navigation", () => {
  it("restores every persisted subview and rejects unknown values", () => {
    for (const view of ["editor", "versions", "telemetry", "report"])
      expect(normalizeDefenseStudioView(view)).toBe(view);
    expect(normalizeDefenseStudioView("unknown")).toBe("editor");
    expect(normalizeDefenseStudioView(null)).toBe("editor");
  });

  it("makes a newly created version selectable before the server reload", () => {
    const previous = {
      items: [
        { id: "published", label: "Published" },
        { id: "draft", label: "Old draft" },
      ],
    };
    const created = { id: "draft", label: "New draft" };

    const merged = mergeCreatedDefenseVersion(previous, created);

    expect(merged.items).toEqual([
      created,
      { id: "published", label: "Published" },
    ]);
    expect(merged.items.some((item) => item.id === created.id)).toBe(true);
  });

  it("never exposes another game's stale versions during a game switch", () => {
    const office = {
      slug: "office-guardians",
      items: [{ id: "office-draft" }],
    };

    expect(defenseVersionsForSlug(office, "cyber-fortress")).toEqual([]);
    expect(defenseVersionsForSlug(office, "office-guardians")).toEqual(
      office.items,
    );
  });
});
