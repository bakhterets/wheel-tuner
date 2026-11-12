// Bulk list input: fills one column of a side table from a list such as
// "[3.15,3.16,3.05]" instead of typing every cell.
import { sideIndexes } from './measurements.js';
import { parseNumber } from './paste.js';

/**
 * Splits a list into raw items. Brackets are optional; items are separated
 * by commas or whitespace. When the list contains a semicolon, semicolons
 * are the only separators, so decimal commas ("3,15; 3,16") also work.
 * An empty item ("3.15,,3.05") skips that spoke.
 */
export function parseList(text) {
  const body = String(text).trim().replace(/^\[/, '').replace(/\]$/, '').trim();
  if (body === '') {
    return [];
  }
  return body.split(body.includes(';') ? /\s*;\s*/ : /\s*,\s*|\s+/);
}

/**
 * Writes the listed values into column key of the side's rows, in spoke
 * order starting from the first spoke of that side. Nothing is changed on
 * error.
 * @returns {{rows: object[], error: string|null}}
 */
export function applyList(rows, side, key, text) {
  const items = parseList(text);
  const targets = sideIndexes(rows, side);
  if (items.length === 0) {
    return { rows, error: 'The list is empty.' };
  }
  if (items.length > targets.length) {
    return { rows, error: `${items.length} values for ${targets.length} ${side}-side spokes.` };
  }
  const values = items.map((raw) => parseNumber(raw));
  const bad = values.findIndex((v) => Number.isNaN(v) || (key.startsWith('tension') && v < 0));
  if (bad >= 0) {
    return { rows, error: `Value #${bad + 1} “${items[bad]}” is not a valid number.` };
  }
  const out = rows.slice();
  values.forEach((v, k) => {
    if (v !== null) {
      const i = targets[k];
      out[i] = { ...out[i], [key]: v, ...(key === 'tension_kgf' ? { tension_reading: null } : {}) };
    }
  });
  return { rows: out, error: null };
}
