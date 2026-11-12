import assert from 'node:assert/strict';
import { before, describe, it } from 'node:test';

import { installFakeDom } from './fakedom.mjs';
import { defaultRows } from '../static/js/measurements.js';

const { container } = installFakeDom();
let h, s, renderWheel, renderRadar, renderResults, renderGrid, bindGrid, hasInvalidCells;

before(async () => {
  ({ h, s } = await import('../static/js/dom.js'));
  ({ renderWheel } = await import('../static/js/viz/wheel.js'));
  ({ renderRadar } = await import('../static/js/viz/radar.js'));
  ({ renderResults } = await import('../static/js/results.js'));
  ({ renderGrid, bindGrid, hasInvalidCells } = await import('../static/js/grid.js'));
});

const wheel = { spokes_count: 8, counting_direction: 'CCW', view_point: 'Drive Side', start_reference: 'Valve Hole' };

function sampleRows() {
  const rows = defaultRows(8);
  rows.forEach((r) => { r.tension_kgf = 100; });
  rows[0].tension_kgf = 140; // L1 +40 % vs L mean of 110 → outlier
  rows[3].lateral_dev_mm = 0.9; // wobble
  rows[5].radial_dev_mm = -0.5; // hop (flat spot)
  rows[5].rad_peak_between_next = true;
  return rows;
}

const result = {
  orientation: 'Start at Valve Hole. View from Drive Side. Count CCW.',
  actions: [
    { spoke_index: 3, side: 'L', direction: 'tighten', instruction: 'Spoke L3: +1/2 turn (Tighten)' },
    { spoke_index: 4, side: 'R', direction: 'loosen', instruction: 'Spoke R4: -1/4 turn (Loosen)' },
  ],
  before: { max_lateral_mm: 0.9, max_radial_mm: 0.5, tension_spread_left_kgf: 17, tension_spread_right_kgf: 0 },
  after: { max_lateral_mm: 0.2, max_radial_mm: 0.1, tension_spread_left_kgf: 9, tension_spread_right_kgf: 1 },
  predicted: Array.from({ length: 8 }, (_, i) => ({ spoke_index: i + 1, tension_kgf: 100 + i })),
};

describe('dom helpers', () => {
  it('builds elements with attributes, classes, text, listeners and nested children', () => {
    let clicked = 0;
    const el = h('button', { class: ['a', false, 'b'], disabled: true, hidden: false, title: null, onclick: () => clicked++ }, ['x', [h('b', { text: 'y' })], null]);
    assert.equal(el.className, 'a b');
    assert.equal(el.getAttribute('disabled'), '');
    assert.equal(el.getAttribute('hidden'), null);
    assert.equal(el.textContent, 'xy');
    el.dispatch('click');
    assert.equal(clicked, 1);
    assert.equal(s('circle', { r: 2 }, 'z').namespaceURI, 'http://www.w3.org/2000/svg');
    assert.equal(h('i').children.length, 0);
  });
});

