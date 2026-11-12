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
		SpokeType:         SpokeJBend,
	}
}

func TestWheelNormalize(t *testing.T) {
	w := WheelSetup{Name: "  Front  ", ViewPoint: " ", CountingDirection: " ccw ", SpokeGaugeLeft: "2.0"}
	w.Normalize()
	want := WheelSetup{Name: "Front", StartReference: DefaultStartReference, ViewPoint: DefaultViewPoint,
		CountingDirection: "CCW", SpokeType: SpokeJBend}
	if w != want {
		t.Fatalf("Normalize = %+v, want %+v", w, want)
	}
	w = WheelSetup{SpokeType: " STRAIGHTPULL ", Tensiometer: " ztto-tc-02 ", SpokeGaugeLeft: " 2.0 ", SpokeGaugeRight: "1.8 "}
	w.Normalize()
	if w.CountingDirection != DefaultCountingDirection || w.SpokeType != SpokeStraightPull ||
		w.Tensiometer != "ztto-tc-02" || w.SpokeGaugeLeft != "2.0" || w.SpokeGaugeRight != "1.8" {
		t.Fatalf("Normalize = %+v", w)
	}
	w = WheelSetup{SpokeType: "j-bend"}
	if w.Normalize(); w.SpokeType != SpokeJBend {
		t.Fatalf("spoke type = %q, want %q", w.SpokeType, SpokeJBend)
	}
}

func TestWheelValidate(t *testing.T) {
	zero, rim := int64(0), int64(3)
	tests := []struct {
		name   string
		mutate func(*WheelSetup)
		field  string
	}{
		{"valid", func(*WheelSetup) {}, ""},
		{"lower case view point", func(w *WheelSetup) { w.ViewPoint = "rotor side" }, ""},
		{"components and tensiometer", func(w *WheelSetup) {
			w.RimComponentID, w.HubComponentID, w.MaxTensionKgf = &rim, &rim, 122
			w.Tensiometer, w.SpokeGaugeLeft, w.SpokeGaugeRight = "ztto-tc-02", "2.0-1.8-2.0", "1.8"
		}, ""},
		{"missing name", func(w *WheelSetup) { w.Name = "" }, "name"},
		{"long name", func(w *WheelSetup) { w.Name = strings.Repeat("x", 101) }, "name"},
		{"missing reference", func(w *WheelSetup) { w.StartReference = "" }, "start_reference"},
		{"long reference", func(w *WheelSetup) { w.StartReference = strings.Repeat("x", 65) }, "start_reference"},
		{"bad direction", func(w *WheelSetup) { w.CountingDirection = "UP" }, "counting_direction"},
		{"bad view point", func(w *WheelSetup) { w.ViewPoint = "Top" }, "view_point"},
		{"bad spoke type", func(w *WheelSetup) { w.SpokeType = "bladed" }, "spoke_type"},
		{"negative max tension", func(w *WheelSetup) { w.MaxTensionKgf = -1 }, "max_tension_kgf"},
		{"NaN max tension", func(w *WheelSetup) { w.MaxTensionKgf = math.NaN() }, "max_tension_kgf"},
		{"infinite max tension", func(w *WheelSetup) { w.MaxTensionKgf = math.Inf(1) }, "max_tension_kgf"},
		{"zero rim id", func(w *WheelSetup) { w.RimComponentID = &zero }, "rim_component_id"},
		{"zero hub id", func(w *WheelSetup) { w.HubComponentID = &zero }, "hub_component_id"},
		{"gauge without tensiometer", func(w *WheelSetup) { w.SpokeGaugeRight = "2.0" }, "tensiometer"},
		{"unknown tensiometer", func(w *WheelSetup) { w.Tensiometer = "nope" }, "tensiometer"},
		{"missing left gauge", func(w *WheelSetup) {
			w.Tensiometer, w.SpokeGaugeRight = "park-tool-tm-1", "2.0"
		}, "spoke_gauge_left"},
		{"unknown right gauge", func(w *WheelSetup) {
			w.Tensiometer, w.SpokeGaugeLeft, w.SpokeGaugeRight = "park-tool-tm-1", "2.0", "1.8-1.5-1.8"
		}, "spoke_gauge_right"},
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

func TestResolveTensions(t *testing.T) {
	reading := func(v float64) *float64 { return &v }
	tc02 := validWheel()
	tc02.Tensiometer, tc02.SpokeGaugeLeft, tc02.SpokeGaugeRight = "ztto-tc-02", "2.0-1.8-2.0", "1.8"
	tests := []struct {
		name  string
		wheel WheelSetup
		ms    []Measurement
		want  []float64
		ok    bool
	}{
		{"per-side gauges", tc02, []Measurement{
			{SpokeIndex: 1, Side: "L", TensionReading: reading(2.8)},
			{SpokeIndex: 2, Side: "R", TensionReading: reading(2.8)},
			{SpokeIndex: 3, Side: "L", TensionKgf: 90},
		}, []float64{102, 100, 90}, true},
		{"reading overrides kgf", tc02, []Measurement{
			{SpokeIndex: 1, Side: "L", TensionKgf: 5, TensionReading: reading(2.75)},
		}, []float64{97.5}, true},
		{"kgf only without tensiometer", validWheel(), []Measurement{
			{SpokeIndex: 1, Side: "L", TensionKgf: 100},
		}, []float64{100}, true},
		{"reading without tensiometer", validWheel(), []Measurement{
			{SpokeIndex: 1, Side: "L", TensionReading: reading(2.8)},
		}, nil, false},
		{"reading out of range", tc02, []Measurement{
			{SpokeIndex: 1, Side: "R", TensionReading: reading(4.9)},
		}, nil, false},
		{"gauge missing in profile", WheelSetup{Tensiometer: "ztto-tc-02"}, []Measurement{
			{SpokeIndex: 1, Side: "L", TensionReading: reading(2.8)},
		}, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ResolveTensions(tc.wheel, tc.ms)
			if !tc.ok {
				var ve *ValidationError
				if !errors.As(err, &ve) || ve.Field != "measurements[1]" {
					t.Fatalf("err = %v, want measurements[1] validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for i, w := range tc.want {
				if math.Abs(tc.ms[i].TensionKgf-w) > 1e-9 {
					t.Fatalf("spoke %d: %v kgf, want %v", i+1, tc.ms[i].TensionKgf, w)
				}
			}
		})
	}
}
