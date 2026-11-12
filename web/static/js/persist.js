// Persists the working state (profile form, grid, last result) in
// localStorage so a page reload keeps the measurements and the calculation.
import { emptyRow } from './measurements.js';

export const STORAGE_KEY = 'wheel-tuner:state';
const VERSION = 1;

const isObject = (v) => typeof v === 'object' && v !== null && !Array.isArray(v);
const isSide = (s) => s === 'L' || s === 'R';
const num = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : null);

/** Rebuilds a stored grid row field by field, so stale or foreign keys never reach the grid. */
function restoreRow(r) {
  const row = emptyRow(r.spoke_index, r.side);
  row.side = r.side;
  for (const key of ['tension_kgf', 'tension_reading', 'lateral_dev_mm', 'radial_dev_mm']) {
    row[key] = num(r[key]);
  }
  row.lat_peak_between_next = r.lat_peak_between_next === true;
  row.rad_peak_between_next = r.rad_peak_between_next === true;
  return row;
}

const validRow = (r, i) => isObject(r) && r.spoke_index === i + 1 && isSide(r.side);

/**
 * Saves the state; failures (quota, storage disabled) are reported, not thrown.
 * @param {Storage|null} storage
 * @param {{wheel: object, rows: object[], result: object|null}} state
 * @returns {boolean} whether the state was written
 */
export function saveState(storage, { wheel, rows, result }) {
  if (!storage) {
    return false;
  }
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify({ version: VERSION, wheel, rows, result }));
    return true;
  } catch {
    return false;
  }
}

/**
 * Loads the saved state, or null when there is none or it is unusable.
 * @param {Storage|null} storage
 * @returns {{wheel: object, rows: object[], result: object|null}|null}
 */
export function loadState(storage) {
  let saved;
  try {
    saved = JSON.parse(storage?.getItem(STORAGE_KEY) ?? 'null');
  } catch {
    return null;
  }
  if (!isObject(saved) || saved.version !== VERSION || !isObject(saved.wheel)
    || !Array.isArray(saved.rows) || !saved.rows.every(validRow)) {
    return null;
  }
  return {
    wheel: saved.wheel,
    rows: saved.rows.map(restoreRow),
    result: isObject(saved.result) && Array.isArray(saved.result.actions) ? saved.result : null,
  };
}

