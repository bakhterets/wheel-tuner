// Package api exposes the wheel profiles, truing sessions and the solver over
// a JSON HTTP API and serves the static frontend.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/bakhterets/wheel-tuner/internal/model"
	"github.com/bakhterets/wheel-tuner/internal/solver"
)

// Repository is the persistence layer consumed by the API.
// Implementations return errors wrapping model.ErrNotFound for missing rows.
type Repository interface {
	CreateWheel(ctx context.Context, w model.WheelSetup) (model.WheelSetup, error)
	GetWheel(ctx context.Context, id int64) (model.WheelSetup, error)
	ListWheels(ctx context.Context) ([]model.WheelSetup, error)
	UpdateWheel(ctx context.Context, w model.WheelSetup) error
	DeleteWheel(ctx context.Context, id int64) error
	CreateSession(ctx context.Context, wheelID int64, ms []model.Measurement) (model.Session, error)
	GetSession(ctx context.Context, id int64) (model.Session, error)
	ListSessions(ctx context.Context, wheelID int64) ([]model.Session, error)
	DeleteSession(ctx context.Context, id int64) error
	ListRims(ctx context.Context) ([]model.RimComponent, error)
	GetRim(ctx context.Context, id int64) (model.RimComponent, error)
	ListHubs(ctx context.Context) ([]model.HubComponent, error)
	GetHub(ctx context.Context, id int64) (model.HubComponent, error)
	Ping(ctx context.Context) error
}

// Server is the HTTP handler of the application.
type Server struct {
	repo    Repository
	params  solver.Params
	log     *slog.Logger
	static  fs.FS
	handler http.Handler
}

// Option configures a Server.
type Option func(*Server)

// WithLogger sets the structured logger (default: slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(s *Server) { s.log = l } }

// WithSolverParams overrides the solver model constants.
func WithSolverParams(p solver.Params) Option { return func(s *Server) { s.params = p } }

// WithStatic serves the frontend from fsys at "/".
func WithStatic(fsys fs.FS) Option { return func(s *Server) { s.static = fsys } }

// New builds the server and its routes.
func New(repo Repository, opts ...Option) *Server {
	s := &Server{repo: repo, params: solver.DefaultParams(), log: slog.Default()}
	for _, o := range opts {
		o(s)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/wheels", s.listWheels)
	mux.HandleFunc("POST /api/wheels", s.createWheel)
	mux.HandleFunc("GET /api/wheels/{id}", s.getWheel)
	mux.HandleFunc("PUT /api/wheels/{id}", s.updateWheel)
	mux.HandleFunc("DELETE /api/wheels/{id}", s.deleteWheel)
	mux.HandleFunc("GET /api/wheels/{id}/sessions", s.listSessions)
	mux.HandleFunc("POST /api/wheels/{id}/sessions", s.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.deleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/solve", s.solveSession)
	mux.HandleFunc("POST /api/solve", s.solve)
	mux.HandleFunc("POST /api/geometry", s.geometry)
	mux.HandleFunc("GET /api/components/rims", s.listRims)
	mux.HandleFunc("GET /api/components/rims/{id}", s.getRim)
	mux.HandleFunc("GET /api/components/hubs", s.listHubs)
	mux.HandleFunc("GET /api/components/hubs/{id}", s.getHub)
	mux.HandleFunc("GET /api/tensiometers", s.listTensiometers)
	if s.static != nil {
		mux.Handle("GET /", http.FileServerFS(s.static))
	}
	s.handler = s.recoverer(s.logRequests(securityHeaders(mux)))
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.Ping(r.Context()); err != nil {
		s.log.ErrorContext(r.Context(), "health check failed", "err", err)
		s.writeJSON(w, r, http.StatusServiceUnavailable, errorBody{Error: "database unavailable"})
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}
