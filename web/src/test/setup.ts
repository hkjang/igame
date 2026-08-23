import '@testing-library/jest-dom/vitest';

// Node 22 defines an experimental `localStorage` global that is unavailable
// unless --localstorage-file is passed, and it shadows the one jsdom would
// otherwise provide. Tests need the browser behaviour, so install a working
// in-memory Storage when nothing usable is present.
if (!globalThis.localStorage) {
  const entries = new Map<string, string>();
  const storage: Storage = {
    getItem: (key) => entries.get(key) ?? null,
    setItem: (key, value) => { entries.set(key, String(value)); },
    removeItem: (key) => { entries.delete(key); },
    clear: () => { entries.clear(); },
    key: (index) => [...entries.keys()][index] ?? null,
    get length() { return entries.size; },
  };
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: storage });
}
