import { describe, expect, it } from 'vitest';
import { visuallyHidden } from './RouteChrome';

describe('visuallyHidden', () => {
  /**
   * MUI's `sx` reads bare numbers through the sizing scale, where any value up
   * to 1 becomes a percentage. The hand-written version of this style used
   * `width: 1, height: 1` and so rendered a full-viewport block: the games
   * catalogue grew a screen of empty scroll below its grid, and the announcer
   * did the same on every page that carried one.
   */
  it('measures in pixels rather than through the spacing scale', () => {
    expect(visuallyHidden.width).toBe('1px');
    expect(visuallyHidden.height).toBe('1px');
    expect(visuallyHidden.margin).toBe('-1px');
  });

  it('stays in the accessibility tree', () => {
    // display:none or visibility:hidden would remove the live region from the
    // tree, which is the one thing this style must not do.
    expect(visuallyHidden).not.toHaveProperty('display');
    expect(visuallyHidden).not.toHaveProperty('visibility');
    expect(visuallyHidden.position).toBe('absolute');
    expect(visuallyHidden.overflow).toBe('hidden');
  });
});
