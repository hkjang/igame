import { useEffect, useRef } from 'react';
import { Box } from '@mui/material';
import { drawEnemyBody, type EnemyPainter } from './enemyArt';
import type { EnemyPresentation } from './enemyPresentation';

/**
 * A 2D-canvas adapter for the battlefield's enemy art.
 *
 * The art is written against a small structural surface, so the same creature a
 * player meets on the field is the one a roster card shows — there is no second
 * drawing of it to drift out of step.
 */
function canvasPainter(context: CanvasRenderingContext2D): EnemyPainter {
  let fill = 'rgba(255,255,255,1)';
  let stroke = 'rgba(255,255,255,1)';
  const css = (colour: number, alpha: number) =>
    `rgba(${(colour >> 16) & 255},${(colour >> 8) & 255},${colour & 255},${alpha})`;
  const painter: EnemyPainter = {
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

export function EnemyPortrait({
  presentation,
  size = 72,
  label,
}: {
  presentation: EnemyPresentation;
  size?: number;
  label: string;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext('2d');
    if (!canvas || !context) return;
    // Match the backing store to the display so the silhouette stays crisp on
    // a high-density screen.
    const ratio = Math.min(3, Math.max(1, window.devicePixelRatio || 1));
    canvas.width = size * ratio;
    canvas.height = size * ratio;
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.clearRect(0, 0, size, size);
    context.save();
    // The tallest silhouettes reach about 1.5 radii above the centre and the
    // boss ring sits at 1.22, so a portrait's radius is a little under a third
    // of the box.
    context.translate(size / 2, size * 0.54);
    drawEnemyBody(canvasPainter(context), presentation, size * 0.29);
    context.restore();
  }, [presentation, size]);

  return (
    <Box
      component="canvas"
      ref={canvasRef}
      role="img"
      aria-label={label}
      sx={{ width: size, height: size, display: 'block', flexShrink: 0 }}
    />
  );
}
