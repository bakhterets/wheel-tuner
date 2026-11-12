package solver

import "math"

// leastSquares minimises Σ w_r·(b_r − A_r·x)² + ridge·Σ x_f² over the free
// components of x, keeping fixed components unchanged.
type leastSquares struct {
	a       *Matrix
	w       []float64
	b       []float64
	ridge   float64
	tol     float64
	maxIter int // 0 selects 10·n+10
}

// solve updates the free entries of x in place and returns the number of
// conjugate gradient iterations performed.
func (s leastSquares) solve(x []float64, fixed []bool) (int, error) {
	var free []int
	for j, f := range fixed {
		if !f {
			free = append(free, j)
		}
	}
	if len(free) == 0 {
		return 0, nil
	}
	// Residual target after removing the contribution of fixed variables.
	rhsRows := make([]float64, s.a.rows)
	for r := range rhsRows {
		row := s.a.row(r)
		v := s.b[r]
		for j, f := range fixed {
			if f {
				v -= row[j] * x[j]
			}
		}
		rhsRows[r] = v
	}
	nf := len(free)
	normal := make([][]float64, nf)
	rhs := make([]float64, nf)
	for p, j := range free {
		normal[p] = make([]float64, nf)
		for q := p; q < nf; q++ {
			k := free[q]
			var v float64
			for r := range s.a.rows {
				v += s.w[r] * s.a.At(r, j) * s.a.At(r, k)
			}
			normal[p][q] = v
		}
		normal[p][p] += s.ridge
		for r := range s.a.rows {
			rhs[p] += s.w[r] * s.a.At(r, j) * rhsRows[r]
		}
	}
	for p := range nf {
		for q := range p {
			normal[p][q] = normal[q][p]
		}
	}
	y := make([]float64, nf)
	maxIter := s.maxIter
	if maxIter == 0 {
		maxIter = 10*nf + 10
	}
	iters, err := conjugateGradient(normal, rhs, y, s.tol, maxIter)
	if err != nil {
		return iters, err
	}
	for p, j := range free {
		x[j] = y[p]
	}
	return iters, nil
}

// conjugateGradient solves the symmetric positive definite system a·x = b.
// x is overwritten; the starting guess is zero.
func conjugateGradient(a [][]float64, b, x []float64, tol float64, maxIter int) (int, error) {
	n := len(b)
	clear(x)
	r := make([]float64, n)
	copy(r, b)
	p := make([]float64, n)
	copy(p, r)
	ap := make([]float64, n)
	rs := dot(r, r)
	bNorm := math.Sqrt(rs)
	if bNorm == 0 {
		return 0, nil
	}
	for k := 1; k <= maxIter; k++ {
		for i := range n {
			ap[i] = dot(a[i], p)
		}
		pap := dot(p, ap)
		if !(pap > 0) {
			return k, ErrNoConvergence
		}
		alpha := rs / pap
		for i := range n {
			x[i] += alpha * p[i]
			r[i] -= alpha * ap[i]
		}
		rsNew := dot(r, r)
		if math.Sqrt(rsNew) <= tol*bNorm {
			return k, nil
		}
		beta := rsNew / rs
		for i := range n {
			p[i] = r[i] + beta*p[i]
		}
		rs = rsNew
	}
	return maxIter, ErrNoConvergence
}
