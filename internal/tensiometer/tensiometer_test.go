package tensiometer

import (
	"errors"
	"math"
	"testing"
)

func TestCatalogTablesAreMonotonic(t *testing.T) {
	models := map[string]bool{}
	for _, m := range Catalog() {
		if models[m.ID] {
			t.Fatalf("duplicate tensiometer %q", m.ID)
		}
		models[m.ID] = true
		if m.Name == "" || m.ReadingUnit == "" || len(m.Gauges) == 0 {
			t.Fatalf("%s: incomplete model %+v", m.ID, m)
		}
		gauges := map[string]bool{}
		for _, g := range m.Gauges {
			if gauges[g.ID] {
				t.Fatalf("%s: duplicate gauge %q", m.ID, g.ID)
			}
			gauges[g.ID] = true
			if g.Label == "" || len(g.Points) < 2 {
				t.Fatalf("%s/%s: needs a label and at least two points", m.ID, g.ID)
			}
			for i := 1; i < len(g.Points); i++ {
				a, b := g.Points[i-1], g.Points[i]
				if !(b.Reading > a.Reading && b.Kgf > a.Kgf) {
					t.Fatalf("%s/%s: points %v → %v are not strictly increasing", m.ID, g.ID, a, b)
				}
			}
		}
	}
}

func TestKgf(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		gauge   string
		reading float64
		want    float64
	}{
		// Spot checks against the ZTTO TC-02 conversion table.
		{"tc02 first entry", "ztto-tc-02", "1.8-1.5-1.8", 1.1, 53},
		{"tc02 last entry", "ztto-tc-02", "1.8-1.5-1.8", 2.8, 152},
		{"tc02 butted 2.0-1.8-2.0", "ztto-tc-02", "2.0-1.8-2.0", 2.8, 102},
		{"tc02 butted 2.0-1.7-1.8", "ztto-tc-02", "2.0-1.7-1.8", 3.2, 152},
		{"tc02 butted 1.8-1.6-1.8", "ztto-tc-02", "1.8-1.6-1.8", 2.4, 100},
		{"tc02 straight 1.8", "ztto-tc-02", "1.8", 3.4, 148},
		{"tc02 straight 2.0", "ztto-tc-02", "2.0", 4.0, 155},
		{"tc02 straight 2.3", "ztto-tc-02", "2.3", 4.7, 150},
		{"interpolates between rows", "ztto-tc-02", "2.0-1.8-2.0", 2.75, 97.5},
		{"interpolates a quarter step", "ztto-tc-02", "2.0", 2.425, 52},
		// Spot checks against the Park Tool TM-1 conversion table.
		{"tm1 round 1.4 first", "park-tool-tm-1", "1.4", 8, 54},
		{"tm1 round 1.8 at 21", "park-tool-tm-1", "1.8", 21, 105},
		{"tm1 round 2.0 last", "park-tool-tm-1", "2.0", 28, 173},
		{"tm1 round 2.6 last", "park-tool-tm-1", "2.6", 36, 172},
		{"tm1 half step", "park-tool-tm-1", "2.0", 22.5, 91},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, err := Lookup(tc.model, tc.gauge)
			if err != nil {
				t.Fatal(err)
			}
			got, err := g.Kgf(tc.reading)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("Kgf(%v) = %v, want %v", tc.reading, got, tc.want)
			}
		})
	}
}

func TestKgfOutOfRange(t *testing.T) {
	g, err := Lookup("ztto-tc-02", "2.0")
	if err != nil {
		t.Fatal(err)
	}
	if lo, hi := g.Range(); lo != 2.4 || hi != 4.0 {
		t.Fatalf("range = %v–%v, want 2.4–4", lo, hi)
	}
	for _, r := range []float64{2.39, 4.01, 0, -1, math.NaN(), math.Inf(1)} {
		if _, err := g.Kgf(r); !errors.Is(err, ErrOutOfRange) {
			t.Errorf("Kgf(%v) error = %v, want ErrOutOfRange", r, err)
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	tests := []struct{ model, gauge string }{
		{"nope", "2.0"},
		{"ztto-tc-02", "9.9"},
		{"", ""},
	}
	for _, tc := range tests {
		if _, err := Lookup(tc.model, tc.gauge); !errors.Is(err, ErrUnknown) {
			t.Errorf("Lookup(%q, %q) error = %v, want ErrUnknown", tc.model, tc.gauge, err)
		}
	}
}

func TestCatalogReturnsIndependentCopies(t *testing.T) {
	a := Catalog()
	a[0].Gauges[0].Points[0].Kgf = -1
	if Catalog()[0].Gauges[0].Points[0].Kgf == -1 {
		t.Fatal("Catalog must not share mutable state between calls")
	}
}
