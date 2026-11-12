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

/** Share of the rim limit at which the server flags tensions (api.tensionWarningRatio). */
export const WARNING_RATIO = 0.95;

const fmtKgf = (v) => `${Math.round(v)} kgf`;

export function tensionWarning(predicted, max) {
  return ` ⚠️ Approaching Max Rim Tension: ${fmtKgf(predicted)} of ${fmtKgf(max)}`;
}

/** Text of the enforced per-side tension caps, or '' when no cap applies. */
export function limitText(result) {
  const l = result.tension_limit_left_kgf ?? 0;
  const r = result.tension_limit_right_kgf ?? 0;
  if (!(l > 0 || r > 0)) {
    return '';
  }
  const cap = (v) => (v > 0 ? `≤ ${fmtKgf(v)}` : 'no cap');
  return `Tension cap (desired limit / rim max): Left ${cap(l)} · Right ${cap(r)}`;
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
      h('li', { class: [a.direction, a.tension_warning && 'warn'] }, [
        h('span', { class: 'badge', text: a.direction === 'tighten' ? '↑' : '↓' }),
        a.instruction,
        a.tension_warning && h('span', { class: 'warning', text: tensionWarning(a.predicted_tension_kgf, result.max_tension_kgf) }),
      ])));
  const warned = (result.predicted ?? []).filter((p) => p.tension_warning).length;

  const table = h('table', { class: 'metrics' }, [
    h('thead', {}, h('tr', {}, ['Metric', 'Before', 'After', 'Δ'].map((t) => h('th', { text: t })))),
    h('tbody', {}, METRICS.map((m) => h('tr', {}, [
      h('th', { text: m.label }),
      h('td', { text: formatValue(result.before[m.key], m.unit) }),
      h('td', { text: formatValue(result.after[m.key], m.unit) }),
      h('td', { text: improvement(result.before[m.key], result.after[m.key]) }),
    ]))),
  ]);

  const limits = limitText(result);
  container.replaceChildren(
    h('p', { class: 'orientation', role: 'note', text: result.orientation }),
    ...(limits ? [h('p', { class: 'limits', text: limits })] : []),
    ...(warned > 0 ? [h('p', {
      class: 'tension-alert', role: 'alert',
      text: `⚠️ Approaching Max Rim Tension: ${warned} spoke(s) predicted at ≥ ${Math.round(WARNING_RATIO * 100)}% of the ${fmtKgf(result.max_tension_kgf)} rim limit.`,
    })] : []),
    list,
    table,
  );
}
