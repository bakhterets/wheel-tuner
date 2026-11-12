package api

import (
	"context"
	"net/http"

	"github.com/bakhterets/wheel-tuner/internal/model"
	"github.com/bakhterets/wheel-tuner/internal/solver"
)

type measurementsRequest struct {
	Measurements []model.Measurement `json:"measurements"`
}

type solveRequest struct {
	Wheel        model.WheelSetup    `json:"wheel"`
	Measurements []model.Measurement `json:"measurements"`
}

// tensionWarningRatio flags spokes predicted at ≥ 95 % of the rim limit.
const tensionWarningRatio = 0.95

type actionDTO struct {
	Spoke               int     `json:"spoke_index"`
	Side                string  `json:"side"`
	Turns               float64 `json:"turns"`
	Fraction            string  `json:"fraction"`
	Direction           string  `json:"direction"`
	Instruction         string  `json:"instruction"`
	PredictedTensionKgf float64 `json:"predicted_tension_kgf"`
	TensionWarning      bool    `json:"tension_warning"`
}

type metricsDTO struct {
	MaxLateralMM          float64 `json:"max_lateral_mm"`
	MaxRadialMM           float64 `json:"max_radial_mm"`
	TensionSpreadLeftKgf  float64 `json:"tension_spread_left_kgf"`
	TensionSpreadRightKgf float64 `json:"tension_spread_right_kgf"`
	Cost                  float64 `json:"cost"`
}

type spokeStateDTO struct {
	Index          int     `json:"spoke_index"`
	Side           string  `json:"side"`
	TensionKgf     float64 `json:"tension_kgf"`
	LateralDevMM   float64 `json:"lateral_dev_mm"`
	RadialDevMM    float64 `json:"radial_dev_mm"`
	TensionWarning bool    `json:"tension_warning"`
}

type solveResponse struct {
	SessionID     int64           `json:"session_id,omitempty"`
	Orientation   string          `json:"orientation"`
	Actions       []actionDTO     `json:"actions"`
	Turns         []float64       `json:"turns"`
	RawTurns      []float64       `json:"raw_turns"`
	Before        metricsDTO      `json:"before"`
	After         metricsDTO      `json:"after"`
	Predicted     []spokeStateDTO `json:"predicted"`
	Iterations    int             `json:"iterations"`
	Geometry      geometryDTO     `json:"geometry"`
	MaxTensionKgf float64         `json:"max_tension_kgf"`
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sessions, err := s.repo.ListSessions(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, sessions)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in measurementsRequest
	if err := decode(w, r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	wheel, err := s.repo.GetWheel(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := resolveInput(wheel, in.Measurements); err != nil {
		s.fail(w, r, err)
		return
	}
	if _, _, err := validateInput(wheel, in.Measurements); err != nil {
		s.fail(w, r, err)
		return
	}
	sess, err := s.repo.CreateSession(r.Context(), id, in.Measurements)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusCreated, sess)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sess, err := s.repo.GetSession(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, sess)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.repo.DeleteSession(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) solveSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp, err := s.solveStored(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, resp)
}

func (s *Server) solveStored(ctx context.Context, sessionID int64) (solveResponse, error) {
	sess, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return solveResponse{}, err
	}
	wheel, err := s.repo.GetWheel(ctx, sess.WheelID)
	if err != nil {
		return solveResponse{}, err
	}
	resp, err := s.run(wheel, sess.Measurements)
	resp.SessionID = sess.ID
	return resp, err
}

// solve runs the solver on an ad-hoc wheel and measurements without storing them.
func (s *Server) solve(w http.ResponseWriter, r *http.Request) {
	var in solveRequest
	if err := decode(w, r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := prepareWheel(&in.Wheel); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := resolveInput(in.Wheel, in.Measurements); err != nil {
		s.fail(w, r, err)
		return
	}
	resp, err := s.run(in.Wheel, in.Measurements)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, resp)
}

