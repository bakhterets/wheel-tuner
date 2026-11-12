// Package solver is the wheel truing math engine.
//
// It builds the spoke influence matrix (Jacobian) from the wheel geometry and
// finds the nipple turns that minimise the weighted cost
//
//	C = W_L·ΣΔL² + W_R·ΣΔR² + W_T·ΣΔT² (+ W_X·Σx² regularisation)
//
// The package is pure: it performs no I/O, holds no global mutable state and
// depends only on the Go standard library.
package solver

import (
	"errors"
	"fmt"
	"math"
)

// ErrInvalidInput is wrapped by every validation error returned by the package.
var ErrInvalidInput = errors.New("solver: invalid input")

// ErrNoConvergence is returned when the iterative linear solver fails to converge.
var ErrNoConvergence = errors.New("solver: optimisation did not converge")

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// ERD bounds accepted by Wheel.Validate (mm).
const (
	MinERDMM = 150.0
	MaxERDMM = 800.0
	// MaxHubOffsetDeg is the largest hub-hole angular offset (cross·720°/N)
	// considered buildable.
	MaxHubOffsetDeg = 90.0
)

// Side identifies the hub flange a spoke is attached to.
type Side byte

// Hub flange sides.
const (
	Left  Side = 'L'
	Right Side = 'R'
)

// ParseSide converts "L"/"R" into a Side.
func ParseSide(s string) (Side, error) {
	switch s {
	case "L":
		return Left, nil
	case "R":
		return Right, nil
	}
	return 0, invalidf("unknown side %q", s)
}

func (s Side) String() string { return string(rune(s)) }

func (s Side) valid() bool { return s == Left || s == Right }

// pull is the lateral direction a tightened spoke moves the rim:
// positive lateral deviation means "towards the right flange".
func (s Side) pull() float64 {
	if s == Right {
		return 1
	}
	return -1
}

// Direction is the spoke counting direction as seen from the view point.
type Direction int

// Counting directions.
const (
	Clockwise Direction = iota + 1
	CounterClockwise
)

// ParseDirection converts "CW"/"CCW" into a Direction.
func ParseDirection(s string) (Direction, error) {
	switch s {
	case "CW":
		return Clockwise, nil
	case "CCW":
		return CounterClockwise, nil
	}
	return 0, invalidf("unknown counting direction %q", s)
}

func (d Direction) String() string {
	switch d {
	case Clockwise:
		return "CW"
	case CounterClockwise:
		return "CCW"
	}
	return fmt.Sprintf("Direction(%d)", int(d))
}

// Wheel holds the physical parameters that shape the influence matrix.
type Wheel struct {
	SpokeCount    int
	ERDMM         float64
	Cross         int
	FlangeLeftMM  float64 // distance from hub centre to the left flange
	FlangeRightMM float64 // distance from hub centre to the right flange
	Direction     Direction
	ViewFrom      Side // side of the wheel the mechanic is looking from
}

// Validate checks that the wheel describes a buildable geometry.
func (w Wheel) Validate() error {
	switch w.SpokeCount {
	case 20, 24, 28, 32, 36:
	default:
		return invalidf("unsupported spoke count %d (want 20, 24, 28, 32 or 36)", w.SpokeCount)
	}
	if !(w.ERDMM >= MinERDMM && w.ERDMM <= MaxERDMM) {
		return invalidf("ERD %.1f mm out of range [%g, %g]", w.ERDMM, MinERDMM, MaxERDMM)
	}
	if w.Cross < 0 || w.Cross > 4 {
		return invalidf("cross pattern %d out of range [0, 4]", w.Cross)
	}
	if off := w.hubOffsetDeg(); off > MaxHubOffsetDeg {
		return invalidf("cross %d on %d spokes gives %.0f° hub offset (max %.0f°)",
			w.Cross, w.SpokeCount, off, MaxHubOffsetDeg)
	}
	for _, f := range [...]struct {
		name string
		v    float64
	}{{"left", w.FlangeLeftMM}, {"right", w.FlangeRightMM}} {
		if !(f.v > 0 && f.v < w.ERDMM/2) {
			return invalidf("%s flange distance %.1f mm out of range (0, ERD/2)", f.name, f.v)
		}
	}
	if w.Direction != Clockwise && w.Direction != CounterClockwise {
		return invalidf("invalid counting direction %d", int(w.Direction))
	}
	if !w.ViewFrom.valid() {
		return invalidf("invalid view side %q", w.ViewFrom.String())
	}
	return nil
}

