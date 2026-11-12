// Measurement input grid with spreadsheet paste support.
import { h } from './dom.js';
import { applyPaste, COLUMNS, parseNumber, parseTable } from './paste.js';
import { applyReadings, readingPreview, tensionPatch } from './tensiometer.js';

const TENSION_COL = COLUMNS.findIndex((c) => c.key === 'tension_kgf');

/** Grid columns for the active tension mode: with a tensiometer the tension cell holds the reading. */
export function columnsFor(conv) {
  return conv ? COLUMNS.map((c, i) => (i === TENSION_COL ? { ...c, key: 'tension_reading' } : c)) : COLUMNS;
}

const isUnconverted = (row, conv) => Boolean(conv) && (row.tension_reading ?? null) !== null && row.tension_kgf === null;

function cellInput(row, rowIdx, col, colIdx, conv) {
  const data = { 'data-row': rowIdx, 'data-col': colIdx, 'aria-label': `${col.key} spoke ${row.spoke_index}` };
  switch (col.type) {
    case 'readonly':
      return h('th', { scope: 'row', tabindex: 0, ...data, text: String(row.spoke_index) });
    case 'side':
      return h('td', {}, h('select', { ...data, name: col.key }, ['L', 'R'].map((v) =>
        h('option', { value: v, selected: row.side === v, text: v }))));
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

/** Re-renders the grid body for the given rows; conv switches the tension column to readings. */
export function renderGrid(tbody, rows, conv = null) {
  const columns = columnsFor(conv);
  tbody.replaceChildren(...rows.map((row, i) =>
    h('tr', { class: row.side === 'L' ? 'side-l' : 'side-r' }, columns.map((col, c) => cellInput(row, i, col, c, conv)))));
}

function markInvalid(el, bad) {
  el.classList.toggle('invalid', bad);
  el.setAttribute('aria-invalid', String(bad));
}

function cellPatch(el, row, col, colIdx, conv) {
  switch (col.type) {
    case 'bool':
      return { [col.key]: el.checked };
    case 'side':
      return { side: el.value };
    default: {
      const value = parseNumber(el.value);
      const patch = colIdx === TENSION_COL ? tensionPatch(row, value, conv) : (Number.isNaN(value) ? null : { [col.key]: value });
      markInvalid(el, patch === null);
      return patch;
    }
  }
}

/**
 * Wires grid events. Typing updates a single row without re-rendering, so the
 * caret is kept; multi-cell paste replaces all rows via onRows.
 * @param {HTMLTableElement} table
 * @param {{getRows: () => object[], onRows: (rows: object[], rerender: boolean) => void,
 *   onPasteErrors: (errors: object[]) => void, getConverter?: () => object|null}} handlers
 */
export function bindGrid(table, { getRows, onRows, onPasteErrors, getConverter = () => null }) {
  const update = (ev) => {
    const el = ev.target;
    const rowIdx = Number(el.dataset.row);
    const colIdx = Number(el.dataset.col);
    const col = COLUMNS[colIdx];
    if (!col || Number.isNaN(rowIdx)) {
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
    // A side change switches the gauge, so the reading is converted again.
    [rows[rowIdx]] = applyReadings([{ ...rows[rowIdx], ...patch }], conv);
    tr.className = rows[rowIdx].side === 'L' ? 'side-l' : 'side-r';
    if (preview) {
      preview.textContent = readingPreview(rows[rowIdx], conv);
      markInvalid(tr.querySelector('input.tension'), isUnconverted(rows[rowIdx], conv));
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
    const startRow = anchor ? Number(anchor.dataset.row) : 0;
    const startCol = anchor ? Number(anchor.dataset.col) : 0;
    const conv = getConverter();
    const { rows, errors } = applyPaste(getRows(), startRow, startCol, parseTable(text), columnsFor(conv));
    onRows(applyReadings(rows, conv), true);
    onPasteErrors(errors);
  });
}

/** True when some cell currently holds an unparsable value. */
export function hasInvalidCells(table) {
  return table.querySelector('.invalid') !== null;
}
