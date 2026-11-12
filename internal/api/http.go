package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bakhterets/wheel-tuner/internal/model"
	"github.com/bakhterets/wheel-tuner/internal/solver"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent; this only happens on a broken connection.
		s.log.WarnContext(r.Context(), "write response", "path", r.URL.Path, "err", err)
	}
}

// fail maps domain errors to HTTP responses.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve *model.ValidationError
	switch {
	case errors.As(err, &ve):
		s.writeJSON(w, r, http.StatusBadRequest, errorBody{Error: ve.Message, Field: ve.Field})
	case errors.Is(err, model.ErrNotFound):
		s.writeJSON(w, r, http.StatusNotFound, errorBody{Error: "not found"})
	case errors.Is(err, solver.ErrInvalidInput):
		s.writeJSON(w, r, http.StatusUnprocessableEntity, errorBody{Error: err.Error()})
	default:
		s.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		s.writeJSON(w, r, http.StatusInternalServerError, errorBody{Error: "internal error"})
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &model.ValidationError{Field: "body", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}
	if dec.More() {
		return &model.ValidationError{Field: "body", Message: "unexpected data after JSON object"}
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, &model.ValidationError{Field: "id", Message: "must be a positive integer"}
	}
	return id, nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.DebugContext(r.Context(), "http request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration", time.Since(start))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				s.fail(w, r, fmt.Errorf("panic: %v", v))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}
