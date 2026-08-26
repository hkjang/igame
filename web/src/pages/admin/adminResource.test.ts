import { describe, expect, it } from 'vitest';
import { __testing } from './AdminResourcePage';

const { rowLabel, jsonFieldErrors, display } = __testing;

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

describe('display', () => {
  const lookups = {
    categories: [{ value: 'cat-1', label: 'Defense Series' }],
    games: [{ value: 'game-1', label: 'RealmGuard' }],
  };

  it('names a referenced row instead of printing its key', () => {
    // The games table showed operators a category UUID, which tells them
    // nothing about what the game was filed under.
    expect(display('cat-1', { key: 'category_id', label: '카테고리', lookup: 'categories' }, lookups))
      .toBe('Defense Series');
  });

  it('still shows a key that no longer resolves', () => {
    // Hiding a stale reference would leave an operator chasing a blank cell.
    expect(display('cat-gone', { key: 'category_id', label: '카테고리', lookup: 'categories' }, lookups))
      .toBe('cat-gone');
  });

  it('leaves a plain value alone', () => {
    expect(display('snake', { key: 'slug', label: 'Slug' }, lookups)).toBe('snake');
    expect(display('', { key: 'slug', label: 'Slug' }, lookups)).toBe('—');
    expect(display(null, { key: 'slug', label: 'Slug' }, lookups)).toBe('—');
  });

  it('formats a timestamp for a reader', () => {
    expect(display('2026-08-26T01:02:03Z', { key: 'created_at', label: '생성' }, lookups))
      .not.toContain('T');
  });
});

describe('optionLabel', () => {
  const { optionLabel, configs } = __testing;

  it('speaks Korean for every value any select can hold', () => {
    // The console is Korean throughout; a dropdown that still offers `draft`
    // and `department_battle` is asking an operator to read the schema.
    const untranslated: string[] = [];
    for (const [resource, config] of Object.entries(configs)) {
      for (const field of config.fields) {
        for (const option of field.options ?? []) {
          if (optionLabel(field, option) === option) untranslated.push(`${resource}.${field.key}=${option}`);
        }
      }
    }
    expect(untranslated).toEqual([]);
  });

  it('lets a field say what the shared wording gets wrong', () => {
    const games = configs.games.fields.find((field) => field.key === 'status')!;
    const users = configs.users.fields.find((field) => field.key === 'status')!;
    // The same `active` is a game being served and a user being allowed in.
    expect(optionLabel(games, 'active')).toBe('서비스 중');
    expect(optionLabel(users, 'active')).toBe('활성');
  });

  it('shows a value it has no label for rather than nothing', () => {
    expect(optionLabel(undefined, 'retired')).toBe('retired');
  });
});

describe('display of a select cell', () => {
  const { display, configs } = __testing;
  const role = configs.users.fields.find((field) => field.key === 'role')!;

  it('reads the label, not the stored value', () => {
    expect(display('admin', role)).toBe('관리자');
    // A value the options no longer list still has to be visible: it is what
    // the operator is being asked to fix.
    expect(display('root', role)).toBe('root');
  });
});
