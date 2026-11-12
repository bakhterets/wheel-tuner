package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bakhterets/wheel-tuner/internal/api"
	"github.com/bakhterets/wheel-tuner/internal/model"
	"github.com/bakhterets/wheel-tuner/internal/solver"
	"github.com/bakhterets/wheel-tuner/internal/store"
)

var quietLog = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

func newServer(t *testing.T, opts ...api.Option) http.Handler {
	t.Helper()
	st, err := store.Open(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	return api.New(st, append([]api.Option{api.WithLogger(quietLog)}, opts...)...)
}

func do(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, r))
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, want, rec.Body.String())
	}
}

func validWheel() model.WheelSetup {
	return model.WheelSetup{
		Name:              "Front 32H",
		SpokesCount:       32,
		ERDMM:             600,
		CrossPattern:      3,
		FlangeDistLeftMM:  35,
		FlangeDistRightMM: 20,
		ViewPoint:         "Drive Side",
		CountingDirection: "ccw",
	}
}

// measurements returns an alternating L/R lacing with a lateral bump
// centred between spokes 9 and 10.
func measurements(n int) []model.Measurement {
	ms := make([]model.Measurement, n)
	for i := range ms {
		idx := i + 1
		side := "L"
		if idx%2 == 0 {
			side = "R"
		}
		d := float64(idx) - 9.5
		ms[i] = model.Measurement{
			SpokeIndex:   idx,
			Side:         side,
			TensionKgf:   110,
			LateralDevMM: 0.8 * math.Exp(-d*d/4.5),
		}
	}
	return ms
}

type solveResult struct {
	SessionID   int64  `json:"session_id"`
	Orientation string `json:"orientation"`
	Actions     []struct {
		Spoke       int     `json:"spoke_index"`
		Side        string  `json:"side"`
		Turns       float64 `json:"turns"`
		Fraction    string  `json:"fraction"`
		Direction   string  `json:"direction"`
		Instruction string  `json:"instruction"`
	} `json:"actions"`
	Turns  []float64 `json:"turns"`
	Before struct {
		MaxLateralMM float64 `json:"max_lateral_mm"`
	} `json:"before"`
	After struct {
		MaxLateralMM float64 `json:"max_lateral_mm"`
	} `json:"after"`
	Predicted []map[string]any `json:"predicted"`
}

func checkSolve(t *testing.T, res solveResult, n int) {
	t.Helper()
	if res.Orientation != "Start at Rim Seam. View from Drive Side. Count CCW." {
		t.Errorf("orientation = %q", res.Orientation)
	}
	if len(res.Actions) == 0 || len(res.Turns) != n || len(res.Predicted) != n {
		t.Fatalf("actions=%d turns=%d predicted=%d", len(res.Actions), len(res.Turns), len(res.Predicted))
	}
	for _, a := range res.Actions {
		want := "loosen"
		if a.Turns > 0 {
			want = "tighten"
		}
		if a.Direction != want || a.Fraction == "" || !strings.HasPrefix(a.Instruction, "Spoke "+a.Side) {
			t.Errorf("inconsistent action %+v", a)
		}
	}
	if !(res.After.MaxLateralMM < res.Before.MaxLateralMM) {
		t.Errorf("lateral not improved: %v -> %v", res.Before.MaxLateralMM, res.After.MaxLateralMM)
	}
}

func TestHealth(t *testing.T) {
	expectStatus(t, do(t, newServer(t), http.MethodGet, "/healthz", nil), http.StatusOK)

	rec := do(t, api.New(&fakeRepo{err: errors.New("down")}, api.WithLogger(quietLog)), http.MethodGet, "/healthz", nil)
	expectStatus(t, rec, http.StatusServiceUnavailable)
}

