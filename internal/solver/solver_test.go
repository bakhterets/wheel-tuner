package solver

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"
)

// mockDataset synthesises the readings of a wheel that becomes perfectly true
// after applying truth: deviation = −A·truth, tension = base − T·truth.
func mockDataset(t *testing.T, w Wheel, truth map[int]float64, base float64) []Measurement {
	t.Helper()
	ms := alternating(w.SpokeCount, base)
	inf, err := BuildInfluence(w, ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	x := make([]float64, w.SpokeCount)
	for spoke, turns := range truth {
		x[spoke-1] = turns
	}
	lat, rad, ten := inf.Lateral.MulVec(x), inf.Radial.MulVec(x), inf.Tension.MulVec(x)
	for i := range ms {
		ms[i].LateralMM = -lat[i]
		ms[i].RadialMM = -rad[i]
		if base > 0 {
			ms[i].TensionKgf = base - ten[i]
		}
	}
	return ms
}

func TestSolveConvergesOnMockDatasets(t *testing.T) {
	tests := []struct {
		name  string
		wheel Wheel
		truth map[int]float64
	}{
		{"32h 3x wobble and hop", testWheel(32, 3), map[int]float64{5: 0.5, 6: -0.25, 18: 1, 21: -0.75}},
		{"36h 4x single spoke", testWheel(36, 4), map[int]float64{10: -0.5}},
		{"28h 2x adjacent pair", testWheel(28, 2), map[int]float64{1: 0.25, 2: 0.25, 28: -0.5}},
		{"24h radial", testWheel(24, 0), map[int]float64{7: 0.75, 13: -1.25}},
		{"20h 1x CCW from left", func() Wheel {
			w := testWheel(20, 1)
			w.Direction, w.ViewFrom = CounterClockwise, Left
			return w
		}(), map[int]float64{3: 1.5, 4: -0.25}},
		{"20h 2x every spoke", testWheel(20, 2), func() map[int]float64 {
			m := map[int]float64{}
			for i := 1; i <= 20; i++ {
				m[i] = float64(i%3+1) * 0.25 * float64(1-2*(i/2%2))
			}
			return m
		}()},
	}
	p := DefaultParams()
	p.Weights.Tension = 0 // tension targets shift with the side mean; checked separately
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := mockDataset(t, tc.wheel, tc.truth, 0)
			res, err := Solve(tc.wheel, ms, p)
			if err != nil {
				t.Fatal(err)
			}
			for j, got := range res.Turns {
				if want := tc.truth[j+1]; got != want {
					t.Fatalf("spoke %d: turns %v, want %v (all: %v)", j+1, got, want, res.Turns)
				}
			}
			if len(res.Actions) != len(tc.truth) {
				t.Fatalf("got %d actions, want %d", len(res.Actions), len(tc.truth))
			}
			for k, a := range res.Actions {
				if a.Turns != tc.truth[a.Spoke] || a.Side != ms[a.Spoke-1].Side {
					t.Fatalf("action %+v does not match truth", a)
				}
				if k > 0 && a.Spoke <= res.Actions[k-1].Spoke {
					t.Fatal("actions must be ordered by spoke index")
				}
			}
			if res.After.MaxLateralMM > 1e-6 || res.After.MaxRadialMM > 1e-6 {
				t.Fatalf("residual runout lateral=%v radial=%v", res.After.MaxLateralMM, res.After.MaxRadialMM)
			}
			if !(res.After.Cost < res.Before.Cost) {
				t.Fatalf("cost did not decrease: %v → %v", res.Before.Cost, res.After.Cost)
			}
			for _, s := range res.Predicted {
				if math.Abs(s.LateralMM) > 1e-6 || math.Abs(s.RadialMM) > 1e-6 {
					t.Fatalf("predicted spoke %d not true: %+v", s.Index, s)
				}
			}
			if res.Iterations < 1 || res.Iterations > tc.wheel.SpokeCount+1 {
				t.Fatalf("iterations = %d", res.Iterations)
			}
			for j, raw := range res.RawTurns {
				if math.Abs(raw-tc.truth[j+1]) > 0.01 {
					t.Fatalf("raw turns %d = %v, want ≈ %v", j+1, raw, tc.truth[j+1])
				}
			}
		})
	}
}

