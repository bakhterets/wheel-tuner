// Component library: cascading Brand → Model pickers that autofill the
// editable profile fields, plus the geometry preview panel.
import { h } from './dom.js';

/** Sorted unique brands of the given components. */
export function brands(items) {
  return [...new Set(items.map((c) => c.brand))].sort();
}

function holes(c) {
  return c.hole_counts?.length ? `${c.hole_counts.join('/')}H` : 'holes n/a';
}

export function rimLabel(r) {
  return `${r.model} ${r.wheel_size}″ — ERD ${r.erd_mm}, ${holes(r)}`;
}

export function hubLabel(c) {
  return `${c.model} — ${c.spoke_type}, ${holes(c)}`;
}

/**
 * Renders a Brand → Model picker pair. An explicit brand wins; otherwise the
 * brand of the selected component is shown. Models are limited to that brand.
 */
export function renderPicker(brandSelect, modelSelect, items, { selectedId = null, brand = null, label }) {
  const selected = items.find((c) => c.id === selectedId);
  const activeBrand = brand ?? selected?.brand ?? '';
  const current = selected?.brand === activeBrand ? selected : undefined;
  brandSelect.replaceChildren(
    h('option', { value: '', text: '— brand —' }),
    ...brands(items).map((b) => h('option', { value: b, selected: b === activeBrand, text: b })),
  );
  brandSelect.value = activeBrand;
  const models = items.filter((c) => c.brand === activeBrand);
  modelSelect.replaceChildren(
    h('option', { value: '', text: models.length ? '— model —' : '— pick a brand —' }),
    ...models.map((c) => h('option', { value: String(c.id), selected: c.id === current?.id, text: label(c) })),
  );
  modelSelect.value = current ? String(current.id) : '';
  modelSelect.disabled = models.length === 0;
}

function fitSpokes(c, spokes) {
  return c.hole_counts?.length && !c.hole_counts.includes(spokes) ? { spokes_count: c.hole_counts[0] } : {};
}

/** Profile fields taken from a rim; the spoke count follows the rim's drillings. */
export function rimPatch(rim, wheel) {
  return {
    rim_component_id: rim.id,
    erd_mm: rim.erd_mm,
    rim_asymmetry_offset_mm: rim.asymmetry_offset_mm,
    max_tension_kgf: rim.max_tension_kgf,
    ...fitSpokes(rim, wheel.spokes_count),
  };
}

/** Profile fields taken from a hub. */
export function hubPatch(hub, wheel) {
  return {
    hub_component_id: hub.id,
    flange_dist_left_mm: hub.flange_dist_left_mm,
    flange_dist_right_mm: hub.flange_dist_right_mm,
    flange_diam_left_mm: hub.flange_diam_left_mm,
    flange_diam_right_mm: hub.flange_diam_right_mm,
    spoke_type: hub.spoke_type,
    ...fitSpokes(hub, wheel.spokes_count),
  };
}

/** Warnings about a profile that does not match its library components. */
export function compatibilityNotes(wheel, rim, hub) {
  const notes = [];
  for (const [kind, c] of [['Rim', rim], ['Hub', hub]]) {
    if (c?.hole_counts?.length && !c.hole_counts.includes(wheel.spokes_count)) {
      notes.push(`${kind} ${c.brand} ${c.model} has no ${wheel.spokes_count}H version (${holes(c)}).`);
    }
  }
  if (rim && hub && rim.hole_counts?.length && hub.hole_counts?.length
    && !rim.hole_counts.some((n) => hub.hole_counts.includes(n))) {
    notes.push('The rim and hub share no drilling.');
  }
  return notes;
}

/** Lists compatibility notes; an empty list hides the element via CSS. */
export function renderNotes(list, notes) {
  list.replaceChildren(...notes.map((n) => h('li', { text: n })));
}

const fmt = (v, digits = 1) => Number(v).toFixed(digits);

/**
 * Renders the spoke geometry preview (POST /api/geometry response).
 * @param {HTMLElement} container
 * @param {{left: object, right: object, tension_ratio: number}|{error: string}|null} geo
 */
export function renderGeometry(container, geo) {
  if (!geo) {
    container.replaceChildren();
    return;
  }
  if (geo.error) {
    container.replaceChildren(h('p', { class: 'muted hint', text: `Geometry: ${geo.error}` }));
    return;
  }
  const row = (label, f) => h('tr', {}, [h('th', { text: label }), h('td', { text: f(geo.left) }), h('td', { text: f(geo.right) })]);
  container.replaceChildren(h('table', { class: 'metrics geometry' }, [
    h('thead', {}, h('tr', {}, ['Spoke geometry', 'Left', 'Right'].map((t) => h('th', { text: t })))),
    h('tbody', {}, [
      row('Spoke length, mm', (s) => fmt(s.spoke_length_mm)),
      row('Flange offset, mm', (s) => fmt(s.flange_offset_mm)),
      row('Bracing angle, °', (s) => fmt(s.bracing_angle_deg, 2)),
      row('ΔT per turn, kgf', (s) => fmt(s.tension_per_turn_kgf)),
      h('tr', {}, [h('th', { text: 'Tension ratio L/R' }), h('td', { colspan: 2, text: fmt(geo.tension_ratio, 2) })]),
    ]),
  ]));
}
