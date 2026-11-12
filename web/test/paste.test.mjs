import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { applyPaste, COLUMNS, parseBool, parseNumber, parseSide, parseTable } from '../static/js/paste.js';
import { applyReadings } from '../static/js/tensiometer.js';
import { defaultRows } from '../static/js/measurements.js';

describe('parseTable', () => {
  it('splits tabs and CRLF lines, trimming cells and trailing blank lines', () => {
    assert.deepEqual(parseTable('1\t L \r\n2\tR\r\n\r\n'), [['1', 'L'], ['2', 'R']]);
  });
  it('keeps empty inner cells', () => {
    assert.deepEqual(parseTable('a\t\tb'), [['a', '', 'b']]);
  });
  it('returns no rows for blank input', () => {
    assert.deepEqual(parseTable('\n \n'), []);
  });
});

describe('parseNumber', () => {
  const cases = [
    ['0.25', 0.25], ['-0,4', -0.4], ['+1', 1], [' 1 200,5 ', 1200.5], ['.5', 0.5], ['1e-2', 0.01], ['', null], ['  ', null],
  ];
  for (const [input, want] of cases) {
    it(`parses ${JSON.stringify(input)}`, () => assert.equal(parseNumber(input), want));
  }
  for (const bad of ['abc', '1.2.3', '--1', '1,2,3', '0x10']) {
    it(`rejects ${JSON.stringify(bad)}`, () => assert.ok(Number.isNaN(parseNumber(bad))));
  }
});

describe('parseSide / parseBool', () => {
  it('maps side aliases', () => {
    assert.equal(parseSide('l'), 'L');
    assert.equal(parseSide(' Right '), 'R');
    assert.equal(parseSide('DS'), 'R');
    assert.equal(parseSide('nds'), 'L');
    assert.equal(parseSide('up'), null);
  });
  it('maps checkbox words', () => {
    for (const t of ['1', 'x', 'TRUE', 'да', '✓']) {
      assert.equal(parseBool(t), true, t);
    }
    for (const f of ['', '0', 'no', 'false', 'нет']) {
      assert.equal(parseBool(f), false, f);
    }
    assert.equal(parseBool('maybe'), null);
  });
});

describe('applyPaste', () => {
  const sideCol = COLUMNS.findIndex((c) => c.key === 'side');

  it('fills a block from the anchor cell and leaves the input untouched', () => {
    const rows = defaultRows(4);
    const { rows: out, errors } = applyPaste(rows, 1, sideCol, parseTable('L\t120\t0,3\tx\t-0.1\t\nR\t118\t-0.2\t\t0\t1'));
    assert.deepEqual(errors, []);
    assert.deepEqual(out[1], {
      spoke_index: 2, side: 'L', tension_kgf: 120, tension_reading: null, lateral_dev_mm: 0.3,
      lat_peak_between_next: true, radial_dev_mm: -0.1, rad_peak_between_next: false,
    });
    assert.equal(out[2].tension_kgf, 118);
    assert.equal(out[2].rad_peak_between_next, true);
    assert.equal(rows[1].tension_kgf, null, 'input rows must not be mutated');
    assert.deepEqual(out[0], rows[0]);
  });

  it('ignores the index column, rows beyond the grid and extra columns', () => {
    const { rows, errors } = applyPaste(defaultRows(2), 0, 0, parseTable('7\tR\t100\n8\tL\t101\n9\tR\t102').map((r) => [...r, '0', 'x', '0', 'x', 'extra']));
    assert.deepEqual(errors, []);
    assert.equal(rows[0].spoke_index, 1);
    assert.equal(rows[0].side, 'R');
    assert.equal(rows[1].tension_kgf, 101);
    assert.equal(rows.length, 2);
  });

  it('skips a header line', () => {
    const { rows } = applyPaste(defaultRows(1), 0, 0, parseTable('Spoke\tSide\tTension\n1\tR\t95'));
    assert.equal(rows[0].tension_kgf, 95);
  });

  it('clears numbers from empty cells and reports unparsable cells', () => {
    const start = defaultRows(1);
    start[0].lateral_dev_mm = 0.5;
    const { rows, errors } = applyPaste(start, 0, sideCol, [['?', 'abc', '', 'maybe']]);
    assert.equal(rows[0].side, 'L');
    assert.equal(rows[0].tension_kgf, null);
    assert.equal(rows[0].lateral_dev_mm, null);
    assert.deepEqual(errors.map((e) => e.column), ['side', 'tension_kgf', 'lat_peak_between_next']);
    assert.deepEqual(errors[0], { row: 1, column: 'side', value: '?' });
  });

  describe('tension fields stay consistent', () => {
    const readingColumns = COLUMNS.map((c) => (c.key === 'tension_kgf' ? { ...c, key: 'tension_reading' } : c));
    const tensionCol = COLUMNS.findIndex((c) => c.key === 'tension_kgf');
    const conv = { convert: (_side, v) => v * 40 };
    const withReadings = () => defaultRows(2).map((r) => ({ ...r, tension_reading: 3, tension_kgf: 120 }));

    it('an empty reading clears the converted kgf in tensiometer mode', () => {
      const { rows, errors } = applyPaste(withReadings(), 0, tensionCol, [[''], ['2.5']], readingColumns);
      assert.deepEqual(errors, []);
      const out = applyReadings(rows, conv);
      assert.equal(out[0].tension_reading, null);
      assert.equal(out[0].tension_kgf, null);
      assert.equal(out[1].tension_reading, 2.5);
      assert.equal(out[1].tension_kgf, 100);
    });

    it('a pasted kgf value drops the reading', () => {
      const { rows } = applyPaste(withReadings(), 0, tensionCol, [['110']]);
      assert.equal(rows[0].tension_kgf, 110);
      assert.equal(rows[0].tension_reading, null);
      assert.equal(rows[1].tension_reading, 3, 'rows outside the paste keep their reading');
    });
  });
});