func TestSolveBalancesTension(t *testing.T) {
	w := testWheel(32, 3)
	ms := alternating(32, 100)
	for i := range ms {
		if ms[i].Side == Right {
			ms[i].TensionKgf = 130
		}
	}
	ms[4].TensionKgf = 80   // loose left spoke
	ms[9].TensionKgf = 160  // tight right spoke
	ms[20].TensionKgf = 115 // tight left spoke
	p := DefaultParams()
	p.Weights.Lateral, p.Weights.Radial = 0, 0
	res, err := Solve(w, ms, p)
	if err != nil {
		t.Fatal(err)
	}
	if !(res.After.TensionSpreadLeftKgf < res.Before.TensionSpreadLeftKgf/2) ||
		!(res.After.TensionSpreadRightKgf < res.Before.TensionSpreadRightKgf/2) {
		t.Fatalf("spread not reduced: before %+v after %+v", res.Before, res.After)
	}
	if !(res.Turns[4] > 0) || !(res.Turns[9] < 0) || !(res.Turns[20] < 0) {
		t.Fatalf("expected tighten 5, loosen 10 and 21: %v", res.Turns)
	}
	if !near(res.Predicted[4].TensionKgf, 100, 5) {
		t.Fatalf("spoke 5 predicted %v kgf, want ≈100", res.Predicted[4].TensionKgf)
	}
}

func TestSolveTensionLimit(t *testing.T) {
	// Spoke 5 is loose and lateral runout asks for tightening; the left limit
	// sits below the balanced tension, so every measured left spoke must end
	// at or under it while right spokes are free.
	tests := []struct {
		name        string
		left, right float64
		binding     bool
	}{
		{"no limit", 0, 0, false},
		{"loose limit is inactive", 200, 200, false},
		{"left limit below the measured tension", 95, 0, true},
		{"both sides limited", 95, 128, true},
	}
	base := func() []Measurement {
		w := testWheel(32, 3)
		ms := mockDataset(t, w, map[int]float64{5: 1, 12: 0.5}, 100)
		for i := range ms {
			if ms[i].Side == Right {
				ms[i].TensionKgf += 30
			}
		}
		ms[30].TensionKgf = 0 // unmeasured spokes are never limited
		return ms
	}
	free, err := Solve(testWheel(32, 3), base(), DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := testWheel(32, 3)
			w.TensionLimitLeftKgf, w.TensionLimitRightKgf = tc.left, tc.right
			res, err := Solve(w, base(), DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range res.Predicted {
				if limit := w.tensionLimit(s.Side); limit > 0 && s.TensionKgf > limit+limitTolKgf {
					t.Fatalf("spoke %d predicted %.2f kgf over the %v limit %.0f", s.Index, s.TensionKgf, s.Side, limit)
				}
			}
			same := true
			for j := range res.Turns {
				same = same && res.Turns[j] == free.Turns[j]
			}
			if same == tc.binding {
				t.Fatalf("binding=%v but turns %v vs unlimited %v", tc.binding, res.Turns, free.Turns)
			}
			if res.Predicted[30].TensionKgf != 0 {
				t.Fatalf("unmeasured spoke predicted %v kgf", res.Predicted[30].TensionKgf)
			}
		})
	}
}

