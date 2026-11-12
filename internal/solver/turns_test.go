package solver

import (
	"math"
	"testing"
)

func TestQuantizeTurns(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{0, 0},
		{0.1, 0},
		{-0.124, 0},
		{0.125, 0.25},
		{-0.125, -0.25},
		{0.3, 0.25},
		{0.4, 0.5},
		{-0.62, -0.5},
		{0.9, 1},
		{1.3, 1.25},
	}
	for _, tc := range tests {
		if got := QuantizeTurns(tc.in, 0.25, 0.125); got != tc.want {
			t.Errorf("QuantizeTurns(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if got := QuantizeTurns(-0.2, 1, 0.1); got != 0 || math.Signbit(got) {
		t.Errorf("rounding to zero must yield +0, got %v", got)
	}
}

func TestFormatTurns(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{0.25, "+1/4"},
		{-0.5, "-1/2"},
		{0.75, "+3/4"},
		{1, "+1"},
		{-1.25, "-1 1/4"},
		{2, "+2"},
		{0.125, "+1/8"},
		{0.375, "+3/8"},
		{0.3, "+0.30"},
		{-0.01, "-0.01"},
	}
	for _, tc := range tests {
		if got := FormatTurns(tc.in); got != tc.want {
			t.Errorf("FormatTurns(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestActionInstruction(t *testing.T) {
	tests := []struct {
		a       Action
		want    string
		tighten bool
	}{
		{Action{Spoke: 14, Side: Right, Turns: 0.5}, "Spoke R14: +1/2 turn (Tighten)", true},
		{Action{Spoke: 3, Side: Left, Turns: -0.25}, "Spoke L3: -1/4 turn (Loosen)", false},
		{Action{Spoke: 7, Side: Left, Turns: 1}, "Spoke L7: +1 turn (Tighten)", true},
		{Action{Spoke: 8, Side: Right, Turns: -1.5}, "Spoke R8: -1 1/2 turns (Loosen)", false},
	}
	for _, tc := range tests {
		if got := tc.a.Instruction(); got != tc.want {
			t.Errorf("Instruction() = %q, want %q", got, tc.want)
		}
		if tc.a.Tighten() != tc.tighten {
			t.Errorf("%v: Tighten() = %v", tc.a, tc.a.Tighten())
		}
	}
}

func TestActionInstructionWithSpokeNumber(t *testing.T) {
	a := Action{Spoke: 27, Side: Left, Turns: -0.25}
	if got, want := a.InstructionWithSpokeNumber(14), "Spoke L14: -1/4 turn (Loosen)"; got != want {
		t.Errorf("InstructionWithSpokeNumber() = %q, want %q", got, want)
	}
}
