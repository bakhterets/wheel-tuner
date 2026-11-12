package solver

import "math"

// SideGeometry describes the spokes of one hub flange.
type SideGeometry struct {
	FlangeOffsetMM    float64 // lateral distance flange → rim spoke bed, rim offset applied
	SpokeLengthMM     float64 // flange hole to ERD point; J-bend hole correction applied
	BracingAngleDeg   float64 // angle between the spoke and the wheel's mid-plane
	TensionPerTurnKgf float64 // Hooke's law: rigidity · pitch / length
	lateral           float64 // sin α: share of spoke tension acting laterally
	radial            float64 // share of spoke tension acting along the rim radius
}

// Geometry is the spoke geometry derived from a Wheel.
type Geometry struct {
	Left, Right SideGeometry
	// TensionRatio is the expected left/right tension ratio of a true wheel:
	// the lateral components of both flanges must cancel, so
	// T_L·sin α_L = T_R·sin α_R (equal spoke counts per flange).
	TensionRatio float64
}

// Side returns the geometry of one flange.
func (g Geometry) Side(s Side) SideGeometry {
	if s == Right {
		return g.Right
	}
	return g.Left
}

// SpokeGeometry validates the wheel and parameters and derives the spoke geometry.
func SpokeGeometry(w Wheel, p Params) (Geometry, error) {
	if err := w.Validate(); err != nil {
		return Geometry{}, err
	}
	if err := p.Validate(); err != nil {
		return Geometry{}, err
	}
	return w.spokeGeometry(p), nil
}

// spokeGeometry assumes a valid wheel and parameters.
func (w Wheel) spokeGeometry(p Params) Geometry {
	g := Geometry{Left: w.side(Left, p), Right: w.side(Right, p)}
	g.TensionRatio = g.Right.lateral / g.Left.lateral
	return g
}

// side computes the spoke vector from the flange hole to the rim. The hub
// end of a crossed spoke is rotated by θ = cross·720°/N (+360°/N for paired
// straight-pull seats), giving the classic length formula
// L = √(D² + F² + R² − 2·D·R·cos θ), D = flange radius, R = ERD/2,
// F = effective flange distance.
func (w Wheel) side(s Side, p Params) SideGeometry {
	f := w.effectiveFlange(s)
	d := w.flangeDiam(s) / 2
	r := w.ERDMM / 2
	theta := w.spanDeg() * math.Pi / 180
	l := math.Sqrt(d*d + f*f + r*r - 2*d*r*math.Cos(theta))
	length := l
	if w.SpokeType == JBend {
		length -= jBendHoleRadiusMM
	}
	return SideGeometry{
		FlangeOffsetMM:    f,
		SpokeLengthMM:     length,
		BracingAngleDeg:   math.Asin(f/l) * 180 / math.Pi,
		TensionPerTurnKgf: p.SpokeRigidityKgf * p.ThreadPitchMM / length,
		lateral:           f / l,
		// Radial projection of the unit spoke vector; equals cos α for radial
		// lacing, crossed spokes lose the tangential share.
		radial: (r - d*math.Cos(theta)) / l,
	}
}
