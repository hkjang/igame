import type Phaser from "phaser";
import type { EnemyPresentation } from "./enemyPresentation";

/**
 * Paints an enemy from its presentation tokens.
 *
 * Kept out of the scene because it is the largest piece of pure drawing in the
 * product and touches no battle state: the kernel decides what an enemy does,
 * this decides only what it looks like. Everything is drawn once when the
 * sprite is created, in local space around (0, 0), scaled by the content's own
 * radius so a boss and a pest use the same code.
 */

type Graphics = Phaser.GameObjects.Graphics;

function hex(value: string): number {
  return Number.parseInt(value.replace("#", ""), 16);
}

function eyes(graphics: Graphics, presentation: EnemyPresentation, radius: number, y: number) {
  const accent = hex(presentation.accent);
  const size = Math.max(1.6, radius * 0.16);
  if (presentation.eyes === 1) {
    graphics.fillStyle(accent, 0.95).fillCircle(0, y, size * 1.25);
    graphics.fillStyle(0x0a1119, 0.85).fillCircle(0, y, size * 0.5);
    return;
  }
  if (presentation.eyes === 3) {
    for (const offset of [-radius * 0.34, 0, radius * 0.34])
      graphics.fillStyle(accent, 0.9).fillCircle(offset, y, size * 0.8);
    return;
  }
  for (const offset of [-radius * 0.3, radius * 0.3]) {
    graphics.fillStyle(accent, 0.95).fillCircle(offset, y, size);
    graphics.fillStyle(0x0a1119, 0.85).fillCircle(offset, y, size * 0.42);
  }
}

function blob(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  graphics.fillStyle(hex(presentation.primary), 1).fillCircle(0, radius * 0.08, radius);
  graphics
    .fillStyle(hex(presentation.secondary), 0.75)
    .fillEllipse(0, radius * 0.72, radius * 1.5, radius * 0.55);
  eyes(graphics, presentation, radius, -radius * 0.16);
}

function bristle(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const secondary = hex(presentation.secondary);
  graphics.fillStyle(secondary, 1);
  for (let index = 0; index < 5; index += 1) {
    const x = -radius * 0.72 + index * radius * 0.36;
    graphics.fillTriangle(x - radius * 0.16, -radius * 0.2, x, -radius * 1.35, x + radius * 0.16, -radius * 0.2);
  }
  graphics.fillStyle(hex(presentation.primary), 1).fillEllipse(0, radius * 0.1, radius * 2, radius * 1.6);
  eyes(graphics, presentation, radius, -radius * 0.08);
}

function runner(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  const secondary = hex(presentation.secondary);
  // Leaning forward: a swift enemy should read as moving even while still.
  graphics
    .fillStyle(primary, 1)
    .fillTriangle(-radius * 1.1, radius * 0.7, radius * 1.15, 0, -radius * 0.75, -radius * 0.75);
  graphics.fillStyle(secondary, 0.9)
    .fillTriangle(-radius * 0.95, -radius * 0.55, -radius * 0.45, -radius * 1.3, -radius * 0.3, -radius * 0.45)
    .fillTriangle(-radius * 0.45, -radius * 0.6, 0, -radius * 1.25, radius * 0.05, -radius * 0.4);
  graphics.fillStyle(hex(presentation.accent), 0.95).fillCircle(radius * 0.55, -radius * 0.05, Math.max(1.5, radius * 0.15));
}

function flyer(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  graphics
    .fillStyle(primary, 0.95)
    .fillTriangle(-radius * 2.1, 0, 0, -radius * 0.85, radius * 2.1, 0)
    .fillTriangle(-radius * 2.1, 0, 0, radius * 0.95, radius * 2.1, 0);
  graphics.fillStyle(hex(presentation.secondary), 0.85).fillEllipse(0, 0, radius * 0.9, radius * 1.5);
  graphics.lineStyle(Math.max(1, radius * 0.1), hex(presentation.accent), 0.6)
    .lineBetween(0, radius * 0.6, 0, radius * 1.9);
  eyes(graphics, presentation, radius, -radius * 0.1);
}

