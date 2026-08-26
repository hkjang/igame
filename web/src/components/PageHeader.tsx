import { alpha } from '@mui/material/styles';
import { Box, Stack, Typography } from '@mui/material';
import type { ReactNode } from 'react';

/**
 * The heading every top-level portal page opens with.
 *
 * The icon lives in its own tile rather than beside the text: set as a sibling
 * of a two-line title block it centres against the whole block and reads as if
 * it is hanging off the heading.
 */
export function PageHeader({
  icon,
  title,
  description,
  tone = 'primary',
  action,
}: {
  icon: ReactNode;
  title: string;
  description?: string;
  tone?: 'primary' | 'secondary' | 'warning';
  action?: ReactNode;
}) {
  return (
    <Stack direction="row" spacing={2} alignItems="center">
      <Box
        aria-hidden
        sx={(theme) => ({
          width: 56,
          height: 56,
          flexShrink: 0,
          borderRadius: 3,
          display: 'grid',
          placeItems: 'center',
          color: `${tone}.main`,
          bgcolor: alpha(theme.palette[tone].main, theme.palette.mode === 'dark' ? .18 : .12),
          '& > *': { fontSize: 32 },
        })}
      >
        {icon}
      </Box>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="h1" sx={{ fontSize: { xs: '2.2rem', md: '3rem' } }}>{title}</Typography>
        {description && <Typography color="text.secondary" mt={.5}>{description}</Typography>}
      </Box>
      {action && <><Box sx={{ flex: 1 }} />{action}</>}
    </Stack>
  );
}
