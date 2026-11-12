// Renders the solver output: orientation reminder, instruction list and metrics.
import { h } from './dom.js';

const METRICS = Object.freeze([
  { key: 'max_lateral_mm', label: 'Max lateral (wobble)', unit: 'mm' },
  { key: 'max_radial_mm', label: 'Max radial (hop)', unit: 'mm' },
  { key: 'tension_spread_left_kgf', label: 'Tension σ, left', unit: 'kgf' },
  { key: 'tension_spread_right_kgf', label: 'Tension σ, right', unit: 'kgf' },
]);

export function formatValue(v, unit) {
  return `${Number(v).toFixed(2)} ${unit}`;
}

/** Percentage improvement from before to after; '' when before is 0. */
export function improvement(before, after) {
  if (!(before > 0)) {
    return '';
  }
  const pct = Math.round(((before - after) / before) * 100);
  return pct >= 0 ? `−${pct}%` : `+${-pct}%`;
}

/**
 * @param {HTMLElement} container
 * @param {object|null} result API solve response
 */
export function renderResults(container, result) {
  if (!result) {
    container.replaceChildren(h('p', { class: 'muted', text: 'Enter measurements and press “Solve”.' }));
    return;
  }
  const list = result.actions.length === 0
    ? h('p', { class: 'ok', text: 'No adjustment needed: every correction is below 1/8 turn.' })
    : h('ol', { class: 'actions' }, result.actions.map((a) =>
      h('li', { class: a.direction }, [
        h('span', { class: 'badge', text: a.direction === 'tighten' ? '↑' : '↓' }),
        a.instruction,
      ])));

  const table = h('table', { class: 'metrics' }, [
    h('thead', {}, h('tr', {}, ['Metric', 'Before', 'After', 'Δ'].map((t) => h('th', { text: t })))),
    h('tbody', {}, METRICS.map((m) => h('tr', {}, [
      h('th', { text: m.label }),
      h('td', { text: formatValue(result.before[m.key], m.unit) }),
      h('td', { text: formatValue(result.after[m.key], m.unit) }),
      h('td', { text: improvement(result.before[m.key], result.after[m.key]) }),
    ]))),
  ]);

  container.replaceChildren(
    h('p', { class: 'orientation', role: 'note', text: result.orientation }),
    list,
    table,
  );
}