function seer(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  // One figure: robe, hood and the orb it holds, all touching.
  graphics
    .fillStyle(primary, 1)
    .fillTriangle(-radius, radius * 1.05, radius, radius * 1.05, 0, -radius * 0.35);
  graphics.fillStyle(hex(presentation.secondary), 0.95).fillCircle(0, -radius * 0.5, radius * 0.52);
  graphics.fillStyle(hex(presentation.accent), 0.9).fillCircle(radius * 0.72, radius * 0.1, radius * 0.3);
  graphics
    .lineStyle(Math.max(1, radius * 0.1), hex(presentation.accent), 0.8)
    .lineBetween(radius * 0.72, radius * 0.34, radius * 0.62, radius * 1.05);
  eyes(graphics, presentation, radius, -radius * 0.5);
}

function crystal(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  const secondary = hex(presentation.secondary);
  // A single tall shard with one shaded face reads at 12px; a cluster does not.
  graphics
    .fillStyle(primary, 1)
    .fillTriangle(0, -radius * 1.2, -radius * 0.9, radius * 0.5, radius * 0.9, radius * 0.5);
  graphics
    .fillStyle(primary, 1)
    .fillTriangle(-radius * 0.9, radius * 0.5, radius * 0.9, radius * 0.5, 0, radius * 1.1);
  graphics
    .fillStyle(secondary, 0.75)
    .fillTriangle(0, -radius * 1.2, radius * 0.9, radius * 0.5, 0, radius * 1.1);
  eyes(graphics, presentation, radius, -radius * 0.15);
}

function treant(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  const secondary = hex(presentation.secondary);
  graphics.fillStyle(secondary, 1)
    .fillTriangle(-radius * 0.95, radius * 1.15, -radius * 0.15, radius * 0.2, -radius * 0.15, radius * 1.15)
    .fillTriangle(radius * 0.95, radius * 1.15, radius * 0.15, radius * 0.2, radius * 0.15, radius * 1.15);
  graphics.fillStyle(primary, 1).fillRoundedRect(-radius * 0.75, -radius * 1.1, radius * 1.5, radius * 2.1, radius * 0.3);
  graphics.fillStyle(secondary, 0.8)
    .fillTriangle(-radius * 1.25, -radius * 0.55, -radius * 0.6, -radius * 0.95, -radius * 0.6, -radius * 0.25)
    .fillTriangle(radius * 1.25, -radius * 0.55, radius * 0.6, -radius * 0.95, radius * 0.6, -radius * 0.25);
  eyes(graphics, presentation, radius, -radius * 0.45);
}

function wraith(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  graphics.fillStyle(primary, 0.92).fillCircle(0, -radius * 0.35, radius * 0.9);
  // A torn hem rather than a closed body: this thing walks through things.
  graphics.fillStyle(primary, 0.82).fillTriangle(-radius * 0.9, -radius * 0.35, radius * 0.9, -radius * 0.35, 0, radius * 1.3);
  graphics.fillStyle(hex(presentation.secondary), 0.9)
    .fillTriangle(-radius * 0.9, -radius * 0.5, -radius * 0.2, -radius * 1.35, radius * 0.15, -radius * 0.5);
  eyes(graphics, presentation, radius, -radius * 0.45);
}

function ram(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  const secondary = hex(presentation.secondary);
  graphics.fillStyle(primary, 1).fillRoundedRect(-radius, -radius * 0.75, radius * 2, radius * 1.5, radius * 0.35);
  graphics.fillStyle(secondary, 1)
    .fillTriangle(radius * 0.85, -radius * 0.6, radius * 1.6, 0, radius * 0.85, radius * 0.6);
  graphics.fillStyle(secondary, 0.9)
    .fillRect(-radius * 0.85, radius * 0.6, radius * 0.5, radius * 0.55)
    .fillRect(radius * 0.35, radius * 0.6, radius * 0.5, radius * 0.55);
  eyes(graphics, presentation, radius, -radius * 0.15);
}

