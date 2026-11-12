import assert from 'node:assert/strict';
import { before, describe, it } from 'node:test';

import { installFakeDom } from './fakedom.mjs';
import { defaultRows } from '../static/js/measurements.js';

const { container } = installFakeDom();
let tm, comp, renderGrid, bindGrid, hasInvalidCells, renderResults;

before(async () => {
  tm = await import('../static/js/tensiometer.js');
  comp = await import('../static/js/components.js');
  ({ renderGrid, bindGrid, hasInvalidCells } = await import('../static/js/grid.js'));
  ({ renderResults } = await import('../static/js/results.js'));
});

// Excerpt in the /api/tensiometers shape (ZTTO TC-02 rows, readings in mm).
const catalog = [{
  id: 'ztto-tc-02', name: 'ZTTO TC-02', reading_unit: 'mm', gauges: [
    { id: '1.8', label: 'Ø1.8 round', points: [{ reading: 2.6, kgf: 85 }, { reading: 2.8, kgf: 100 }, { reading: 3.0, kgf: 120 }] },
    { id: '2.0-1.8-2.0', label: 'Ø2.0-1.8-2.0 butted', points: [{ reading: 2.6, kgf: 87 }, { reading: 2.8, kgf: 102 }, { reading: 3.0, kgf: 122 }] },
  ],
}];
const wheel = { tensiometer: 'ztto-tc-02', spoke_gauge_left: '2.0-1.8-2.0', spoke_gauge_right: '1.8' };

describe('tensiometer conversion', () => {
  const [g] = catalog[0].gauges;

  it('interpolates inside the calibrated range only', () => {
    assert.equal(tm.gaugeKgf(g, 2.6), 85);
    assert.equal(tm.gaugeKgf(g, 2.8), 100);
    assert.equal(tm.gaugeKgf(g, 3.0), 120);
    assert.ok(Math.abs(tm.gaugeKgf(g, 2.9) - 110) < 1e-9);
    for (const r of [2.59, 3.01, Number.NaN, null]) {
      assert.equal(tm.gaugeKgf(g, r), null, `reading ${r}`);
    }
    assert.equal(tm.gaugeRange(g), '2.6–3');
  });

  it('builds a per-side converter from the wheel profile', () => {
    assert.equal(tm.createConverter(catalog, { tensiometer: '' }), null);
    assert.equal(tm.createConverter(catalog, { tensiometer: 'unknown' }), null);
    const conv = tm.createConverter(catalog, wheel);
    assert.equal(conv.label, 'ZTTO TC-02 reading, mm');
    assert.equal(conv.convert('L', 2.8), 102);
    assert.equal(conv.convert('R', 2.8), 100);
    const partial = tm.createConverter(catalog, { ...wheel, spoke_gauge_right: '' });
    assert.equal(partial.gauges.R, null);
    assert.equal(partial.convert('R', 2.8), null);
    assert.equal(tm.converterKey(wheel), 'ztto-tc-02|2.0-1.8-2.0|1.8');
    assert.equal(tm.converterKey({ tensiometer: '' }), '');
  });

  it('re-applies readings when the converter changes', () => {
    const conv = tm.createConverter(catalog, wheel);
    const rows = [
      { side: 'L', tension_kgf: 50, tension_reading: 2.8 },
      { side: 'R', tension_kgf: 90, tension_reading: null },
      { side: 'R', tension_kgf: 90 },
      { side: 'R', tension_kgf: 99, tension_reading: 9 },
    ];
    const out = tm.applyReadings(rows, conv);
    assert.equal(out[0].tension_kgf, 102);
    assert.equal(out[1], rows[1]);
    assert.equal(out[2], rows[2]);
    assert.equal(out[3].tension_kgf, null, 'out of range');
    assert.deepEqual(tm.applyReadings(rows, null)[0], { side: 'L', tension_kgf: 50, tension_reading: null });
  });

  it('turns typed values into row patches', () => {
    const conv = tm.createConverter(catalog, wheel);
    const row = { side: 'R' };
    assert.equal(tm.tensionPatch(row, Number.NaN, null), null);
    assert.equal(tm.tensionPatch(row, -1, null), null);
    assert.deepEqual(tm.tensionPatch(row, 110, null), { tension_kgf: 110, tension_reading: null });
    assert.deepEqual(tm.tensionPatch(row, null, null), { tension_kgf: null, tension_reading: null });
    assert.deepEqual(tm.tensionPatch(row, null, conv), { tension_kgf: null, tension_reading: null });
    assert.deepEqual(tm.tensionPatch(row, 2.8, conv), { tension_kgf: 100, tension_reading: 2.8 });
    assert.equal(tm.tensionPatch(row, 5, conv), null);
  });

  it('previews converted readings', () => {
    const conv = tm.createConverter(catalog, wheel);
    assert.equal(tm.readingPreview({ tension_reading: 2.8, tension_kgf: 100 }, null), '');
    assert.equal(tm.readingPreview({ tension_reading: null }, conv), '');
    assert.equal(tm.readingPreview({}, conv), '');
    assert.equal(tm.readingPreview({ side: 'L', tension_reading: 2.9, tension_kgf: 111.6 }, conv), '≈ 112 kgf');
    assert.equal(tm.readingPreview({ side: 'R', tension_reading: 9, tension_kgf: null }, conv), 'outside 2.6–3');
    const partial = tm.createConverter(catalog, { ...wheel, spoke_gauge_left: '' });
    assert.equal(tm.readingPreview({ side: 'L', tension_reading: 2.8, tension_kgf: null }, partial), 'no gauge');
  });

  it('renders tensiometer and gauge options', () => {
    const sel = container();
    tm.renderTensiometerOptions(sel, catalog, 'ztto-tc-02');
    assert.deepEqual(sel.children.map((o) => o.value), ['', 'ztto-tc-02']);
    assert.equal(sel.value, 'ztto-tc-02');
    tm.renderTensiometerOptions(sel, catalog, undefined);
    assert.equal(sel.value, '');

    const gauge = container();
    assert.equal(tm.renderGaugeOptions(gauge, undefined, '1.8'), '');
    assert.equal(gauge.disabled, true);
    assert.equal(tm.renderGaugeOptions(gauge, catalog[0], 'nope'), '1.8');
    assert.equal(gauge.disabled, false);
    assert.equal(tm.renderGaugeOptions(gauge, catalog[0], '2.0-1.8-2.0'), '2.0-1.8-2.0');
    assert.equal(gauge.children[1].textContent, 'Ø2.0-1.8-2.0 butted (2.6–3 mm)');
  });
});