describe('renderWheel', () => {
  it('colour-codes adjusted spokes and marks near/far flanges', () => {
    const c = container();
    renderWheel(c, { wheel, rows: sampleRows(), result });
    const spokes = c.querySelectorAll('line');
    assert.equal(spokes.length, 8);
    assert.ok(spokes[2].classList.contains('tighten'));
    assert.ok(spokes[3].classList.contains('loosen'));
    assert.ok(spokes[0].classList.contains('idle'));
    assert.ok(spokes[0].classList.contains('far'), 'L spokes are on the far side when viewing from the drive side');
    assert.ok(spokes[1].classList.contains('near'));
    assert.equal(spokes[2].textContent, 'Spoke L3: +1/2 turn (Tighten)');
    assert.equal(spokes[0].textContent, 'Spoke L1');
    assert.equal(c.querySelectorAll('text.tighten').length, 1);
  });

  it('highlights wobble and hop zones and draws the orientation', () => {
    const c = container();
    renderWheel(c, { wheel, rows: sampleRows(), result: null });
    const wobble = c.querySelector('g.lateral').children;
    const hop = c.querySelector('g.radial').children;
    assert.equal(wobble.length, 1);
    assert.equal(wobble[0].className, 'pos');
    assert.match(wobble[0].textContent, /Wobble at 4: 0.90 mm/);
    assert.equal(hop.length, 1);
    assert.equal(hop[0].className, 'neg');
    assert.match(hop[0].textContent, /Hop at 6–7: -0.50 mm/);
    assert.equal(c.querySelector('text.reference').textContent, 'Valve Hole');
    assert.match(c.querySelector('text.direction').textContent, /↺ CCW/);
    assert.match(c.querySelector('svg').getAttribute('aria-label'), /8 spokes/);
  });

  it('draws CW direction', () => {
    const c = container();
    renderWheel(c, { wheel: { ...wheel, counting_direction: 'CW', view_point: 'Rotor Side' }, rows: defaultRows(8), result: null });
    assert.match(c.querySelector('text.direction').textContent, /↻ CW/);
    assert.ok(c.querySelectorAll('line')[0].classList.contains('near'));
  });
});

describe('renderRadar', () => {
  it('plots both sides relative to their averages and flags outliers', () => {
    const c = container();
    renderRadar(c, { wheel, rows: sampleRows(), result: null });
    assert.equal(c.querySelectorAll('polygon').length, 2);
    assert.equal(c.querySelectorAll('circle.dot').length, 8);
    const outliers = c.querySelectorAll('circle.outlier');
    assert.equal(outliers.length, 1);
    assert.match(outliers[0].textContent, /L1: \+27\.3% vs L average/);
    assert.equal(c.querySelector('text.empty'), null);
  });

  it('overlays the predicted tensions of measured spokes only', () => {
    const rows = sampleRows();
    rows[7].tension_kgf = null; // R8 unmeasured: no predicted point
    const c = container();
    renderRadar(c, { wheel, rows, result });
    assert.equal(c.querySelectorAll('polygon').length, 4);
    const predicted = c.querySelectorAll('polygon.predicted');
    assert.equal(predicted.length, 2);
    assert.equal(predicted[0].getAttribute('points').split(' ').length, 3, 'DS: R2, R4, R6');
    assert.equal(c.querySelectorAll('circle.dot')[0].textContent, 'R2: +0.0% vs R average');
  });

  it('shows a hint without data and uses a polyline for sparse sides', () => {
    const empty = container();
    renderRadar(empty, { wheel, rows: defaultRows(8), result: null });
    assert.equal(empty.querySelector('text.empty').textContent, 'Enter tensions to see the chart');

    const rows = defaultRows(8);
    rows[1].tension_kgf = 100;
    rows[3].tension_kgf = 110;
    const sparse = container();
    renderRadar(sparse, { wheel, rows, result: { ...result, predicted: [] } });
    assert.equal(sparse.querySelectorAll('polyline').length, 1);
    assert.equal(sparse.querySelectorAll('polygon').length, 0);
  });
});

describe('renderResults', () => {
  it('shows a placeholder before solving', () => {
    const c = container();
    renderResults(c, null);
    assert.match(c.textContent, /press “Solve”/);
  });

  it('lists instructions under the orientation header with metrics', () => {
    const c = container();
    renderResults(c, result);
    assert.equal(c.querySelector('p.orientation').textContent, result.orientation);
    const items = c.querySelectorAll('li');
    assert.deepEqual(items.map((li) => li.className), ['tighten', 'loosen']);
    assert.match(items[0].textContent, /↑Spoke L3: \+1\/2 turn \(Tighten\)/);
    assert.match(items[1].textContent, /↓Spoke R4/);
    const firstRow = c.querySelector('tbody').children[0].textContent;
    assert.equal(firstRow, 'Max lateral (wobble)0.90 mm0.20 mm−78%');
  });

  it('reports a true wheel', () => {
    const c = container();
    renderResults(c, { ...result, actions: [] });
    assert.match(c.querySelector('p.ok').textContent, /No adjustment needed/);
  });
});