function core(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  graphics.fillStyle(primary, 1)
    .fillTriangle(0, -radius * 1.2, radius * 1.15, 0, 0, radius * 1.2)
    .fillTriangle(0, -radius * 1.2, -radius * 1.15, 0, 0, radius * 1.2);
  graphics.lineStyle(Math.max(1.2, radius * 0.14), hex(presentation.secondary), 0.9).strokeCircle(0, 0, radius * 0.72);
  graphics.fillStyle(hex(presentation.accent), 0.95).fillCircle(0, 0, radius * 0.3);
}

function sovereign(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  const secondary = hex(presentation.secondary);
  const accent = hex(presentation.accent);
  graphics.fillStyle(primary, 1)
    .fillTriangle(-radius * 0.95, radius * 1.15, radius * 0.95, radius * 1.15, 0, -radius * 0.35);
  graphics.fillStyle(secondary, 0.95).fillCircle(0, -radius * 0.55, radius * 0.5);
  graphics.fillStyle(accent, 1)
    .fillTriangle(-radius * 0.58, -radius * 0.88, -radius * 0.4, -radius * 1.28, -radius * 0.2, -radius * 0.88)
    .fillTriangle(-radius * 0.2, -radius * 0.88, 0, -radius * 1.42, radius * 0.2, -radius * 0.88)
    .fillTriangle(radius * 0.2, -radius * 0.88, radius * 0.4, -radius * 1.28, radius * 0.58, -radius * 0.88);
  eyes(graphics, presentation, radius, -radius * 0.6);
}

function drake(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const primary = hex(presentation.primary);
  const secondary = hex(presentation.secondary);
  const accent = hex(presentation.accent);
  graphics.fillStyle(secondary, 0.92)
    .fillTriangle(-radius * 0.35, -radius * 0.3, -radius * 1.25, -radius * 1.05, -radius * 0.45, radius * 0.55)
    .fillTriangle(radius * 0.35, -radius * 0.3, radius * 1.25, -radius * 1.05, radius * 0.45, radius * 0.55);
  // Long coiled body, then a narrow head on top of it.
  graphics.fillStyle(primary, 1).fillEllipse(0, radius * 0.35, radius * 1.25, radius * 1.6);
  graphics.fillStyle(primary, 1)
    .fillTriangle(-radius * 0.52, -radius * 0.25, radius * 0.52, -radius * 0.25, 0, -radius * 1.15);
  graphics.fillStyle(accent, 0.95)
    .fillTriangle(-radius * 0.46, -radius * 0.55, -radius * 0.24, -radius * 1.15, -radius * 0.12, -radius * 0.5)
    .fillTriangle(radius * 0.46, -radius * 0.55, radius * 0.24, -radius * 1.15, radius * 0.12, -radius * 0.5);
  eyes(graphics, presentation, radius * 0.72, -radius * 0.5);
}

const PAINTERS: Record<EnemyPresentation["silhouette"], (graphics: Graphics, presentation: EnemyPresentation, radius: number) => void> = {
  blob,
  bristle,
  runner,
  flyer,
  seer,
  crystal,
  treant,
  wraith,
  ram,
  core,
  sovereign,
  drake,
};

/**
 * A mark the silhouette already communicates. A ray shape does not also need a
 * pair of wing lines, and stacking every trait at once turned the boss into a
 * pile of rings — the marks are worth drawing precisely where the shape does
 * not already say it, such as an armoured runner.
 */
const EVERY_TRAIT_MARK = [
  "armored",
  "flying",
  "healer",
  "splitting",
  "phasing",
  "siege",
  "regenerating",
  "swift",
  "stealth",
  "berserk",
] as const;

const IMPLIED: Partial<Record<EnemyPresentation["silhouette"], readonly string[]>> = {
  flyer: ["flying"],
  ram: ["siege"],
  seer: ["healer"],
  crystal: ["splitting"],
  wraith: ["phasing"],
  core: ["regenerating"],
  bristle: ["armored"],
  runner: ["swift"],
  // A boss already arrives with a name plate over its head and a gold ring
  // around it. Stacking its three or four traits on top turned the two set
  // pieces of the campaign into a pile of rings.
  sovereign: EVERY_TRAIT_MARK,
  drake: EVERY_TRAIT_MARK,
};

