import { useEffect, useRef, useState, type RefObject } from 'react';
import { Box } from '@mui/material';
import { useLocation } from 'react-router-dom';
import { titleForPath } from '../pages/routeTitles';

/** The landmark the skip link and route focus both target. */
export const MAIN_CONTENT_ID = 'main-content';

const offscreen = {
  position: 'absolute',
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: 'hidden',
  clipPath: 'inset(50%)',
  whiteSpace: 'nowrap',
  border: 0,
} as const;

/**
 * Supplies the parts of the page chrome a single-page app has to provide for
 * itself: a way past the navigation, and a spoken confirmation that the page
 * actually changed.
 *
 * Render it as the first child of a layout so the skip link is the first thing
 * a keyboard user reaches.
 */
export function RouteChrome({ mainRef }: { mainRef: RefObject<HTMLElement | null> }) {
  const { pathname } = useLocation();
  const [announcement, setAnnouncement] = useState('');
  const firstRoute = useRef(true);

  useEffect(() => {
    // The first render is the browser's own page load, which assistive
    // technology already announces and where focus already starts at the top.
    if (firstRoute.current) {
      firstRoute.current = false;
      return;
    }
    // A client-side navigation swaps the content without telling anyone. Move
    // focus to the top of the new page so the next Tab continues from there
    // instead of from the link that was just activated, and say what loaded.
    mainRef.current?.focus({ preventScroll: true });
    setAnnouncement(`${titleForPath(pathname)} 페이지로 이동했습니다.`);
  }, [pathname, mainRef]);

  const focusMain = () => mainRef.current?.focus();

  return (
    <>
      <Box
        component="a"
        href={`#${MAIN_CONTENT_ID}`}
        onClick={(event: React.MouseEvent) => { event.preventDefault(); focusMain(); }}
        sx={{
          position: 'fixed',
          top: 12,
          left: 12,
          zIndex: (theme) => theme.zIndex.tooltip + 1,
          px: 2.25,
          py: 1.25,
          borderRadius: 2,
          fontWeight: 700,
          bgcolor: 'primary.main',
          color: 'primary.contrastText',
          // Kept in the tab order and the accessibility tree, out of sight
          // until it is focused.
          transform: 'translateY(calc(-100% - 24px))',
          '&:focus-visible': { transform: 'none' },
        }}
      >
        본문으로 건너뛰기
      </Box>
      <Box role="status" aria-live="polite" sx={offscreen}>{announcement}</Box>
    </>
  );
}
