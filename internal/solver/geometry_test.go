package solver

import (
	"math"
	"testing"
)

// specLength is the spoke length formula of the feature spec, in degrees:
// L = √(D² + F² + (ERD/2)² − D·ERD·cos(720·X/N)).
func specLength(pcd, flange, erd float64, cross, spokes int) float64 {
	d := pcd / 2
	a := 720 * float64(cross) / float64(spokes) * math.Pi / 180
	return math.Sqrt(d*d + flange*flange + erd*erd/4 - d*erd*math.Cos(a))
}

func TestSpokeGeometryLength(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Wheel)
		side   Side
		want   float64
	}{
		{"hand-computed 32h 3x J-bend", func(w *Wheel) { w.FlangeDiamLeftMM = 58 }, Left, 291.045},
		{"J-bend subtracts the hole radius", func(w *Wheel) { w.FlangeDiamLeftMM = 58 }, Left,
			specLength(58, 35, 600, 3, 32) - jBendHoleRadiusMM},
		{"straight-pull keeps the full length", func(w *Wheel) {
			w.FlangeDiamRightMM, w.SpokeType = 46, StraightPull
		}, Right, specLength(46, 20, 600, 3, 32)},
		{"radial lacing", func(w *Wheel) {
			w.Cross, w.FlangeDiamRightMM, w.SpokeType = 0, 46, StraightPull
		}, Right, math.Hypot(300-23, 20)},
		{"unknown flange diameter models from the axle", func(w *Wheel) { w.SpokeType = StraightPull }, Left, math.Hypot(300, 35)},
		{"Newmen Fade SP front on F 572 29, 28h 2x", func(w *Wheel) {
			*w = Wheel{SpokeCount: 28, ERDMM: 602, Cross: 2, FlangeLeftMM: 29, FlangeRightMM: 34,
				FlangeDiamLeftMM: 38.5, FlangeDiamRightMM: 38.5, SpokeType: StraightPull,
				Direction: Clockwise, ViewFrom: Left}
		}, Left, specLength(38.5, 29, 602, 2, 28)},
		{"paired straight-pull: Fade SP 28h on F 572 29, 3x (user build, 303 mm)", func(w *Wheel) {
			*w = Wheel{SpokeCount: 28, ERDMM: 602, Cross: 3, FlangeLeftMM: 29, FlangeRightMM: 34,
				FlangeDiamLeftMM: 38.5, FlangeDiamRightMM: 38.5, SpokeType: StraightPull, PairedSpokes: true,
				Direction: Clockwise, ViewFrom: Left}
		}, Left, math.Sqrt(19.25*19.25 + 29*29 + 301*301)}, // θ = 3·720/28 + 360/28 = 90°: 303.006
		{"paired straight-pull right side", func(w *Wheel) {
			*w = Wheel{SpokeCount: 28, ERDMM: 602, Cross: 3, FlangeLeftMM: 29, FlangeRightMM: 34,
				FlangeDiamLeftMM: 38.5, FlangeDiamRightMM: 38.5, SpokeType: StraightPull, PairedSpokes: true,
				Direction: Clockwise, ViewFrom: Left}
		}, Right, math.Sqrt(19.25*19.25 + 34*34 + 301*301)}, // 303.525
		{"paired radial lacing has no extra span", func(w *Wheel) {
			w.Cross, w.FlangeDiamRightMM, w.SpokeType, w.PairedSpokes = 0, 46, StraightPull, true
		}, Right, math.Hypot(300-23, 20)},
		{"rim offset shortens the left spoke", func(w *Wheel) {
			w.FlangeDiamLeftMM, w.RimOffsetMM, w.SpokeType = 58, 3, StraightPull
		}, Left, specLength(58, 32, 600, 3, 32)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := testWheel(32, 3)
			tc.mutate(&w)
			g, err := SpokeGeometry(w, DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			if got := g.Side(tc.side).SpokeLengthMM; !near(got, tc.want, 1e-3) {
				t.Fatalf("length = %.4f, want %.4f", got, tc.want)
			}
		})
	}
}

