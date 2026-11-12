package solver

import (
	"math"
	"slices"
	"testing"
)

func TestBuildInfluenceDimensions(t *testing.T) {
	tests := []struct {
		name         string
		spokes       int
		latFlags     []int // 1-based spokes flagged lateral
		radFlags     []int
		wantLatRows  int
		wantRadRows  int
		wantVirtual  []float64 // lateral virtual positions
		wantRadVirtu []float64
	}{
		{"20 no flags", 20, nil, nil, 20, 20, nil, nil},
		{"32 one lateral", 32, []int{5}, nil, 33, 32, []float64{5.5}, nil},
		{"36 mixed", 36, []int{1, 36}, []int{10}, 38, 37, []float64{1.5, 36.5}, []float64{10.5}},
		{"24 every spoke", 24, seq(24), seq(24), 48, 48, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := alternating(tc.spokes, 100)
			for _, i := range tc.latFlags {
				ms[i-1].LateralPeakNext = true
			}
			for _, i := range tc.radFlags {
				ms[i-1].RadialPeakNext = true
			}
			inf, err := BuildInfluence(testWheel(tc.spokes, 0), ms, DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			checkDims(t, "lateral", inf.Lateral, inf.LateralPoints, tc.wantLatRows, tc.spokes)
			checkDims(t, "radial", inf.Radial, inf.RadialPoints, tc.wantRadRows, tc.spokes)
			if r, c := inf.Tension.Dims(); r != tc.spokes || c != tc.spokes {
				t.Errorf("tension dims %d×%d", r, c)
			}
			if tc.wantVirtual != nil && !slices.Equal(virtualPositions(inf.LateralPoints), tc.wantVirtual) {
				t.Errorf("lateral virtual = %v, want %v", virtualPositions(inf.LateralPoints), tc.wantVirtual)
			}
			if tc.wantRadVirtu != nil && !slices.Equal(virtualPositions(inf.RadialPoints), tc.wantRadVirtu) {
				t.Errorf("radial virtual = %v, want %v", virtualPositions(inf.RadialPoints), tc.wantRadVirtu)
			}
		})
	}
}

func seq(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i + 1
	}
	return s
}

func checkDims(t *testing.T, name string, m *Matrix, pts []Point, rows, cols int) {
	t.Helper()
	if r, c := m.Dims(); r != rows || c != cols || len(pts) != rows {
		t.Errorf("%s dims %d×%d with %d points, want %d×%d", name, r, c, len(pts), rows, cols)
	}
}

func virtualPositions(pts []Point) []float64 {
	var out []float64
	for _, p := range pts {
		if p.Virtual {
			out = append(out, p.Position)
		}
	}
	return out
}

func TestInterpolationPoints(t *testing.T) {
	tests := []struct {
		name  string
		spoke int // flagged spoke, 1-based
		next  int // expected next spoke, 1-based
	}{
		{"first", 1, 2},
		{"middle", 14, 15},
		{"wraps around", 32, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := alternating(32, 100)
			ms[tc.spoke-1].LateralPeakNext = true
			ms[tc.spoke-1].RadialPeakNext = true
			inf, err := BuildInfluence(testWheel(32, 3), ms, DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range []struct {
				name string
				m    *Matrix
				pts  []Point
			}{{"lateral", inf.Lateral, inf.LateralPoints}, {"radial", inf.Radial, inf.RadialPoints}} {
				vr := slices.IndexFunc(c.pts, func(p Point) bool { return p.Virtual })
				pt := c.pts[vr]
				if pt.Spoke != tc.spoke || pt.Position != float64(tc.spoke)+0.5 {
					t.Fatalf("%s virtual point = %+v", c.name, pt)
				}
				if vr != tc.spoke {
					t.Fatalf("%s virtual row %d must follow spoke row %d", c.name, vr, tc.spoke-1)
				}
				nextRow := tc.next - 1
				if tc.next > tc.spoke {
					nextRow++ // shifted by the virtual row
				}
				for j := range 32 {
					want := (c.m.At(tc.spoke-1, j) + c.m.At(nextRow, j)) / 2
					if !near(c.m.At(vr, j), want, eps) {
						t.Fatalf("%s A[%v][%d] = %v, want %v", c.name, pt.Position, j, c.m.At(vr, j), want)
					}
				}
			}
		})
	}
}

func TestCrossPartners(t *testing.T) {
	tests := []struct {
		name   string
		spokes int
		cross  int
		dir    Direction
		view   Side
		spoke  int   // 1-based spoke under test
		want   []int // 1-based crossing partners
	}{
		{"radial has no crossings", 32, 0, Clockwise, Right, 1, []int{}},
		{"1x CW leading", 32, 1, Clockwise, Right, 1, []int{3}},
		{"2x CW leading", 32, 2, Clockwise, Right, 1, []int{3, 7}},
		{"3x CW leading", 32, 3, Clockwise, Right, 1, []int{3, 7, 11}},
		{"3x CW trailing", 32, 3, Clockwise, Right, 3, []int{1, 25, 29}},
		{"3x CCW mirrors", 32, 3, CounterClockwise, Right, 1, []int{23, 27, 31}},
		{"3x CCW from left equals CW from right", 32, 3, CounterClockwise, Left, 1, []int{3, 7, 11}},
		{"3x CW from left mirrors", 32, 3, Clockwise, Left, 1, []int{23, 27, 31}},
		{"3x right flange", 32, 3, Clockwise, Right, 2, []int{4, 8, 12}},
		{"4x on 36", 36, 4, Clockwise, Right, 1, []int{3, 7, 11, 15}},
		{"2x on 20 wraps", 20, 2, Clockwise, Right, 3, []int{1, 17}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := testWheel(tc.spokes, tc.cross)
			w.Direction, w.ViewFrom = tc.dir, tc.view
			inf, err := BuildInfluence(w, alternating(tc.spokes, 100), DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			got := inf.CrossPartners[tc.spoke-1]
			if !slices.Equal(got, tc.want) {
				t.Fatalf("partners of %d = %v, want %v", tc.spoke, got, tc.want)
			}
			for i, ps := range inf.CrossPartners {
				if len(ps) != tc.cross {
					t.Fatalf("spoke %d crosses %d spokes, want %d", i+1, len(ps), tc.cross)
				}
				for _, j := range ps {
					if !slices.Contains(inf.CrossPartners[j-1], i+1) {
						t.Fatalf("crossing %d→%d is not symmetric", i+1, j)
					}
					if (i+1-j)%2 != 0 {
						t.Fatalf("spoke %d crosses %d on the other flange", i+1, j)
					}
				}
			}
		})
	}
}

