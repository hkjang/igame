import { alpha } from '@mui/material/styles';
import { Box } from '@mui/material';

/**
 * The portal's level progress bar.
 *
 * MUI's `LinearProgress` in a palette colour paints its track in the same hue
 * as its bar, so 19% of the way through a level was indistinguishable from a
 * full one at a glance. This keeps the track neutral and gives the fill a
 * gradient, and it carries the progressbar role that a bare div did not.
 */
export function LevelMeter({ percent, label }: { percent: number; label: string }) {
  const clamped = Math.min(100, Math.max(0, percent));
  return (
    <Box
      role="progressbar"
      aria-valuenow={clamped}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={label}
      sx={(theme) => ({
        height: 10,
        borderRadius: 10,
        overflow: 'hidden',
        bgcolor: alpha(theme.palette.text.primary, theme.palette.mode === 'dark' ? .16 : .12),
      })}
    >
      <Box
        sx={(theme) => ({
          width: `${clamped}%`,
          height: '100%',
          borderRadius: 10,
          background: `linear-gradient(90deg, ${theme.palette.secondary.main}, ${theme.palette.primary.main})`,
          transition: 'width .3s',
        })}
      />
    </Box>
  );
}
