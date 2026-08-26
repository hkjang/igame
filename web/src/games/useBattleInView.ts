import { useEffect, useRef } from 'react';

/**
 * How far below the top of the page the battlefield comes to rest.
 *
 * The portal's header is sticky and 73px tall, so putting the battlefield at
 * viewport zero would park the HUD underneath it. This is that height plus a
 * little air, and it matches the scroll margin the ranking anchor already uses.
 */
export const BATTLE_SCROLL_MARGIN = 96;

/**
 * Puts the battlefield on screen when a battle starts.
 *
 * Starting a battle replaces a tall lobby with a shorter one, and the page kept
 * whatever scroll position the lobby had left behind. On a phone that put the
 * RealmGuard shell 164px above the top of the window and the Defense shell 24px
 * above it, so in both games the player spent the battle unable to see their
 * lives, their gold or which wave they were on — the HUD sits at the top of the
 * shell, and the top of the shell was off screen.
 *
 * The scroll is computed and issued here rather than left to `scrollIntoView`
 * with a scroll margin: the lobby unmounting in the same commit changes the
 * height of the document, and scrollIntoView measured against that in-between
 * state and landed the shell flush against the top with the header over it. A
 * frame later the layout has settled and the arithmetic is simply right.
 *
 * Returns the ref to attach to the battle shell.
 */
export function useBattleInView<T extends HTMLElement>(active: boolean) {
  const ref = useRef<T | null>(null);
  useEffect(() => {
    if (!active) return;
    const element = ref.current;
    if (!element) return;
    const frame = requestAnimationFrame(() => {
      const top = element.getBoundingClientRect().top + window.scrollY - BATTLE_SCROLL_MARGIN;
      window.scrollTo({ top: Math.max(0, top), behavior: 'smooth' });
    });
    return () => cancelAnimationFrame(frame);
  }, [active]);
  return ref;
}
