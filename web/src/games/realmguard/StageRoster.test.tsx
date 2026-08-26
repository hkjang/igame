import { ThemeProvider } from '@mui/material/styles';
import { cleanup, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { afterEach, describe, expect, it } from 'vitest';
import { createAppTheme } from '../../theme';
import { ENEMIES, STAGES } from './content';
import { StageRoster } from './StageRoster';

const stage = STAGES.find((item) => item.id === 'stage-1')!;

// The roster reads the product's own surface palette, so it needs the product's
// theme rather than MUI's default one.
const draw = (element: ReactElement) =>
  render(<ThemeProvider theme={createAppTheme('dark')}>{element}</ThemeProvider>);

// vitest runs without `globals`, so testing-library's automatic cleanup is off
// and one test's roster would still be mounted during the next one.
afterEach(cleanup);

describe('StageRoster', () => {
  it('lists each creature the stage sends, once', () => {
    draw(<StageRoster stage={stage} enemies={ENEMIES} />);
    const expected = new Set(stage.waves.flatMap((wave) => wave.entries.map((entry) => entry.enemy)));
    for (const id of expected) {
      const enemy = ENEMIES.find((item) => item.id === id)!;
      // A wave table repeats its enemies across waves; the roster is a set.
      expect(screen.getAllByText(enemy.name)).toHaveLength(1);
    }
  });

  it('names the traits a player has to counter', () => {
    draw(<StageRoster stage={stage} enemies={ENEMIES} />);
    // stage-1 opens with 가시등, whose armour is the first counter decision.
    expect(screen.getByText('방어')).toBeInTheDocument();
    expect(screen.getByLabelText('가시등 외형')).toBeInTheDocument();
  });

  it('renders nothing when the pinned content has no matching enemy', () => {
    const { container } = draw(<StageRoster stage={stage} enemies={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('follows the selected stage', () => {
    const later = STAGES.find((item) => item.id === 'stage-5')!;
    draw(<StageRoster stage={later} enemies={ENEMIES} />);
    expect(screen.queryByText(/이끼빛 관문/)).toBeNull();
    expect(screen.getByText(new RegExp(later.name))).toBeInTheDocument();
  });
});
