package solver

import "math"

// Weights are the cost function coefficients.
type Weights struct {
	Lateral float64 // W_L, per mm²
	Radial  float64 // W_R, per mm²
	Tension float64 // W_T, per kgf²
	Turns   float64 // W_X, Tikhonov regularisation per turn²
}

// Params are the empirical model constants. Use DefaultParams as a baseline.
type Params struct {
	ThreadPitchMM float64 // spoke shortening per full nipple turn
	// SpokeRigidityKgf is the effective axial rigidity E·A of a spoke (kgf),
	// reduced for rim compression and spoke wind-up. Hooke's law gives the
	// tension gained per turn: SpokeRigidityKgf·ThreadPitchMM/length.
	SpokeRigidityKgf  float64
	LateralCompliance float64 // rim lateral travel per kgf of lateral spoke force (mm/kgf)
	RadialCompliance  float64 // rim radial travel per kgf of radial spoke force (mm/kgf)
	LateralDecayMM    float64 // rim arc length over which lateral influence decays by 1/e
	RadialDecayMM     float64 // rim arc length over which radial influence decays by 1/e
	NeighborCoupling  float64 // tension transferred to nearby spokes, fraction of the turned spoke's gain
	CrossTalk         float64 // tension transferred to each crossing spoke, fraction of the turned spoke's gain
	TurnStep          float64 // smallest practical nipple turn
	MinTurn           float64 // adjustments below this are dropped
	Tolerance         float64 // relative residual tolerance of the linear solver
	MaxIterations     int     // linear solver iteration cap; 0 selects 10·N+10
	Weights           Weights
}

// DefaultParams returns constants tuned for typical steel spokes and alloy rims.
func DefaultParams() Params {
	return Params{
		ThreadPitchMM: 0.45, // 56 TPI
		// ≈25 kgf per turn on a 290 mm spoke (the stage-01 calibration).
		SpokeRigidityKgf: 16000,
		// The rim is far stiffer radially than laterally. On a 32h/ERD 600
		// wheel a quarter turn moves the rim ≈0.1–0.2 mm sideways (more on
		// the wider-braced flange) and ≈0.07 mm radially.
		LateralCompliance: 0.3,
		RadialCompliance:  0.012,
		// Lateral rim bending spreads over ≈2 spoke spacings, radial over ≈1
		// (values for a 32h rim with ERD ≈ 600 mm).
		LateralDecayMM:   120,
		RadialDecayMM:    50,
		NeighborCoupling: 0.15,
		CrossTalk:        0.1,
		TurnStep:         0.25,
		MinTurn:          0.125,
		Tolerance:        1e-10,
		Weights: Weights{
			Lateral: 1,
			Radial:  1,
			// 1 mm of runout costs as much as 100 kgf of tension deviation:
			// geometry dominates, tension breaks ties and pulls in gross outliers.
			Tension: 1e-4,
			Turns:   1e-3,
		},
	}
}

// Validate checks that all constants are finite and in range.
func (p Params) Validate() error {
	positive := [...]struct {
		name string
		v    float64
	}{
		{"thread pitch", p.ThreadPitchMM},
		{"spoke rigidity", p.SpokeRigidityKgf},
		{"lateral compliance", p.LateralCompliance},
		{"radial compliance", p.RadialCompliance},
		{"lateral decay", p.LateralDecayMM},
		{"radial decay", p.RadialDecayMM},
		{"turn step", p.TurnStep},
		{"tolerance", p.Tolerance},
	}
	for _, c := range positive {
		if !(c.v > 0) || math.IsInf(c.v, 0) {
			return invalidf("%s must be positive and finite, got %g", c.name, c.v)
		}
	}
	nonNegative := [...]struct {
		name string
		v    float64
	}{
		{"neighbor coupling", p.NeighborCoupling},
		{"cross talk", p.CrossTalk},
		{"min turn", p.MinTurn},
		{"lateral weight", p.Weights.Lateral},
		{"radial weight", p.Weights.Radial},
		{"tension weight", p.Weights.Tension},
		{"turns weight", p.Weights.Turns},
	}
	for _, c := range nonNegative {
		if !(c.v >= 0) || math.IsInf(c.v, 0) {
			return invalidf("%s must be non-negative and finite, got %g", c.name, c.v)
		}
	}
	if p.MaxIterations < 0 {
		return invalidf("max iterations must be non-negative, got %d", p.MaxIterations)
	}
	if p.Weights.Lateral+p.Weights.Radial+p.Weights.Tension == 0 {
		return invalidf("at least one of the lateral, radial or tension weights must be positive")
	}
	return nil
}
