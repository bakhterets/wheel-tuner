// Pure parsing of spreadsheet clipboard data (Excel, LibreOffice, Google
// Sheets copy cells as tab-separated text).

/** Grid columns in display order; the index column is read-only. */
export const COLUMNS = Object.freeze([
  { key: 'spoke_index', type: 'readonly' },
  { key: 'side', type: 'side' },
  { key: 'tension_kgf', type: 'number' },
  { key: 'lateral_dev_mm', type: 'number' },
  { key: 'lat_peak_between_next', type: 'bool' },
  { key: 'radial_dev_mm', type: 'number' },
  { key: 'rad_peak_between_next', type: 'bool' },
]);

/** Splits clipboard text into a matrix of trimmed cells, dropping trailing blank lines. */
export function parseTable(text) {
  const lines = String(text).replace(/\r\n?/g, '\n').split('\n');
  while (lines.length > 0 && lines[lines.length - 1].trim() === '') {
    lines.pop();
  }
  return lines.map((line) => line.split('\t').map((c) => c.trim()));
}

/** Parses a number accepting a decimal comma; '' yields null, garbage yields NaN. */
export function parseNumber(s) {
  const v = String(s).replace(/\s+/g, '').replace(',', '.');
  if (v === '') {
    return null;
  }
  return /^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/.test(v) ? Number(v) : Number.NaN;
}

const SIDE_ALIASES = new Map([
  ['l', 'L'], ['left', 'L'], ['nds', 'L'], ['rotor', 'L'],
  ['r', 'R'], ['right', 'R'], ['ds', 'R'], ['drive', 'R'],
]);

/** Parses a side label; returns 'L', 'R' or null. */
export function parseSide(s) {
  return SIDE_ALIASES.get(String(s).trim().toLowerCase()) ?? null;
}

const TRUE_WORDS = new Set(['1', 'x', 'y', 'yes', 'true', '✓', '✔', 'да', '+']);
const FALSE_WORDS = new Set(['', '0', 'n', 'no', 'false', '-', 'нет']);

/** Parses a checkbox cell; returns true, false or null when unrecognised. */
export function parseBool(s) {
  const v = String(s).trim().toLowerCase();
  if (TRUE_WORDS.has(v)) {
    return true;
  }
  return FALSE_WORDS.has(v) ? false : null;
}

function parseCell(type, raw) {
  switch (type) {
    case 'side':
      return parseSide(raw);
    case 'number': {
      const n = parseNumber(raw);
      return Number.isNaN(n) ? undefined : n;
    }
    default: // 'bool'
      return parseBool(raw);
  }
}

function isHeader(cells) {
  return cells.length > 0 && cells.every((c) => c !== '' && Number.isNaN(parseNumber(c)) && parseSide(c) === null && parseBool(c) === null);
}

/**
 * Applies a pasted matrix onto rows starting at (startRow, startCol).
 * Cells that cannot be parsed are left unchanged and reported in errors.
 * A header line (all non-data words) is skipped. columns lets the caller
 * remap keys, e.g. the tension column to tension_reading.
 * @returns {{rows: object[], errors: {row: number, column: string, value: string}[]}}
 */
export function applyPaste(rows, startRow, startCol, matrix, columns = COLUMNS) {
  const out = rows.map((r) => ({ ...r }));
  const errors = [];
  const data = matrix.length > 0 && isHeader(matrix[0]) ? matrix.slice(1) : matrix;
  data.forEach((cells, dr) => {
    const row = out[startRow + dr];
    if (!row) {
      return;
    }
    cells.forEach((raw, dc) => {
      const col = columns[startCol + dc];
      if (!col || col.type === 'readonly') {
        return;
      }
      const v = parseCell(col.type, raw);
      if (v === undefined || (v === null && col.type !== 'number')) {
        errors.push({ row: row.spoke_index, column: col.key, value: raw });
        return;
      }
      row[col.key] = v;
    });
  });
  return { rows: out, errors };
}
