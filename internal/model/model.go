// Package model defines the persisted domain entities shared by the store and
// the HTTP API, together with their input validation.
package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/bakhterets/wheel-tuner/internal/tensiometer"
)

// Defaults applied by WheelSetup.Normalize.
const (
	DefaultStartReference    = "Rim Seam"
	DefaultViewPoint         = "Rotor Side"
	DefaultCountingDirection = "CW"
)

const (
	maxNameLen = 100
	maxRefLen  = 64
)

// ValidationError reports an invalid input field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// IsValidation reports whether err is (or wraps) a *ValidationError.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// ErrNotFound is returned by repositories when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// viewPointSide maps an accepted view point label (case-insensitive) to the
// physical side of the wheel ("L" or "R"). Disc rotors mount on the left,
// the drivetrain on the right.
func viewPointSide(v string) (string, bool) {
	switch strings.ToLower(v) {
	case "rotor side", "non-drive side", "left":
		return "L", true
	case "drive side", "non-rotor side", "right":
		return "R", true
	}
	return "", false
}

// WheelSetup is a wheel profile (table wheel_setups).
//
// Rim and hub components only pre-fill the geometric fields: the profile keeps
// its own (editable) copy, so a stored profile never changes when the
// component library is updated.
type WheelSetup struct {
	ID                   int64   `json:"id"`
	Name                 string  `json:"name"`
	SpokesCount          int     `json:"spokes_count"`
	ERDMM                float64 `json:"erd_mm"`
	CrossPattern         int     `json:"cross_pattern"`
	FlangeDistLeftMM     float64 `json:"flange_dist_left_mm"`
	FlangeDistRightMM    float64 `json:"flange_dist_right_mm"`
	StartReference       string  `json:"start_reference"`
	ViewPoint            string  `json:"view_point"`
	CountingDirection    string  `json:"counting_direction"`
	RimComponentID       *int64  `json:"rim_component_id"`
	HubComponentID       *int64  `json:"hub_component_id"`
	RimAsymmetryOffsetMM float64 `json:"rim_asymmetry_offset_mm"`
	// Flange (pitch circle) diameters; 0 means unknown.
	FlangeDiamLeftMM  float64 `json:"flange_diam_left_mm"`
	FlangeDiamRightMM float64 `json:"flange_diam_right_mm"`
	SpokeType         string  `json:"spoke_type"`
	// MaxTensionKgf is the rim's tension limit; 0 disables the warning.
	MaxTensionKgf float64 `json:"max_tension_kgf"`
	// Tensiometer is a tensiometer.Model ID; empty means tensions in kgf.
	Tensiometer     string `json:"tensiometer"`
	SpokeGaugeLeft  string `json:"spoke_gauge_left"`
	SpokeGaugeRight string `json:"spoke_gauge_right"`
	// PairedSpokes marks straight-pull hubs whose spokes leave the flange in
	// leading/trailing pairs (e.g. Newmen Fade SP); it lengthens crossed spokes.
	PairedSpokes bool `json:"paired_spokes"`
	// TensionLimit is the desired upper spoke tension, in kgf or (unit
	// "reading") as a tensiometer reading converted per side; 0 = no limit.
	TensionLimit     float64 `json:"tension_limit"`
	TensionLimitUnit string  `json:"tension_limit_unit"`
}

// Tension limit units.
const (
	LimitUnitKgf     = "kgf"
	LimitUnitReading = "reading"
)

// Normalize trims text fields and fills defaults for empty ones.
func (w *WheelSetup) Normalize() {
	w.Name = strings.TrimSpace(w.Name)
	w.StartReference = strings.TrimSpace(w.StartReference)
	w.ViewPoint = strings.TrimSpace(w.ViewPoint)
	w.CountingDirection = strings.ToUpper(strings.TrimSpace(w.CountingDirection))
	w.SpokeType = canonicalSpokeType(strings.TrimSpace(w.SpokeType))
	w.Tensiometer = strings.TrimSpace(w.Tensiometer)
	w.SpokeGaugeLeft = strings.TrimSpace(w.SpokeGaugeLeft)
	w.SpokeGaugeRight = strings.TrimSpace(w.SpokeGaugeRight)
	w.TensionLimitUnit = strings.ToLower(strings.TrimSpace(w.TensionLimitUnit))
	if w.TensionLimitUnit == "" {
		w.TensionLimitUnit = LimitUnitKgf
	}
	if w.StartReference == "" {
		w.StartReference = DefaultStartReference
	}
	if w.ViewPoint == "" {
		w.ViewPoint = DefaultViewPoint
	}
	if w.CountingDirection == "" {
		w.CountingDirection = DefaultCountingDirection
	}
	if w.SpokeType == "" {
		w.SpokeType = SpokeJBend
	}
	if w.Tensiometer == "" {
		w.SpokeGaugeLeft, w.SpokeGaugeRight = "", ""
	}
}

