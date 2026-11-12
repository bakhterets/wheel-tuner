// Tensiometer support: converts dial readings to kgf with the calibration
// tables served by GET /api/tensiometers. The interpolation mirrors
// internal/tensiometer so the grid preview matches what the server stores.
import { h } from './dom.js';

/** Converts a reading by linear interpolation; null outside the calibrated range. */
export function gaugeKgf(gauge, reading) {
  const pts = gauge.points;
  if (!(reading >= pts[0].reading && reading <= pts[pts.length - 1].reading)) {
    return null;
  }
  let i = 1;
  while (reading > pts[i].reading) {
    i++;
  }
  const a = pts[i - 1];
  const b = pts[i];
  return a.kgf + ((reading - a.reading) / (b.reading - a.reading)) * (b.kgf - a.kgf);
}

/** Calibrated reading range of a gauge as "lo–hi". */
export function gaugeRange(gauge) {
  return `${gauge.points[0].reading}–${gauge.points[gauge.points.length - 1].reading}`;
}

/**
 * Builds the reading converter for a wheel profile, or null when the profile
 * records tensions directly in kgf.
 */
export function createConverter(catalog, wheel) {
  const model = catalog.find((m) => m.id === wheel.tensiometer);
  if (!model) {
    return null;
  }
  const gauge = (id) => model.gauges.find((g) => g.id === id) ?? null;
  const gauges = { L: gauge(wheel.spoke_gauge_left), R: gauge(wheel.spoke_gauge_right) };
  return {
    model,
    gauges,
    label: `${model.name} reading, ${model.reading_unit}`,
    convert: (side, reading) => (gauges[side] ? gaugeKgf(gauges[side], reading) : null),
  };
}

/** Identity of a converter: rows only need re-conversion when it changes. */
export function converterKey(wheel) {
  return wheel.tensiometer ? `${wheel.tensiometer}|${wheel.spoke_gauge_left}|${wheel.spoke_gauge_right}` : '';
}

/**
 * Re-derives tension_kgf of rows holding a reading. Without a converter the
 * readings are dropped and the last converted kgf value is kept.
 */
export function applyReadings(rows, conv) {
  return rows.map((r) => {
    if (r.tension_reading === null || r.tension_reading === undefined) {
      return r;
    }
    if (!conv) {
      return { ...r, tension_reading: null };
    }
    return { ...r, tension_kgf: conv.convert(r.side, r.tension_reading) };
  });
}

/**
 * Row patch for a typed tension cell (a parsed number, null for empty or NaN),
 * or null when the value is invalid.
 */
export function tensionPatch(row, value, conv) {
  if (Number.isNaN(value) || value < 0) {
    return null;
  }
  if (!conv) {
    return { tension_kgf: value, tension_reading: null };
  }
  if (value === null) {
    return { tension_kgf: null, tension_reading: null };
  }
  const kgf = conv.convert(row.side, value);
  return kgf === null ? null : { tension_kgf: kgf, tension_reading: value };
}

/** Preview text next to a reading cell. */
export function readingPreview(row, conv) {
  if (!conv || row.tension_reading === null || row.tension_reading === undefined) {
    return '';
  }
  if (row.tension_kgf === null) {
    const g = conv.gauges[row.side];
    return g ? `outside ${gaugeRange(g)}` : 'no gauge';
  }
  return `≈ ${Math.round(row.tension_kgf)} kgf`;
}

/** Fills the tensiometer select: "direct kgf" plus every catalog model. */
export function renderTensiometerOptions(select, catalog, selected) {
  select.replaceChildren(
    h('option', { value: '', selected: !selected, text: 'None — enter kgf directly' }),
    ...catalog.map((m) => h('option', { value: m.id, selected: m.id === selected, text: m.name })),
  );
  select.value = selected ?? '';
}

/**
 * Fills a gauge select for the chosen model. An unknown selection falls back
 * to the first gauge; without a model the select is emptied and disabled.
 * @returns {string} the selected gauge id
 */
export function renderGaugeOptions(select, model, selected) {
  if (!model) {
    select.replaceChildren(h('option', { value: '', text: '—' }));
    select.value = '';
    select.disabled = true;
    return '';
  }
  const value = model.gauges.some((g) => g.id === selected) ? selected : model.gauges[0].id;
  select.replaceChildren(...model.gauges.map((g) =>
    h('option', { value: g.id, selected: g.id === value, text: `${g.label} (${gaugeRange(g)} ${model.reading_unit})` })));
  select.value = value;
  select.disabled = false;
  return value;
}
