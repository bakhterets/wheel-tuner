package api_test

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/bakhterets/wheel-tuner/internal/api"
	"github.com/bakhterets/wheel-tuner/internal/model"
	"github.com/bakhterets/wheel-tuner/internal/solver"
	"github.com/bakhterets/wheel-tuner/internal/tensiometer"
)

// withComponents returns validWheel referring to library components; 0 means none.
func withComponents(rim, hub int64) model.WheelSetup {
	w := validWheel()
	if rim != 0 {
		w.RimComponentID = &rim
	}
	if hub != 0 {
		w.HubComponentID = &hub
	}
	return w
}

// fadeOnF572 is a DT Swiss F 572 29" rim on a Newmen Fade SP front Boost hub.
func fadeOnF572(t *testing.T, h http.Handler) model.WheelSetup {
	t.Helper()
	rims := decodeBody[[]model.RimComponent](t, do(t, h, http.MethodGet, "/api/components/rims", nil))
	hubs := decodeBody[[]model.HubComponent](t, do(t, h, http.MethodGet, "/api/components/hubs", nil))
	var rim model.RimComponent
	for _, r := range rims {
		if r.Model == "F 572" && r.WheelSize == "29" {
			rim = r
		}
	}
	var hub model.HubComponent
	for _, c := range hubs {
		if c.Brand == "Newmen" && c.Model == "Fade SP Front 15x110 Boost 6-bolt/CL" {
			hub = c
		}
	}
	if rim.ID == 0 || hub.ID == 0 {
		t.Fatalf("seeded F 572 / Fade SP not found: %+v %+v", rim, hub)
	}
	return model.WheelSetup{
		Name: "Front F 572 / Fade SP", SpokesCount: hub.HoleCounts[0], ERDMM: rim.ERDMM, CrossPattern: 2,
		FlangeDistLeftMM: hub.FlangeDistLeftMM, FlangeDistRightMM: hub.FlangeDistRightMM,
		FlangeDiamLeftMM: hub.FlangeDiamLeftMM, FlangeDiamRightMM: hub.FlangeDiamRightMM,
		SpokeType: hub.SpokeType, RimAsymmetryOffsetMM: rim.AsymmetryOffsetMM, MaxTensionKgf: rim.MaxTensionKgf,
		RimComponentID: &rim.ID, HubComponentID: &hub.ID, ViewPoint: "Rotor Side", CountingDirection: "CW",
	}
}

func TestComponentLibrary(t *testing.T) {
	h := newServer(t)
	w := fadeOnF572(t, h)
	if w.ERDMM != 602 || w.SpokesCount != 28 || w.MaxTensionKgf != 122 || w.SpokeType != model.SpokeStraightPull {
		t.Fatalf("library wheel = %+v", w)
	}
	rec := do(t, h, http.MethodGet, "/api/components/rims/"+itoa(*w.RimComponentID), nil)
	expectStatus(t, rec, http.StatusOK)
	if got := decodeBody[model.RimComponent](t, rec); got.Model != "F 572" || !equalInts(got.HoleCounts, []int{28, 32}) {
		t.Errorf("rim = %+v", got)
	}
	rec = do(t, h, http.MethodGet, "/api/components/hubs/"+itoa(*w.HubComponentID), nil)
	expectStatus(t, rec, http.StatusOK)
	if got := decodeBody[model.HubComponent](t, rec); got.FlangeDiamLeftMM != 38.5 || got.Position != "front" {
		t.Errorf("hub = %+v", got)
	}
	for _, path := range []string{"/api/components/rims/9999", "/api/components/hubs/9999"} {
		expectStatus(t, do(t, h, http.MethodGet, path, nil), http.StatusNotFound)
	}
	for _, path := range []string{"/api/components/rims/x", "/api/components/hubs/0"} {
		expectStatus(t, do(t, h, http.MethodGet, path, nil), http.StatusBadRequest)
	}

	rec = do(t, h, http.MethodPost, "/api/wheels", w)
	expectStatus(t, rec, http.StatusCreated)
	created := decodeBody[model.WheelSetup](t, rec)
	if *created.RimComponentID != *w.RimComponentID || created.FlangeDiamRightMM != 38.5 {
		t.Fatalf("created = %+v", created)
	}
	tests := []struct {
		name  string
		body  model.WheelSetup
		field string
	}{
		{"unknown rim", withComponents(9999, 0), "rim_component_id"},
		{"unknown hub", withComponents(0, 9999), "hub_component_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, m := range []string{http.MethodPost, http.MethodPut} {
				path := "/api/wheels"
				if m == http.MethodPut {
					path += "/" + itoa(created.ID)
				}
				rec := do(t, h, m, path, tc.body)
				expectStatus(t, rec, http.StatusBadRequest)
				if body := decodeBody[map[string]string](t, rec); body["field"] != tc.field {
					t.Errorf("%s: body = %v, want field %s", m, body, tc.field)
				}
			}
		})
	}
}

