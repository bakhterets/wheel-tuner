// Application entry point: wires state, API calls, forms and visualisations.
import { createClient } from './api.js';
import { bindGrid, hasInvalidCells, renderGrid } from './grid.js';
import { resizeRows, rowsFromMeasurements, toMeasurements } from './measurements.js';
import { renderResults } from './results.js';
import { DEFAULT_WHEEL, fillForm, readForm, renderSessionOptions, renderWheelOptions } from './setup.js';
import { createStore } from './state.js';
import { renderRadar } from './viz/radar.js';
import { renderWheel } from './viz/wheel.js';

const $ = (sel) => document.querySelector(sel);

const ui = {
  wheelSelect: $('#wheel-select'),
  form: $('#wheel-form'),
  deleteBtn: $('#delete-wheel'),
  table: $('#grid'),
  tbody: $('#grid tbody'),
  solveBtn: $('#solve'),
  clearBtn: $('#clear'),
  sessionSelect: $('#session-select'),
  results: $('#results'),
  wheelViz: $('#wheel-viz'),
  radarViz: $('#radar-viz'),
  status: $('#status'),
};

const api = createClient();
const store = createStore({
  wheels: [],
  wheel: { ...DEFAULT_WHEEL },
  rows: resizeRows([], DEFAULT_WHEEL.spokes_count),
  gridVersion: 0,
  sessions: [],
  result: null,
  busy: false,
});

function setStatus(message, kind = 'info') {
  ui.status.textContent = message;
  ui.status.dataset.kind = kind;
}

async function guarded(fn) {
  store.set({ busy: true });
  try {
    await fn();
  } catch (err) {
    setStatus(err.message, 'error');
  } finally {
    store.set({ busy: false });
  }
}

function replaceRows(rows) {
  store.set((st) => ({ rows, gridVersion: st.gridVersion + 1, result: null }));
}

async function loadSessions(wheelId) {
  const sessions = wheelId ? await api.listSessions(wheelId) : [];
  store.set({ sessions });
}

async function selectWheel(id) {
  const wheel = id ? store.get().wheels.find((w) => w.id === id) : null;
  const next = wheel ?? { ...DEFAULT_WHEEL };
  fillForm(ui.form, next);
  store.set({ wheel: next });
  replaceRows(resizeRows(store.get().rows, next.spokes_count));
  await loadSessions(next.id);
}

/** Reloads profiles and selects selectId (undefined: the first profile, 0: a new one). */
async function refreshWheels(selectId) {
  const wheels = await api.listWheels();
  const id = selectId ?? wheels[0]?.id ?? 0;
  store.set({ wheels });
  renderWheelOptions(ui.wheelSelect, wheels, id);
  await selectWheel(id);
}

function onFormChange() {
  const wheel = readForm(ui.form);
  store.set({ wheel, result: null });
  if (wheel.spokes_count !== store.get().rows.length) {
    replaceRows(resizeRows(store.get().rows, wheel.spokes_count));
  }
}

async function saveWheel(ev) {
  ev.preventDefault();
  await guarded(async () => {
    const wheel = readForm(ui.form);
    const saved = wheel.id ? await api.updateWheel(wheel) : await api.createWheel(wheel);
    await refreshWheels(saved.id);
    setStatus(`Profile “${saved.name}” saved.`, 'ok');
  });
}

async function deleteWheel() {
  const { wheel } = store.get();
  if (!wheel.id || !globalThis.confirm(`Delete profile “${wheel.name}” and all its sessions?`)) {
    return;
  }
  await guarded(async () => {
    await api.deleteWheel(wheel.id);
    await refreshWheels(0);
    setStatus('Profile deleted.', 'ok');
  });
}

async function solve() {
  if (hasInvalidCells(ui.table)) {
    setStatus('Fix the highlighted cells first.', 'error');
    return;
  }
  await guarded(async () => {
    const wheel = readForm(ui.form);
    const ms = toMeasurements(store.get().rows);
    let result;
    if (wheel.id) {
      const session = await api.createSession(wheel.id, ms);
      result = await api.solveSession(session.id);
      await loadSessions(wheel.id);
    } else {
      result = await api.solve(wheel, ms);
    }
    store.set({ result });
    setStatus(wheel.id ? `Session #${result.session_id} saved and solved.` : 'Solved (profile not saved, session not stored).', 'ok');
    ui.results.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  });
}

async function loadSession() {
  const id = Number(ui.sessionSelect.value);
  if (!id) {
    return;
  }
  await guarded(async () => {
    const session = await api.getSession(id);
    replaceRows(rowsFromMeasurements(session.measurements, store.get().wheel.spokes_count));
    setStatus(`Session #${id} loaded.`, 'ok');
  });
}

function render(state, prev) {
  if (state.gridVersion !== prev?.gridVersion) {
    renderGrid(ui.tbody, state.rows);
  }
  if (state.sessions !== prev?.sessions) {
    renderSessionOptions(ui.sessionSelect, state.sessions);
  }
  if (state.result !== prev?.result) {
    renderResults(ui.results, state.result);
  }
  if (state.rows !== prev?.rows || state.wheel !== prev?.wheel || state.result !== prev?.result) {
    const data = { wheel: state.wheel, rows: state.rows, result: state.result };
    renderWheel(ui.wheelViz, data);
    renderRadar(ui.radarViz, data);
  }
  ui.deleteBtn.disabled = !state.wheel.id || state.busy;
  ui.solveBtn.disabled = state.busy;
}

store.subscribe(render);
render(store.get());

bindGrid(ui.table, {
  getRows: () => store.get().rows,
  onRows: (rows, rerender) => store.set((st) => ({
    rows, result: null, gridVersion: rerender ? st.gridVersion + 1 : st.gridVersion,
  })),
  onPasteErrors: (errors) => {
    if (errors.length > 0) {
      const e = errors[0];
      setStatus(`${errors.length} pasted cell(s) ignored, e.g. spoke ${e.row} ${e.column}: “${e.value}”.`, 'error');
    } else {
      setStatus('Pasted.', 'ok');
    }
  },
});

ui.form.addEventListener('change', onFormChange);
ui.form.addEventListener('submit', saveWheel);
ui.deleteBtn.addEventListener('click', deleteWheel);
ui.wheelSelect.addEventListener('change', () => guarded(() => selectWheel(Number(ui.wheelSelect.value))));
ui.solveBtn.addEventListener('click', solve);
ui.sessionSelect.addEventListener('change', loadSession);
ui.clearBtn.addEventListener('click', () => replaceRows(resizeRows([], store.get().wheel.spokes_count)));

fillForm(ui.form, DEFAULT_WHEEL);
guarded(() => refreshWheels());
