package api

import (
	"net/http"

	"github.com/bakhterets/wheel-tuner/internal/model"
	"github.com/bakhterets/wheel-tuner/internal/solver"
)

// prepareWheel normalises and fully validates a profile, including the
// geometric constraints enforced by the solver.
func prepareWheel(w *model.WheelSetup) error {
	w.Normalize()
	if err := w.Validate(); err != nil {
		return err
	}
	sw := toSolverWheel(*w)
	return sw.Validate()
}

func (s *Server) listWheels(w http.ResponseWriter, r *http.Request) {
	wheels, err := s.repo.ListWheels(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, wheels)
}

func (s *Server) createWheel(w http.ResponseWriter, r *http.Request) {
	var in model.WheelSetup
	if err := decode(w, r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	in.ID = 0
	if err := prepareWheel(&in); err != nil {
		s.fail(w, r, err)
		return
	}
	created, err := s.repo.CreateWheel(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusCreated, created)
}

func (s *Server) getWheel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	wheel, err := s.repo.GetWheel(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, wheel)
}

func (s *Server) updateWheel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in model.WheelSetup
	if err := decode(w, r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	in.ID = id
	if err := prepareWheel(&in); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.repo.UpdateWheel(r.Context(), in); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, in)
}

func (s *Server) deleteWheel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.repo.DeleteWheel(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toSolverWheel converts a profile that already passed WheelSetup.Validate,
// which guarantees a known counting direction and view point.
func toSolverWheel(w model.WheelSetup) solver.Wheel {
	dir := solver.Clockwise
	if w.CountingDirection == "CCW" {
		dir = solver.CounterClockwise
	}
	view := solver.Left
	if w.ViewSide() == "R" {
		view = solver.Right
	}
	return solver.Wheel{
		SpokeCount:    w.SpokesCount,
		ERDMM:         w.ERDMM,
		Cross:         w.CrossPattern,
		FlangeLeftMM:  w.FlangeDistLeftMM,
		FlangeRightMM: w.FlangeDistRightMM,
		Direction:     dir,
		ViewFrom:      view,
	}
}
