// Minimal observable store; state is replaced immutably on every update.

export function createStore(initial) {
  let state = Object.freeze({ ...initial });
  const listeners = new Set();
  return {
    get: () => state,
    /** Shallow-merges patch (or the result of patch(state)) and notifies subscribers. */
    set(patch) {
      const delta = typeof patch === 'function' ? patch(state) : patch;
      const prev = state;
      state = Object.freeze({ ...state, ...delta });
      for (const fn of listeners) {
        fn(state, prev);
      }
    },
    subscribe(fn) {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
  };
}
