import { useEffect, useRef } from 'react';
import { Box } from '@mui/material';
import { drawEnemyBody } from './enemyArt';
import type { EnemyPresentation } from './enemyPresentation';
import { canvasShapePainter } from './shapePainter';

/**
 * A creature drawn with the battlefield's own art, so a roster card and the
 * enemy a player meets are the same thing.
 */
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
    drawEnemyBody(canvasShapePainter(context), presentation, size * 0.29);
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