func TestTensiometerCatalog(t *testing.T) {
	rec := do(t, newServer(t), http.MethodGet, "/api/tensiometers", nil)
	expectStatus(t, rec, http.StatusOK)
	got := decodeBody[[]tensiometer.Model](t, rec)
	if len(got) != len(tensiometer.Catalog()) || got[0].ID != "ztto-tc-02" || len(got[0].Gauges) != 7 {
		t.Fatalf("tensiometers = %+v", got)
	}
}

func TestGeometryEndpoint(t *testing.T) {
	h := newServer(t)
	w := fadeOnF572(t, h)
	rec := do(t, h, http.MethodPost, "/api/geometry", w)
	expectStatus(t, rec, http.StatusOK)
	type side struct {
		FlangeOffsetMM    float64 `json:"flange_offset_mm"`
		SpokeLengthMM     float64 `json:"spoke_length_mm"`
		BracingAngleDeg   float64 `json:"bracing_angle_deg"`
		TensionPerTurnKgf float64 `json:"tension_per_turn_kgf"`
	}
	g := decodeBody[struct {
		Left, Right  side
		TensionRatio float64 `json:"tension_ratio"`
	}](t, rec)
	// Spec formula, 28h 2x: L = √(D² + F² + (ERD/2)² − D·ERD·cos(720·2/28°)).
	d, f, erd := 38.5/2, 29.0, 602.0
	want := math.Sqrt(d*d + f*f + erd*erd/4 - d*erd*math.Cos(720*2/28.0*math.Pi/180))
	if math.Abs(g.Left.SpokeLengthMM-want) > 1e-6 || g.Left.FlangeOffsetMM != 29 {
		t.Fatalf("left = %+v, want length %v", g.Left, want)
	}
	// The disc-side flange sits closer to the centre and carries more tension.
	if !(g.TensionRatio > 1.1 && g.TensionRatio < 1.25) || !(g.Left.BracingAngleDeg < g.Right.BracingAngleDeg) {
		t.Fatalf("geometry = %+v", g)
	}

	// Paired straight-pull seats (the user's 3x build): θ = 3·720/28 + 360/28 = 90°.
	paired := w
	paired.CrossPattern, paired.PairedSpokes = 3, true
	rec = do(t, h, http.MethodPost, "/api/geometry", paired)
	expectStatus(t, rec, http.StatusOK)
	pg := decodeBody[struct{ Left, Right side }](t, rec)
	if math.Abs(pg.Left.SpokeLengthMM-303.006) > 1e-3 || math.Abs(pg.Right.SpokeLengthMM-303.525) > 1e-3 {
		t.Fatalf("paired 3x lengths = %v / %v, want 303.006 / 303.525", pg.Left.SpokeLengthMM, pg.Right.SpokeLengthMM)
	}
	bad := w
	bad.PairedSpokes, bad.SpokeType = true, model.SpokeJBend
	expectStatus(t, do(t, h, http.MethodPost, "/api/geometry", bad), http.StatusBadRequest)
	bad = w
	bad.SpokeType = "bladed"
	expectStatus(t, do(t, h, http.MethodPost, "/api/geometry", bad), http.StatusBadRequest)
	bad = w
	bad.RimAsymmetryOffsetMM = 40
	expectStatus(t, do(t, h, http.MethodPost, "/api/geometry", bad), http.StatusUnprocessableEntity)
	expectStatus(t, do(t, h, http.MethodPost, "/api/geometry", `[`), http.StatusBadRequest)

	invalid := solver.DefaultParams()
	invalid.SpokeRigidityKgf = 0
	rec = do(t, newServer(t, api.WithSolverParams(invalid)), http.MethodPost, "/api/geometry", validWheel())
	expectStatus(t, rec, http.StatusUnprocessableEntity)
}