// Validate checks the descriptive fields of a profile. Geometric constraints
// (spoke count, ERD, lacing, flanges, rim offset) are validated by the solver.
func (w WheelSetup) Validate() error {
	switch {
	case w.Name == "":
		return invalid("name", "is required")
	case len(w.Name) > maxNameLen:
		return invalid("name", "must be at most %d characters", maxNameLen)
	case w.StartReference == "":
		return invalid("start_reference", "is required")
	case len(w.StartReference) > maxRefLen:
		return invalid("start_reference", "must be at most %d characters", maxRefLen)
	case w.CountingDirection != "CW" && w.CountingDirection != "CCW":
		return invalid("counting_direction", "must be CW or CCW")
	case w.SpokeType != SpokeJBend && w.SpokeType != SpokeStraightPull:
		return invalid("spoke_type", "must be %s or %s", SpokeJBend, SpokeStraightPull)
	case !(w.MaxTensionKgf >= 0) || math.IsInf(w.MaxTensionKgf, 0):
		return invalid("max_tension_kgf", "must be a finite non-negative number")
	case w.PairedSpokes && w.SpokeType != SpokeStraightPull:
		return invalid("paired_spokes", "requires %s spokes", SpokeStraightPull)
	case !(w.TensionLimit >= 0) || math.IsInf(w.TensionLimit, 0):
		return invalid("tension_limit", "must be a finite non-negative number")
	case w.TensionLimitUnit != "" && w.TensionLimitUnit != LimitUnitKgf && w.TensionLimitUnit != LimitUnitReading:
		return invalid("tension_limit_unit", "must be %s or %s", LimitUnitKgf, LimitUnitReading)
	case w.RimComponentID != nil && *w.RimComponentID <= 0:
		return invalid("rim_component_id", "must be positive")
	case w.HubComponentID != nil && *w.HubComponentID <= 0:
		return invalid("hub_component_id", "must be positive")
	}
	if _, ok := viewPointSide(w.ViewPoint); !ok {
		return invalid("view_point", "unknown view point %q", w.ViewPoint)
	}
	return w.validateTensiometer()
}

// TensionLimitsKgf returns the upper tension per side (0 = none): the lower
// of the desired limit and the rim's maximum. A "reading" limit is converted
// with each side's spoke gauge and must lie within the gauge table. It must
// run after Validate.
func (w WheelSetup) TensionLimitsKgf() (left, right float64, err error) {
	left, right = w.TensionLimit, w.TensionLimit
	if w.TensionLimit > 0 && w.TensionLimitUnit == LimitUnitReading {
		if w.Tensiometer == "" {
			return 0, 0, invalid("tension_limit_unit", "a reading limit requires a tensiometer")
		}
		if left, err = w.readingKgf(w.SpokeGaugeLeft); err != nil {
			return 0, 0, err
		}
		if right, err = w.readingKgf(w.SpokeGaugeRight); err != nil {
			return 0, 0, err
		}
	}
	return minLimit(left, w.MaxTensionKgf), minLimit(right, w.MaxTensionKgf), nil
}

func (w WheelSetup) readingKgf(gauge string) (float64, error) {
	g, err := tensiometer.Lookup(w.Tensiometer, gauge)
	if err != nil {
		return 0, invalid("tension_limit", "%v", err)
	}
	kgf, err := g.Kgf(w.TensionLimit)
	if err != nil {
		return 0, invalid("tension_limit", "%v", err)
	}
	return kgf, nil
}

// minLimit combines two limits where 0 means "no limit".
func minLimit(a, b float64) float64 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	}
	return math.Min(a, b)
}

func (w WheelSetup) validateTensiometer() error {
	if w.Tensiometer == "" {
		if w.SpokeGaugeLeft != "" || w.SpokeGaugeRight != "" {
			return invalid("tensiometer", "is required with spoke gauges")
		}
		return nil
	}
	m, err := tensiometer.Find(w.Tensiometer)
	if err != nil {
		return invalid("tensiometer", "unknown tensiometer %q", w.Tensiometer)
	}
	for _, g := range [...]struct{ field, id string }{
		{"spoke_gauge_left", w.SpokeGaugeLeft},
		{"spoke_gauge_right", w.SpokeGaugeRight},
	} {
		if _, err := m.Gauge(g.id); err != nil {
			return invalid(g.field, "unknown spoke gauge %q for %s", g.id, m.Name)
		}
	}
	return nil
}

// ViewSide returns the physical side ("L" or "R") the mechanic looks from.
func (w WheelSetup) ViewSide() string {
	s, _ := viewPointSide(w.ViewPoint)
	return s
}

// OrientationHeader is the reminder shown with the results, e.g.
// "Start at Rim Seam. View from Rotor Side. Count CCW."
func (w WheelSetup) OrientationHeader() string {
	return fmt.Sprintf("Start at %s. View from %s. Count %s.", w.StartReference, w.ViewPoint, w.CountingDirection)
}

// Session is a truing session (table sessions).
type Session struct {
	ID           int64         `json:"id"`
	WheelID      int64         `json:"wheel_id"`
	CreatedAt    time.Time     `json:"created_at"`
	Measurements []Measurement `json:"measurements,omitempty"`
}

