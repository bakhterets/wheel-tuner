// Pure helpers for the measurement grid rows. A row mirrors the API
// measurement but keeps null for cells the user left empty.

export const SPOKE_COUNTS = Object.freeze([20, 24, 28, 32, 36]);

/**
 * Side of a spoke on an alternately laced wheel: odd spokes sit on the flange
 * of spoke #1 (the left / rotor flange by default), even ones on the other.
 */
export function defaultSide(index, first = 'L') {
  const other = first === 'L' ? 'R' : 'L';
  return index % 2 === 1 ? first : other;
}

export function emptyRow(index, first = 'L') {
  return {
    spoke_index: index,
    side: defaultSide(index, first),
    tension_kgf: null,
    tension_reading: null,
    lateral_dev_mm: null,
    lat_peak_between_next: false,
    radial_dev_mm: null,
    rad_peak_between_next: false,
  };
}

export function defaultRows(n) {
  return Array.from({ length: n }, (_, i) => emptyRow(i + 1));
}

/** Returns n rows, keeping the data of existing rows with index <= n; new rows continue the alternation. */
export function resizeRows(rows, n) {
  const first = firstSide(rows);
  return Array.from({ length: n }, (_, i) => (rows[i] ? { ...rows[i] } : emptyRow(i + 1, first)));
}

/** Flange of spoke #1 ('L' for an empty grid). */
export function firstSide(rows) {
  return rows[0]?.side ?? 'L';
}

/** Re-assigns alternating sides with spoke #1 on first; measured values stay with their spoke number. */
export function assignSides(rows, first) {
  return rows.map((r) => ({ ...r, side: defaultSide(r.spoke_index, first) }));
}

/** Grid row indexes (positions in rows) of the spokes on one side, in spoke order. */
export function sideIndexes(rows, side) {
  return rows.flatMap((r, i) => (r.side === side ? [i] : []));
}

/** Builds grid rows from stored API measurements. */
export function rowsFromMeasurements(ms, n) {
  const rows = defaultRows(n);
  for (const m of ms) {
    if (m.spoke_index >= 1 && m.spoke_index <= n) {
      rows[m.spoke_index - 1] = {
        spoke_index: m.spoke_index,
        side: m.side,
        tension_kgf: m.tension_kgf || null,
        tension_reading: m.tension_reading ?? null,
        lateral_dev_mm: m.lateral_dev_mm,
        lat_peak_between_next: Boolean(m.lat_peak_between_next),
        radial_dev_mm: m.radial_dev_mm,
        rad_peak_between_next: Boolean(m.rad_peak_between_next),
      };
    }
  }
  return rows;
}

/**
 * Converts grid rows to the API payload: empty deviations are 0, empty tension
 * is 0 (unmeasured). Readings are sent as-is; the server converts them to kgf.
 */
export function toMeasurements(rows) {
  return rows.map((r) => ({
    spoke_index: r.spoke_index,
    side: r.side,
    tension_kgf: r.tension_kgf ?? 0,
    lateral_dev_mm: r.lateral_dev_mm ?? 0,
    lat_peak_between_next: r.lat_peak_between_next,
    radial_dev_mm: r.radial_dev_mm ?? 0,
    rad_peak_between_next: r.rad_peak_between_next,
    ...(r.tension_reading === null || r.tension_reading === undefined ? {} : { tension_reading: r.tension_reading }),
  }));
}
