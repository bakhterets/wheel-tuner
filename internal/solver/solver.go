package solver

import "math"

// SpokeState is the predicted state of a spoke after applying the actions.
type SpokeState struct {
	Index      int
	Side       Side
	TensionKgf float64 // 0 when the spoke tension was not measured
	LateralMM  float64
	RadialMM   float64 // relative to the mean rim radius
}

// Metrics summarise the rim state. Radial values are relative to the mean
// rim radius; lateral values are absolute (dish is preserved).
type Metrics struct {
	MaxLateralMM          float64
	MaxRadialMM           float64
	TensionSpreadLeftKgf  float64 // standard deviation over measured left spokes
	TensionSpreadRightKgf float64 // standard deviation over measured right spokes
	Cost                  float64 // weighted cost without regularisation
}

// Result is the solver output.
type Result struct {
	Actions    []Action     // non-zero adjustments ordered by spoke index
	Turns      []float64    // discrete turns per spoke (index i-1)
	RawTurns   []float64    // continuous optimum per spoke
	Before     Metrics      // state as measured
	After      Metrics      // state predicted after applying Turns
	Predicted  []SpokeState // per-spoke state after applying Turns
	Iterations int          // optimisation passes (successive rounding)
}

// Solve finds the practical nipple turns that minimise the weighted error.
//
// It first solves the continuous weighted least-squares problem, then runs a
// successive rounding loop: the largest free adjustment is snapped to the
// turn grid and fixed, the remaining spokes are re-optimised to compensate,
// until the largest free adjustment falls below Params.MinTurn.
func Solve(w Wheel, ms []Measurement, p Params) (*Result, error) {
	inf, sorted, err := build(w, ms, p)
	if err != nil {
		return nil, err
	}
	pr := newProblem(inf, sorted, p)
	raw, turns, iters, err := pr.optimize(p)
	if err != nil {
		return nil, err
	}
	res := &Result{
		Turns:      turns,
		RawTurns:   raw,
		Before:     pr.metrics(make([]float64, pr.n)),
		After:      pr.metrics(turns),
		Predicted:  pr.predict(turns),
		Iterations: iters,
	}
	for j, t := range turns {
		if t != 0 {
			res.Actions = append(res.Actions, Action{Spoke: j + 1, Side: sorted[j].Side, Turns: t})
		}
	}
	return res, nil
}

// problem is the stacked weighted least-squares system e0 + A·x → 0.
type problem struct {
	n                int
	ms               []Measurement
	a                *Matrix
	w                []float64
	e0               []float64
	latRows, radRows int
	latValueRow      []int // row holding each spoke's entered lateral value
	radValueRow      []int
}

func newProblem(inf *Influence, ms []Measurement, p Params) problem {
	n := len(ms)
	latVals, radVals := make([]float64, n), make([]float64, n)
	latFlags, radFlags := make([]bool, n), make([]bool, n)
	for i, m := range ms {
		latVals[i], radVals[i] = m.LateralMM, m.RadialMM
		latFlags[i], radFlags[i] = m.LateralPeakNext, m.RadialPeakNext
	}
	eL := deviationErrors(latVals, latFlags)
	eR := deviationErrors(radVals, radFlags)
	rad := centerRadial(inf.Radial, inf.RadialPoints, eR)
	eT, wT := tensionErrors(ms, p.Weights.Tension)

	latRows, radRows := len(eL), len(eR)
	w := make([]float64, 0, latRows+radRows+n)
	for range latRows {
		w = append(w, p.Weights.Lateral)
	}
	for range radRows {
		w = append(w, p.Weights.Radial)
	}
	w = append(w, wT...)

	e0 := make([]float64, 0, latRows+radRows+n)
	e0 = append(append(append(e0, eL...), eR...), eT...)

	return problem{
		n:           n,
		ms:          ms,
		a:           stack(inf.Lateral, rad, inf.Tension),
		w:           w,
		e0:          e0,
		latRows:     latRows,
		radRows:     radRows,
		latValueRow: valueRows(inf.LateralPoints, n),
		radValueRow: valueRows(inf.RadialPoints, n),
	}
}

// deviationErrors maps entered values onto matrix rows. A flagged value is
// the peak at i+0.5; the spoke-i row then takes the mean of its neighbours.
func deviationErrors(vals []float64, flags []bool) []float64 {
	n := len(vals)
	out := make([]float64, 0, n)
	for i, v := range vals {
		if !flags[i] {
			out = append(out, v)
			continue
		}
		prev, next := vals[(i-1+n)%n], vals[(i+1)%n]
		out = append(out, (prev+next)/2, v)
	}
	return out
}

// valueRows returns, per spoke, the row index the entered value refers to.
func valueRows(pts []Point, n int) []int {
	rows := make([]int, n)
	for r, pt := range pts {
		rows[pt.Spoke-1] = r // virtual rows follow their spoke row and win
	}
	return rows
}

