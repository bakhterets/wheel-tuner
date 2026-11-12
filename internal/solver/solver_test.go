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
	if !(res.After.MaxRadialMM < res.Before.MaxRadialMM*0.85) {
		t.Fatalf("hop %.2f → %.2f mm", res.Before.MaxRadialMM, res.After.MaxRadialMM)
	}
	if !(res.Turns[19] > 0) || res.After.MaxLateralMM > 0.2 {
		t.Fatalf("expected tightening at spoke 20 without wobble: turns %v, lateral %.2f", res.Turns[16:24], res.After.MaxLateralMM)
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
		{"middle flag", []float64{1, 5, 3}, []bool{false, true, false}, []float64{1, 2, 5, 3}},
		{"wrap flags", []float64{4, 0, 2}, []bool{true, false, true}, []float64{1, 4, 0, 2, 2}},
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
