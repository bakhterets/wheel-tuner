import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { applyList, parseList } from '../static/js/bulk.js';
import { defaultRows } from '../static/js/measurements.js';
import { loadState, saveState, STORAGE_KEY } from '../static/js/persist.js';

describe('bulk list input', () => {
  it('splits bracketed, comma, whitespace and semicolon lists', () => {
    const cases = [
      ['[3.15,3.16,3.05]', ['3.15', '3.16', '3.05']],
      [' [ 3.15, 3.16 ,3.05 ] ', ['3.15', '3.16', '3.05']],
      ['3.15 3.16\n3.05', ['3.15', '3.16', '3.05']],
      ['[3,15; 3,16;3,05]', ['3,15', '3,16', '3,05']],
      ['3.15,,3.05', ['3.15', '', '3.05']],
      ['[]', []],
      ['  ', []],
    ];
    for (const [text, want] of cases) {
      assert.deepEqual(parseList(text), want, text);
    }
  });

  it("fills the side's spokes in order and leaves the other side alone", () => {
    const rows = defaultRows(6);
    rows[2].tension_reading = 2.5;
    const { rows: out, error } = applyList(rows, 'L', 'tension_kgf', '[101, 102,]');
    assert.equal(error, null);
    assert.deepEqual(out.map((r) => r.tension_kgf), [101, null, 102, null, null, null]);
    assert.equal(out[2].tension_reading, null, 'a kgf value replaces a stale reading');
    assert.equal(rows[0].tension_kgf, null, 'input rows are not mutated');

    const skipped = applyList(out, 'R', 'lateral_dev_mm', '0.1,,-0.2').rows;
    assert.deepEqual(skipped.map((r) => r.lateral_dev_mm), [null, 0.1, null, null, null, -0.2]);
    assert.equal(skipped[3], out[3], 'an empty item skips the spoke');

    const readings = applyList(rows, 'R', 'tension_reading', '2.8;2,9').rows;
    assert.deepEqual(readings.map((r) => r.tension_reading), [null, 2.8, 2.5, 2.9, null, null]);
  });

  it('rejects the whole list on errors', () => {
    const rows = defaultRows(4);
    const cases = [
      ['', 'The list is empty.'],
      ['1,2,3', '3 values for 2 L-side spokes.'],
      ['1,x', 'Value #2 “x” is not a valid number.'],
      ['-1', 'Value #1 “-1” is not a valid number.'],
    ];
    for (const [text, want] of cases) {
      const res = applyList(rows, 'L', 'tension_kgf', text);
      assert.equal(res.error, want, text);
      assert.equal(res.rows, rows);
    }
    assert.equal(applyList(rows, 'L', 'radial_dev_mm', '-0.3').rows[0].radial_dev_mm, -0.3, 'deviations may be negative');
  });
});

function memoryStorage(initial = {}) {
  const data = new Map(Object.entries(initial));
  return {
    data,
    getItem: (k) => (data.has(k) ? data.get(k) : null),
    setItem: (k, v) => data.set(k, String(v)),
  };
}

describe('state persistence', () => {
  const wheel = { id: 3, name: 'Front F 572', spokes_count: 4, tension_limit: 2.8, tension_limit_unit: 'reading' };
  const result = { session_id: 7, actions: [{ instruction: 'Spoke L1: +1/4 turn (Tighten)' }] };

  it('round-trips the profile form, grid and result', () => {
    const storage = memoryStorage();
    const rows = defaultRows(4);
    Object.assign(rows[0], { tension_kgf: 102, tension_reading: 2.8, lat_peak_between_next: true });
    assert.equal(saveState(storage, { wheel, rows, result, busy: true }), true);
    const saved = JSON.parse(storage.data.get(STORAGE_KEY));
    assert.ok(!('busy' in saved), 'only the working state is stored');
    assert.deepEqual(loadState(storage), { wheel, rows, result });
  });

  it('sanitises stored rows and drops a malformed result', () => {
    const rows = [{ spoke_index: 1, side: 'R', tension_kgf: 'x', lateral_dev_mm: 0.3, extra: 1, rad_peak_between_next: 'yes' }];
    const storage = memoryStorage({ [STORAGE_KEY]: JSON.stringify({ version: 1, wheel, rows, result: { actions: 'no' } }) });
    const got = loadState(storage);
    assert.deepEqual(got.rows, [{
      spoke_index: 1, side: 'R', tension_kgf: null, tension_reading: null, lateral_dev_mm: 0.3,
      lat_peak_between_next: false, radial_dev_mm: null, rad_peak_between_next: false,
    }]);
    assert.equal(got.result, null);
  });

  it('ignores missing, corrupt or foreign state', () => {
    const state = (patch) => JSON.stringify({ version: 1, wheel, rows: defaultRows(2), result: null, ...patch });
    const cases = {
      'no storage': null,
      empty: memoryStorage(),
      'invalid JSON': memoryStorage({ [STORAGE_KEY]: '{' }),
      'not an object': memoryStorage({ [STORAGE_KEY]: '[]' }),
      'other version': memoryStorage({ [STORAGE_KEY]: state({ version: 2 }) }),
      'no wheel': memoryStorage({ [STORAGE_KEY]: state({ wheel: null }) }),
      'rows not a list': memoryStorage({ [STORAGE_KEY]: state({ rows: {} }) }),
      'rows out of order': memoryStorage({ [STORAGE_KEY]: state({ rows: defaultRows(2).reverse() }) }),
      'bad side': memoryStorage({ [STORAGE_KEY]: state({ rows: [{ spoke_index: 1, side: 'X' }] }) }),
      'storage throws': { getItem: () => { throw new Error('denied'); } },
    };
    for (const [name, storage] of Object.entries(cases)) {
      assert.equal(loadState(storage), null, name);
    }
  });

  it('reports write failures instead of throwing', () => {
    const full = { setItem: () => { throw new Error('QuotaExceededError'); } };
    assert.equal(saveState(full, { wheel, rows: [], result: null }), false);
    assert.equal(saveState(null, { wheel, rows: [], result: null }), false);
  });
});