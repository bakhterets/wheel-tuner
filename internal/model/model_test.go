package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

func validWheel() WheelSetup {
	return WheelSetup{
		Name:              "Rear 29er",
		SpokesCount:       32,
		ERDMM:             600,
		CrossPattern:      3,
		FlangeDistLeftMM:  35,
		FlangeDistRightMM: 20,
		StartReference:    "Valve Hole",
		ViewPoint:         "Drive Side",
		CountingDirection: "CCW",
	}
}

func TestWheelNormalize(t *testing.T) {
	w := WheelSetup{Name: "  Front  ", ViewPoint: " ", CountingDirection: " ccw "}
	w.Normalize()
	want := WheelSetup{Name: "Front", StartReference: DefaultStartReference, ViewPoint: DefaultViewPoint, CountingDirection: "CCW"}
	if w != want {
		t.Fatalf("Normalize = %+v, want %+v", w, want)
	}
	w = WheelSetup{}
	w.Normalize()
	if w.CountingDirection != DefaultCountingDirection {
		t.Fatalf("default direction = %q", w.CountingDirection)
	}
}

func TestWheelValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*WheelSetup)
		field  string
	}{
		{"valid", func(*WheelSetup) {}, ""},
		{"lower case view point", func(w *WheelSetup) { w.ViewPoint = "rotor side" }, ""},
		{"missing name", func(w *WheelSetup) { w.Name = "" }, "name"},
		{"long name", func(w *WheelSetup) { w.Name = strings.Repeat("x", 101) }, "name"},
		{"missing reference", func(w *WheelSetup) { w.StartReference = "" }, "start_reference"},
		{"long reference", func(w *WheelSetup) { w.StartReference = strings.Repeat("x", 65) }, "start_reference"},
		{"bad direction", func(w *WheelSetup) { w.CountingDirection = "UP" }, "counting_direction"},
		{"bad view point", func(w *WheelSetup) { w.ViewPoint = "Top" }, "view_point"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := validWheel()
			tc.mutate(&w)
			err := w.Validate()
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("err = %v, want field %q", err, tc.field)
			}
			if !IsValidation(fmt.Errorf("wrapped: %w", err)) {
				t.Fatal("IsValidation must see through wrapping")
			}
		})
	}
	if IsValidation(errors.New("other")) {
		t.Fatal("plain errors are not validation errors")
	}
}

func TestViewSide(t *testing.T) {
	tests := map[string]string{
		"Rotor Side":     "L",
		"Non-Drive Side": "L",
		"Left":           "L",
		"Drive Side":     "R",
		"Non-Rotor Side": "R",
		"RIGHT":          "R",
		"Top":            "",
	}
	for vp, want := range tests {
		if got := (WheelSetup{ViewPoint: vp}).ViewSide(); got != want {
			t.Errorf("ViewSide(%q) = %q, want %q", vp, got, want)
		}
	}
}

func TestOrientationHeader(t *testing.T) {
	w := WheelSetup{StartReference: "Rim Seam", ViewPoint: "Rotor Side", CountingDirection: "CCW"}
	if got, want := w.OrientationHeader(), "Start at Rim Seam. View from Rotor Side. Count CCW."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestValidateMeasurements(t *testing.T) {
	rows := func() []Measurement {
		ms := make([]Measurement, 4)
		for i := range ms {
			ms[i] = Measurement{SpokeIndex: i + 1, Side: []string{"L", "R"}[i%2], TensionKgf: 100}
		}
		return ms
	}
	tests := []struct {
		name   string
		mutate func([]Measurement) []Measurement
		ok     bool
	}{
		{"valid", func(m []Measurement) []Measurement { return m }, true},
		{"zero tension allowed", func(m []Measurement) []Measurement { m[0].TensionKgf = 0; return m }, true},
		{"wrong count", func(m []Measurement) []Measurement { return m[:3] }, false},
		{"index zero", func(m []Measurement) []Measurement { m[0].SpokeIndex = 0; return m }, false},
		{"index too high", func(m []Measurement) []Measurement { m[0].SpokeIndex = 5; return m }, false},
		{"duplicate", func(m []Measurement) []Measurement { m[1].SpokeIndex = 1; return m }, false},
		{"bad side", func(m []Measurement) []Measurement { m[1].Side = "X"; return m }, false},
		{"NaN", func(m []Measurement) []Measurement { m[1].LateralDevMM = math.NaN(); return m }, false},
		{"Inf", func(m []Measurement) []Measurement { m[1].RadialDevMM = math.Inf(-1); return m }, false},
		{"negative tension", func(m []Measurement) []Measurement { m[1].TensionKgf = -3; return m }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMeasurements(4, tc.mutate(rows()))
			if tc.ok != (err == nil) {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if err != nil && !IsValidation(err) {
				t.Fatalf("want ValidationError, got %T", err)
			}
		})
	}
}
