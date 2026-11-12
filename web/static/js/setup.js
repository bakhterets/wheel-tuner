// Wheel profile form: reads and fills the static form in index.html.
import { h } from './dom.js';

export const DEFAULT_WHEEL = Object.freeze({
  id: 0,
  name: '',
  spokes_count: 32,
  erd_mm: 600,
  cross_pattern: 3,
  flange_dist_left_mm: 35,
  flange_dist_right_mm: 20,
  flange_diam_left_mm: 0,
  flange_diam_right_mm: 0,
  spoke_type: 'J-Bend',
  paired_spokes: false,
  rim_asymmetry_offset_mm: 0,
  max_tension_kgf: 0,
  tension_limit: 0,
  tension_limit_unit: 'kgf',
  rim_component_id: null,
  hub_component_id: null,
  tensiometer: '',
  spoke_gauge_left: '',
  spoke_gauge_right: '',
  start_reference: 'Rim Seam',
  view_point: 'Rotor Side',
  counting_direction: 'CW',
});

const NUMERIC = new Set([
  'id', 'spokes_count', 'erd_mm', 'cross_pattern', 'flange_dist_left_mm', 'flange_dist_right_mm',
  'flange_diam_left_mm', 'flange_diam_right_mm', 'rim_asymmetry_offset_mm', 'max_tension_kgf', 'tension_limit',
]);
const CHECKBOXES = new Set(['paired_spokes']);
const COMPONENT_IDS = new Set(['rim_component_id', 'hub_component_id']);

/** Reads the profile form into an API wheel object; empty component ids are null. */
export function readForm(form) {
  const fd = new FormData(form);
  const w = { ...DEFAULT_WHEEL };
  for (const key of Object.keys(DEFAULT_WHEEL)) {
    const v = fd.get(key);
    if (v === null) {
      continue; // also an unchecked checkbox: keeps the default false
    }
    if (CHECKBOXES.has(key)) {
      w[key] = true;
    } else if (COMPONENT_IDS.has(key)) {
      w[key] = Number(v) > 0 ? Number(v) : null;
    } else {
      w[key] = NUMERIC.has(key) ? Number(String(v).replace(',', '.')) : String(v);
    }
  }
  // Paired heads exist on straight-pull hubs only; the server rejects other combinations.
  w.paired_spokes &&= w.spoke_type === 'Straightpull';
  return w;
}

/** Fills the profile form; toggle (radio) groups select the matching option. */
export function fillForm(form, wheel) {
  for (const [key, value] of Object.entries({ ...DEFAULT_WHEEL, ...wheel })) {
    const field = form.elements.namedItem(key);
    if (!field) {
      continue;
    }
    if (field instanceof RadioNodeList) {
      let matched = false;
      for (const r of field) {
        r.checked = r.value.toLowerCase() === String(value).toLowerCase();
        matched ||= r.checked;
      }
      if (!matched && field[0]) {
        field[0].checked = true;
      }
    } else if (CHECKBOXES.has(key)) {
      field.checked = Boolean(value);
    } else {
      field.value = value === null ? '' : String(value);
    }
  }
}

/** Renders the profile selector options. */
export function renderWheelOptions(select, wheels, selectedId) {
  select.replaceChildren(
    h('option', { value: '0', text: '— New profile —' }),
    ...wheels.map((w) => h('option', { value: String(w.id), selected: w.id === selectedId, text: `${w.name} (${w.spokes_count}H, ${w.cross_pattern}x)` })),
  );
  select.value = String(selectedId || 0);
}

/** Renders the stored sessions selector options. */
export function renderSessionOptions(select, sessions) {
  select.replaceChildren(
    h('option', { value: '', text: sessions.length ? 'Load a saved session…' : 'No saved sessions' }),
    ...sessions.map((s) => h('option', { value: String(s.id), text: `#${s.id} — ${new Date(s.created_at).toLocaleString()}` })),
  );
  select.disabled = sessions.length === 0;
}
