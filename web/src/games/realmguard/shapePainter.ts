/**
 * The drawing surface the game's code-native art is written against.
 *
 * Described structurally rather than as a Phaser type so the same creature or
 * tower can be painted onto the battlefield and into a card in the DOM without
 * the art being written twice. Phaser's Graphics satisfies this shape as it
 * stands, and `canvasShapePainter` supplies the DOM half.
 */
export interface ShapePainter {
  fillStyle(colour: number, alpha?: number): ShapePainter;
  lineStyle(width: number, colour: number, alpha?: number): ShapePainter;
  fillCircle(x: number, y: number, radius: number): ShapePainter;
  fillEllipse(x: number, y: number, width: number, height: number): ShapePainter;
  fillTriangle(x1: number, y1: number, x2: number, y2: number, x3: number, y3: number): ShapePainter;
  fillRect(x: number, y: number, width: number, height: number): ShapePainter;
  fillRoundedRect(x: number, y: number, width: number, height: number, radius?: number): ShapePainter;
  lineBetween(x1: number, y1: number, x2: number, y2: number): ShapePainter;
  strokeCircle(x: number, y: number, radius: number): ShapePainter;
  strokeEllipse(x: number, y: number, width: number, height: number): ShapePainter;
}

/** Reads a `#rrggbb` token as the numeric colour the painter takes. */
export function hexColour(value: string): number {
  return Number.parseInt(value.replace('#', ''), 16);
}

/** Paints onto a 2D canvas context, in the context's own coordinate space. */
export function canvasShapePainter(context: CanvasRenderingContext2D): ShapePainter {
  let fill = 'rgba(255,255,255,1)';
  let stroke = 'rgba(255,255,255,1)';
  const css = (colour: number, alpha: number) =>
    `rgba(${(colour >> 16) & 255},${(colour >> 8) & 255},${colour & 255},${alpha})`;
  const painter: ShapePainter = {
    fillStyle(colour, alpha = 1) { fill = css(colour, alpha); return painter; },
    lineStyle(width, colour, alpha = 1) { context.lineWidth = width; stroke = css(colour, alpha); return painter; },
    fillCircle(x, y, radius) {
      context.fillStyle = fill;
      context.beginPath();
      context.arc(x, y, Math.max(0, radius), 0, Math.PI * 2);
      context.fill();
      return painter;
    },
    fillEllipse(x, y, width, height) {
      context.fillStyle = fill;
      context.beginPath();
      context.ellipse(x, y, Math.max(0, width / 2), Math.max(0, height / 2), 0, 0, Math.PI * 2);
      context.fill();
      return painter;
    },
    fillTriangle(x1, y1, x2, y2, x3, y3) {
      context.fillStyle = fill;
      context.beginPath();
      context.moveTo(x1, y1);
      context.lineTo(x2, y2);
      context.lineTo(x3, y3);
      context.closePath();
      context.fill();
      return painter;
    },
    fillRect(x, y, width, height) { context.fillStyle = fill; context.fillRect(x, y, width, height); return painter; },
    fillRoundedRect(x, y, width, height, radius = 0) {
      context.fillStyle = fill;
      context.beginPath();
      context.roundRect(x, y, width, height, radius);
      context.fill();
      return painter;
    },
    lineBetween(x1, y1, x2, y2) {
      context.strokeStyle = stroke;
      context.beginPath();
      context.moveTo(x1, y1);
      context.lineTo(x2, y2);
      context.stroke();
      return painter;
    },
    strokeCircle(x, y, radius) {
      context.strokeStyle = stroke;
      context.beginPath();
      context.arc(x, y, Math.max(0, radius), 0, Math.PI * 2);
      context.stroke();
      return painter;
    },
    strokeEllipse(x, y, width, height) {
      context.strokeStyle = stroke;
      context.beginPath();
      context.ellipse(x, y, Math.max(0, width / 2), Math.max(0, height / 2), 0, 0, Math.PI * 2);
      context.stroke();
      return painter;
    },
  };
  return painter;
}
