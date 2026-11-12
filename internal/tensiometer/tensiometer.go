// Package tensiometer converts spoke tensiometer readings into tension (kgf)
// using the manufacturers' calibration tables.
//
// The package is pure: the catalog is rebuilt on every call, so callers can
// never mutate shared state.
package tensiometer

import (
	"errors"
	"fmt"
	"math"
)

// ErrUnknown is returned for an unknown tensiometer or spoke gauge.
var ErrUnknown = errors.New("tensiometer: unknown model or spoke gauge")

// ErrOutOfRange is returned for a reading outside the calibrated table range.
var ErrOutOfRange = errors.New("tensiometer: reading outside the calibration table")

// Point is one calibration table entry.
type Point struct {
	Reading float64 `json:"reading"`
	Kgf     float64 `json:"kgf"`
}

// Gauge is the calibration curve for one spoke type, ordered by reading.
type Gauge struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Points []Point `json:"points"`
}

// Model is a tensiometer with its calibration tables.
type Model struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ReadingUnit string  `json:"reading_unit"`
	Note        string  `json:"note,omitempty"`
	Gauges      []Gauge `json:"gauges"`
}

// Catalog returns all supported tensiometers.
func Catalog() []Model {
	return []Model{tc02(), tm1()}
}

// Find returns the model with the given ID.
func Find(id string) (Model, error) {
	for _, m := range Catalog() {
		if m.ID == id {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("%w: tensiometer %q", ErrUnknown, id)
}

// Gauge returns the calibration curve with the given ID.
func (m Model) Gauge(id string) (Gauge, error) {
	for _, g := range m.Gauges {
		if g.ID == id {
			return g, nil
		}
	}
	return Gauge{}, fmt.Errorf("%w: spoke gauge %q for %s", ErrUnknown, id, m.Name)
}

// Lookup returns the calibration curve of a tensiometer for a spoke gauge.
func Lookup(modelID, gaugeID string) (Gauge, error) {
	m, err := Find(modelID)
	if err != nil {
		return Gauge{}, err
	}
	return m.Gauge(gaugeID)
}

// Range returns the lowest and highest calibrated readings.
func (g Gauge) Range() (lo, hi float64) {
	return g.Points[0].Reading, g.Points[len(g.Points)-1].Reading
}

// Kgf converts a reading by linear interpolation between table entries.
// Readings outside the table are rejected: extrapolating a non-linear
// calibration curve gives misleading tensions.
func (g Gauge) Kgf(reading float64) (float64, error) {
	lo, hi := g.Range()
	if !(reading >= lo && reading <= hi) {
		return 0, fmt.Errorf("%w: %g is outside %g–%g for %s", ErrOutOfRange, reading, lo, hi, g.Label)
	}
	i := 1
	for reading > g.Points[i].Reading {
		i++
	}
	a, b := g.Points[i-1], g.Points[i]
	t := (reading - a.Reading) / (b.Reading - a.Reading)
	return a.Kgf + t*(b.Kgf-a.Kgf), nil
}

// series builds a gauge whose readings start at from and grow by step.
func series(id, label string, from, step float64, kgf ...float64) Gauge {
	g := Gauge{ID: id, Label: label, Points: make([]Point, len(kgf))}
	for i, v := range kgf {
		// Rounding keeps table readings exact (2.1, not 2.1000000000000005).
		r := math.Round((from+float64(i)*step)*1000) / 1000
		g.Points[i] = Point{Reading: r, Kgf: v}
	}
	return g
}