// centerRadial removes the mean over spoke rows from the radial errors (in
// place) and from every matrix column: only radial runout matters, not the
// absolute rim radius, so uniform tightening must not count as a correction.
func centerRadial(m *Matrix, pts []Point, e []float64) *Matrix {
	rows, cols := m.Dims()
	spokeRows := 0
	var mean float64
	for r, pt := range pts {
		if !pt.Virtual {
			mean += e[r]
			spokeRows++
		}
	}
	mean /= float64(spokeRows)
	for r := range e {
		e[r] -= mean
	}
	out := NewMatrix(rows, cols)
	for j := range cols {
		var cm float64
		for r, pt := range pts {
			if !pt.Virtual {
				cm += m.At(r, j)
			}
		}
		cm /= float64(spokeRows)
		for r := range rows {
			out.Set(r, j, m.At(r, j)-cm)
		}
	}
	return out
}

// tensionErrors returns the deviation of each spoke from its side's mean
// tension and the row weights; unmeasured spokes get zero weight.
func tensionErrors(ms []Measurement, weight float64) (e, w []float64) {
	means := sideMeans(ms, func(i int) float64 { return ms[i].TensionKgf })
	e = make([]float64, len(ms))
	w = make([]float64, len(ms))
	for i, m := range ms {
		if m.TensionKgf > 0 {
			e[i] = m.TensionKgf - means[m.Side]
			w[i] = weight
		}
	}
	return e, w
}

// sideMeans averages value(i) over measured spokes per side.
func sideMeans(ms []Measurement, value func(i int) float64) map[Side]float64 {
	sums := map[Side]float64{}
	counts := map[Side]int{}
	for i, m := range ms {
		if m.TensionKgf > 0 {
			sums[m.Side] += value(i)
			counts[m.Side]++
		}
	}
	means := map[Side]float64{}
	for s, c := range counts {
		means[s] = sums[s] / float64(c)
	}
	return means
}

func (pr problem) optimize(p Params) (raw, turns []float64, iterations int, err error) {
	s := leastSquares{
		a: pr.a, w: pr.w, b: negate(pr.e0),
		ridge: p.Weights.Turns, tol: p.Tolerance, maxIter: p.MaxIterations,
	}
	x := make([]float64, pr.n)
	fixed := make([]bool, pr.n)
	for free := pr.n; free > 0; free-- {
		if _, err := s.solve(x, fixed); err != nil {
			return nil, nil, iterations, err
		}
		iterations++
		if raw == nil {
			raw = append([]float64(nil), x...)
		}
		k := -1
		for j := range x {
			if !fixed[j] && (k < 0 || math.Abs(x[j]) > math.Abs(x[k])) {
				k = j
			}
		}
		q := QuantizeTurns(x[k], p.TurnStep, p.MinTurn)
		if q == 0 {
			for j := range x {
				if !fixed[j] {
					x[j] = 0
				}
			}
			break
		}
		x[k], fixed[k] = q, true
	}
	return raw, x, iterations, nil
}

func negate(v []float64) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = -x
	}
	return out
}

func (pr problem) residual(x []float64) []float64 {
	r := pr.a.MulVec(x)
	for i := range r {
		r[i] += pr.e0[i]
	}
	return r
}

func (pr problem) metrics(x []float64) Metrics {
	r := pr.residual(x)
	var m Metrics
	for i, v := range r {
		m.Cost += pr.w[i] * v * v
		switch {
		case i < pr.latRows:
			m.MaxLateralMM = math.Max(m.MaxLateralMM, math.Abs(v))
		case i < pr.latRows+pr.radRows:
			m.MaxRadialMM = math.Max(m.MaxRadialMM, math.Abs(v))
		}
	}
	tension := pr.tensionAfter(x)
	means := sideMeans(pr.ms, func(i int) float64 { return tension[i] })
	vars := sideMeans(pr.ms, func(i int) float64 {
		d := tension[i] - means[pr.ms[i].Side]
		return d * d
	})
	m.TensionSpreadLeftKgf = math.Sqrt(vars[Left])
	m.TensionSpreadRightKgf = math.Sqrt(vars[Right])
	return m
}

func (pr problem) tensionAfter(x []float64) []float64 {
	off := pr.latRows + pr.radRows
	t := make([]float64, pr.n)
	for i, m := range pr.ms {
		if m.TensionKgf > 0 {
			t[i] = m.TensionKgf + dot(pr.a.row(off+i), x)
		}
	}
	return t
}

// predict reports each spoke's state at the point its entered value refers
// to (i+0.5 for flagged rows). Radial values are relative to the mean radius.
func (pr problem) predict(x []float64) []SpokeState {
	r := pr.residual(x)
	tension := pr.tensionAfter(x)
	out := make([]SpokeState, pr.n)
	for i, m := range pr.ms {
		out[i] = SpokeState{
			Index:      m.Index,
			Side:       m.Side,
			TensionKgf: tension[i],
			LateralMM:  r[pr.latValueRow[i]],
			RadialMM:   r[pr.latRows+pr.radValueRow[i]],
		}
	}
	return out
}
