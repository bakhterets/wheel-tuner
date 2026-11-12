package solver

import (
	"errors"
	"testing"
)

func TestConjugateGradient(t *testing.T) {
	spd := [][]float64{{4, 1, 0}, {1, 3, 1}, {0, 1, 2}}
	tests := []struct {
		name    string
		a       [][]float64
		b       []float64
		maxIter int
		tol     float64
		want    []float64
		wantErr error
	}{
		{"SPD system", spd, []float64{5, 5, 3}, 10, 1e-12, []float64{1, 1, 1}, nil},
		{"zero rhs", spd, []float64{0, 0, 0}, 10, 1e-12, []float64{0, 0, 0}, nil},
		{"not positive definite", [][]float64{{-1, 0}, {0, -1}}, []float64{1, 1}, 10, 1e-12, nil, ErrNoConvergence},
		{"iteration budget exhausted", spd, []float64{5, 5, 3}, 1, 1e-12, nil, ErrNoConvergence},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := []float64{9, 9, 9}[:len(tc.b)]
			_, err := conjugateGradient(tc.a, tc.b, x, tc.tol, tc.maxIter)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			for i := range x {
				if !near(x[i], tc.want[i], 1e-9) {
					t.Fatalf("x = %v, want %v", x, tc.want)
				}
			}
		})
	}
}

func TestLeastSquaresFixedVariables(t *testing.T) {
	a := NewMatrix(2, 2)
	a.Set(0, 0, 1)
	a.Set(1, 1, 1)
	a.Set(1, 0, 1)
	s := leastSquares{a: a, w: []float64{1, 1}, b: []float64{1, 3}, tol: 1e-12}

	x := []float64{0, 0}
	if _, err := s.solve(x, []bool{false, false}); err != nil {
		t.Fatal(err)
	}
	if !near(x[0], 1, 1e-9) || !near(x[1], 2, 1e-9) {
		t.Fatalf("free solve = %v, want [1 2]", x)
	}

	x = []float64{0.5, 0}
	if _, err := s.solve(x, []bool{true, false}); err != nil {
		t.Fatal(err)
	}
	if x[0] != 0.5 || !near(x[1], 2.5, 1e-9) {
		t.Fatalf("fixed solve = %v, want [0.5 2.5]", x)
	}

	x = []float64{0.5, 0.25}
	iters, err := s.solve(x, []bool{true, true})
	if err != nil || iters != 0 || x[0] != 0.5 || x[1] != 0.25 {
		t.Fatalf("all fixed: iters=%d err=%v x=%v", iters, err, x)
	}
}
