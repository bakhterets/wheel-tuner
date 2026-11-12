// Package model defines the persisted domain entities shared by the store and
// the HTTP API, together with their input validation.
package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
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
type WheelSetup struct {
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	SpokesCount       int     `json:"spokes_count"`
	ERDMM             float64 `json:"erd_mm"`
	CrossPattern      int     `json:"cross_pattern"`
	FlangeDistLeftMM  float64 `json:"flange_dist_left_mm"`
	FlangeDistRightMM float64 `json:"flange_dist_right_mm"`
	StartReference    string  `json:"start_reference"`
	ViewPoint         string  `json:"view_point"`
	CountingDirection string  `json:"counting_direction"`
}

// Normalize trims text fields and fills defaults for empty ones.
func (w *WheelSetup) Normalize() {
	w.Name = strings.TrimSpace(w.Name)
	w.StartReference = strings.TrimSpace(w.StartReference)
	w.ViewPoint = strings.TrimSpace(w.ViewPoint)
	w.CountingDirection = strings.ToUpper(strings.TrimSpace(w.CountingDirection))
	if w.StartReference == "" {
		w.StartReference = DefaultStartReference
	}
	if w.ViewPoint == "" {
		w.ViewPoint = DefaultViewPoint
	}
	if w.CountingDirection == "" {
		w.CountingDirection = DefaultCountingDirection
	}
}

// Validate checks the descriptive fields of a profile. Geometric constraints
// (spoke count, ERD, lacing, flanges) are validated by the solver.
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
	}
	if _, ok := viewPointSide(w.ViewPoint); !ok {
		return invalid("view_point", "unknown view point %q", w.ViewPoint)
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
type Measurement struct {
	ID                 int64   `json:"id,omitempty"`
	SessionID          int64   `json:"session_id,omitempty"`
	SpokeIndex         int     `json:"spoke_index"`
	Side               string  `json:"side"`
	TensionKgf         float64 `json:"tension_kgf"`
	LateralDevMM       float64 `json:"lateral_dev_mm"`
	LatPeakBetweenNext bool    `json:"lat_peak_between_next"`
	RadialDevMM        float64 `json:"radial_dev_mm"`
	RadPeakBetweenNext bool    `json:"rad_peak_between_next"`
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
