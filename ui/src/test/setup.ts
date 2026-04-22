import "@testing-library/jest-dom";

function createStorageMock() {
  const store = new Map<string, string>();

  return {
    clear() {
      store.clear();
    },
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
    get length() {
      return store.size;
    },
  };
}

function ensureStorage(name: "localStorage" | "sessionStorage") {
  const storage = window[name];
  if (storage && typeof storage.clear === "function") return;

  Object.defineProperty(window, name, {
    configurable: true,
    value: createStorageMock(),
  });
}

ensureStorage("localStorage");
ensureStorage("sessionStorage");