func TestTensionCrossTalkCoefficients(t *testing.T) {
	const t0, ct = 20.0, 0.1
	tests := []struct {
		name  string
		i, j  int // 1-based row (affected) and column (turned)
		cross int
		want  float64
	}{
		{"own spoke", 1, 1, 3, t0},
		{"crossing partner", 1, 3, 3, t0 * ct},
		{"far crossing partner", 1, 11, 3, t0 * ct},
		{"partner is symmetric", 11, 1, 3, t0 * ct},
		{"same flange non-partner", 1, 5, 3, 0},
		{"opposite flange", 1, 2, 3, 0},
		{"radial lacing has no cross talk", 1, 3, 0, 0},
	}
	p := DefaultParams()
	p.TensionPerTurnKgf, p.CrossTalk, p.NeighborCoupling = t0, ct, 0
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inf, err := BuildInfluence(testWheel(32, tc.cross), alternating(32, 100), p)
			if err != nil {
				t.Fatal(err)
			}
			if got := inf.Tension.At(tc.i-1, tc.j-1); !near(got, tc.want, eps) {
				t.Fatalf("T[%d][%d] = %v, want %v", tc.i, tc.j, got, tc.want)
			}
		})
	}
}

func TestTensionNeighborCoupling(t *testing.T) {
	p := DefaultParams()
	p.CrossTalk = 0
	inf, err := BuildInfluence(testWheel(32, 0), alternating(32, 100), p)
	if err != nil {
		t.Fatal(err)
	}
	opp, same := inf.Tension.At(1, 0), inf.Tension.At(2, 0)
	if !(opp > 0) {
		t.Errorf("opposite-side neighbour must gain tension, got %v", opp)
	}
	if !(same < 0) {
		t.Errorf("same-side neighbour must lose tension, got %v", same)
	}
	if !(opp > -same) {
		t.Errorf("coupling must decay with distance: |%v| <= |%v|", opp, same)
	}
}

func TestDeviationSignsAndAttenuation(t *testing.T) {
	inf, err := BuildInfluence(testWheel(32, 3), alternating(32, 100), DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	left, right := inf.Lateral.At(0, 0), inf.Lateral.At(1, 1)
	if !(left < 0 && right > 0) {
		t.Fatalf("left spoke must pull left, right spoke right: %v, %v", left, right)
	}
	// The right flange sits closer to the centre (20 vs 35 mm): a smaller
	// bracing angle moves the rim further laterally per turn.
	if !(right > -left) {
		t.Errorf("right lateral gain %v must exceed left %v", right, -left)
	}
	for i := 1; i < 16; i++ {
		if !(math.Abs(inf.Lateral.At(i, 0)) < math.Abs(inf.Lateral.At(i-1, 0))) {
			t.Fatalf("lateral influence must decay with distance at %d", i)
		}
		if !(inf.Radial.At(i, 0) < 0) || !(inf.Radial.At(i, 0) > inf.Radial.At(i-1, 0)) {
			t.Fatalf("radial influence must pull inwards and decay at %d", i)
		}
	}
	if !near(inf.Lateral.At(1, 0), inf.Lateral.At(31, 0), eps) {
		t.Error("attenuation must be symmetric around the rim")
	}
}

func TestERDAttenuation(t *testing.T) {
	tests := []struct {
		small, large float64
	}{
		{400, 600},
		{540, 620},
	}
	ratio := func(erd float64) float64 {
		w := testWheel(32, 3)
		w.ERDMM = erd
		inf, err := BuildInfluence(w, alternating(32, 100), DefaultParams())
		if err != nil {
			t.Fatal(err)
		}
		return inf.Lateral.At(2, 0) / inf.Lateral.At(0, 0)
	}
	for _, tc := range tests {
		if !(ratio(tc.large) < ratio(tc.small)) {
			t.Errorf("ERD %v must attenuate faster than %v", tc.large, tc.small)
		}
	}
}

func TestBuildInfluenceRejectsInvalidInput(t *testing.T) {
	bad := DefaultParams()
	bad.TurnStep = 0
	w := testWheel(32, 3)
	tests := []struct {
		name string
		w    Wheel
		ms   []Measurement
		p    Params
	}{
		{"wheel", Wheel{}, alternating(32, 100), DefaultParams()},
		{"params", w, alternating(32, 100), bad},
		{"measurements", w, alternating(28, 100), DefaultParams()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildInfluence(tc.w, tc.ms, tc.p)
			requireInvalid(t, err)
		})
	}
}