const rims = [
  { id: 1, brand: 'DT Swiss', model: 'F 572', wheel_size: '29', hole_counts: [28, 32], erd_mm: 602, asymmetry_offset_mm: 0, max_tension_kgf: 122 },
  { id: 2, brand: 'DT Swiss', model: 'EX 511', wheel_size: '29', hole_counts: [28, 32], erd_mm: 601, asymmetry_offset_mm: 0, max_tension_kgf: 122 },
  { id: 3, brand: 'Acme', model: 'Old', wheel_size: '26', hole_counts: [36], erd_mm: 540, asymmetry_offset_mm: 3, max_tension_kgf: 0 },
];
const hubs = [
  {
    id: 10, brand: 'Newmen', model: 'Fade SP Front 15x110 Boost 6-bolt/CL', spoke_type: 'Straightpull', hole_counts: [28],
    flange_dist_left_mm: 29, flange_dist_right_mm: 34, flange_diam_left_mm: 38.5, flange_diam_right_mm: 38.5,
  },
  {
    id: 11, brand: 'DT Swiss', model: '350 Front', spoke_type: 'J-Bend', hole_counts: null,
    flange_dist_left_mm: 23, flange_dist_right_mm: 33, flange_diam_left_mm: 58, flange_diam_right_mm: 58,
  },
];

