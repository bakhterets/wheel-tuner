package solver

import (
	"errors"
	"math"
	"strings"
	"testing"
)

const eps = 1e-9

func testWheel(spokes, cross int) Wheel {
	return Wheel{
		SpokeCount:    spokes,
		ERDMM:         600,
		Cross:         cross,
		FlangeLeftMM:  35,
		FlangeRightMM: 20,
		Direction:     Clockwise,
		ViewFrom:      Right,
	}
}

// alternating returns true measurements with sides L,R,L,R… and the given tension.
func alternating(n int, tension float64) []Measurement {
	ms := make([]Measurement, n)
	for i := range ms {
		s := Left
		if i%2 == 1 {
			s = Right
		}
		ms[i] = Measurement{Index: i + 1, Side: s, TensionKgf: tension}
	}
	return ms
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func requireInvalid(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestParseSide(t *testing.T) {
	tests := []struct {
		in      string
		want    Side
		wantErr bool
	}{
		{"L", Left, false},
		{"R", Right, false},
		{"l", 0, true},
		{"", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseSide(tc.in)
		if tc.wantErr {
			requireInvalid(t, err)
			continue
		}
		if err != nil || got != tc.want || got.String() != tc.in {
			t.Errorf("ParseSide(%q) = %v, %v", tc.in, got, err)
		}
	}
}

func TestParseDirection(t *testing.T) {
	tests := []struct {
		in      string
		want    Direction
		wantErr bool
	}{
		{"CW", Clockwise, false},
		{"CCW", CounterClockwise, false},
		{"cw", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseDirection(tc.in)
		if tc.wantErr {
			requireInvalid(t, err)
			continue
		}
		if err != nil || got != tc.want || got.String() != tc.in {
			t.Errorf("ParseDirection(%q) = %v, %v", tc.in, got, err)
		}
	}
	if got := Direction(7).String(); got != "Direction(7)" {
		t.Errorf("unknown direction String() = %q", got)
	}
}

func TestParseSpokeType(t *testing.T) {
	tests := []struct {
		in      string
		want    SpokeType
		wantErr bool
	}{
		{"J-Bend", JBend, false},
		{"j-bend", JBend, false},
		{"Straightpull", StraightPull, false},
		{"STRAIGHTPULL", StraightPull, false},
		{"", 0, true},
		{"bladed", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseSpokeType(tc.in)
		if tc.wantErr {
			requireInvalid(t, err)
			continue
		}
		if err != nil || got != tc.want || !strings.EqualFold(got.String(), tc.in) {
			t.Errorf("ParseSpokeType(%q) = %v, %v", tc.in, got, err)
		}
	}
	if got := SpokeType(7).String(); got != "SpokeType(7)" {
		t.Errorf("unknown spoke type String() = %q", got)
	}
}

func TestWheelValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Wheel)
		ok     bool
	}{
		{"valid", func(*Wheel) {}, true},
		{"radial", func(w *Wheel) { w.Cross = 0 }, true},
		{"36 spokes 4x", func(w *Wheel) { w.SpokeCount, w.Cross = 36, 4 }, true},
		{"24 spokes 3x (90°)", func(w *Wheel) { w.SpokeCount, w.Cross = 24, 3 }, true},
		{"unsupported spokes", func(w *Wheel) { w.SpokeCount = 30 }, false},
		{"ERD too small", func(w *Wheel) { w.ERDMM = 100 }, false},
		{"ERD too large", func(w *Wheel) { w.ERDMM = 900 }, false},
		{"ERD NaN", func(w *Wheel) { w.ERDMM = math.NaN() }, false},
		{"negative cross", func(w *Wheel) { w.Cross = -1 }, false},
		{"cross 5", func(w *Wheel) { w.Cross = 5 }, false},
		{"20 spokes 3x exceeds hub offset", func(w *Wheel) { w.SpokeCount, w.Cross = 20, 3 }, false},
		{"zero left flange", func(w *Wheel) { w.FlangeLeftMM = 0 }, false},
		{"right flange beyond rim", func(w *Wheel) { w.FlangeRightMM = 300 }, false},
		{"bad direction", func(w *Wheel) { w.Direction = 0 }, false},
		{"bad view side", func(w *Wheel) { w.ViewFrom = 'X' }, false},
		{"with flange diameters and offset", func(w *Wheel) {
			w.FlangeDiamLeftMM, w.FlangeDiamRightMM, w.RimOffsetMM, w.SpokeType = 58, 46, 4, StraightPull
		}, true},
		{"negative flange diameter", func(w *Wheel) { w.FlangeDiamLeftMM = -1 }, false},
		{"flange diameter reaches ERD/2", func(w *Wheel) { w.FlangeDiamRightMM = 300 }, false},
		{"NaN flange diameter", func(w *Wheel) { w.FlangeDiamRightMM = math.NaN() }, false},
		{"NaN rim offset", func(w *Wheel) { w.RimOffsetMM = math.NaN() }, false},
		{"rim offset swallows left flange", func(w *Wheel) { w.RimOffsetMM = 35 }, false},
		{"rim offset swallows right flange", func(w *Wheel) { w.RimOffsetMM = -20 }, false},
		{"bad spoke type", func(w *Wheel) { w.SpokeType = 9 }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := testWheel(32, 3)
			tc.mutate(&w)
			err := w.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok {
				requireInvalid(t, err)
			}
		})
	}
}

func TestRotationSense(t *testing.T) {
	tests := []struct {
		dir  Direction
		view Side
		want int
	}{
		{Clockwise, Right, 1},
		{CounterClockwise, Right, -1},
		{Clockwise, Left, -1},
		{CounterClockwise, Left, 1},
	}
	for _, tc := range tests {
		w := testWheel(32, 3)
		w.Direction, w.ViewFrom = tc.dir, tc.view
		if got := w.rotationSense(); got != tc.want {
			t.Errorf("%v from %v: sense = %d, want %d", tc.dir, tc.view, got, tc.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name   string
		cross  int
		mutate func([]Measurement) []Measurement
		ok     bool
	}{
		{"valid", 3, func(m []Measurement) []Measurement { return m }, true},
		{"reordered", 3, func(m []Measurement) []Measurement {
			m[0], m[31] = m[31], m[0]
			return m
		}, true},
		{"too few", 3, func(m []Measurement) []Measurement { return m[:31] }, false},
		{"index zero", 3, func(m []Measurement) []Measurement { m[0].Index = 0; return m }, false},
		{"index too high", 3, func(m []Measurement) []Measurement { m[0].Index = 33; return m }, false},
		{"duplicate", 3, func(m []Measurement) []Measurement { m[1].Index = 1; return m }, false},
		{"bad side", 3, func(m []Measurement) []Measurement { m[2].Side = 'X'; return m }, false},
		{"NaN lateral", 3, func(m []Measurement) []Measurement { m[2].LateralMM = math.NaN(); return m }, false},
		{"Inf radial", 3, func(m []Measurement) []Measurement { m[2].RadialMM = math.Inf(1); return m }, false},
		{"negative tension", 3, func(m []Measurement) []Measurement { m[2].TensionKgf = -1; return m }, false},
		{"odd flange count", 3, func(m []Measurement) []Measurement { m[1].Side = Left; return m }, false},
		{"odd flange count radial is fine", 0, func(m []Measurement) []Measurement { m[1].Side = Left; return m }, true},
		{"single-sided radial", 0, func(m []Measurement) []Measurement {
			for i := range m {
				m[i].Side = Right
			}
			return m
		}, false},
		{"single-sided crossed", 3, func(m []Measurement) []Measurement {
			for i := range m {
				m[i].Side = Left
			}
			return m
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := testWheel(32, tc.cross)
			got, err := normalize(w, tc.mutate(alternating(32, 100)))
			if !tc.ok {
				requireInvalid(t, err)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for i, m := range got {
				if m.Index != i+1 {
					t.Fatalf("position %d holds index %d", i, m.Index)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	w := testWheel(32, 3)
	if err := Validate(w, alternating(32, 100)); err != nil {
		t.Fatal(err)
	}
	requireInvalid(t, Validate(Wheel{}, alternating(32, 100)))
	requireInvalid(t, Validate(w, alternating(31, 100)))
}

func TestNormalizeRejectsCrossOnShortFlange(t *testing.T) {
	// 20 spokes, 2-cross needs at least 4 spokes per flange; give the right
	// flange only 2.
	w := testWheel(20, 2)
	ms := alternating(20, 100)
	for i := range ms {
		ms[i].Side = Left
	}
	ms[0].Side, ms[1].Side = Right, Right
	_, err := normalize(w, ms)
	requireInvalid(t, err)
}
