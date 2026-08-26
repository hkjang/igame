import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import { createAppTheme } from '../../theme';
import { JsonSubField } from './JsonSubField';

afterEach(cleanup);

function renderField(onChange = vi.fn(), initial: unknown = [{ x: 1 }]) {
  render(
    <ThemeProvider theme={createAppTheme('light')}>
      <JsonSubField resetKey="0:path" initial={initial} label="path JSON" helperText="1개" onChange={onChange} />
    </ThemeProvider>,
  );
  return screen.getByLabelText('path JSON') as HTMLTextAreaElement;
}

describe('JsonSubField', () => {
  it('lets an edit pass through the invalid states it has to pass through', () => {
    // Every real edit is briefly unparseable. A field bound to the model drops
    // those keystrokes and snaps back, so the only way to change it is to paste
    // complete valid JSON.
    const onChange = vi.fn();
    const field = renderField(onChange);
    fireEvent.change(field, { target: { value: '[{ "x": ' } });
    expect(field.value).toBe('[{ "x": ');
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.change(field, { target: { value: '[{ "x": 2 }]' } });
    expect(onChange).toHaveBeenCalledWith([{ x: 2 }]);
  });

  it('says why an unparseable edit has not been applied', () => {
    const field = renderField();
    fireEvent.change(field, { target: { value: '[{,}]' } });
    expect(screen.queryByText('1개')).toBeNull();
    expect(field.getAttribute('aria-invalid')).toBe('true');
  });

  it('reports validity to a parent that gates its save button', () => {
    const onValidity = vi.fn();
    render(
      <ThemeProvider theme={createAppTheme('light')}>
        <JsonSubField resetKey="0:path" initial={[]} label="path JSON" helperText="0개" onChange={vi.fn()} onValidity={onValidity} />
      </ThemeProvider>,
    );
    const field = screen.getByLabelText('path JSON');
    fireEvent.change(field, { target: { value: '[' } });
    expect(onValidity).toHaveBeenLastCalledWith('0:path', expect.stringMatching(/.+/));
    fireEvent.change(field, { target: { value: '[]' } });
    expect(onValidity).toHaveBeenLastCalledWith('0:path', '');
  });
});
