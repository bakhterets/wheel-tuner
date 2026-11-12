package solver

import (
	"fmt"
	"math"
)

// QuantizeTurns snaps a raw turn value to the nearest multiple of step and
// drops adjustments smaller than minTurn.
func QuantizeTurns(v, step, minTurn float64) float64 {
	if math.Abs(v) < minTurn {
		return 0
	}
	q := math.Round(v/step) * step
	if q == 0 {
		return 0 // normalise -0
	}
	return q
}

// FormatTurns renders turns as a signed mechanic fraction: "+1/4", "-1 1/2",
// "+2". Values that are not multiples of 1/8 fall back to two decimals.
func FormatTurns(t float64) string {
	a := math.Abs(t)
	eighths := math.Round(a * 8)
	if eighths == 0 && a < 1e-9 {
		return "0"
	}
	sign := "+"
	if t < 0 {
		sign = "-"
	}
	if math.Abs(a*8-eighths) > 1e-6 {
		return fmt.Sprintf("%s%.2f", sign, a)
	}
	e := int(eighths)
	whole, rem := e/8, e%8
	den := 8
	for rem > 0 && rem%2 == 0 {
		rem /= 2
		den /= 2
	}
	switch {
	case rem == 0:
		return fmt.Sprintf("%s%d", sign, whole)
	case whole == 0:
		return fmt.Sprintf("%s%d/%d", sign, rem, den)
	default:
		return fmt.Sprintf("%s%d %d/%d", sign, whole, rem, den)
	}
}

// Action is a single nipple adjustment.
type Action struct {
	Spoke int
	Side  Side
	Turns float64 // positive tightens
}

// Tighten reports whether the action tightens the spoke.
func (a Action) Tighten() bool { return a.Turns > 0 }

// Fraction returns the signed fractional turn, e.g. "+1/2".
func (a Action) Fraction() string { return FormatTurns(a.Turns) }

// Instruction renders the action for the mechanic,
// e.g. "Spoke R14: +1/2 turn (Tighten)".
func (a Action) Instruction() string {
	unit := "turn"
	if math.Abs(a.Turns) > 1 {
		unit = "turns"
	}
	verb := "Loosen"
	if a.Tighten() {
		verb = "Tighten"
	}
	return fmt.Sprintf("Spoke %s%d: %s %s (%s)", a.Side, a.Spoke, a.Fraction(), unit, verb)
}