func TestTensiometerReadings(t *testing.T) {
	h := newServer(t)
	w := validWheel()
	w.Tensiometer, w.SpokeGaugeLeft, w.SpokeGaugeRight = "ztto-tc-02", "2.0-1.8-2.0", "1.8"
	expectStatus(t, do(t, h, http.MethodPost, "/api/wheels", w), http.StatusCreated)
	expectStatus(t, do(t, h, http.MethodPost, "/api/wheels", validWheel()), http.StatusCreated)

	reading := func(ms []model.Measurement, v float64) []model.Measurement {
		for i := range ms {
			ms[i].TensionKgf = 0
			ms[i].TensionReading = &v
		}
		return ms
	}
	rec := do(t, h, http.MethodPost, "/api/wheels/1/sessions", map[string]any{"measurements": reading(measurements(32), 2.8)})
	expectStatus(t, rec, http.StatusCreated)
	sess := decodeBody[model.Session](t, rec)
	// TC-02 at 2.8 mm: 102 kgf for 2.0-1.8-2.0 (left), 100 kgf for 1.8 (right).
	if m := sess.Measurements; m[0].TensionKgf != 102 || m[1].TensionKgf != 100 || *m[1].TensionReading != 2.8 {
		t.Fatalf("converted = %+v / %+v", m[0], m[1])
	}
	rec = do(t, h, http.MethodGet, "/api/sessions/1", nil)
	if m := decodeBody[model.Session](t, rec).Measurements; m[0].TensionKgf != 102 || *m[0].TensionReading != 2.8 {
		t.Fatalf("stored = %+v", m[0])
	}
	expectStatus(t, do(t, h, http.MethodPost, "/api/sessions/1/solve", nil), http.StatusOK)

	tests := []struct {
		name string
		path string
		body any
	}{
		{"out of range", "/api/wheels/1/sessions", map[string]any{"measurements": reading(measurements(32), 9)}},
		{"no tensiometer", "/api/wheels/2/sessions", map[string]any{"measurements": reading(measurements(32), 2.8)}},
		{"wrong row count", "/api/wheels/1/sessions", map[string]any{"measurements": reading(measurements(4), 2.8)}},
		{"stateless out of range", "/api/solve", map[string]any{"wheel": w, "measurements": reading(measurements(32), 1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, tc.path, tc.body)
			expectStatus(t, rec, http.StatusBadRequest)
			if body := decodeBody[map[string]string](t, rec); !strings.HasPrefix(body["field"], "measurements") {
				t.Errorf("body = %v", body)
			}
		})
	}

	rec = do(t, h, http.MethodPost, "/api/solve", map[string]any{"wheel": w, "measurements": reading(measurements(32), 2.8)})
	expectStatus(t, rec, http.StatusOK)
	if res := decodeBody[solveResult](t, rec); len(res.Predicted) != 32 || res.Predicted[0]["tension_kgf"].(float64) == 0 {
		t.Fatalf("stateless solve with readings: %+v", res.Predicted[:2])
	}
}