func TestSolveNearLimitStillImproves(t *testing.T) {
	// Every spoke sits 2 kgf under its cap, so doing nothing is feasible. A
	// cap row left active once its spoke fell below the cap used to pull the
	// tension up towards it and made the result worse than no action.
	w := Wheel{SpokeCount: 32, ERDMM: 600, Cross: 3, FlangeLeftMM: 35, FlangeRightMM: 20, Direction: CounterClockwise, ViewFrom: Right}
	w.TensionLimitLeftKgf, w.TensionLimitRightKgf = 112, 112
	ms := alternating(32, 110)
	for i := range ms {
		d := float64(i+1) - 9.5
		ms[i].LateralMM = 0.8 * math.Exp(-d*d/4.5)
	}
	res, err := Solve(w, ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if !(res.After.Cost < res.Before.Cost/2) || !(res.After.MaxLateralMM < res.Before.MaxLateralMM*0.6) {
		t.Fatalf("cost %.2f → %.2f, wobble %.2f → %.2f mm", res.Before.Cost, res.After.Cost, res.Before.MaxLateralMM, res.After.MaxLateralMM)
	}
	for _, s := range res.Predicted {
		if s.TensionKgf > 112+limitTolKgf {
			t.Fatalf("spoke %d predicted %.2f kgf over the cap", s.Index, s.TensionKgf)
		}
	}
}

func TestCapTurn(t *testing.T) {
	// Spoke 1 drives itself by 10 kgf/turn and, through cross-talk, spoke 3
	// by 4 kgf/turn; spoke 2 drives only itself.
	pr := problem{n: 3, tension: NewMatrix(3, 3), ms: []Measurement{{TensionKgf: 100}, {TensionKgf: 100}, {TensionKgf: 100}}}
	pr.tension.Set(0, 0, 10)
	pr.tension.Set(2, 0, 4)
	pr.tension.Set(1, 1, 10)
	own := []float64{103, 0, 0}
	cross := []float64{0, 0, 101}
	tests := []struct {
		name   string
		caps   []float64
		fixed  []bool
		k      int
		raw, q float64
		want   float64
	}{
		{"rounding up over the cap steps down", own, nil, 0, 0.2, 0.5, 0.25},
		{"rounding up under the cap is kept", own, nil, 0, 0.05, 0.25, 0.25},
		{"rounding down still over the cap steps further", own, nil, 0, 0.6, 0.5, 0.25},
		{"uncapped spoke", own, nil, 1, 0.2, 0.5, 0.5},
		{"rounding up pushes a fixed spoke over its cap", cross, []bool{false, false, true}, 0, 0.2, 0.5, 0.25},
		{"rounding down is not blamed for a fixed spoke", cross, []bool{false, false, true}, 0, 0.6, 0.5, 0.5},
		{"free spokes are left to the re-optimisation", cross, nil, 0, 0.2, 0.5, 0.5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixed := tc.fixed
			if fixed == nil {
				fixed = make([]bool, pr.n)
			}
			x := make([]float64, pr.n)
			x[tc.k] = tc.raw
			if got := pr.capTurn(tc.caps, x, fixed, tc.k, tc.q, 0.25); got != tc.want {
				t.Fatalf("capTurn = %v, want %v", got, tc.want)
			}
			if x[tc.k] != tc.raw {
				t.Fatal("capTurn must not modify x")
			}
		})
	}
}

func TestTensionRowsFollowSideMean(t *testing.T) {
	w := testWheel(32, 3)
	ms := alternating(32, 100)
	for i := range ms {
		if ms[i].Side == Right {
			ms[i].TensionKgf = 130
		}
	}
	ms[4].TensionKgf = 80
	ms[7].TensionKgf = 0 // unmeasured spokes stay out of the side means
	p := DefaultParams()
	inf, sorted, err := build(w, ms, p)
	if err != nil {
		t.Fatal(err)
	}
	pr := newProblem(inf, sorted, p)
	off := pr.latRows + pr.radRows
	tensionCost := func(x []float64) float64 {
		var c float64
		for i, v := range pr.residual(x)[off:] {
			c += pr.w[off+i] * v * v
		}
		return c
	}

	x := make([]float64, pr.n)
	for i := range x {
		x[i] = 0.1 * float64(i%5)
	}
	tension := pr.tensionAfter(x)
	means := sideMeans(pr.ms, func(i int) float64 { return tension[i] })
	r := pr.residual(x)
	for i, m := range pr.ms {
		want := 0.0
		if m.TensionKgf > 0 {
			want = tension[i] - means[m.Side]
		}
		if !near(r[off+i], want, 1e-9) {
			t.Fatalf("spoke %d: tension residual %v, want deviation from the new side mean %v", i+1, r[off+i], want)
		}
	}

	uniform := make([]float64, pr.n)
	for i, m := range pr.ms {
		if m.Side == Left {
			uniform[i] = 0.5
		}
	}
	before, after := tensionCost(make([]float64, pr.n)), tensionCost(uniform)
	if after > before+1e-9 {
		t.Fatalf("uniform left tightening raised the tension cost %v → %v", before, after)
	}
	if got := pr.tensionAfter(uniform)[0]; !(got > ms[0].TensionKgf) {
		t.Fatalf("absolute tension must still rise: %v", got)
	}
}

