package solver

import (
	"math"
	"testing"
)

func TestParamsValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Params)
		ok     bool
	}{
		{"defaults", func(*Params) {}, true},
		{"no regularisation", func(p *Params) { p.Weights.Turns = 0 }, true},
		{"zero pitch", func(p *Params) { p.ThreadPitchMM = 0 }, false},
		{"NaN tension per turn", func(p *Params) { p.TensionPerTurnKgf = math.NaN() }, false},
		{"infinite decay", func(p *Params) { p.LateralDecayMM = math.Inf(1) }, false},
		{"negative radial compliance", func(p *Params) { p.RadialCompliance = -1 }, false},
		{"zero turn step", func(p *Params) { p.TurnStep = 0 }, false},
		{"zero tolerance", func(p *Params) { p.Tolerance = 0 }, false},
		{"negative cross talk", func(p *Params) { p.CrossTalk = -0.1 }, false},
		{"NaN coupling", func(p *Params) { p.NeighborCoupling = math.NaN() }, false},
		{"infinite weight", func(p *Params) { p.Weights.Lateral = math.Inf(1) }, false},
		{"negative max iterations", func(p *Params) { p.MaxIterations = -1 }, false},
		{"all weights zero", func(p *Params) { p.Weights = Weights{Turns: 1} }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultParams()
			tc.mutate(&p)
			err := p.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok {
				requireInvalid(t, err)
			}
		})
	}
}

func TestMatrix(t *testing.T) {
	m := NewMatrix(2, 3)
	if r, c := m.Dims(); r != 2 || c != 3 {
		t.Fatalf("Dims = %d×%d", r, c)
	}
	for i := range 2 {
		for j := range 3 {
			m.Set(i, j, float64(i*3+j+1))
		}
	}
	row := m.Row(1)
	row[0] = 100
	if m.At(1, 0) != 4 {
		t.Fatal("Row must return a copy")
	}
	got := m.MulVec([]float64{1, 0, -1})
	if got[0] != -2 || got[1] != -2 {
		t.Fatalf("MulVec = %v", got)
	}
	s := stack(m, m)
	if r, c := s.Dims(); r != 4 || c != 3 || s.At(3, 2) != 6 {
		t.Fatalf("stack = %d×%d, last = %v", r, c, s.At(3, 2))
	}
}