func TestTensionWarnings(t *testing.T) {
	h := newServer(t)
	type result struct {
		MaxTensionKgf float64 `json:"max_tension_kgf"`
		Actions       []struct {
			PredictedTensionKgf float64 `json:"predicted_tension_kgf"`
			TensionWarning      bool    `json:"tension_warning"`
		} `json:"actions"`
		Predicted []struct {
			TensionKgf     float64 `json:"tension_kgf"`
			TensionWarning bool    `json:"tension_warning"`
		} `json:"predicted"`
	}
	solve := func(maxKgf float64, ms []model.Measurement) result {
		w := validWheel()
		w.MaxTensionKgf = maxKgf
		rec := do(t, h, http.MethodPost, "/api/solve", map[string]any{"wheel": w, "measurements": ms})
		expectStatus(t, rec, http.StatusOK)
		return decodeBody[result](t, rec)
	}
	// All spokes sit at ≈110 kgf: with a 112 kgf rim limit (95 % = 106.4)
	// every measured spoke warns; without a limit none does.
	for _, tc := range []struct {
		maxKgf float64
		warn   bool
	}{{112, true}, {0, false}, {1000, false}} {
		res := solve(tc.maxKgf, measurements(32))
		if res.MaxTensionKgf != tc.maxKgf || len(res.Actions) == 0 {
			t.Fatalf("max %v: %+v", tc.maxKgf, res)
		}
		warned := false
		for _, a := range res.Actions {
			want := tc.maxKgf > 0 && a.PredictedTensionKgf >= 0.95*tc.maxKgf
			warned = warned || a.TensionWarning
			if a.TensionWarning != want || !(a.PredictedTensionKgf > 0) {
				t.Errorf("max %v: action %+v", tc.maxKgf, a)
			}
		}
		if warned != tc.warn {
			t.Errorf("max %v: any action warning = %v, want %v", tc.maxKgf, warned, tc.warn)
		}
		for _, p := range res.Predicted {
			if p.TensionWarning != (tc.maxKgf > 0 && p.TensionKgf >= 0.95*tc.maxKgf) {
				t.Errorf("max %v: predicted %+v", tc.maxKgf, p)
			}
			// The rim maximum is also enforced as a hard cap.
			if tc.maxKgf > 0 && p.TensionKgf > tc.maxKgf+1e-6 {
				t.Errorf("max %v: predicted %+v over the rim limit", tc.maxKgf, p)
			}
		}
	}
	// Unmeasured tensions are predicted as 0 and never warn.
	ms := measurements(32)
	for i := range ms {
		ms[i].TensionKgf = 0
	}
	for _, a := range solve(112, ms).Actions {
		if a.TensionWarning || a.PredictedTensionKgf != 0 {
			t.Errorf("unmeasured action %+v", a)
		}
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTensionLimit(t *testing.T) {
	h := newServer(t)
	type result struct {
		LimitLeft  float64 `json:"tension_limit_left_kgf"`
		LimitRight float64 `json:"tension_limit_right_kgf"`
		Predicted  []struct {
			Side       string  `json:"side"`
			TensionKgf float64 `json:"tension_kgf"`
		} `json:"predicted"`
	}
	tc02 := func(g string, reading float64) float64 {
		gauge, err := tensiometer.Lookup("ztto-tc-02", g)
		if err != nil {
			t.Fatal(err)
		}
		kgf, err := gauge.Kgf(reading)
		if err != nil {
			t.Fatal(err)
		}
		return kgf
	}
	withReadingLimit := func(reading float64) model.WheelSetup {
		w := validWheel()
		w.Tensiometer, w.SpokeGaugeLeft, w.SpokeGaugeRight = "ztto-tc-02", "2.0-1.8-2.0", "1.8"
		w.TensionLimit, w.TensionLimitUnit = reading, "Reading"
		return w
	}
	kgfLimit := validWheel()
	kgfLimit.TensionLimit = 108
	tests := []struct {
		name        string
		wheel       model.WheelSetup
		left, right float64
	}{
		{"no limit", validWheel(), 0, 0},
		{"desired limit in kgf", kgfLimit, 108, 108},
		{"TC-02 reading per side gauge", withReadingLimit(2.85), tc02("2.0-1.8-2.0", 2.85), tc02("1.8", 2.85)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/solve", map[string]any{"wheel": tc.wheel, "measurements": measurements(32)})
			expectStatus(t, rec, http.StatusOK)
			res := decodeBody[result](t, rec)
			if res.LimitLeft != tc.left || res.LimitRight != tc.right {
				t.Fatalf("limits = %v / %v, want %v / %v", res.LimitLeft, res.LimitRight, tc.left, tc.right)
			}
			for _, p := range res.Predicted {
				limit := map[string]float64{"L": tc.left, "R": tc.right}[p.Side]
				if limit > 0 && p.TensionKgf > limit+1e-6 {
					t.Fatalf("predicted %+v over the %v kgf limit", p, limit)
				}
			}
		})
	}

	// The limit is stored with the profile and enforced on stored sessions.
	rec := do(t, h, http.MethodPost, "/api/wheels", withReadingLimit(2.85))
	expectStatus(t, rec, http.StatusCreated)
	if w := decodeBody[model.WheelSetup](t, rec); w.TensionLimit != 2.85 || w.TensionLimitUnit != model.LimitUnitReading {
		t.Fatalf("stored limit = %v %q", w.TensionLimit, w.TensionLimitUnit)
	}

	for name, tc := range map[string]struct {
		wheel model.WheelSetup
		field string
	}{
		"reading outside the gauge table": {withReadingLimit(9), "tension_limit"},
		"reading without tensiometer": {func() model.WheelSetup {
			w := validWheel()
			w.TensionLimit, w.TensionLimitUnit = 2.8, model.LimitUnitReading
			return w
		}(), "tension_limit_unit"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/api/wheels", "/api/geometry"} {
				rec := do(t, h, http.MethodPost, path, tc.wheel)
				expectStatus(t, rec, http.StatusBadRequest)
				if body := decodeBody[map[string]string](t, rec); body["field"] != tc.field {
					t.Errorf("%s: body = %v, want field %s", path, body, tc.field)
				}
			}
		})
	}
}