func TestWheelCRUD(t *testing.T) {
	h := newServer(t)

	rec := do(t, h, http.MethodGet, "/api/wheels", nil)
	expectStatus(t, rec, http.StatusOK)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list = %s, want []", rec.Body.String())
	}

	in := validWheel()
	in.ID = 99 // ignored on create
	rec = do(t, h, http.MethodPost, "/api/wheels", in)
	expectStatus(t, rec, http.StatusCreated)
	created := decodeBody[model.WheelSetup](t, rec)
	if created.ID == 99 || created.ID < 1 || created.CountingDirection != "CCW" || created.StartReference != model.DefaultStartReference {
		t.Fatalf("created = %+v", created)
	}

	rec = do(t, h, http.MethodGet, "/api/wheels/1", nil)
	expectStatus(t, rec, http.StatusOK)
	if got := decodeBody[model.WheelSetup](t, rec); !reflect.DeepEqual(got, created) {
		t.Errorf("get = %+v, want %+v", got, created)
	}

	upd := created
	upd.ID = 42 // path id wins
	upd.Name = "Rear"
	rec = do(t, h, http.MethodPut, "/api/wheels/1", upd)
	expectStatus(t, rec, http.StatusOK)
	if got := decodeBody[model.WheelSetup](t, rec); got.ID != 1 || got.Name != "Rear" {
		t.Errorf("update = %+v", got)
	}

	rec = do(t, h, http.MethodGet, "/api/wheels", nil)
	if got := decodeBody[[]model.WheelSetup](t, rec); len(got) != 1 || got[0].Name != "Rear" {
		t.Errorf("list = %+v", got)
	}

	expectStatus(t, do(t, h, http.MethodDelete, "/api/wheels/1", nil), http.StatusNoContent)
	expectStatus(t, do(t, h, http.MethodGet, "/api/wheels/1", nil), http.StatusNotFound)
	expectStatus(t, do(t, h, http.MethodPut, "/api/wheels/1", upd), http.StatusNotFound)
	expectStatus(t, do(t, h, http.MethodDelete, "/api/wheels/1", nil), http.StatusNotFound)
}

func TestWheelValidation(t *testing.T) {
	h := newServer(t)
	mutate := func(f func(*model.WheelSetup)) model.WheelSetup {
		w := validWheel()
		f(&w)
		return w
	}
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
		wantField  string
	}{
		{"malformed json", http.MethodPost, "/api/wheels", `{"name":`, http.StatusBadRequest, "body"},
		{"unknown field", http.MethodPost, "/api/wheels", `{"bogus":1}`, http.StatusBadRequest, "body"},
		{"trailing data", http.MethodPost, "/api/wheels", `{"name":"a"} {}`, http.StatusBadRequest, "body"},
		{"too large", http.MethodPost, "/api/wheels", `{"name":"` + strings.Repeat("a", 1<<20) + `"}`, http.StatusBadRequest, "body"},
		{"empty name", http.MethodPost, "/api/wheels", mutate(func(w *model.WheelSetup) { w.Name = " " }), http.StatusBadRequest, "name"},
		{"bad direction", http.MethodPost, "/api/wheels", mutate(func(w *model.WheelSetup) { w.CountingDirection = "up" }), http.StatusBadRequest, "counting_direction"},
		{"unsupported spokes", http.MethodPost, "/api/wheels", mutate(func(w *model.WheelSetup) { w.SpokesCount = 30 }), http.StatusUnprocessableEntity, ""},
		{"bad geometry on update", http.MethodPut, "/api/wheels/1", mutate(func(w *model.WheelSetup) { w.ERDMM = 10 }), http.StatusUnprocessableEntity, ""},
		{"bad update body", http.MethodPut, "/api/wheels/1", `[]`, http.StatusBadRequest, "body"},
		{"non-numeric id", http.MethodGet, "/api/wheels/abc", nil, http.StatusBadRequest, "id"},
		{"zero id", http.MethodGet, "/api/wheels/0", nil, http.StatusBadRequest, "id"},
		{"bad id on update", http.MethodPut, "/api/wheels/x", validWheel(), http.StatusBadRequest, "id"},
		{"bad id on delete", http.MethodDelete, "/api/wheels/-1", nil, http.StatusBadRequest, "id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, tt.body)
			expectStatus(t, rec, tt.wantStatus)
			body := decodeBody[map[string]string](t, rec)
			if body["field"] != tt.wantField || body["error"] == "" {
				t.Errorf("body = %v, want field %q", body, tt.wantField)
			}
		})
	}
}

