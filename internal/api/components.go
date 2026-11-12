package api

import (
	"net/http"

	"github.com/bakhterets/wheel-tuner/internal/model"
	"github.com/bakhterets/wheel-tuner/internal/solver"
	"github.com/bakhterets/wheel-tuner/internal/tensiometer"
)

type sideGeometryDTO struct {
	FlangeOffsetMM    float64 `json:"flange_offset_mm"`
	SpokeLengthMM     float64 `json:"spoke_length_mm"`
	BracingAngleDeg   float64 `json:"bracing_angle_deg"`
	TensionPerTurnKgf float64 `json:"tension_per_turn_kgf"`
}

type geometryDTO struct {
	Left         sideGeometryDTO `json:"left"`
	Right        sideGeometryDTO `json:"right"`
	TensionRatio float64         `json:"tension_ratio"`
}

func toGeometryDTO(g solver.Geometry) geometryDTO {
	side := func(s solver.SideGeometry) sideGeometryDTO {
		return sideGeometryDTO{
			FlangeOffsetMM:    s.FlangeOffsetMM,
			SpokeLengthMM:     s.SpokeLengthMM,
			BracingAngleDeg:   s.BracingAngleDeg,
			TensionPerTurnKgf: s.TensionPerTurnKgf,
		}
	}
	return geometryDTO{Left: side(g.Left), Right: side(g.Right), TensionRatio: g.TensionRatio}
}

func (s *Server) listRims(w http.ResponseWriter, r *http.Request) {
	rims, err := s.repo.ListRims(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, rims)
}

func (s *Server) getRim(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rim, err := s.repo.GetRim(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, rim)
}

func (s *Server) listHubs(w http.ResponseWriter, r *http.Request) {
	hubs, err := s.repo.ListHubs(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, hubs)
}

func (s *Server) getHub(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hub, err := s.repo.GetHub(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, hub)
}

func (s *Server) listTensiometers(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, http.StatusOK, tensiometer.Catalog())
}

// geometry previews spoke lengths, bracing angles and the tension balance of
// an unsaved profile.
func (s *Server) geometry(w http.ResponseWriter, r *http.Request) {
	var in model.WheelSetup
	if err := decode(w, r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := prepareWheel(&in); err != nil {
		s.fail(w, r, err)
		return
	}
	g, err := solver.SpokeGeometry(toSolverWheel(in), s.params)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, toGeometryDTO(g))
}