func TestSolveGeometryDominatesTensionOutlier(t *testing.T) {
	w := Wheel{SpokeCount: 32, ERDMM: 600, Cross: 3, FlangeLeftMM: 35, FlangeRightMM: 20, Direction: CounterClockwise, ViewFrom: Left}
	ms := alternating(32, 100)
	for i := range ms {
		if ms[i].Side == Left {
			ms[i].TensionKgf = 115
		}
		d := float64(i+1) - 9.5
		ms[i].LateralMM = math.Round(80*math.Exp(-d*d/4.5)) / 100
	}
	ms[4].TensionKgf = 140
	res, err := Solve(w, ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if !(res.After.MaxLateralMM < res.Before.MaxLateralMM/2) {
		t.Fatalf("wobble %.2f → %.2f mm: tension prior overwhelms geometry", res.Before.MaxLateralMM, res.After.MaxLateralMM)
	}
	if !(res.After.TensionSpreadLeftKgf < res.Before.TensionSpreadLeftKgf) {
		t.Fatalf("left spread %.1f → %.1f kgf", res.Before.TensionSpreadLeftKgf, res.After.TensionSpreadLeftKgf)
	}
}

func TestSolveIsolatedHopTightensHighSpot(t *testing.T) {
	w := Wheel{SpokeCount: 32, ERDMM: 600, Cross: 3, FlangeLeftMM: 35, FlangeRightMM: 20, Direction: CounterClockwise, ViewFrom: Left}
	ms := alternating(32, 100)
	ms[19].RadialMM = 0.4
	res, err := Solve(w, ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if !(res.After.MaxRadialMM < res.Before.MaxRadialMM*0.7) {
		t.Fatalf("hop %.2f → %.2f mm", res.Before.MaxRadialMM, res.After.MaxRadialMM)
	}
	if !(res.Turns[19] > 0) || res.After.MaxLateralMM > 0.2 {
		t.Fatalf("expected tightening at spoke 20 without wobble: turns %v, lateral %.2f", res.Turns[16:24], res.After.MaxLateralMM)
	}
}

// Guards the stage-02 force-model calibration: with the stage-01 kinematic
// model the hop in this mix improved by only ≈10 %.
func TestSolveMixedWobbleAndHop(t *testing.T) {
	w := Wheel{SpokeCount: 32, ERDMM: 600, Cross: 3, FlangeLeftMM: 35, FlangeRightMM: 20, Direction: CounterClockwise, ViewFrom: Left}
	ms := alternating(32, 100)
	for i := range ms {
		if ms[i].Side == Left {
			ms[i].TensionKgf = 115
		}
		d := float64(i+1) - 9.5
		ms[i].LateralMM = math.Round(80*math.Exp(-d*d/4.5)) / 100
	}
	ms[4].TensionKgf = 140
	ms[13].RadialMM = 0.4
	res, err := Solve(w, ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if !(res.After.MaxLateralMM < res.Before.MaxLateralMM/2) || !(res.After.MaxRadialMM < res.Before.MaxRadialMM*0.75) {
		t.Fatalf("wobble %.2f → %.2f mm, hop %.2f → %.2f mm", res.Before.MaxLateralMM, res.After.MaxLateralMM,
			res.Before.MaxRadialMM, res.After.MaxRadialMM)
	}
}

func TestSolveTensionMetrics(t *testing.T) {
	ms := alternating(20, 0)
	ms[0].TensionKgf, ms[2].TensionKgf = 90, 110 // left: σ = 10
	ms[1].TensionKgf = 120                       // right: single spoke, σ = 0
	res, err := Solve(testWheel(20, 2), ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if !near(res.Before.TensionSpreadLeftKgf, 10, eps) || res.Before.TensionSpreadRightKgf != 0 {
		t.Fatalf("before metrics %+v", res.Before)
	}
	for i, s := range res.Predicted {
		if ms[i].TensionKgf == 0 && s.TensionKgf != 0 {
			t.Fatalf("unmeasured spoke %d predicted %v kgf", i+1, s.TensionKgf)
		}
	}
}

func TestSolveIgnoresUniformRadialOffset(t *testing.T) {
	ms := alternating(32, 0)
	for i := range ms {
		ms[i].RadialMM = 2
	}
	res, err := Solve(testWheel(32, 3), ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Actions) != 0 || res.Before.MaxRadialMM != 0 {
		t.Fatalf("uniform radius is not runout: %+v", res)
	}
}

func TestSolveTrueWheelNeedsNoAction(t *testing.T) {
	res, err := Solve(testWheel(36, 3), alternating(36, 110), DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Actions) != 0 || res.Before.Cost != 0 || res.After.Cost != 0 {
		t.Fatalf("true wheel produced %+v", res)
	}
}

func TestSolveDropsMicroAdjustments(t *testing.T) {
	w := testWheel(32, 3)
	ms := mockDataset(t, w, map[int]float64{8: 0.05}, 0)
	res, err := Solve(w, ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Actions) != 0 {
		t.Fatalf("micro adjustment produced actions %v", res.Actions)
	}
	if !(math.Abs(res.RawTurns[7]) > 0.01) {
		t.Fatalf("raw solution lost the adjustment: %v", res.RawTurns[7])
	}
}

func TestSolveInterSpokePeak(t *testing.T) {
	// A smooth lateral bump (towards the right flange) peaks between spokes
	// 9 (L) and 10 (R). The mechanic records the peak on spoke 9 and flags it.
	const n = 32
	bump := func(pos float64) float64 {
		d := math.Mod(math.Abs(pos-9.5), n)
		d = math.Min(d, n-d) / 1.5
		return math.Exp(-d * d)
	}
	ms := alternating(n, 0)
	for i := range ms {
		ms[i].LateralMM = bump(float64(i + 1))
	}
	ms[8].LateralMM, ms[8].LateralPeakNext = bump(9.5), true

	res, err := Solve(testWheel(n, 3), ms, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if !(res.After.MaxLateralMM < res.Before.MaxLateralMM/4) {
		t.Fatalf("lateral runout %v → %v", res.Before.MaxLateralMM, res.After.MaxLateralMM)
	}
	if math.Abs(res.Predicted[8].LateralMM) > 0.2 {
		t.Fatalf("peak at 9.5 not corrected: %v", res.Predicted[8].LateralMM)
	}
	// The pull towards the left is shared by both spokes around the peak.
	if !(res.Turns[8] > 0) || !(res.Turns[9] < 0) {
		t.Fatalf("want tighten L9 and loosen R10, got %v", res.Turns)
	}
}

func TestDeviationErrors(t *testing.T) {
	tests := []struct {
		name  string
		vals  []float64
		flags []bool
		want  []float64
	}{
		{"no flags", []float64{1, 2, 3}, []bool{false, false, false}, []float64{1, 2, 3}},
		{"middle flag", []float64{1, 5, 3}, []bool{false, true, false}, []float64{1, 11.0 / 3, 5, 3}},
		{"wrap flags", []float64{4, 0, 2}, []bool{true, false, true}, []float64{3, 4, 0, 4.0 / 3, 2}},
		{"consecutive flags", []float64{0, 2, 6}, []bool{false, true, true}, []float64{0, 4.0 / 3, 2, 4, 6}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deviationErrors(tc.vals, tc.flags)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestSolveNeverIncreasesCost(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := range 50 {
		spokes := []int{20, 24, 28, 32, 36}[trial%5]
		w := testWheel(spokes, trial%3)
		ms := alternating(spokes, 0)
		for i := range ms {
			ms[i].LateralMM = rng.NormFloat64() * 0.8
			ms[i].RadialMM = rng.NormFloat64() * 0.4
			ms[i].TensionKgf = 100 + rng.NormFloat64()*15
			ms[i].LateralPeakNext = rng.IntN(6) == 0
			ms[i].RadialPeakNext = rng.IntN(6) == 0
		}
		res, err := Solve(w, ms, DefaultParams())
		if err != nil {
			t.Fatalf("trial %d: %v", trial, err)
		}
		if res.After.Cost > res.Before.Cost {
			t.Fatalf("trial %d: cost increased %v → %v", trial, res.Before.Cost, res.After.Cost)
		}
		for _, a := range res.Actions {
			if q := a.Turns / 0.25; q != math.Trunc(q) {
				t.Fatalf("trial %d: action %v is not on the quarter-turn grid", trial, a)
			}
		}
	}
}

func TestSolveErrors(t *testing.T) {
	_, err := Solve(testWheel(32, 3), alternating(10, 100), DefaultParams())
	requireInvalid(t, err)

	p := DefaultParams()
	p.MaxIterations = 1
	ms := mockDataset(t, testWheel(32, 3), map[int]float64{3: 1}, 0)
	if _, err := Solve(testWheel(32, 3), ms, p); !errors.Is(err, ErrNoConvergence) {
		t.Fatalf("want ErrNoConvergence, got %v", err)
	}
}