describe('grid', () => {
  function setup(rows) {
    const table = container();
    const tbody = container();
    table.append(tbody);
    renderGrid(tbody, rows);
    const state = { rows, updates: [], pasteErrors: null };
    bindGrid(table, {
      getRows: () => state.rows,
      onRows: (next, rerender) => {
        state.rows = next;
        state.updates.push(rerender);
      },
      onPasteErrors: (errors) => { state.pasteErrors = errors; },
    });
    return { table, tbody, state };
  }
  const cell = (tbody, row, col) => tbody.children[row].querySelectorAll('[data-col]').find((e) => e.dataset.col === String(col));

  it('renders one row per spoke with side styling and current values', () => {
    const rows = defaultRows(3);
    rows[1].lateral_dev_mm = 0.25;
    rows[1].lat_peak_between_next = true;
    const { tbody } = setup(rows);
    assert.equal(tbody.children.length, 3);
    assert.deepEqual(tbody.children.map((tr) => tr.className), ['side-l', 'side-r', 'side-l']);
    assert.equal(cell(tbody, 1, 0).textContent, '2');
    assert.equal(cell(tbody, 1, 3).value, '0.25');
    assert.equal(cell(tbody, 1, 2).value, '');
    assert.equal(cell(tbody, 1, 4).checked, true);
    assert.equal(cell(tbody, 1, 1).children.find((o) => o.getAttribute('selected') !== null).value, 'R');
  });

  it('updates rows on typing without re-rendering and flags invalid numbers', () => {
    const { table, tbody, state } = setup(defaultRows(2));
    const input = cell(tbody, 0, 3);
    input.value = '-0,3';
    input.dispatch('input');
    assert.equal(state.rows[0].lateral_dev_mm, -0.3);
    assert.deepEqual(state.updates, [false]);

    const tension = cell(tbody, 0, 2);
    tension.value = '-5';
    tension.dispatch('input');
    assert.ok(hasInvalidCells(table));
    assert.equal(tension.getAttribute('aria-invalid'), 'true');
    assert.equal(state.rows[0].tension_kgf, null);
    tension.value = '120';
    tension.dispatch('input');
    assert.ok(!hasInvalidCells(table));
    assert.equal(state.rows[0].tension_kgf, 120);

    const peak = cell(tbody, 1, 6);
    peak.checked = true;
    peak.dispatch('change');
    assert.equal(state.rows[1].rad_peak_between_next, true);

    const side = cell(tbody, 1, 1);
    side.value = 'L';
    side.dispatch('change');
    assert.equal(state.rows[1].side, 'L');
    assert.equal(tbody.children[1].className, 'side-l');
    side.value = 'R';
    side.dispatch('change');
    assert.equal(tbody.children[1].className, 'side-r');

    const before = state.updates.length;
    tbody.dispatch('input'); // event without a cell target
    assert.equal(state.updates.length, before);
  });

  it('pastes spreadsheet blocks from the focused cell', () => {
    const { tbody, state } = setup(defaultRows(3));
    const clip = (text) => ({ clipboardData: { getData: () => text } });
    const ev = cell(tbody, 1, 2).dispatch('paste', clip('101\t0.4\n102\tbad'));
    assert.ok(ev.defaultPrevented);
    assert.equal(state.rows[1].tension_kgf, 101);
    assert.equal(state.rows[1].lateral_dev_mm, 0.4);
    assert.equal(state.rows[2].tension_kgf, 102);
    assert.deepEqual(state.updates, [true]);
    assert.equal(state.pasteErrors.length, 1);

    const single = cell(tbody, 0, 2).dispatch('paste', clip('99'));
    assert.ok(!single.defaultPrevented, 'single values use the native paste');

    tbody.dispatch('paste', clip('R\nL'));
    assert.equal(state.rows[0].spoke_index, 1, 'anchor-less paste starts at the index column');
    tbody.dispatch('paste', {});
    assert.equal(state.updates.length, 2);
  });
});
