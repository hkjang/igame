import { useEffect, useState } from 'react';
import TextField from '@mui/material/TextField';

/**
 * A JSON sub-field that keeps its own text while it is being typed.
 *
 * Binding the textarea straight to `JSON.stringify(model)` makes these fields
 * unusable: every keystroke that leaves the text momentarily invalid is dropped
 * and the field snaps back to the previous value, so the only way to change one
 * is to paste complete valid JSON. Holding the text here and reporting only the
 * parses that succeed lets an operator type through the invalid states that
 * every real edit passes through.
 *
 * Shared by both content studios, which had drifted: RealmGuard's copy was
 * fixed and the Defense studio's was not.
 */
export function JsonSubField({ resetKey, initial, label, helperText, onChange, onValidity }: {
  /** Changing this resyncs the field: it identifies which item is being edited. */
  resetKey: string;
  initial: unknown;
  label: string;
  helperText: string;
  onChange: (value: unknown) => void;
  /** Lets a parent gate its save button on the field parsing. */
  onValidity?: (key: string, message: string) => void;
}) {
  const [text, setText] = useState(() => JSON.stringify(initial, null, 2));
  const [error, setError] = useState('');
  // Resync only when the edited item changes, never while typing.
  useEffect(() => {
    setText(JSON.stringify(initial, null, 2));
    setError('');
    onValidity?.(resetKey, '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);
  useEffect(() => () => onValidity?.(resetKey, ''), [onValidity, resetKey]);
  return <TextField
    label={label}
    multiline
    minRows={3}
    // Without a ceiling the field grows to the length of its content, and a
    // stage's path pushed the editor column past 2000px while the full-document
    // editor beside it stayed a fixed, scrollable height.
    maxRows={14}
    value={text}
    error={Boolean(error)}
    helperText={error || helperText}
    inputProps={{ spellCheck: false }}
    // JSON is written to be read in columns; a proportional font loses that.
    sx={{ '& textarea': { fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', fontSize: '0.95rem', lineHeight: 1.5 } }}
    onChange={(event) => {
      const next = event.target.value;
      setText(next);
      try {
        onChange(JSON.parse(next));
        setError('');
        onValidity?.(resetKey, '');
      } catch (cause) {
        const message = cause instanceof Error ? cause.message : 'JSON 오류';
        setError(message);
        onValidity?.(resetKey, message);
      }
    }}
  />;
}
