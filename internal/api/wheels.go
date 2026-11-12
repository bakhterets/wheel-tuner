package api

import (
	"context"
	"errors"
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

// checkComponents verifies that the library components a profile refers to exist.
func (s *Server) checkComponents(ctx context.Context, w model.WheelSetup) error {
	if w.RimComponentID != nil {
		if _, err := s.repo.GetRim(ctx, *w.RimComponentID); err != nil {
			return componentErr("rim_component_id", err)
		}
	}
	if w.HubComponentID != nil {
		if _, err := s.repo.GetHub(ctx, *w.HubComponentID); err != nil {
			return componentErr("hub_component_id", err)
		}
	}
	return nil
}

func componentErr(field string, err error) error {
	if errors.Is(err, model.ErrNotFound) {
		return &model.ValidationError{Field: field, Message: "unknown component"}
	}
	return err
}

// storeWheel prepares a profile from the request body for create or update.
func (s *Server) storeWheel(r *http.Request, w *model.WheelSetup) error {
	if err := prepareWheel(w); err != nil {
		return err
	}
	return s.checkComponents(r.Context(), *w)
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
	if err := s.storeWheel(r, &in); err != nil {
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
	if err := s.storeWheel(r, &in); err != nil {
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
// which guarantees a known counting direction, view point and spoke type.
func toSolverWheel(w model.WheelSetup) solver.Wheel {
	dir := solver.Clockwise
	if w.CountingDirection == "CCW" {
		dir = solver.CounterClockwise
	}
	view := solver.Left
	if w.ViewSide() == "R" {
		view = solver.Right
	}
	spoke := solver.JBend
	if w.SpokeType == model.SpokeStraightPull {
		spoke = solver.StraightPull
	}
	return solver.Wheel{
		SpokeCount:        w.SpokesCount,
		ERDMM:             w.ERDMM,
		Cross:             w.CrossPattern,
		FlangeLeftMM:      w.FlangeDistLeftMM,
		FlangeRightMM:     w.FlangeDistRightMM,
		FlangeDiamLeftMM:  w.FlangeDiamLeftMM,
		FlangeDiamRightMM: w.FlangeDiamRightMM,
		RimOffsetMM:       w.RimAsymmetryOffsetMM,
		SpokeType:         spoke,
		Direction:         dir,
		ViewFrom:          view,
	}
}