describe('component library', () => {
  it('lists brands and labels components', () => {
    assert.deepEqual(comp.brands(rims), ['Acme', 'DT Swiss']);
    assert.equal(comp.rimLabel(rims[0]), 'F 572 29″ — ERD 602, 28/32H');
    assert.equal(comp.hubLabel(hubs[0]), 'Fade SP Front 15x110 Boost 6-bolt/CL — Straightpull, 28H');
    assert.equal(comp.hubLabel(hubs[1]), '350 Front — J-Bend, holes n/a');
  });

  it('renders cascading brand → model pickers', () => {
    const brand = container();
    const model = container();
    comp.renderPicker(brand, model, rims, { selectedId: 1, label: comp.rimLabel });
    assert.equal(brand.value, 'DT Swiss');
    assert.equal(model.value, '1');
    assert.equal(model.children.length, 3, 'placeholder + two DT Swiss rims');
    assert.equal(model.disabled, false);

    comp.renderPicker(brand, model, rims, { selectedId: 1, brand: 'Acme', label: comp.rimLabel });
    assert.equal(brand.value, 'Acme');
    assert.equal(model.value, '', 'the selected rim belongs to another brand');
    assert.deepEqual(model.children.map((o) => o.value), ['', '3']);

    comp.renderPicker(brand, model, rims, { label: comp.rimLabel });
    assert.equal(brand.value, '');
    assert.equal(model.disabled, true);
    assert.equal(model.children[0].textContent, '— pick a brand —');
  });

  it('autofills profile fields and adapts the spoke count to the drilling', () => {
    assert.deepEqual(comp.rimPatch(rims[0], { spokes_count: 32 }), {
      rim_component_id: 1, erd_mm: 602, rim_asymmetry_offset_mm: 0, max_tension_kgf: 122,
    });
    assert.equal(comp.rimPatch(rims[2], { spokes_count: 32 }).spokes_count, 36);
    assert.deepEqual(comp.hubPatch(hubs[0], { spokes_count: 32 }), {
      hub_component_id: 10, flange_dist_left_mm: 29, flange_dist_right_mm: 34,
      flange_diam_left_mm: 38.5, flange_diam_right_mm: 38.5, spoke_type: 'Straightpull', spokes_count: 28,
    });
    assert.ok(!('spokes_count' in comp.hubPatch(hubs[1], { spokes_count: 32 })), 'unknown drilling keeps the count');
  });

  it('warns about drillings that do not match', () => {
    assert.deepEqual(comp.compatibilityNotes({ spokes_count: 28 }, rims[0], hubs[0]), []);
    assert.deepEqual(comp.compatibilityNotes({ spokes_count: 32 }, undefined, undefined), []);
    assert.deepEqual(comp.compatibilityNotes({ spokes_count: 32 }, rims[0], hubs[0]), [
      'Hub Newmen Fade SP Front 15x110 Boost 6-bolt/CL has no 32H version (28H).',
    ]);
    assert.deepEqual(comp.compatibilityNotes({ spokes_count: 28 }, rims[2], hubs[0]), [
      'Rim Acme Old has no 28H version (36H).',
      'The rim and hub share no drilling.',
    ]);
    assert.deepEqual(comp.compatibilityNotes({ spokes_count: 32 }, rims[0], hubs[1]), []);
    const list = container();
    comp.renderNotes(list, ['a', 'b']);
    assert.equal(list.textContent, 'ab');
  });

  it('renders the geometry preview', () => {
    const c = container();
    comp.renderGeometry(c, null);
    assert.equal(c.children.length, 0);
    comp.renderGeometry(c, { error: 'erd_mm: out of range' });
    assert.equal(c.textContent, 'Geometry: erd_mm: out of range');
    const side = (len, off, deg, dt) => ({ spoke_length_mm: len, flange_offset_mm: off, bracing_angle_deg: deg, tension_per_turn_kgf: dt });
    comp.renderGeometry(c, { left: side(289.04, 29, 5.76, 9.9), right: side(290.1, 34, 6.75, 9.8), tension_ratio: 1.1712 });
    const rows = c.querySelector('tbody').children.map((tr) => tr.textContent);
    assert.deepEqual(rows, [
      'Spoke length, mm289.0290.1', 'Flange offset, mm29.034.0', 'Bracing angle, °5.766.75',
      'ΔT per turn, kgf9.99.8', 'Tension ratio L/R1.17',
    ]);
  });
});

