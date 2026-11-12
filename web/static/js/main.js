// Application entry point: wires state, API calls, forms and visualisations.
import { createClient } from './api.js';
import { applyList } from './bulk.js';
import {
  compatibilityNotes, hubLabel, hubPatch, renderGeometry, renderNotes, renderPicker, rimLabel, rimPatch,
} from './components.js';
import { bindGrid, hasInvalidCells, renderGrid } from './grid.js';
import {
  assignSides, firstSide, resizeRows, rowsFromMeasurements, toMeasurements,
} from './measurements.js';
import { loadState, saveState } from './persist.js';
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
  measurements: $('#measurements'),
  tables: { L: $('#grid-left'), R: $('#grid-right') },
  tbodies: { L: $('#grid-left tbody'), R: $('#grid-right tbody') },
  tensionHeads: document.querySelectorAll('.tension-head'),
  bulkForms: document.querySelectorAll('form.bulk'),
  firstSide: document.querySelectorAll('input[name="first_side"]'),
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
const SIDES = ['L', 'R'];

/** localStorage, or null where it is unavailable (privacy mode, sandboxed frames). */
function browserStorage() {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}
const storage = browserStorage();
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
  // undefined: the first profile; an id that no longer exists: a new profile.
  const id = selectId === undefined ? wheels[0]?.id ?? 0 : (wheels.some((w) => w.id === selectId) ? selectId : 0);
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
  if (hasInvalidCells(ui.measurements)) {
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
    for (const side of SIDES) {
      renderGrid(ui.tbodies[side], state.rows, side, state.conv);
    }
    for (const head of ui.tensionHeads) {
      head.textContent = state.conv ? state.conv.label : 'Tension, kgf';
    }
    for (const radio of ui.firstSide) {
      radio.checked = radio.value === firstSide(state.rows);
    }
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

/** Fills one column of a side table from the list typed into its bulk form. */
function fillFromList(form) {
  const { rows, conv } = store.get();
  const side = form.dataset.side;
  const column = form.elements.namedItem('column');
  const key = column.value === 'tension' ? (conv ? 'tension_reading' : 'tension_kgf') : column.value;
  const { rows: next, error } = applyList(rows, side, key, form.elements.namedItem('values').value);
  if (error) {
    setStatus(error, 'error');
    return;
  }
  replaceRows(applyReadings(next, conv));
  const label = column.selectedOptions[0].textContent;
  setStatus(`${side === 'L' ? 'Left' : 'Right'} side: ${label} filled.`, 'ok');
}

// Restoring overwrites nothing until the saved state has been applied.
let persisting = false;

/** Restores the state saved before the last reload: profile form (with unsaved edits), grid and result. */
function restore(saved) {
  const wheel = { ...DEFAULT_WHEEL, ...saved.wheel, id: Number(ui.wheelSelect.value) };
  syncDependentFields(wheel);
  fillForm(ui.form, wheel);
  applyWheel(readForm(ui.form), true);
  replaceRows(applyReadings(resizeRows(saved.rows, store.get().wheel.spokes_count), store.get().conv));
  store.set({ result: saved.result });
}

store.subscribe(render);
store.subscribe((state, prev) => {
  if (persisting && (state.wheel !== prev.wheel || state.rows !== prev.rows || state.result !== prev.result)) {
    saveState(storage, state);
  }
});
render(store.get());

for (const side of SIDES) {
  bindGrid(ui.tables[side], {
    side,
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
}
for (const form of ui.bulkForms) {
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    fillFromList(form);
  });
}
for (const radio of ui.firstSide) {
  radio.addEventListener('change', () => {
    const { rows, conv } = store.get();
    replaceRows(applyReadings(assignSides(rows, radio.value), conv));
  });
}

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
ui.clearBtn.addEventListener('click', () => {
  const { rows, wheel } = store.get();
  replaceRows(assignSides(resizeRows([], wheel.spokes_count), firstSide(rows)));
});

fillForm(ui.form, DEFAULT_WHEEL);
guarded(async () => {
  const [rims, hubs, tensiometers] = await Promise.all([api.listRims(), api.listHubs(), api.listTensiometers()]);
  store.set({ library: { rims, hubs, tensiometers } });
  const saved = loadState(storage);
  await refreshWheels(saved ? saved.wheel.id ?? 0 : undefined);
  if (saved) {
    restore(saved);
  }
  persisting = true;
});
