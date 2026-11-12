// SVG wheel model: rim, spokes coloured by solver action, and wobble
// (lateral) / hop (radial) problem zones.
import { s } from '../dom.js';
import { adjustmentMap, angleAt, arcPath, deviationSamples, polar, problemZones, viewSideOf } from './geometry.js';

const SIZE = 460;
const C = SIZE / 2;
const R = {
  hub: 22,
  radialIn: 148,
  radialOut: 164,
  rim: 172,
  lateralIn: 182,
  lateralOut: 196,
  label: 212,
};

function zoneLayer(cls, zones, r0, r1, describe) {
  return s('g', { class: cls }, zones.map((z) =>
    s('path', {
      d: arcPath(C, C, r0, r1, z.a0, z.a1),
      'fill-opacity': (0.25 + 0.75 * z.intensity).toFixed(2),
      class: z.sign >= 0 ? 'pos' : 'neg',
    }, s('title', { text: describe(z) }))));
}

const pos = (p) => (Number.isInteger(p) ? String(p) : `${p - 0.5}–${p + 0.5}`);

/**
 * Renders the wheel into container.
 * @param {HTMLElement} container
 * @param {{wheel: object, rows: object[], result: object|null}} data
 */
export function renderWheel(container, { wheel, rows, result }) {
  const n = rows.length;
  const dir = wheel.counting_direction;
  const viewSide = viewSideOf(wheel.view_point);
  const actions = adjustmentMap(result?.actions);

  const wobble = problemZones(deviationSamples(rows, 'lateral_dev_mm', 'lat_peak_between_next'), n, dir);
  const hop = problemZones(deviationSamples(rows, 'radial_dev_mm', 'rad_peak_between_next'), n, dir);

  const spokes = rows.map((r) => {
    const a = angleAt(r.spoke_index, n, dir);
    const p0 = polar(C, C, R.hub, a);
    const p1 = polar(C, C, R.radialIn, a);
    const act = actions.get(r.spoke_index);
    const kind = act ? act.direction : 'idle';
    return s('line', {
      x1: p0.x.toFixed(2), y1: p0.y.toFixed(2), x2: p1.x.toFixed(2), y2: p1.y.toFixed(2),
      class: ['spoke', kind, r.side === viewSide ? 'near' : 'far'],
    }, s('title', { text: act ? act.instruction : `Spoke ${r.side}${r.spoke_index}` }));
  });

  const labels = rows.map((r) => {
    const p = polar(C, C, R.label, angleAt(r.spoke_index, n, dir));
    const act = actions.get(r.spoke_index);
    return s('text', {
      x: p.x.toFixed(2), y: p.y.toFixed(2), class: ['spoke-label', act?.direction],
      'text-anchor': 'middle', 'dominant-baseline': 'central',
    }, `${r.side}${r.spoke_index}`);
  });

  const ref = polar(C, C, R.radialIn - 14, 0);
  const svg = s('svg', {
    viewBox: `0 0 ${SIZE} ${SIZE}`, role: 'img', class: 'wheel-svg',
    'aria-label': `Wheel model, ${n} spokes, viewed from ${wheel.view_point}, counting ${dir}`,
  }, [
    zoneLayer('zones lateral', wobble, R.lateralIn, R.lateralOut, (z) => `Wobble at ${pos(z.position)}: ${z.value.toFixed(2)} mm`),
    zoneLayer('zones radial', hop, R.radialIn, R.radialOut, (z) => `Hop at ${pos(z.position)}: ${z.value.toFixed(2)} mm`),
    s('circle', { cx: C, cy: C, r: R.rim, class: 'rim' }),
    s('g', { class: 'spokes' }, spokes),
    s('circle', { cx: C, cy: C, r: R.hub, class: 'hub' }),
    s('g', { class: 'labels' }, labels),
    s('path', { d: `M${C} ${R.lateralIn - 2} l-7 -12 h14 z`, class: 'reference-mark' }),
    s('text', { x: ref.x, y: ref.y, class: 'reference', 'text-anchor': 'middle' }, wheel.start_reference),
    s('text', { x: C, y: C + R.hub + 18, class: 'direction', 'text-anchor': 'middle' }, `${dir === 'CCW' ? '↺' : '↻'} ${dir}`),
    s('text', { x: C, y: C + R.hub + 34, class: 'view', 'text-anchor': 'middle' }, `View: ${wheel.view_point}`),
  ]);
  container.replaceChildren(svg);
}
