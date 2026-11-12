import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  adjustmentMap, angleAt, arcPath, deviationSamples, polar, problemZones, radarRadius,
  relativeTensions, sideMeans, viewSideOf,
} from '../static/js/viz/geometry.js';
import { defaultRows } from '../static/js/measurements.js';
import { formatValue, improvement } from '../static/js/results.js';

const close = (a, b, eps = 1e-9) => assert.ok(Math.abs(a - b) < eps, `${a} ≈ ${b}`);

describe('angles', () => {
  it('starts at 12 o\'clock and follows the counting direction', () => {
    close(angleAt(1, 32, 'CW'), 0);
    close(angleAt(9, 32, 'CW'), Math.PI / 2);
    close(angleAt(9, 32, 'CCW'), -Math.PI / 2);
    close(angleAt(1.5, 4, 'CW'), Math.PI / 4);
  });

  it('places points on screen with y growing downward', () => {
    const top = polar(100, 100, 10, 0);
    close(top.x, 100);
    close(top.y, 90);
    const right = polar(100, 100, 10, Math.PI / 2);
    close(right.x, 110);
    close(right.y, 100);
  });
});

describe('arcPath', () => {
  it('draws a closed annular sector regardless of angle order', () => {
    const d = arcPath(0, 0, 5, 10, Math.PI / 2, 0);
    assert.equal(d, 'M0 -10 A10 10 0 0 1 10 0 L5 0 A5 5 0 0 0 0 -5 Z');
  });
  it('sets the large-arc flag beyond 180°', () => {
    assert.match(arcPath(0, 0, 5, 10, 0, 1.5 * Math.PI), /A10 10 0 1 1/);
  });
});

describe('tension statistics', () => {
  const rows = defaultRows(4);
  rows[0].tension_kgf = 100; // L
  rows[2].tension_kgf = 120; // L
  rows[1].tension_kgf = 90; // R
  // rows[3] (R) unmeasured

  it('averages measured spokes per side', () => {
    assert.deepEqual(sideMeans(rows), { L: 110, R: 90 });
    assert.deepEqual(sideMeans(defaultRows(2)), { L: null, R: null });
  });

  it('expresses tension relative to the side mean', () => {
    const rel = relativeTensions(rows);
    close(rel[0].rel, 100 / 110);
    close(rel[2].rel, 120 / 110);
    assert.equal(rel[1].rel, 1);
    assert.equal(rel[3].rel, null);
    assert.deepEqual(rel[3], { index: 4, side: 'R', rel: null });
  });

  it('maps relative tension to a clamped radius', () => {
    assert.equal(radarRadius(1, 0, 100, 0.5), 50);
    assert.equal(radarRadius(1.5, 0, 100, 0.5), 100);
    assert.equal(radarRadius(3, 0, 100, 0.5), 100);
    assert.equal(radarRadius(0, 10, 100, 0.5), 10);
    close(radarRadius(1.1, 0, 100), 60);
  });
});

describe('deviation zones', () => {
  const rows = defaultRows(8);
  rows[2].lateral_dev_mm = 0.05; // below minMM
  rows[3].lateral_dev_mm = 0.8;
  rows[3].lat_peak_between_next = true; // peak at 4.5
  rows[5].lateral_dev_mm = -0.4;
  rows[6].lateral_dev_mm = 0.2; // below 30% of the peak

  it('places flagged peaks at i.5 and skips empty cells', () => {
    const samples = deviationSamples(rows, 'lateral_dev_mm', 'lat_peak_between_next');
    assert.deepEqual(samples, [
      { position: 3, value: 0.05 }, { position: 4.5, value: 0.8 }, { position: 6, value: -0.4 }, { position: 7, value: 0.2 },
    ]);
  });

  it('keeps only significant samples, spanning half a spacing each side', () => {
    const zones = problemZones(deviationSamples(rows, 'lateral_dev_mm', 'lat_peak_between_next'), 8, 'CW');
    assert.deepEqual(zones.map((z) => [z.position, z.sign, z.intensity]), [[4.5, 1, 1], [6, -1, 0.5]]);
    close(zones[0].a0, angleAt(4, 8, 'CW'));
    close(zones[0].a1, angleAt(5, 8, 'CW'));
  });

  it('returns no zones for a true wheel', () => {
    assert.deepEqual(problemZones([{ position: 1, value: 0 }], 8, 'CW', { minMM: 0 }), []);
  });
});

describe('misc helpers', () => {
  it('indexes actions by spoke', () => {
    const m = adjustmentMap([{ spoke_index: 3, direction: 'tighten' }]);
    assert.equal(m.get(3).direction, 'tighten');
    assert.equal(adjustmentMap().size, 0);
  });

  it('mirrors the backend view point mapping', () => {
    assert.equal(viewSideOf('Rotor Side'), 'L');
    assert.equal(viewSideOf('non-drive side'), 'L');
    assert.equal(viewSideOf(' Drive Side '), 'R');
    assert.equal(viewSideOf('Non-Rotor Side'), 'R');
    assert.equal(viewSideOf(undefined), 'L');
  });

  it('formats metrics and improvement', () => {
    assert.equal(formatValue(0.456, 'mm'), '0.46 mm');
    assert.equal(improvement(2, 0.5), '−75%');
    assert.equal(improvement(1, 1.2), '+20%');
    assert.equal(improvement(0, 0), '');
  });
});