func TestSpokeGeometryAsymmetryAndBracing(t *testing.T) {
	w := testWheel(32, 3)
	w.FlangeDiamLeftMM, w.FlangeDiamRightMM = 58, 58
	sym, err := SpokeGeometry(w, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	w.RimOffsetMM = 4
	asym, err := SpokeGeometry(w, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if asym.Left.FlangeOffsetMM != 31 || asym.Right.FlangeOffsetMM != 24 {
		t.Fatalf("effective flanges = %v / %v, want 31 / 24", asym.Left.FlangeOffsetMM, asym.Right.FlangeOffsetMM)
	}
	if !(asym.Left.BracingAngleDeg < sym.Left.BracingAngleDeg && asym.Right.BracingAngleDeg > sym.Right.BracingAngleDeg) {
		t.Fatalf("offset must narrow the left and widen the right bracing angle: %+v → %+v", sym, asym)
	}
	for _, g := range []Geometry{sym, asym} {
		for _, s := range []SideGeometry{g.Left, g.Right} {
			// sin α = F / L_geometric.
			l := s.SpokeLengthMM + jBendHoleRadiusMM
			if !near(math.Sin(s.BracingAngleDeg*math.Pi/180), s.FlangeOffsetMM/l, 1e-12) {
				t.Fatalf("bracing angle %v inconsistent with F=%v, L=%v", s.BracingAngleDeg, s.FlangeOffsetMM, l)
			}
		}
		if want := g.Right.lateral / g.Left.lateral; !near(g.TensionRatio, want, 1e-12) {
			t.Fatalf("tension ratio %v, want %v", g.TensionRatio, want)
		}
	}
	// Dish: the narrow right flange carries more tension (ratio < 1); the
	// asymmetric rim reduces the imbalance.
	if !(sym.TensionRatio < asym.TensionRatio && asym.TensionRatio < 1) {
		t.Fatalf("tension ratio %v → %v must move towards 1", sym.TensionRatio, asym.TensionRatio)
	}
}

func TestSpokeGeometryFrontDiscTensionRatio(t *testing.T) {
	// DT Swiss 240 Boost front (6-bolt): the disc-side flange sits closer to
	// the centre, so its spokes must run tighter (≈120 vs ≈80 kgf).
	w := Wheel{SpokeCount: 28, ERDMM: 602, Cross: 2, FlangeLeftMM: 27.3, FlangeRightMM: 41,
		FlangeDiamLeftMM: 57.8, FlangeDiamRightMM: 49.4, Direction: Clockwise, ViewFrom: Left}
	g, err := SpokeGeometry(w, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if !(g.TensionRatio > 1.4 && g.TensionRatio < 1.6) {
		t.Fatalf("left/right tension ratio = %v, want ≈1.5", g.TensionRatio)
	}
}

func TestSpokeGeometryHookesLaw(t *testing.T) {
	p := DefaultParams()
	short, long := testWheel(32, 3), testWheel(32, 3)
	short.ERDMM, long.ERDMM = 540, 620
	gs, err := SpokeGeometry(short, p)
	if err != nil {
		t.Fatal(err)
	}
	gl, err := SpokeGeometry(long, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []SideGeometry{gs.Left, gs.Right, gl.Left, gl.Right} {
		if !near(s.TensionPerTurnKgf*s.SpokeLengthMM, p.SpokeRigidityKgf*p.ThreadPitchMM, 1e-9) {
			t.Fatalf("ΔT·L must equal E·A·pitch: %+v", s)
		}
	}
	if !(gl.Left.TensionPerTurnKgf < gs.Left.TensionPerTurnKgf) {
		t.Fatalf("longer spokes must gain less tension per turn: %v vs %v", gl.Left.TensionPerTurnKgf, gs.Left.TensionPerTurnKgf)
	}
	inf, err := BuildInfluence(long, alternating(32, 100), p)
	if err != nil {
		t.Fatal(err)
	}
	if got := inf.Tension.At(0, 0); !near(got, gl.Left.TensionPerTurnKgf, 1e-12) {
		t.Fatalf("tension matrix diagonal %v, want %v", got, gl.Left.TensionPerTurnKgf)
	}
}

func TestInfluenceUsesEffectiveFlanges(t *testing.T) {
	// A 7.5 mm offset centres the spoke bed between flanges at 35 and 20 mm:
	// both flanges then pull with the same lateral and radial gain.
	w := testWheel(32, 3)
	w.RimOffsetMM = 7.5
	inf, err := BuildInfluence(w, alternating(32, 100), DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if l, r := inf.Lateral.At(0, 0), inf.Lateral.At(1, 1); !near(-l, r, 1e-12) {
		t.Fatalf("lateral gains %v / %v must mirror", l, r)
	}
	if l, r := inf.Radial.At(0, 0), inf.Radial.At(1, 1); !near(l, r, 1e-12) {
		t.Fatalf("radial gains %v / %v must match", l, r)
	}
}

func TestSpokeGeometryRejectsInvalidInput(t *testing.T) {
	bad := DefaultParams()
	bad.SpokeRigidityKgf = 0
	tests := []struct {
		name string
		w    Wheel
		p    Params
	}{
		{"wheel", Wheel{}, DefaultParams()},
		{"params", testWheel(32, 3), bad},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SpokeGeometry(tc.w, tc.p)
			requireInvalid(t, err)
		})
	}
}