func (w Wheel) hubOffsetDeg() float64 {
	return float64(w.Cross) * 720 / float64(w.SpokeCount)
}

// rotationSense is +1 when increasing spoke index runs clockwise as seen from
// the right side of the wheel, -1 otherwise. It anchors the lacing convention
// (see crossPartners) to the physical wheel instead of the index space.
func (w Wheel) rotationSense() int {
	s := 1
	if w.Direction == CounterClockwise {
		s = -s
	}
	if w.ViewFrom == Left {
		s = -s
	}
	return s
}

// bracingAngle returns the spoke bracing angle for a flange side in radians.
func (w Wheel) bracingAngle(s Side) float64 {
	f := w.FlangeLeftMM
	if s == Right {
		f = w.FlangeRightMM
	}
	return math.Atan2(f, w.ERDMM/2)
}

// spacingMM is the rim arc length between two neighbouring spoke holes.
func (w Wheel) spacingMM() float64 {
	return math.Pi * w.ERDMM / float64(w.SpokeCount)
}

// Measurement is the state of one spoke as entered by the mechanic.
//
// LateralMM is positive towards the right flange; RadialMM is positive when the
// rim is too far from the hub (a hop). When a PeakNext flag is set the value is
// the peak located between this spoke and the next one (position i+0.5).
// A zero TensionKgf means "not measured" and excludes the spoke from tension
// balancing.
type Measurement struct {
	Index           int // 1..SpokeCount in counting direction
	Side            Side
	TensionKgf      float64
	LateralMM       float64
	RadialMM        float64
	LateralPeakNext bool
	RadialPeakNext  bool
}

// Validate checks the wheel and measurements without building the model.
func Validate(w Wheel, ms []Measurement) error {
	if err := w.Validate(); err != nil {
		return err
	}
	_, err := normalize(w, ms)
	return err
}

// normalize validates measurements and returns them ordered by index.
func normalize(w Wheel, ms []Measurement) ([]Measurement, error) {
	n := w.SpokeCount
	if len(ms) != n {
		return nil, invalidf("got %d measurements for %d spokes", len(ms), n)
	}
	out := make([]Measurement, n)
	seen := make([]bool, n)
	for _, m := range ms {
		if m.Index < 1 || m.Index > n {
			return nil, invalidf("spoke index %d out of range [1, %d]", m.Index, n)
		}
		if seen[m.Index-1] {
			return nil, invalidf("duplicate spoke index %d", m.Index)
		}
		seen[m.Index-1] = true
		if !m.Side.valid() {
			return nil, invalidf("spoke %d: invalid side %q", m.Index, m.Side.String())
		}
		for _, v := range [...]float64{m.TensionKgf, m.LateralMM, m.RadialMM} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, invalidf("spoke %d: non-finite value", m.Index)
			}
		}
		if m.TensionKgf < 0 {
			return nil, invalidf("spoke %d: negative tension", m.Index)
		}
		out[m.Index-1] = m
	}
	var counts [2]int
	for _, m := range out {
		if m.Side == Left {
			counts[0]++
		} else {
			counts[1]++
		}
	}
	for i, side := range [...]Side{Left, Right} {
		c := counts[i]
		if c == 0 {
			return nil, invalidf("no spokes on side %s: a wheel needs spokes on both flanges", side)
		}
		if w.Cross > 0 && (c%2 != 0 || 2*w.Cross-1 >= c) {
			return nil, invalidf("%d spokes on side %s cannot be laced %d-cross", c, side, w.Cross)
		}
	}
	return out, nil
}
