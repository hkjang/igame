import '@testing-library/jest-dom/vitest';

// Install a deterministic in-memory Storage for every run.
//
// Which localStorage the suite sees otherwise depends on the environment:
// Node defines an experimental `localStorage` global that is unavailable
// without --localstorage-file and shadows jsdom's, so the same test can be
// backed by a plain object locally and by jsdom's proxy-backed Storage in CI.
// Tests that replace or inspect it must not have to care which one they got.
const entries = new Map<string, string>();
const storage: Storage = {
  getItem: (key) => entries.get(key) ?? null,
  setItem: (key, value) => { entries.set(key, String(value)); },
  removeItem: (key) => { entries.delete(key); },
  clear: () => { entries.clear(); },
  key: (index) => [...entries.keys()][index] ?? null,
  get length() { return entries.size; },
};
Object.defineProperty(globalThis, 'localStorage', { configurable: true, writable: true, value: storage });
