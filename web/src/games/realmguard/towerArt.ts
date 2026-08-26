import type { ShapePainter } from "./shapePainter";
import type { BranchEmblem, TowerForm, TowerPresentation } from "./towerPresentation";

/**
 * Paints a tower from its form, its level and the branch it took.
 *
 * Written against the shared painting surface and around the origin, like the
 * enemy art, so the battlefield and any card in the DOM draw the same tower.
 * Nothing here reaches the battle rules: the kernel decides what a tower does.
 */

const BASE_RADIUS = 27;

function ring(painter: ShapePainter, colour: number, level: number) {
  painter.fillStyle(0x0a1522, 0.9).fillCircle(0, 0, BASE_RADIUS);
  painter.lineStyle(3 + level, colour, 0.95).strokeCircle(0, 0, 20 + level * 2);
  // Level as pips on the ring rather than a text badge: the badge sat on the
  // foot of the tower, which is where the branch emblem has to go.
  for (let index = 0; index < 3; index += 1) {
    const angle = -Math.PI / 2 + (index - 1) * 0.42;
    const x = Math.cos(angle) * (BASE_RADIUS - 2);
    const y = Math.sin(angle) * (BASE_RADIUS - 2);
    painter.fillStyle(index < level ? 0xffffff : 0x2a3b4d, index < level ? 0.95 : 0.9).fillCircle(x, y, 3);
  }
}

function spire(painter: ShapePainter, colour: number) {
  painter.fillStyle(colour, 1).fillTriangle(0, -22, -13, 13, 13, 13);
  painter.fillStyle(0x0a1522, 0.55).fillTriangle(0, -12, -6, 12, 6, 12);
}

function garden(painter: ShapePainter, colour: number) {
  for (let index = 0; index < 6; index += 1) {
    painter
      .fillStyle(colour, 0.85)
      .fillCircle(Math.cos((index * Math.PI) / 3) * 13, Math.sin((index * Math.PI) / 3) * 13, 7);
  }
  painter.fillStyle(0xffffff, 0.55).fillCircle(0, 0, 5);
}

function mortar(painter: ShapePainter, colour: number) {
  painter.fillStyle(colour, 1).fillRoundedRect(-15, -15, 30, 30, 5);
  painter.fillStyle(0x202938, 1).fillRect(-5, -28, 10, 25);
}

function barracks(painter: ShapePainter, colour: number) {
  painter.fillStyle(0x334155, 1).fillRoundedRect(-17, -15, 34, 27, 5);
  painter
    .fillStyle(colour, 1)
    .fillTriangle(-15, -15, -7, -28, 0, -15)
    .fillTriangle(0, -15, 8, -28, 15, -15);
  painter.fillStyle(0xdffcff, 1).fillCircle(-10, 13, 5).fillCircle(10, 13, 5);
}

function beacon(painter: ShapePainter, colour: number) {
  painter.fillStyle(colour, 1).fillRoundedRect(-9, -10, 18, 24, 4);
  painter
    .fillStyle(colour, 0.9)
    .fillTriangle(0, -26, -11, -8, 11, -8);
  painter.fillStyle(0xffffff, 0.6).fillCircle(0, -14, 4);
}

const FORMS: Record<TowerForm, (painter: ShapePainter, colour: number) => void> = {
  spire,
  garden,
  mortar,
  barracks,
  beacon,
};

/**
 * The branch badge, drawn at the foot of the tower where no form has geometry,
 * so the same badge means the same thing on every archetype.
 */
const EMBLEMS: Record<BranchEmblem, (painter: ShapePainter, colour: number) => void> = {
  none: () => {},
  volley: (painter, colour) => {
    for (const x of [-7, 0, 7]) painter.fillStyle(colour, 1).fillTriangle(x - 3, 24, x + 3, 24, x, 15);
  },
  reach: (painter, colour) => {
    painter.fillStyle(colour, 1).fillRect(-13, 19, 26, 3.5);
    painter.fillStyle(colour, 1).fillTriangle(13, 15, 13, 26, 21, 20.5);
  },
  burst: (painter, colour) => {
    painter.lineStyle(2.4, colour, 0.95).strokeCircle(0, 20, 6);
    painter.lineStyle(1.6, colour, 0.6).strokeCircle(0, 20, 10);
  },
  pierce: (painter, colour) => {
    painter.fillStyle(colour, 1).fillRect(-1.8, 13, 3.6, 12);
    painter.fillStyle(colour, 1).fillTriangle(-6, 16, 6, 16, 0, 8);
  },
  chill: (painter, colour) => {
    for (const y of [15, 20, 25])
      painter.fillStyle(colour, 0.95).fillTriangle(-7, y, 7, y, 0, y + 4);
  },
  heavy: (painter, colour) => {
    painter.fillStyle(colour, 1).fillCircle(0, 20, 6.5);
    painter.fillStyle(0x0a1522, 0.8).fillCircle(0, 20, 2.6);
  },
};

/**
 * Draws one tower. `accent` is the branch colour: a specialised tower carries a
 * badge in it so two level-three towers with opposite branches can be told
 * apart at a glance.
 */
export function drawTowerBody(
  painter: ShapePainter,
  presentation: TowerPresentation,
  level: number,
  colour: number,
  accent: number,
): void {
  ring(painter, colour, level);
  FORMS[presentation.form](painter, colour);
  EMBLEMS[presentation.emblem](painter, accent);
}
