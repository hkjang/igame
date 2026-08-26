import type { ReactNode } from 'react';
import { Box } from '@mui/material';
import { ThemeProvider, type Theme } from '@mui/material/styles';
import { createAppTheme } from '../theme';

/**
 * The theme a game's own surface renders against.
 *
 * Both games paint their lobby and their battlefield on a fixed dark gradient —
 * that is the art, and it does not follow the portal's light and dark. What did
 * follow it was everything drawn on top: in light mode the RealmGuard wordmark,
 * every section heading, the difficulty picker and the skill buttons came out
 * near-black on near-black, and the stage cards were white blocks on a dark
 * panel. The surface is dark, so its contents are themed dark, whichever mode
 * the person browsing the portal is in.
 *
 * A nested ThemeProvider alone is not enough. The app's theme is built with
 * `cssVariables`, so a component asks for `var(--mui-palette-background-paper)`
 * rather than for a colour, and that variable is declared once on `:root` by
 * whichever theme the shell mounted. Nesting a second provider changes what
 * `sx` callbacks compute and nothing else — which is exactly the half-converted
 * state the first attempt at this produced. Redeclaring the variables on the
 * wrapper makes them cascade to the subtree, so the components follow too.
 */
const gameTheme = createAppTheme('dark');

/**
 * The dark theme's palette as CSS custom property declarations.
 *
 * Read off the theme rather than written out by hand: `theme.vars` holds the
 * variable each value is published under and `theme.palette` holds the value,
 * so a palette entry added later is carried without touching this file.
 */
function paletteVariables(theme: Theme): Record<string, string> {
  const declarations: Record<string, string> = {};
  const walk = (names: unknown, values: unknown) => {
    if (!names || typeof names !== 'object' || !values || typeof values !== 'object') return;
    for (const [key, name] of Object.entries(names as Record<string, unknown>)) {
      const value = (values as Record<string, unknown>)[key];
      if (typeof name === 'string') {
        const variable = /^var\((--[^,)]+)/.exec(name)?.[1];
        if (variable && typeof value === 'string') declarations[variable] = value;
      } else {
        walk(name, value);
      }
    }
  };
  walk(gameTheme.vars?.palette, theme.palette);
  return declarations;
}

const darkPalette = paletteVariables(gameTheme);

export function GameSurface({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider theme={gameTheme}>
      {/* display: contents keeps the wrapper out of the layout while still
          carrying the variables down the DOM tree, which is where custom
          properties inherit. colorScheme lets native controls follow. */}
      <Box sx={{ display: 'contents', colorScheme: 'dark', ...darkPalette }}>{children}</Box>
    </ThemeProvider>
  );
}