// resolveInput converts the tensiometer readings of newly submitted
// measurements to kgf in place. Stored sessions are not re-resolved: their
// kgf values were fixed with the tensiometer settings at recording time.
func resolveInput(wheel model.WheelSetup, ms []model.Measurement) error {
	if err := checkInput(wheel, ms); err != nil {
		return err
	}
	return model.ResolveTensions(wheel, ms)
}

func checkInput(wheel model.WheelSetup, ms []model.Measurement) error {
	// Stored profiles are re-validated: toSolverWheel relies on it.
	if err := wheel.Validate(); err != nil {
		return err
	}
	return model.ValidateMeasurements(wheel.SpokesCount, ms)
}

func validateInput(wheel model.WheelSetup, ms []model.Measurement) (solver.Wheel, []solver.Measurement, error) {
	if err := checkInput(wheel, ms); err != nil {
		return solver.Wheel{}, nil, err
	}
	sw := toSolverWheel(wheel)
	sms := make([]solver.Measurement, len(ms))
	for i, m := range ms {
		sms[i] = solver.Measurement{
			Index: m.SpokeIndex,
			// ValidateMeasurements guarantees Side is exactly "L" or "R".
			Side:            solver.Side(m.Side[0]),
			TensionKgf:      m.TensionKgf,
			LateralMM:       m.LateralDevMM,
			RadialMM:        m.RadialDevMM,
			LateralPeakNext: m.LatPeakBetweenNext,
			RadialPeakNext:  m.RadPeakBetweenNext,
		}
	}
	return sw, sms, solver.Validate(sw, sms)
}

func (s *Server) run(wheel model.WheelSetup, ms []model.Measurement) (solveResponse, error) {
	sw, sms, err := validateInput(wheel, ms)
	if err != nil {
		return solveResponse{}, err
	}
	geo, err := solver.SpokeGeometry(sw, s.params)
	if err != nil {
		return solveResponse{}, err
	}
	res, err := solver.Solve(sw, sms, s.params)
	if err != nil {
		return solveResponse{}, err
	}
	warn := func(kgf float64) bool {
		return wheel.MaxTensionKgf > 0 && kgf >= tensionWarningRatio*wheel.MaxTensionKgf
	}
	resp := solveResponse{
		Orientation:   wheel.OrientationHeader(),
		Actions:       make([]actionDTO, len(res.Actions)),
		Turns:         res.Turns,
		RawTurns:      res.RawTurns,
		Before:        toMetricsDTO(res.Before),
		After:         toMetricsDTO(res.After),
		Predicted:     make([]spokeStateDTO, len(res.Predicted)),
		Iterations:    res.Iterations,
		Geometry:      toGeometryDTO(geo),
		MaxTensionKgf: wheel.MaxTensionKgf,
	}
	for i, a := range res.Actions {
		dir := "loosen"
		if a.Tighten() {
			dir = "tighten"
		}
		// Predicted is indexed by spoke; unmeasured tensions stay 0 and never warn.
		kgf := res.Predicted[a.Spoke-1].TensionKgf
		resp.Actions[i] = actionDTO{
			Spoke:               a.Spoke,
			Side:                a.Side.String(),
			Turns:               a.Turns,
			Fraction:            a.Fraction(),
			Direction:           dir,
			Instruction:         a.Instruction(),
			PredictedTensionKgf: kgf,
			TensionWarning:      warn(kgf),
		}
	}
	for i, p := range res.Predicted {
		resp.Predicted[i] = spokeStateDTO{
			Index:          p.Index,
			Side:           p.Side.String(),
			TensionKgf:     p.TensionKgf,
			LateralDevMM:   p.LateralMM,
			RadialDevMM:    p.RadialMM,
			TensionWarning: warn(p.TensionKgf),
		}
	}
	return resp, nil
}

func toMetricsDTO(m solver.Metrics) metricsDTO {
	return metricsDTO{
		MaxLateralMM:          m.MaxLateralMM,
		MaxRadialMM:           m.MaxRadialMM,
		TensionSpreadLeftKgf:  m.TensionSpreadLeftKgf,
		TensionSpreadRightKgf: m.TensionSpreadRightKgf,
		Cost:                  m.Cost,
	}
}
