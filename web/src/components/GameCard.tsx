import FavoriteRounded from '@mui/icons-material/FavoriteRounded';
import FavoriteBorderRounded from '@mui/icons-material/FavoriteBorderRounded';
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded';
import { alpha, useTheme } from '@mui/material/styles';
import { Box, Button, Card, CardActions, CardContent, Chip, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import type { Game } from '../types';

export function GameCard({ game, onFavorite }: { game: Game; onFavorite?: (game: Game) => void }) {
  const theme = useTheme();
  // A game may declare its own accent; otherwise it takes the service primary.
  const accent = game.accent ?? theme.palette.primary.main;
  const unavailable = game.status !== 'active';
  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        transition: 'transform .18s, border-color .18s, box-shadow .18s',
        '&:hover': {
          transform: 'translateY(-4px)',
          borderColor: alpha(accent, .55),
          boxShadow: `0 14px 32px ${alpha(theme.palette.common.black, theme.palette.mode === 'dark' ? .5 : .14)}`,
        },
        '&:hover .game-art': { transform: 'scale(1.04)' },
      }}
    >
      <Box
        className="game-art-frame"
        sx={{
          position: 'relative',
          // A fixed ratio rather than a fixed height: every card in a row then
          // reaches its title at the same place whatever art it carries.
          aspectRatio: '16 / 9',
          overflow: 'hidden',
          display: 'grid',
          placeItems: 'center',
          background: `radial-gradient(circle at 50% 35%, ${alpha(accent, .35)}, transparent 55%), linear-gradient(145deg, ${theme.palette.surface.sunken}, ${theme.palette.surface.code})`,
        }}
      >
        {game.thumbnail
          // Absolute rather than `height: 100%`: a percentage height on a
          // replaced element in an auto-sized grid area resolves to the image's
          // own aspect ratio, which used to render the art a hundred pixels
          // taller than its frame and paint it over the title below.
          ? <Box component="img" className="game-art" src={game.thumbnail} alt="" aria-hidden loading="lazy" sx={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover', transition: 'transform .35s' }} />
          : <Typography aria-hidden sx={{ fontWeight: 900, fontSize: game.icon && game.icon.length > 2 ? '2.6rem' : '4rem', color: accent, textShadow: `0 0 36px ${accent}` }}>{game.icon ?? game.name.slice(0, 1)}</Typography>}
        {/* The banner artwork carries the game's own title along its top edge,
            so the overlay row sits on a scrim at the bottom where it can never
            cover it. */}
        <Box aria-hidden sx={{ position: 'absolute', inset: 0, background: `linear-gradient(to top, ${alpha(theme.palette.surface.code, .82)} 0%, transparent 42%)`, pointerEvents: 'none' }} />
        <Chip
          label={game.status === 'active' ? game.category : '점검 중'}
          color={game.status === 'active' ? 'default' : 'warning'}
          size="small"
          sx={{ position: 'absolute', left: 12, bottom: 12, fontWeight: 700, color: theme.palette.getContrastText(theme.palette.surface.code), bgcolor: alpha(theme.palette.surface.code, .72), border: `1px solid ${alpha(theme.palette.common.white, .18)}`, backdropFilter: 'blur(6px)' }}
        />
        {onFavorite && (
          <Tooltip title={game.favorite ? '즐겨찾기 해제' : '즐겨찾기 추가'}>
            <IconButton
              aria-label={game.favorite ? '즐겨찾기 해제' : '즐겨찾기 추가'}
              onClick={() => onFavorite(game)}
              sx={{
                position: 'absolute',
                right: 8,
                top: 8,
                color: game.favorite ? 'accent.favorite' : theme.palette.getContrastText(theme.palette.surface.code),
                bgcolor: alpha(theme.palette.surface.code, .55),
                backdropFilter: 'blur(6px)',
                '&:hover': { bgcolor: alpha(theme.palette.surface.code, .78) },
              }}
            >
              {game.favorite ? <FavoriteRounded /> : <FavoriteBorderRounded />}
            </IconButton>
          </Tooltip>
        )}
      </Box>
      <CardContent sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
        <Typography variant="h3" component="h3" gutterBottom>{game.name}</Typography>
        <Typography color="text.secondary">{game.description}</Typography>
        {/* mt:auto pins the tags to the bottom of the text block so the play
            buttons line up across a row even when descriptions differ in
            length. */}
        <Stack direction="row" spacing={.7} mt="auto" pt={2} flexWrap="wrap" useFlexGap>{game.tags.slice(0, 3).map((tag) => <Chip key={tag} label={tag} size="small" variant="outlined" />)}</Stack>
      </CardContent>
      <CardActions sx={{ p: 2, pt: 0 }}>
        <Button fullWidth component={RouterLink} to={`/games/${game.slug}`} variant="contained" startIcon={<PlayArrowRounded />} disabled={unavailable}>{unavailable ? '이용 불가' : '플레이'}</Button>
      </CardActions>
    </Card>
  );
}
