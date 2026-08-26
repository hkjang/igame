import { alpha } from '@mui/material/styles';
import { Box, Stack, Typography } from '@mui/material';
import type { ReactNode } from 'react';

/**
 * The shape every "there is nothing here yet" panel takes.
 *
 * A one-line alert in the middle of an otherwise blank page reads like a
 * failure. Giving the state an icon, a sentence that explains why it is empty
 * and somewhere to go turns it into a starting point instead.
 */
export function EmptyState({
  icon,
  title,
  description,
  action,
  tone = 'primary',
}: {
  icon: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  tone?: 'primary' | 'secondary' | 'warning';
}) {
  return (
    <Stack
      alignItems="center"
      textAlign="center"
      spacing={1.5}
      sx={(theme) => ({
        py: { xs: 6, md: 9 },
        px: 3,
        borderRadius: 3,
        border: `1px dashed ${theme.palette.divider}`,
        bgcolor: alpha(theme.palette.text.primary, theme.palette.mode === 'dark' ? .03 : .015),
      })}
    >
      <Box
        aria-hidden
        sx={(theme) => ({
          width: 72,
          height: 72,
          borderRadius: '50%',
          display: 'grid',
          placeItems: 'center',
          color: `${tone}.main`,
          bgcolor: alpha(theme.palette[tone].main, theme.palette.mode === 'dark' ? .16 : .1),
          '& > *': { fontSize: 36 },
        })}
      >
        {icon}
      </Box>
      <Typography variant="h3">{title}</Typography>
      {description && <Typography color="text.secondary" sx={{ maxWidth: 460 }}>{description}</Typography>}
      {action && <Box pt={1}>{action}</Box>}
    </Stack>
  );
}