func TestSessionsFlow(t *testing.T) {
	h := newServer(t)
	expectStatus(t, do(t, h, http.MethodPost, "/api/wheels", validWheel()), http.StatusCreated)
	ms := measurements(32)

	rec := do(t, h, http.MethodPost, "/api/wheels/1/sessions", map[string]any{"measurements": ms})
	expectStatus(t, rec, http.StatusCreated)
	sess := decodeBody[model.Session](t, rec)
	if sess.ID != 1 || sess.WheelID != 1 || len(sess.Measurements) != 32 {
		t.Fatalf("session = %+v", sess)
	}

	rec = do(t, h, http.MethodGet, "/api/wheels/1/sessions", nil)
	expectStatus(t, rec, http.StatusOK)
	if list := decodeBody[[]model.Session](t, rec); len(list) != 1 || list[0].ID != 1 {
		t.Errorf("list = %+v", list)
	}

	rec = do(t, h, http.MethodGet, "/api/sessions/1", nil)
	expectStatus(t, rec, http.StatusOK)
	if got := decodeBody[model.Session](t, rec); len(got.Measurements) != 32 || got.Measurements[8].LateralDevMM != ms[8].LateralDevMM {
		t.Errorf("get = %+v", got)
	}

	rec = do(t, h, http.MethodPost, "/api/sessions/1/solve", nil)
	expectStatus(t, rec, http.StatusOK)
	res := decodeBody[solveResult](t, rec)
	if res.SessionID != 1 {
		t.Errorf("session_id = %d", res.SessionID)
	}
	checkSolve(t, res, 32)

	expectStatus(t, do(t, h, http.MethodDelete, "/api/sessions/1", nil), http.StatusNoContent)
	expectStatus(t, do(t, h, http.MethodGet, "/api/sessions/1", nil), http.StatusNotFound)
	expectStatus(t, do(t, h, http.MethodPost, "/api/sessions/1/solve", nil), http.StatusNotFound)
	expectStatus(t, do(t, h, http.MethodDelete, "/api/sessions/1", nil), http.StatusNotFound)
}

func TestSessionValidation(t *testing.T) {
	h := newServer(t)
	expectStatus(t, do(t, h, http.MethodPost, "/api/wheels", validWheel()), http.StatusCreated)

	allLeft := measurements(32)
	for i := range allLeft {
		allLeft[i].Side = "L"
	}
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
	}{
		{"missing wheel", http.MethodPost, "/api/wheels/7/sessions", map[string]any{"measurements": measurements(32)}, http.StatusNotFound},
		{"list for missing wheel", http.MethodGet, "/api/wheels/7/sessions", nil, http.StatusNotFound},
		{"wrong row count", http.MethodPost, "/api/wheels/1/sessions", map[string]any{"measurements": measurements(28)}, http.StatusBadRequest},
		{"bad body", http.MethodPost, "/api/wheels/1/sessions", `{"rows":[]}`, http.StatusBadRequest},
		{"lacing inconsistent with cross pattern", http.MethodPost, "/api/wheels/1/sessions", map[string]any{"measurements": allLeft}, http.StatusUnprocessableEntity},
		{"bad id on create", http.MethodPost, "/api/wheels/z/sessions", map[string]any{}, http.StatusBadRequest},
		{"bad id on list", http.MethodGet, "/api/wheels/z/sessions", nil, http.StatusBadRequest},
		{"bad id on get", http.MethodGet, "/api/sessions/z", nil, http.StatusBadRequest},
		{"bad id on delete", http.MethodDelete, "/api/sessions/z", nil, http.StatusBadRequest},
		{"bad id on solve", http.MethodPost, "/api/sessions/z/solve", nil, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectStatus(t, do(t, h, tt.method, tt.path, tt.body), tt.wantStatus)
		})
	}
}

