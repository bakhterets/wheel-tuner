// Pure helpers for the measurement grid rows. A row mirrors the API
// measurement but keeps null for cells the user left empty.

export const SPOKE_COUNTS = Object.freeze([20, 24, 28, 32, 36]);

/** Default side for a spoke: odd spokes on the left flange, even on the right. */
export function defaultSide(index) {
  return index % 2 === 1 ? 'L' : 'R';
}

export function emptyRow(index) {
  return {
    spoke_index: index,
    side: defaultSide(index),
    tension_kgf: null,
    lateral_dev_mm: null,
    lat_peak_between_next: false,
    radial_dev_mm: null,
    rad_peak_between_next: false,
  };
}

export function defaultRows(n) {
  return Array.from({ length: n }, (_, i) => emptyRow(i + 1));
}

/** Returns n rows, keeping the data of existing rows with index <= n. */
export function resizeRows(rows, n) {
  return Array.from({ length: n }, (_, i) => (rows[i] ? { ...rows[i] } : emptyRow(i + 1)));
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
        lateral_dev_mm: m.lateral_dev_mm,
        lat_peak_between_next: Boolean(m.lat_peak_between_next),
        radial_dev_mm: m.radial_dev_mm,
        rad_peak_between_next: Boolean(m.rad_peak_between_next),
      };
    }
  }
  return rows;
}

/** Converts grid rows to the API payload: empty deviations are 0, empty tension is 0 (unmeasured). */
export function toMeasurements(rows) {
  return rows.map((r) => ({
    spoke_index: r.spoke_index,
    side: r.side,
    tension_kgf: r.tension_kgf ?? 0,
    lateral_dev_mm: r.lateral_dev_mm ?? 0,
    lat_peak_between_next: r.lat_peak_between_next,
    radial_dev_mm: r.radial_dev_mm ?? 0,
    rad_peak_between_next: r.rad_peak_between_next,
  }));
}
