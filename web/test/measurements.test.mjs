import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { defaultRows, defaultSide, resizeRows, rowsFromMeasurements, toMeasurements } from '../static/js/measurements.js';

describe('measurement rows', () => {
  it('alternates sides L/R starting with L', () => {
    assert.deepEqual(defaultRows(4).map((r) => `${r.side}${r.spoke_index}`), ['L1', 'R2', 'L3', 'R4']);
    assert.equal(defaultSide(36), 'R');
  });

  it('resizes keeping existing data', () => {
    const rows = defaultRows(2);
    rows[1].tension_kgf = 110;
    const grown = resizeRows(rows, 3);
    assert.equal(grown.length, 3);
    assert.equal(grown[1].tension_kgf, 110);
    assert.notEqual(grown[1], rows[1], 'rows are copied');
    assert.equal(grown[2].spoke_index, 3);
    assert.equal(resizeRows(grown, 1).length, 1);
  });

  it('converts to API measurements with empty cells as zero', () => {
    const rows = defaultRows(1);
    rows[0].lateral_dev_mm = -0.3;
    assert.deepEqual(toMeasurements(rows), [{
      spoke_index: 1, side: 'L', tension_kgf: 0, lateral_dev_mm: -0.3,
      lat_peak_between_next: false, radial_dev_mm: 0, rad_peak_between_next: false,
    }]);
  });

  it('round-trips stored measurements and ignores out-of-range rows', () => {
    const ms = [
      { id: 9, session_id: 1, spoke_index: 2, side: 'L', tension_kgf: 0, lateral_dev_mm: 0.2, lat_peak_between_next: true, radial_dev_mm: -0.1, rad_peak_between_next: false },
      { spoke_index: 5, side: 'R', tension_kgf: 100, lateral_dev_mm: 0, radial_dev_mm: 0 },
    ];
    const rows = rowsFromMeasurements(ms, 3);
    assert.equal(rows.length, 3);
    assert.deepEqual(rows[1], {
      spoke_index: 2, side: 'L', tension_kgf: null, tension_reading: null, lateral_dev_mm: 0.2,
      lat_peak_between_next: true, radial_dev_mm: -0.1, rad_peak_between_next: false,
    });
    assert.equal(rows[0].side, 'L');
    const { id: _id, session_id: _sid, ...payload } = ms[0];
    assert.deepEqual(toMeasurements(rows)[1], payload);
  });

  it('carries tensiometer readings both ways', () => {
    const ms = [{ spoke_index: 1, side: 'L', tension_kgf: 102, tension_reading: 2.8, lateral_dev_mm: 0, radial_dev_mm: 0 }];
    const rows = rowsFromMeasurements(ms, 2);
    assert.equal(rows[0].tension_reading, 2.8);
    assert.equal(rows[1].tension_reading, null);
    const out = toMeasurements(rows);
    assert.equal(out[0].tension_reading, 2.8);
    assert.equal(out[0].tension_kgf, 102);
    assert.ok(!('tension_reading' in out[1]), 'no reading is sent for kgf rows');
    assert.ok(!('tension_reading' in toMeasurements([{ ...rows[1], tension_reading: undefined }])[0]));
  });
});
