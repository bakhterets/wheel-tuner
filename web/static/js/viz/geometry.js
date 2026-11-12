// Pure geometry and data shaping for the visualisations. Angles are in
// radians, measured clockwise on screen from 12 o'clock, where the start
// reference (rim seam / valve hole) is drawn. The wheel is shown as seen
// from the user's view point, so CW counting runs clockwise on screen.

const TAU = 2 * Math.PI;

function sense(direction) {
  return direction === 'CCW' ? -1 : 1;
}

/** Screen angle of a (possibly fractional, e.g. 9.5) spoke position. */
export function angleAt(position, n, direction) {
  return (sense(direction) * (position - 1) * TAU) / n;
}

/** Cartesian point at radius r and screen angle a around (cx, cy). */
export function polar(cx, cy, r, a) {
  return { x: cx + r * Math.sin(a), y: cy - r * Math.cos(a) };
}

const fmt = (v) => Number(v.toFixed(3));

/** SVG path of an annular sector between radii r0 < r1 and angles a0, a1 (any order). */
export function arcPath(cx, cy, r0, r1, a0, a1) {
  const [from, to] = a0 <= a1 ? [a0, a1] : [a1, a0];
  const large = to - from > Math.PI ? 1 : 0;
  const p1 = polar(cx, cy, r1, from);
  const p2 = polar(cx, cy, r1, to);
  const p3 = polar(cx, cy, r0, to);
  const p4 = polar(cx, cy, r0, from);
  return [
    `M${fmt(p1.x)} ${fmt(p1.y)}`,
    `A${fmt(r1)} ${fmt(r1)} 0 ${large} 1 ${fmt(p2.x)} ${fmt(p2.y)}`,
    `L${fmt(p3.x)} ${fmt(p3.y)}`,
    `A${fmt(r0)} ${fmt(r0)} 0 ${large} 0 ${fmt(p4.x)} ${fmt(p4.y)}`,
    'Z',
  ].join(' ');
}

/** Mean of measured (> 0) tensions per side. A side without data has mean null. */
export function sideMeans(rows, key = 'tension_kgf') {
  const acc = { L: { sum: 0, count: 0 }, R: { sum: 0, count: 0 } };
  for (const r of rows) {
    const t = r[key];
    if (t > 0 && acc[r.side]) {
      acc[r.side].sum += t;
      acc[r.side].count += 1;
    }
  }
  const mean = (s) => (s.count > 0 ? s.sum / s.count : null);
  return { L: mean(acc.L), R: mean(acc.R) };
}

/**
 * Tension of each spoke relative to its side average (1 = average).
 * Unmeasured spokes get rel null. `means` defaults to the rows' own means.
 */
export function relativeTensions(rows, key = 'tension_kgf', means = sideMeans(rows, key)) {
  return rows.map((r) => {
    const t = r[key];
    const m = means[r.side];
    return { index: r.spoke_index, side: r.side, rel: t > 0 && m ? t / m : null };
  });
}

/**
 * Deviation samples along the rim. A value flagged as peaking between spoke
 * i and i+1 is placed at i.5, matching the solver's interpolation.
 */
export function deviationSamples(rows, key, peakKey) {
  return rows
    .filter((r) => r[key] !== null && r[key] !== undefined)
    .map((r) => ({ position: r[peakKey] ? r.spoke_index + 0.5 : r.spoke_index, value: r[key] }));
}

/**
 * Problem zones: samples whose magnitude reaches max(minMM, fraction·max|v|).
 * Each zone spans half a spoke spacing either side of its sample.
 */
export function problemZones(samples, n, direction, { minMM = 0.1, fraction = 0.3 } = {}) {
  const peak = samples.reduce((m, s) => Math.max(m, Math.abs(s.value)), 0);
  const threshold = Math.max(minMM, fraction * peak);
  return samples
    .filter((s) => Math.abs(s.value) >= threshold && peak > 0)
    .map((s) => ({
      position: s.position,
      a0: angleAt(s.position - 0.5, n, direction),
      a1: angleAt(s.position + 0.5, n, direction),
      intensity: Math.abs(s.value) / peak,
      sign: Math.sign(s.value),
      value: s.value,
    }));
}

/** Maps a relative tension to a radar radius; rel 1 sits halfway, clamped to [1-range, 1+range]. */
export function radarRadius(rel, rMin, rMax, range = 0.5) {
  const t = (Math.min(Math.max(rel, 1 - range), 1 + range) - (1 - range)) / (2 * range);
  return rMin + t * (rMax - rMin);
}

/** Index of solver actions by spoke number. */
export function adjustmentMap(actions = []) {
  const map = new Map();
  for (const a of actions) {
    map.set(a.spoke_index, a);
  }
  return map;
}

const VIEW_SIDES = new Map([
  ['rotor side', 'L'], ['non-drive side', 'L'], ['left', 'L'],
  ['drive side', 'R'], ['non-rotor side', 'R'], ['right', 'R'],
]);

/** Physical side ('L'/'R') the user looks from; mirrors model.ViewSide. Unknown → 'L'. */
export function viewSideOf(viewPoint) {
  return VIEW_SIDES.get(String(viewPoint ?? '').trim().toLowerCase()) ?? 'L';
}
