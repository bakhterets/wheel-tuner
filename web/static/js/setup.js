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
  start_reference: 'Rim Seam',
  view_point: 'Rotor Side',
  counting_direction: 'CW',
});

const NUMERIC = ['spokes_count', 'erd_mm', 'cross_pattern', 'flange_dist_left_mm', 'flange_dist_right_mm'];

/** Reads the profile form into an API wheel object. */
export function readForm(form) {
  const fd = new FormData(form);
  const w = { ...DEFAULT_WHEEL };
  for (const key of Object.keys(DEFAULT_WHEEL)) {
    const v = fd.get(key);
    if (v === null) {
      continue;
    }
    w[key] = NUMERIC.includes(key) || key === 'id' ? Number(String(v).replace(',', '.')) : String(v);
  }
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
    } else {
      field.value = String(value);
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