// Measurement is the state of one spoke (table measurements).
//
// TensionReading is the raw tensiometer reading; when set, ResolveTensions
// overwrites TensionKgf with its converted value.
type Measurement struct {
	ID                 int64    `json:"id,omitempty"`
	SessionID          int64    `json:"session_id,omitempty"`
	SpokeIndex         int      `json:"spoke_index"`
	Side               string   `json:"side"`
	TensionKgf         float64  `json:"tension_kgf"`
	TensionReading     *float64 `json:"tension_reading,omitempty"`
	LateralDevMM       float64  `json:"lateral_dev_mm"`
	LatPeakBetweenNext bool     `json:"lat_peak_between_next"`
	RadialDevMM        float64  `json:"radial_dev_mm"`
	RadPeakBetweenNext bool     `json:"rad_peak_between_next"`
}

// ResolveTensions converts the tensiometer readings in ms to kgf in place,
// using the tensiometer and per-side spoke gauges of w. It must run after
// w.Validate and ValidateMeasurements.
func ResolveTensions(w WheelSetup, ms []Measurement) error {
	for i := range ms {
		m := &ms[i]
		if m.TensionReading == nil {
			continue
		}
		field := fmt.Sprintf("measurements[%d]", m.SpokeIndex)
		if w.Tensiometer == "" {
			return invalid(field, "tension reading requires a tensiometer in the wheel profile")
		}
		gauge := w.SpokeGaugeLeft
		if m.Side == "R" {
			gauge = w.SpokeGaugeRight
		}
		g, err := tensiometer.Lookup(w.Tensiometer, gauge)
		if err != nil {
			return invalid(field, "%v", err)
		}
		kgf, err := g.Kgf(*m.TensionReading)
		if err != nil {
			return invalid(field, "%v", err)
		}
		m.TensionKgf = kgf
	}
	return nil
}

// Spoke types accepted by WheelSetup and HubComponent.
const (
	SpokeJBend        = "J-Bend"
	SpokeStraightPull = "Straightpull"
)

func canonicalSpokeType(s string) string {
	switch strings.ToLower(s) {
	case "j-bend":
		return SpokeJBend
	case "straightpull":
		return SpokeStraightPull
	}
	return s
}

// RimComponent is a library rim (table rim_components).
type RimComponent struct {
	ID        int64  `json:"id"`
	Brand     string `json:"brand"`
	Model     string `json:"model"`
	WheelSize string `json:"wheel_size"`
	// HoleCounts lists the drillings sold with this ERD; empty means unknown.
	HoleCounts        []int   `json:"hole_counts"`
	ERDMM             float64 `json:"erd_mm"`
	AsymmetryOffsetMM float64 `json:"asymmetry_offset_mm"`
	// MaxTensionKgf is the published tension limit; 0 means unknown.
	MaxTensionKgf float64 `json:"max_tension_kgf"`
	Source        string  `json:"source"`
}

// HubComponent is a library hub (table hub_components). Left is the disc
// side of front hubs and the non-drive side of rear hubs.
type HubComponent struct {
	ID                int64   `json:"id"`
	Brand             string  `json:"brand"`
	Model             string  `json:"model"`
	Position          string  `json:"position"`
	Axle              string  `json:"axle"`
	FlangeDistLeftMM  float64 `json:"flange_dist_left_mm"`
	FlangeDistRightMM float64 `json:"flange_dist_right_mm"`
	FlangeDiamLeftMM  float64 `json:"flange_diam_left_mm"`
	FlangeDiamRightMM float64 `json:"flange_diam_right_mm"`
	SpokeType         string  `json:"spoke_type"`
	// PairedSpokes: straight-pull flange with leading/trailing spoke pairs.
	PairedSpokes bool   `json:"paired_spokes"`
	HoleCounts   []int  `json:"hole_counts"`
	Source       string `json:"source"`
}

// ValidateMeasurements checks that ms holds exactly one entry per spoke.
func ValidateMeasurements(spokes int, ms []Measurement) error {
	if len(ms) != spokes {
		return invalid("measurements", "got %d rows, want %d", len(ms), spokes)
	}
	seen := make(map[int]bool, spokes)
	for _, m := range ms {
		field := fmt.Sprintf("measurements[%d]", m.SpokeIndex)
		if m.SpokeIndex < 1 || m.SpokeIndex > spokes {
			return invalid(field, "spoke index out of range [1, %d]", spokes)
		}
		if seen[m.SpokeIndex] {
			return invalid(field, "duplicate spoke index")
		}
		seen[m.SpokeIndex] = true
		if m.Side != "L" && m.Side != "R" {
			return invalid(field, "side must be L or R")
		}
		for _, v := range [...]float64{m.TensionKgf, m.LateralDevMM, m.RadialDevMM} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return invalid(field, "values must be finite")
			}
		}
		if m.TensionKgf < 0 {
			return invalid(field, "tension must not be negative")
		}
	}
	return nil
}