describe('grid with a tensiometer', () => {
  const conv = () => tm.createConverter(catalog, wheel);
  function setup(rows) {
    const table = container();
    const tbody = container();
    table.append(tbody);
    renderGrid(tbody, rows, conv());
    const state = { rows, updates: [] };
    bindGrid(table, {
      getRows: () => state.rows,
      getConverter: conv,
      onRows: (next, rerender) => {
        state.rows = next;
        state.updates.push(rerender);
      },
      onPasteErrors: () => {},
    });
    return { table, tbody, state };
  }
  const cell = (tbody, row, col) => tbody.children[row].querySelectorAll('[data-col]').find((e) => e.dataset.col === String(col));
  const preview = (tbody, row) => tbody.children[row].querySelector('span.kgf').textContent;

  it('shows readings with a kgf preview and flags unconverted ones', () => {
    const rows = defaultRows(3);
    Object.assign(rows[0], { tension_reading: 2.8, tension_kgf: 102 });
    Object.assign(rows[1], { tension_reading: 9, tension_kgf: null });
    const { table, tbody } = setup(rows);
    assert.equal(cell(tbody, 0, 2).value, '2.8');
    assert.equal(cell(tbody, 0, 2).getAttribute('aria-label'), 'tension_reading spoke 1');
    assert.equal(preview(tbody, 0), '≈ 102 kgf');
    assert.equal(preview(tbody, 1), 'outside 2.6–3');
    assert.ok(cell(tbody, 1, 2).classList.contains('invalid'));
    assert.equal(preview(tbody, 2), '');
    assert.ok(hasInvalidCells(table));
  });

  it('converts typed readings and re-converts on side changes', () => {
    const { table, tbody, state } = setup(defaultRows(2));
    const input = cell(tbody, 0, 2);
    input.value = '2,9';
    input.dispatch('input');
    assert.equal(state.rows[0].tension_reading, 2.9);
    assert.ok(Math.abs(state.rows[0].tension_kgf - 112) < 1e-9);
    assert.equal(preview(tbody, 0), '≈ 112 kgf');
    assert.deepEqual(state.updates, [false]);

    const side = cell(tbody, 0, 1);
    side.value = 'R';
    side.dispatch('change');
    assert.ok(Math.abs(state.rows[0].tension_kgf - 110) < 1e-9, 'the R gauge is the plain 1.8 table');
    assert.equal(tbody.children[0].className, 'side-r');
    assert.equal(preview(tbody, 0), '≈ 110 kgf');

    input.value = '3.5';
    input.dispatch('input');
    assert.ok(hasInvalidCells(table));
    assert.equal(preview(tbody, 0), 'outside 2.6–3');
    assert.equal(state.rows[0].tension_reading, 2.9, 'invalid readings are not stored');

    input.value = 'abc';
    input.dispatch('input');
    assert.equal(preview(tbody, 0), '');

    input.value = '';
    input.dispatch('input');
    assert.ok(!hasInvalidCells(table));
    assert.equal(state.rows[0].tension_reading, null);
    assert.equal(state.rows[0].tension_kgf, null);

    const lateral = cell(tbody, 1, 3);
    cell(tbody, 1, 2).value = '2.8';
    cell(tbody, 1, 2).dispatch('input');
    lateral.value = 'x';
    lateral.dispatch('input');
    assert.ok(lateral.classList.contains('invalid'));
    assert.equal(preview(tbody, 1), '≈ 100 kgf', 'other columns leave the preview alone');
  });

  it('pastes readings into the reading column', () => {
    const { tbody, state } = setup(defaultRows(2));
    cell(tbody, 0, 2).dispatch('paste', { clipboardData: { getData: () => '2.8\n2.8' } });
    assert.deepEqual(state.rows.map((r) => [r.tension_reading, r.tension_kgf]), [[2.8, 102], [2.8, 100]]);
    assert.deepEqual(state.updates, [true]);
  });
});

describe('results tension warnings', () => {
  const base = {
    orientation: 'o', max_tension_kgf: 122,
    before: { max_lateral_mm: 0, max_radial_mm: 0, tension_spread_left_kgf: 0, tension_spread_right_kgf: 0 },
    after: { max_lateral_mm: 0, max_radial_mm: 0, tension_spread_left_kgf: 0, tension_spread_right_kgf: 0 },
  };

  it('flags actions and summarises spokes approaching the rim limit', () => {
    const c = container();
    renderResults(c, {
      ...base,
      actions: [
        { direction: 'tighten', instruction: 'Spoke L3: +1/4 turn (Tighten)', predicted_tension_kgf: 118.4, tension_warning: true },
        { direction: 'loosen', instruction: 'Spoke R4: -1/4 turn (Loosen)', predicted_tension_kgf: 100, tension_warning: false },
      ],
      predicted: [{ tension_warning: true }, { tension_warning: false }],
    });
    const items = c.querySelectorAll('li');
    assert.deepEqual(items.map((li) => li.className), ['tighten warn', 'loosen']);
    assert.equal(items[0].querySelector('span.warning').textContent, ' ⚠️ Approaching Max Rim Tension: 118 kgf of 122 kgf');
    assert.equal(items[1].querySelector('span.warning'), null);
    assert.equal(c.querySelector('p.tension-alert').textContent,
      '⚠️ Approaching Max Rim Tension: 1 spoke(s) predicted at ≥ 95% of the 122 kgf rim limit.');
  });

  it('omits the alert without warnings or predictions', () => {
    const c = container();
    renderResults(c, { ...base, actions: [] });
    assert.equal(c.querySelector('p.tension-alert'), null);
    assert.equal(c.children.length, 3);
  });
});
