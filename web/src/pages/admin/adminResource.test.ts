import { describe, expect, it } from 'vitest';
import { __testing } from './AdminResourcePage';

const { rowLabel, jsonFieldErrors } = __testing;

describe('rowLabel', () => {
  it('prefers the most human field available', () => {
    expect(rowLabel({ id: 'uuid-1', name: '스네이크' })).toBe('스네이크');
    expect(rowLabel({ id: 'uuid-1', title: '점검 공지' })).toBe('점검 공지');
    expect(rowLabel({ id: 'uuid-1', username: 'admin', display_name: '홍길동' })).toBe('홍길동');
    expect(rowLabel({ id: 'uuid-1', code: 'FIRST_WIN' })).toBe('FIRST_WIN');
  });

  it('falls back to the identifier rather than an empty confirmation', () => {
    expect(rowLabel({ id: 'uuid-1' })).toBe('uuid-1');
    expect(rowLabel({ id: 'uuid-1', name: '   ' })).toBe('uuid-1');
    expect(rowLabel({})).toBe('항목');
  });
});

describe('jsonFieldErrors', () => {
  const fields = [
    { key: 'name', label: '이름' },
    { key: 'rules', label: '규칙', type: 'json' as const },
    { key: 'criteria', label: '조건', type: 'json' as const },
  ];

  it('reports only the JSON field that is actually malformed', () => {
    const errors = jsonFieldErrors(fields, { name: '{', rules: '{"a":1}', criteria: '{oops' });
    expect(Object.keys(errors)).toEqual(['criteria']);
  });

  it('treats an empty JSON field as acceptable', () => {
    expect(jsonFieldErrors(fields, { rules: '', criteria: '   ' })).toEqual({});
  });

  it('passes valid JSON of any shape', () => {
    expect(jsonFieldErrors(fields, { rules: '[]', criteria: 'null' })).toEqual({});
  });
});
