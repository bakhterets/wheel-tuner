// Measurement input grid with spreadsheet paste support.
import { h } from './dom.js';
import { applyPaste, COLUMNS, parseNumber, parseTable } from './paste.js';

function cellInput(row, rowIdx, col, colIdx) {
  const data = { 'data-row': rowIdx, 'data-col': colIdx, 'aria-label': `${col.key} spoke ${row.spoke_index}` };
  switch (col.type) {
    case 'readonly':
      return h('th', { scope: 'row', tabindex: 0, ...data, text: String(row.spoke_index) });
    case 'side':
      return h('td', {}, h('select', { ...data, name: col.key }, ['L', 'R'].map((v) =>
        h('option', { value: v, selected: row.side === v, text: v }))));
    case 'bool':
      return h('td', { class: 'center' }, h('input', { ...data, type: 'checkbox', checked: row[col.key] }));
    default:
      return h('td', {}, h('input', {
        ...data, type: 'text', inputmode: 'decimal', autocomplete: 'off',
        value: row[col.key] === null ? '' : String(row[col.key]),
      }));
  }
}

/** Re-renders the grid body for the given rows. */
export function renderGrid(tbody, rows) {
  tbody.replaceChildren(...rows.map((row, i) =>
    h('tr', { class: row.side === 'L' ? 'side-l' : 'side-r' }, COLUMNS.map((col, c) => cellInput(row, i, col, c)))));
}

/**
 * Wires grid events. Typing updates a single row without re-rendering, so the
 * caret is kept; multi-cell paste replaces all rows via onRows.
 * @param {HTMLTableElement} table
 * @param {{getRows: () => object[], onRows: (rows: object[], rerender: boolean) => void, onPasteErrors: (errors: object[]) => void}} handlers
 */
export function bindGrid(table, { getRows, onRows, onPasteErrors }) {
  const update = (ev) => {
    const el = ev.target;
    const rowIdx = Number(el.dataset.row);
    const col = COLUMNS[Number(el.dataset.col)];
    if (!col || Number.isNaN(rowIdx)) {
      return;
    }
    let value;
    if (col.type === 'bool') {
      value = el.checked;
    } else if (col.type === 'side') {
      value = el.value;
      el.closest('tr').className = value === 'L' ? 'side-l' : 'side-r';
    } else {
      value = parseNumber(el.value);
      const bad = Number.isNaN(value) || (col.key === 'tension_kgf' && value < 0);
      el.classList.toggle('invalid', bad);
      el.setAttribute('aria-invalid', String(bad));
      if (bad) {
        return;
      }
    }
    const rows = getRows().slice();
    rows[rowIdx] = { ...rows[rowIdx], [col.key]: value };
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
    const { rows, errors } = applyPaste(getRows(), startRow, startCol, parseTable(text));
    onRows(rows, true);
    onPasteErrors(errors);
  });
}

/** True when some cell currently holds an unparsable value. */
export function hasInvalidCells(table) {
  return table.querySelector('.invalid') !== null;
}