/** Trait marks, drawn over the body so they read the same on every silhouette. */
function paintMarks(graphics: Graphics, presentation: EnemyPresentation, radius: number) {
  const accent = hex(presentation.accent);
  const implied = IMPLIED[presentation.silhouette] ?? [];
  const has = (mark: string) =>
    !implied.includes(mark) && presentation.marks.includes(mark as EnemyPresentation["marks"][number]);
  if (has("armored")) {
    // Across the chest, where every silhouette has body: higher up it covered
    // the face of the tall ones.
    graphics.fillStyle(0xd6d4c8, 0.9)
      .fillTriangle(-radius * 0.62, radius * 0.62, 0, -radius * 0.2, radius * 0.62, radius * 0.62);
    graphics.lineStyle(Math.max(1, radius * 0.09), 0x8f8d80, 0.8)
      .lineBetween(-radius * 0.36, radius * 0.24, radius * 0.36, radius * 0.24);
  }
  if (has("flying")) {
    graphics.lineStyle(Math.max(1.4, radius * 0.13), 0xcaf3ff, 0.75)
      .strokeEllipse(0, radius * 1.15, radius * 1.5, radius * 0.42);
  }
  if (has("healer")) {
    graphics.fillStyle(0x8ff0bd, 0.95)
      .fillRect(-radius * 0.11, -radius * 1.15, radius * 0.22, radius * 0.62)
      .fillRect(-radius * 0.32, -radius * 0.95, radius * 0.64, radius * 0.22);
  }
  if (has("splitting")) {
    graphics.lineStyle(Math.max(1, radius * 0.1), 0xffffff, 0.55)
      .lineBetween(-radius * 0.5, -radius * 0.5, radius * 0.15, radius * 0.35)
      .lineBetween(radius * 0.5, -radius * 0.35, -radius * 0.1, radius * 0.5);
  }
  if (has("phasing")) {
    graphics.lineStyle(Math.max(1, radius * 0.1), accent, 0.5)
      .strokeCircle(0, 0, radius * 1.15);
  }
  if (has("siege")) {
    graphics.fillStyle(0x9aa4ad, 0.95)
      .fillRect(radius * 0.75, -radius * 0.26, radius * 0.5, radius * 0.52);
  }
  if (has("regenerating")) {
    graphics.lineStyle(Math.max(1, radius * 0.09), 0x9cf56b, 0.7)
      .strokeCircle(0, 0, radius * 0.95);
  }
  if (has("swift")) {
    graphics.lineStyle(Math.max(1, radius * 0.08), 0xffffff, 0.55)
      .lineBetween(-radius * 1.5, -radius * 0.3, -radius * 1.05, -radius * 0.3)
      .lineBetween(-radius * 1.35, radius * 0.15, -radius, radius * 0.15);
  }
  if (has("stealth")) {
    graphics.fillStyle(0x0b1220, 0.28).fillCircle(0, 0, radius * 1.15);
  }
  if (has("berserk")) {
    graphics.fillStyle(0xff6f72, 0.95)
      .fillTriangle(-radius * 0.7, -radius * 0.8, -radius * 0.44, -radius * 1.22, -radius * 0.2, -radius * 0.8)
      .fillTriangle(radius * 0.2, -radius * 0.8, radius * 0.44, -radius * 1.22, radius * 0.7, -radius * 0.8);
  }
  if (presentation.marks.includes("boss")) {
    graphics.lineStyle(Math.max(1.5, radius * 0.09), 0xffd166, 0.9).strokeCircle(0, 0, radius * 1.22);
  }
}

/**
 * Draws one enemy into a fresh Graphics object. The caller owns the object and
 * its position; this only ever paints around the origin.
 */
export function drawEnemyBody(
  graphics: Graphics,
  presentation: EnemyPresentation,
  radius: number,
): void {
  graphics.fillStyle(0x07111e, 0.45).fillEllipse(radius * 0.18, radius * 1.05, radius * 1.8, radius * 0.6);
  PAINTERS[presentation.silhouette](graphics, presentation, radius);
  paintMarks(graphics, presentation, radius);
}
