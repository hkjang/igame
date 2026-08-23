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
    <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden', transition: 'transform .18s, border-color .18s', '&:hover': { transform: 'translateY(-4px)', borderColor: alpha(accent, .55) } }}>
      <Box sx={{ height: 150, position: 'relative', display: 'grid', placeItems: 'center', background: `radial-gradient(circle at 50% 35%, ${alpha(accent, .35)}, transparent 55%), linear-gradient(145deg, ${theme.palette.surface.sunken}, ${theme.palette.surface.code})` }}>
        {game.thumbnail ? <Box component="img" src={game.thumbnail} alt="" aria-hidden sx={{ width: '100%', height: '100%', objectFit: 'cover' }} /> : <Typography aria-hidden sx={{ fontWeight: 900, fontSize: game.icon && game.icon.length > 2 ? '2.6rem' : '4rem', color: accent, textShadow: `0 0 36px ${accent}` }}>{game.icon ?? game.name.slice(0, 1)}</Typography>}
        <Chip label={game.status === 'active' ? game.category : '점검 중'} color={game.status === 'active' ? 'default' : 'warning'} size="small" sx={(theme) => ({ position: 'absolute', left: 12, top: 12, bgcolor: alpha(theme.palette.surface.ground, .78) })} />
        {onFavorite && <Tooltip title={game.favorite ? '즐겨찾기 해제' : '즐겨찾기 추가'}><IconButton aria-label={game.favorite ? '즐겨찾기 해제' : '즐겨찾기 추가'} onClick={() => onFavorite(game)} sx={(theme) => ({ position: 'absolute', right: 8, top: 7, color: game.favorite ? 'accent.favorite' : 'text.secondary', bgcolor: alpha(theme.palette.surface.ground, .62) })}>{game.favorite ? <FavoriteRounded /> : <FavoriteBorderRounded />}</IconButton></Tooltip>}
      </Box>
      <CardContent sx={{ flex: 1 }}>
        <Typography variant="h3" component="h3" gutterBottom>{game.name}</Typography>
        <Typography color="text.secondary">{game.description}</Typography>
        <Stack direction="row" spacing={.7} mt={2} flexWrap="wrap" useFlexGap>{game.tags.slice(0, 3).map((tag) => <Chip key={tag} label={tag} size="small" variant="outlined" />)}</Stack>
      </CardContent>
      <CardActions sx={{ p: 2, pt: 0 }}>
        <Button fullWidth component={RouterLink} to={`/games/${game.slug}`} variant="contained" startIcon={<PlayArrowRounded />} disabled={unavailable}>{unavailable ? '이용 불가' : '플레이'}</Button>
      </CardActions>
    </Card>
  );
}
