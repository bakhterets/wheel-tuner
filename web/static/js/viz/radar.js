// Polar tension chart in the spirit of the Park Tool tension app: each side's
// spokes are plotted relative to that side's average tension, with a ±20%
// tolerance band. Drive side = right flange, non-drive side = left flange.
import { s } from '../dom.js';
import { sideNumbers } from '../measurements.js';
import { angleAt, polar, radarRadius, relativeTensions } from './geometry.js';

const SIZE = 460;
const C = SIZE / 2;
const R_MIN = 30;
const R_MAX = 190;
const RANGE = 0.5; // plotted span: ±50 % around the average
const BAND = 0.2; // tolerance band: ±20 %

export const SERIES = Object.freeze([
  { side: 'R', label: 'Drive side (R)', cls: 'ds' },
  { side: 'L', label: 'Non-drive side (L)', cls: 'nds' },
]);

const rad = (rel) => radarRadius(rel, R_MIN, R_MAX, RANGE);

function series(rel, n, dir, cls, predicted, numberByIndex) {
  return SERIES.map(({ side, cls: sideCls }) => {
    const pts = rel.filter((p) => p.side === side && p.rel !== null)
      .map((p) => ({ ...p, ...polar(C, C, rad(p.rel), angleAt(p.index, n, dir)) }));
    if (pts.length === 0) {
      return null;
    }
    const outline = s(pts.length > 2 ? 'polygon' : 'polyline', {
      points: pts.map((p) => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' '),
      class: ['series', sideCls, cls],
    });
    const dots = predicted ? [] : pts.map((p) => {
      const out = Math.abs(p.rel - 1) > BAND;
      const pct = (p.rel - 1) * 100;
      return s('circle', {
        cx: p.x.toFixed(2), cy: p.y.toFixed(2), r: out ? 5 : 3, class: ['dot', sideCls, out && 'outlier'],
      }, s('title', { text: `${side}${numberByIndex.get(p.index)}: ${pct >= 0 ? '+' : ''}${pct.toFixed(1)}% vs ${side} average` }));
    });
    return s('g', {}, [outline, dots]);
  });
}

/**
 * @param {HTMLElement} container
 * @param {{rows: object[], wheel: object, result: object|null}} data
 */
export function renderRadar(container, { rows, wheel, result }) {
  const n = rows.length;
  const dir = wheel.counting_direction;
  const numbers = sideNumbers(rows);
  const numberByIndex = new Map(rows.map((row, i) => [row.spoke_index, numbers[i]]));
  const measured = relativeTensions(rows);
  const hasData = measured.some((p) => p.rel !== null);

  let predicted = [];
  if (result?.predicted?.length === n) {
    // Only compare spokes that were actually measured.
    const after = rows.map((r, i) => ({ ...r, tension_kgf: r.tension_kgf > 0 ? result.predicted[i].tension_kgf : null }));
    predicted = relativeTensions(after);
  }

  const axes = rows.map((r, i) => {
    const a = angleAt(r.spoke_index, n, dir);
    const p = polar(C, C, R_MAX, a);
    const l = polar(C, C, R_MAX + 14, a);
    return [
      s('line', { x1: C, y1: C, x2: p.x.toFixed(2), y2: p.y.toFixed(2), class: 'axis' }),
      s('text', { x: l.x.toFixed(2), y: l.y.toFixed(2), class: 'axis-label', 'text-anchor': 'middle', 'dominant-baseline': 'central' }, `${r.side}${numbers[i]}`),
    ];
  });

  const svg = s('svg', {
    viewBox: `0 0 ${SIZE} ${SIZE}`, role: 'img', class: 'radar-svg',
    'aria-label': 'Relative spoke tension per side',
  }, [
    s('circle', { cx: C, cy: C, r: rad(1 + BAND), class: 'band' }),
    s('circle', { cx: C, cy: C, r: rad(1 - BAND), class: 'band-hole' }),
    s('g', {}, axes),
    [1 - BAND, 1, 1 + BAND].map((v) => s('circle', { cx: C, cy: C, r: rad(v), class: v === 1 ? 'ring avg' : 'ring' })),
    s('text', { x: C + 4, y: C - rad(1) - 3, class: 'ring-label' }, 'avg'),
    s('text', { x: C + 4, y: C - rad(1 + BAND) - 3, class: 'ring-label' }, `+${BAND * 100}%`),
    s('text', { x: C + 4, y: C - rad(1 - BAND) - 3, class: 'ring-label' }, `−${BAND * 100}%`),
    series(predicted, n, dir, 'predicted', true, numberByIndex),
    series(measured, n, dir, 'measured', false, numberByIndex),
    hasData ? null : s('text', { x: C, y: C, class: 'empty', 'text-anchor': 'middle' }, 'Enter tensions to see the chart'),
  ]);
  container.replaceChildren(svg);
}