func TestStatelessSolve(t *testing.T) {
	h := newServer(t)
	rec := do(t, h, http.MethodPost, "/api/solve", map[string]any{"wheel": validWheel(), "measurements": measurements(32)})
	expectStatus(t, rec, http.StatusOK)
	res := decodeBody[solveResult](t, rec)
	if strings.Contains(rec.Body.String(), "session_id") {
		t.Error("stateless solve must omit session_id")
	}
	checkSolve(t, res, 32)

	// A perfectly true wheel needs no actions and must encode them as [].
	flat := measurements(32)
	for i := range flat {
		flat[i].LateralDevMM = 0
	}
	rec = do(t, h, http.MethodPost, "/api/solve", map[string]any{"wheel": validWheel(), "measurements": flat})
	expectStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"actions":[]`) {
		t.Errorf("want empty actions array, got %s", rec.Body.String())
	}

	bad := validWheel()
	bad.Name = ""
	expectStatus(t, do(t, h, http.MethodPost, "/api/solve", map[string]any{"wheel": bad, "measurements": measurements(32)}), http.StatusBadRequest)
	expectStatus(t, do(t, h, http.MethodPost, "/api/solve", `nope`), http.StatusBadRequest)
}

func TestSolverParamErrors(t *testing.T) {
	body := map[string]any{"wheel": validWheel(), "measurements": measurements(32)}

	invalid := solver.DefaultParams()
	invalid.TurnStep = 0
	expectStatus(t, do(t, newServer(t, api.WithSolverParams(invalid)), http.MethodPost, "/api/solve", body), http.StatusUnprocessableEntity)

	starved := solver.DefaultParams()
	starved.MaxIterations = 1
	expectStatus(t, do(t, newServer(t, api.WithSolverParams(starved)), http.MethodPost, "/api/solve", body), http.StatusInternalServerError)
}

func TestStaticAndHeaders(t *testing.T) {
	static := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Wheel Tuner</title>")}}
	h := newServer(t, api.WithStatic(static))

	rec := do(t, h, http.MethodGet, "/", nil)
	expectStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "Wheel Tuner") {
		t.Errorf("body = %s", rec.Body.String())
	}
	for _, hdr := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy"} {
		if rec.Header().Get(hdr) == "" {
			t.Errorf("missing header %s", hdr)
		}
	}
	expectStatus(t, do(t, newServer(t), http.MethodGet, "/", nil), http.StatusNotFound)
}

type fakeRepo struct {
	err      error // returned by every method
	wheelErr error // returned by GetWheel
	wheel    model.WheelSetup
	session  model.Session
	panicV   any // panics in ListWheels
}

func (f *fakeRepo) CreateWheel(_ context.Context, w model.WheelSetup) (model.WheelSetup, error) {
	return w, f.err
}

func (f *fakeRepo) GetWheel(context.Context, int64) (model.WheelSetup, error) {
	return f.wheel, f.wheelErr
}

func (f *fakeRepo) ListWheels(context.Context) ([]model.WheelSetup, error) {
	if f.panicV != nil {
		panic(f.panicV)
	}
	return nil, f.err
}

func (f *fakeRepo) UpdateWheel(context.Context, model.WheelSetup) error { return f.err }
func (f *fakeRepo) DeleteWheel(context.Context, int64) error            { return f.err }

func (f *fakeRepo) CreateSession(context.Context, int64, []model.Measurement) (model.Session, error) {
	return model.Session{}, f.err
}

func (f *fakeRepo) GetSession(context.Context, int64) (model.Session, error) {
	return f.session, f.err
}

func (f *fakeRepo) ListSessions(context.Context, int64) ([]model.Session, error) {
	return nil, f.err
}

func (f *fakeRepo) DeleteSession(context.Context, int64) error { return f.err }
func (f *fakeRepo) Ping(context.Context) error                 { return f.err }

func (f *fakeRepo) ListRims(context.Context) ([]model.RimComponent, error) { return nil, f.err }
func (f *fakeRepo) ListHubs(context.Context) ([]model.HubComponent, error) { return nil, f.err }

func (f *fakeRepo) GetRim(context.Context, int64) (model.RimComponent, error) {
	return model.RimComponent{}, f.err
}

func (f *fakeRepo) GetHub(context.Context, int64) (model.HubComponent, error) {
	return model.HubComponent{}, f.err
}

func TestRepositoryFailures(t *testing.T) {
	boom := errors.New("boom")
	wheel := validWheel()
	wheel.Normalize()
	sessionBody := map[string]any{"measurements": measurements(32)}

	tests := []struct {
		name   string
		repo   *fakeRepo
		method string
		path   string
		body   any
	}{
		{"list wheels", &fakeRepo{err: boom}, http.MethodGet, "/api/wheels", nil},
		{"create wheel", &fakeRepo{err: boom}, http.MethodPost, "/api/wheels", validWheel()},
		{"get wheel", &fakeRepo{wheelErr: boom}, http.MethodGet, "/api/wheels/1", nil},
		{"update wheel", &fakeRepo{err: boom}, http.MethodPut, "/api/wheels/1", validWheel()},
		{"delete wheel", &fakeRepo{err: boom}, http.MethodDelete, "/api/wheels/1", nil},
		{"list sessions", &fakeRepo{err: boom}, http.MethodGet, "/api/wheels/1/sessions", nil},
		{"create session: wheel lookup", &fakeRepo{wheelErr: boom}, http.MethodPost, "/api/wheels/1/sessions", sessionBody},
		{"create session: insert", &fakeRepo{err: boom, wheel: wheel}, http.MethodPost, "/api/wheels/1/sessions", sessionBody},
		{"get session", &fakeRepo{err: boom}, http.MethodGet, "/api/sessions/1", nil},
		{"delete session", &fakeRepo{err: boom}, http.MethodDelete, "/api/sessions/1", nil},
		{"solve session: wheel lookup", &fakeRepo{wheelErr: boom}, http.MethodPost, "/api/sessions/1/solve", nil},
		{"list rims", &fakeRepo{err: boom}, http.MethodGet, "/api/components/rims", nil},
		{"get rim", &fakeRepo{err: boom}, http.MethodGet, "/api/components/rims/1", nil},
		{"list hubs", &fakeRepo{err: boom}, http.MethodGet, "/api/components/hubs", nil},
		{"get hub", &fakeRepo{err: boom}, http.MethodGet, "/api/components/hubs/1", nil},
		{"create wheel: rim lookup", &fakeRepo{err: boom}, http.MethodPost, "/api/wheels", withComponents(1, 0)},
		{"update wheel: hub lookup", &fakeRepo{err: boom}, http.MethodPut, "/api/wheels/1", withComponents(0, 1)},
		{"panic", &fakeRepo{panicV: "kaboom"}, http.MethodGet, "/api/wheels", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, api.New(tt.repo, api.WithLogger(quietLog)), tt.method, tt.path, tt.body)
			expectStatus(t, rec, http.StatusInternalServerError)
			if body := decodeBody[map[string]string](t, rec); body["error"] != "internal error" {
				t.Errorf("leaked error details: %v", body)
			}
		})
	}
}

func TestCorruptStoredWheel(t *testing.T) {
	ms := measurements(32)
	for _, mut := range []func(*model.WheelSetup){
		func(w *model.WheelSetup) { w.CountingDirection = "sideways" },
		func(w *model.WheelSetup) { w.ViewPoint = "above" },
	} {
		wheel := validWheel()
		wheel.Normalize()
		mut(&wheel)
		repo := &fakeRepo{wheel: wheel, session: model.Session{ID: 1, WheelID: 1, Measurements: ms}}
		rec := do(t, api.New(repo, api.WithLogger(quietLog)), http.MethodPost, "/api/sessions/1/solve", nil)
		expectStatus(t, rec, http.StatusBadRequest)
	}
}

func TestAbortHandlerPropagates(t *testing.T) {
	h := api.New(&fakeRepo{panicV: http.ErrAbortHandler}, api.WithLogger(quietLog))
	defer func() {
		if err, ok := recover().(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler", err)
		}
	}()
	do(t, h, http.MethodGet, "/api/wheels", nil)
	t.Error("expected panic")
}

func TestUnencodableResponse(t *testing.T) {
	wheel := validWheel()
	wheel.ERDMM = math.NaN()
	rec := do(t, api.New(&fakeRepo{wheel: wheel}, api.WithLogger(quietLog)), http.MethodGet, "/api/wheels/1", nil)
	// Headers are already committed when encoding fails; the body is truncated.
	expectStatus(t, rec, http.StatusOK)
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q", rec.Body.String())
	}
}
