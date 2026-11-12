package solver

import (
	"math"
	"slices"
)

// Point identifies a row of a deviation matrix.
type Point struct {
	Position float64 // 1-based rim position; virtual points sit at Spoke+0.5
	Spoke    int     // spoke whose measurement row produced the point
	Virtual  bool    // true for interpolated inter-spoke points
}

// Influence is the Jacobian of rim state with respect to nipple turns.
// Column j is the effect of one full tightening turn of spoke j+1.
type Influence struct {
	Lateral       *Matrix // mm per turn, rows follow LateralPoints
	LateralPoints []Point
	Radial        *Matrix // mm per turn (negative pulls the rim in), rows follow RadialPoints
	RadialPoints  []Point
	Tension       *Matrix // kgf per turn, N×N
	CrossPartners [][]int // 1-based crossing spokes on the same flange, per spoke
}

// BuildInfluence validates the input and builds the influence matrices.
func BuildInfluence(w Wheel, ms []Measurement, p Params) (*Influence, error) {
	inf, _, err := build(w, ms, p)
	return inf, err
}

func build(w Wheel, ms []Measurement, p Params) (*Influence, []Measurement, error) {
	if err := w.Validate(); err != nil {
		return nil, nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, nil, err
	}
	sorted, err := normalize(w, ms)
	if err != nil {
		return nil, nil, err
	}
	return newGeometry(w, sorted, p).influence(sorted), sorted, nil
}

type geometry struct {
	n        int
	stepMM   float64
	sides    []Side
	latGain  []float64
	radGain  []float64
	turnKgf  []float64 // tension gained by each spoke per full turn
	partners [][]int   // 0-based
	p        Params
}

func newGeometry(w Wheel, ms []Measurement, p Params) geometry {
	n := len(ms)
	g := geometry{
		n:       n,
		stepMM:  w.spacingMM(),
		sides:   make([]Side, n),
		latGain: make([]float64, n),
		radGain: make([]float64, n),
		turnKgf: make([]float64, n),
		p:       p,
	}
	sg := w.spokeGeometry(p)
	for j, m := range ms {
		s := sg.Side(m.Side)
		g.sides[j] = m.Side
		g.turnKgf[j] = s.TensionPerTurnKgf
		// One turn adds ΔT = TensionPerTurnKgf; its lateral (ΔT·sin α) and
		// radial components deflect the rim through its compliance.
		g.latGain[j] = m.Side.pull() * s.TensionPerTurnKgf * s.lateral * p.LateralCompliance
		g.radGain[j] = -s.TensionPerTurnKgf * s.radial * p.RadialCompliance
	}
	g.partners = crossPartners(g.sides, w.Cross, w.rotationSense())
	return g
}

// decay is the rim attenuation between two rim positions (1-based or
// 0-based, only the difference matters). A larger ERD spreads the spokes
// further apart and therefore attenuates faster per spoke.
func (g geometry) decay(a, b, lengthMM float64) float64 {
	d := math.Mod(math.Abs(a-b), float64(g.n))
	d = math.Min(d, float64(g.n)-d)
	return math.Exp(-d * g.stepMM / lengthMM)
}

func (g geometry) influence(ms []Measurement) *Influence {
	latFlags := make([]bool, g.n)
	radFlags := make([]bool, g.n)
	for i, m := range ms {
		latFlags[i] = m.LateralPeakNext
		radFlags[i] = m.RadialPeakNext
	}
	lat, latPts := g.deviationMatrix(g.latGain, g.p.LateralDecayMM, latFlags)
	rad, radPts := g.deviationMatrix(g.radGain, g.p.RadialDecayMM, radFlags)
	partners := make([][]int, g.n)
	for i, ps := range g.partners {
		partners[i] = make([]int, len(ps))
		for k, j := range ps {
			partners[i][k] = j + 1
		}
	}
	return &Influence{
		Lateral:       lat,
		LateralPoints: latPts,
		Radial:        rad,
		RadialPoints:  radPts,
		Tension:       g.tensionMatrix(),
		CrossPartners: partners,
	}
}

// deviationMatrix builds one row per spoke plus a virtual row after every
// flagged spoke: A[i.5][j] = (A[i][j] + A[i+1][j]) / 2.
func (g geometry) deviationMatrix(gain []float64, lengthMM float64, flags []bool) (*Matrix, []Point) {
	rows := g.n
	for _, f := range flags {
		if f {
			rows++
		}
	}
	m := NewMatrix(rows, g.n)
	pts := make([]Point, 0, rows)
	at := func(i, j int) float64 { return gain[j] * g.decay(float64(i), float64(j), lengthMM) }
	r := 0
	for i := range g.n {
		for j := range g.n {
			m.Set(r, j, at(i, j))
		}
		pts = append(pts, Point{Position: float64(i + 1), Spoke: i + 1})
		r++
		if !flags[i] {
			continue
		}
		next := (i + 1) % g.n
		for j := range g.n {
			m.Set(r, j, (at(i, j)+at(next, j))/2)
		}
		pts = append(pts, Point{Position: float64(i+1) + 0.5, Spoke: i + 1, Virtual: true})
		r++
	}
	return m, pts
}

// tensionMatrix models how one turn of spoke j changes every spoke's tension
// (t_j = TensionPerTurnKgf of spoke j):
//   - the turned spoke gains t_j;
//   - the rim pulled towards its flange relaxes same-side neighbours and loads
//     opposite-side neighbours (t_j·NeighborCoupling, attenuated along the rim);
//   - each crossing spoke on the same flange gains t_j·CrossTalk.
func (g geometry) tensionMatrix() *Matrix {
	m := NewMatrix(g.n, g.n)
	for i := range g.n {
		for j := range g.n {
			t0 := g.turnKgf[j]
			if i == j {
				m.Set(i, j, t0)
				continue
			}
			c := t0 * g.p.NeighborCoupling * g.decay(float64(i), float64(j), g.p.LateralDecayMM)
			if g.sides[i] == g.sides[j] {
				c = -c
			}
			m.Set(i, j, c)
		}
		for _, j := range g.partners[i] {
			m.Set(i, j, m.At(i, j)+g.turnKgf[j]*g.p.CrossTalk)
		}
	}
	return m
}

// crossPartners returns, per spoke, the 0-based indices of spokes it crosses.
//
// On each flange spokes alternate between leading (hub end ahead in index
// order) and trailing. A leading spoke at flange position m crosses the
// trailing spokes at m+1, m+3, …, m+2k−1. Convention: the first spoke of each
// flange (lowest index) has its hub end displaced clockwise as seen from the
// right side, so the pattern mirrors when the counting direction or view
// side flips.
func crossPartners(sides []Side, cross, sense int) [][]int {
	partners := make([][]int, len(sides))
	if cross == 0 {
		return partners
	}
	for _, side := range [...]Side{Left, Right} {
		var flange []int
		for i, s := range sides {
			if s == side {
				flange = append(flange, i)
			}
		}
		for m, spoke := range flange {
			if (m%2 == 0) != (sense > 0) {
				continue
			}
			for off := 1; off < 2*cross; off += 2 {
				other := flange[(m+off)%len(flange)]
				partners[spoke] = append(partners[spoke], other)
				partners[other] = append(partners[other], spoke)
			}
		}
	}
	for _, ps := range partners {
		slices.Sort(ps)
	}
	return partners
}
