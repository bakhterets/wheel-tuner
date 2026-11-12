// Measurement input: one table per flange (left = rotor side, right) with
// spreadsheet paste support. Rows keep their global spoke number; a table
// shows only the spokes of its side.
import { h } from './dom.js';
import { sideIndexes } from './measurements.js';
import { applyPaste, COLUMNS, parseNumber, parseTable } from './paste.js';
import { applyReadings, readingPreview, tensionPatch } from './tensiometer.js';

/** Columns of a side table: the side is implied by the table. */
export const GRID_COLUMNS = Object.freeze(COLUMNS.filter((c) => c.type !== 'side'));

const TENSION_COL = GRID_COLUMNS.findIndex((c) => c.key === 'tension_kgf');

/** Grid columns for the active tension mode: with a tensiometer the tension cell holds the reading. */
export function columnsFor(conv) {
  return conv ? GRID_COLUMNS.map((c, i) => (i === TENSION_COL ? { ...c, key: 'tension_reading' } : c)) : GRID_COLUMNS;
}

const isUnconverted = (row, conv) => Boolean(conv) && (row.tension_reading ?? null) !== null && row.tension_kgf === null;

function cellInput(row, rowIdx, col, colIdx, conv) {
  const data = { 'data-row': rowIdx, 'data-col': colIdx, 'aria-label': `${col.key} spoke ${row.side}${row.spoke_index}` };
  switch (col.type) {
    case 'readonly':
      return h('th', { scope: 'row', tabindex: 0, ...data, text: String(row.spoke_index) });
    case 'bool':
      return h('td', { class: 'center' }, h('input', { ...data, type: 'checkbox', checked: row[col.key] }));
    default: {
      const tension = colIdx === TENSION_COL;
      const bad = tension && isUnconverted(row, conv);
      const input = h('input', {
        ...data, type: 'text', inputmode: 'decimal', autocomplete: 'off',
        class: [tension && 'tension', bad && 'invalid'], 'aria-invalid': bad ? 'true' : null,
        value: row[col.key] === null ? '' : String(row[col.key]),
      });
      return tension && conv
        ? h('td', {}, [input, h('span', { class: 'kgf', text: readingPreview(row, conv) })])
        : h('td', {}, input);
    }
  }
}

/**
 * Re-renders a side table body with the rows of that side; data-row holds
 * the position in rows. conv switches the tension column to readings.
 */
export function renderGrid(tbody, rows, side, conv = null) {
  const columns = columnsFor(conv);
  const cls = side === 'L' ? 'side-l' : 'side-r';
  tbody.replaceChildren(...sideIndexes(rows, side).map((i) =>
    h('tr', { class: cls }, columns.map((col, c) => cellInput(rows[i], i, col, c, conv)))));
}

function markInvalid(el, bad) {
  el.classList.toggle('invalid', bad);
  el.setAttribute('aria-invalid', String(bad));
}

function cellPatch(el, row, col, colIdx, conv) {
  if (col.type === 'bool') {
    return { [col.key]: el.checked };
  }
  const value = parseNumber(el.value);
  const patch = colIdx === TENSION_COL ? tensionPatch(row, value, conv) : (Number.isNaN(value) ? null : { [col.key]: value });
  markInvalid(el, patch === null);
  return patch;
}

/**
 * Pastes a matrix into the side's rows: pasted lines go down the spokes of
 * that side (L1, L3, L5…), not the global spoke order.
 */
export function pasteSide(rows, side, anchorRow, startCol, matrix, columns) {
  const targets = sideIndexes(rows, side);
  const start = Math.max(0, targets.indexOf(anchorRow));
  const pasted = applyPaste(targets.map((i) => rows[i]), start, startCol, matrix, columns);
  const out = rows.slice();
  targets.forEach((i, k) => { out[i] = pasted.rows[k]; });
  return { rows: out, errors: pasted.errors };
}

/**
 * Wires the events of one side table. Typing updates a single row without
 * re-rendering, so the caret is kept; multi-cell paste replaces all rows via onRows.
 * @param {HTMLTableElement} table
 * @param {{side: string, getRows: () => object[], onRows: (rows: object[], rerender: boolean) => void,
 *   onPasteErrors: (errors: object[]) => void, getConverter?: () => object|null}} handlers
 */
export function bindGrid(table, { side, getRows, onRows, onPasteErrors, getConverter = () => null }) {
  const update = (ev) => {
    const el = ev.target;
    const rowIdx = Number(el.dataset.row);
    const colIdx = Number(el.dataset.col);
    const col = GRID_COLUMNS[colIdx];
    if (!col || Number.isNaN(rowIdx) || col.type === 'readonly') {
      return;
    }
    const conv = getConverter();
    const rows = getRows().slice();
    const tr = el.closest('tr');
    const preview = tr.querySelector('span.kgf');
    const patch = cellPatch(el, rows[rowIdx], col, colIdx, conv);
    if (patch === null) {
      if (preview && colIdx === TENSION_COL) {
        const reading = parseNumber(el.value);
        preview.textContent = reading > 0 ? readingPreview({ ...rows[rowIdx], tension_reading: reading, tension_kgf: null }, conv) : '';
      }
      return;
    }
    rows[rowIdx] = { ...rows[rowIdx], ...patch };
    if (preview && colIdx === TENSION_COL) {
      preview.textContent = readingPreview(rows[rowIdx], conv);
    }
    onRows(rows, false);
  };
  table.addEventListener('input', update);
  table.addEventListener('change', update);

  table.addEventListener('paste', (ev) => {
    const text = ev.clipboardData?.getData('text/plain') ?? '';
    if (!/[\t\n]/.test(text.trim())) {
      return; // single value: let the input handle it
    }
    ev.preventDefault();
    const anchor = ev.target.closest('[data-row]');
    const anchorRow = anchor ? Number(anchor.dataset.row) : -1;
    const startCol = anchor ? Number(anchor.dataset.col) : 0;
    const conv = getConverter();
    const { rows, errors } = pasteSide(getRows(), side, anchorRow, startCol, parseTable(text), columnsFor(conv));
    onRows(applyReadings(rows, conv), true);
    onPasteErrors(errors);
  });
}

/** True when some cell under root currently holds an unparsable value. */
export function hasInvalidCells(root) {
  return root.querySelector('.invalid') !== null;
}