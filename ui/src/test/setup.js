import "@testing-library/jest-dom/vitest";
// Registers testing-library cleanup after every test; globals are off here.
import "@testing-library/svelte/vitest";

const storage = () => {
  const store = new Map();
  return {
    getItem: (key) => (store.has(key) ? store.get(key) : null),
    setItem: (key, value) => {
      store.set(key, String(value));
    },
    removeItem: (key) => {
      store.delete(key);
    },
    clear: () => {
      store.clear();
    }
  };
};

Object.defineProperty(globalThis, "localStorage", {
  value: storage(),
  configurable: true
});
