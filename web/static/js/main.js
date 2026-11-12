// Application entry point: wires state, API calls, forms and visualisations.
import { createClient } from './api.js';
import {
  compatibilityNotes, hubLabel, hubPatch, renderGeometry, renderNotes, renderPicker, rimLabel, rimPatch,
} from './components.js';
import { bindGrid, hasInvalidCells, renderGrid } from './grid.js';
import { resizeRows, rowsFromMeasurements, toMeasurements } from './measurements.js';
import { renderResults } from './results.js';
import { DEFAULT_WHEEL, fillForm, readForm, renderSessionOptions, renderWheelOptions } from './setup.js';
import { createStore } from './state.js';
import {
  applyReadings, converterKey, createConverter, renderGaugeOptions, renderTensiometerOptions,
} from './tensiometer.js';
import { renderRadar } from './viz/radar.js';
import { renderWheel } from './viz/wheel.js';

const $ = (sel) => document.querySelector(sel);

const ui = {
  wheelSelect: $('#wheel-select'),
  form: $('#wheel-form'),
  rimBrand: $('#rim-brand'),
  rimModel: $('#rim-model'),
  hubBrand: $('#hub-brand'),
  hubModel: $('#hub-model'),
  notes: $('#library-notes'),
  geometry: $('#geometry'),
  deleteBtn: $('#delete-wheel'),
  table: $('#grid'),
  tbody: $('#grid tbody'),
  tensionHead: $('#tension-head'),
  solveBtn: $('#solve'),
  clearBtn: $('#clear'),
  sessionSelect: $('#session-select'),
  results: $('#results'),
  wheelViz: $('#wheel-viz'),
  radarViz: $('#radar-viz'),
  status: $('#status'),
};
const field = (name) => ui.form.elements.namedItem(name);

const api = createClient();
const store = createStore({
  library: { rims: [], hubs: [], tensiometers: [] },
  wheels: [],
  wheel: { ...DEFAULT_WHEEL },
  conv: null,
  rows: resizeRows([], DEFAULT_WHEEL.spokes_count),
  gridVersion: 0,
  sessions: [],
  result: null,
  geometry: null,
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

let geometrySeq = 0;

/** Previews spoke geometry; only the latest request's answer is kept. */
async function refreshGeometry(wheel) {
  const seq = ++geometrySeq;
  let geometry;
  try {
    geometry = await api.geometry({ ...wheel, name: wheel.name || 'preview' });
  } catch (err) {
    geometry = { error: err.message };
  }
  if (seq === geometrySeq) {
    store.set({ geometry });
  }
}

/** Applies an edited profile: readings are re-converted when the tensiometer setup changes. */
function applyWheel(wheel, force = false) {
  const st = store.get();
  const patch = { wheel, result: null };
  if (force || converterKey(wheel) !== converterKey(st.wheel)) {
    patch.conv = createConverter(st.library.tensiometers, wheel);
    patch.rows = applyReadings(st.rows, patch.conv);
    patch.gridVersion = st.gridVersion + 1;
  }
  store.set(patch);
  if (wheel.spokes_count !== store.get().rows.length) {
    replaceRows(resizeRows(store.get().rows, wheel.spokes_count));
  }
  refreshGeometry(wheel);
}

function syncGauges(wheel) {
  const model = store.get().library.tensiometers.find((m) => m.id === wheel.tensiometer);
  renderGaugeOptions(field('spoke_gauge_left'), model, wheel.spoke_gauge_left);
  renderGaugeOptions(field('spoke_gauge_right'), model, wheel.spoke_gauge_right);
}

/** Re-renders the selects whose options depend on the profile: library pickers and gauges. */
function syncDependentFields(wheel) {
  const { library } = store.get();
  renderPicker(ui.rimBrand, ui.rimModel, library.rims, { selectedId: wheel.rim_component_id, label: rimLabel });
  renderPicker(ui.hubBrand, ui.hubModel, library.hubs, { selectedId: wheel.hub_component_id, label: hubLabel });
  renderTensiometerOptions(field('tensiometer'), library.tensiometers, wheel.tensiometer);
  syncGauges(wheel);
}

async function loadSessions(wheelId) {
  const sessions = wheelId ? await api.listSessions(wheelId) : [];
  store.set({ sessions });
}

async function selectWheel(id) {
  const wheel = id ? store.get().wheels.find((w) => w.id === id) : null;
  const next = { ...DEFAULT_WHEEL, ...wheel };
  syncDependentFields(next);
  fillForm(ui.form, next);
  replaceRows(resizeRows(store.get().rows, next.spokes_count));
  applyWheel(readForm(ui.form), true);
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

function onFormChange(ev) {
  if (ev.target === field('tensiometer')) {
    syncGauges(readForm(ui.form));
  }
  applyWheel(readForm(ui.form));
}

/** Copies a picked component into the form; the bubbling change event then applies it. */
function pickComponent(items, select, idKey, toPatch) {
  const current = readForm(ui.form);
  const component = items.find((c) => c.id === Number(select.value));
  fillForm(ui.form, { ...current, ...(component ? toPatch(component, current) : { [idKey]: null }) });
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
    renderGrid(ui.tbody, state.rows, state.conv);
    ui.tensionHead.textContent = state.conv ? state.conv.label : 'Tension, kgf';
  }
  if (state.sessions !== prev?.sessions) {
    renderSessionOptions(ui.sessionSelect, state.sessions);
  }
  if (state.result !== prev?.result) {
    renderResults(ui.results, state.result);
  }
  if (state.geometry !== prev?.geometry) {
    renderGeometry(ui.geometry, state.geometry);
  }
  if (state.wheel !== prev?.wheel || state.library !== prev?.library) {
    const { rims, hubs } = state.library;
    renderNotes(ui.notes, compatibilityNotes(
      state.wheel,
      rims.find((r) => r.id === state.wheel.rim_component_id),
      hubs.find((c) => c.id === state.wheel.hub_component_id),
    ));
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
  getConverter: () => store.get().conv,
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

ui.rimBrand.addEventListener('change', () => renderPicker(ui.rimBrand, ui.rimModel, store.get().library.rims, {
  selectedId: store.get().wheel.rim_component_id, brand: ui.rimBrand.value, label: rimLabel,
}));
ui.hubBrand.addEventListener('change', () => renderPicker(ui.hubBrand, ui.hubModel, store.get().library.hubs, {
  selectedId: store.get().wheel.hub_component_id, brand: ui.hubBrand.value, label: hubLabel,
}));
ui.rimModel.addEventListener('change', () => pickComponent(store.get().library.rims, ui.rimModel, 'rim_component_id', rimPatch));
ui.hubModel.addEventListener('change', () => pickComponent(store.get().library.hubs, ui.hubModel, 'hub_component_id', hubPatch));
ui.form.addEventListener('change', onFormChange);
ui.form.addEventListener('submit', saveWheel);
ui.deleteBtn.addEventListener('click', deleteWheel);
ui.wheelSelect.addEventListener('change', () => guarded(() => selectWheel(Number(ui.wheelSelect.value))));
ui.solveBtn.addEventListener('click', solve);
ui.sessionSelect.addEventListener('change', loadSession);
ui.clearBtn.addEventListener('click', () => replaceRows(resizeRows([], store.get().wheel.spokes_count)));

fillForm(ui.form, DEFAULT_WHEEL);
guarded(async () => {
  const [rims, hubs, tensiometers] = await Promise.all([api.listRims(), api.listHubs(), api.listTensiometers()]);
  store.set({ library: { rims, hubs, tensiometers } });
  await refreshWheels();
});
