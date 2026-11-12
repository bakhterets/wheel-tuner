package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bakhterets/wheel-tuner/internal/model"
)

func findRim(t *testing.T, rims []model.RimComponent, name, size string) model.RimComponent {
	t.Helper()
	for _, r := range rims {
		if r.Brand == "DT Swiss" && r.Model == name && r.WheelSize == size {
			return r
		}
	}
	t.Fatalf("rim %s %s not seeded", name, size)
	return model.RimComponent{}
}

func findHub(t *testing.T, hubs []model.HubComponent, brand, name string) model.HubComponent {
	t.Helper()
	for _, h := range hubs {
		if h.Brand == brand && h.Model == name {
			return h
		}
	}
	t.Fatalf("hub %s %s not seeded", brand, name)
	return model.HubComponent{}
}

func TestComponentLibrarySeeded(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	rims, err := s.ListRims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rims) != len(rimSeeds()) {
		t.Fatalf("%d rims, want %d", len(rims), len(rimSeeds()))
	}
	for i := 1; i < len(rims); i++ {
		a, b := rims[i-1], rims[i]
		if a.Brand+a.Model+a.WheelSize > b.Brand+b.Model+b.WheelSize {
			t.Fatalf("rims not ordered: %s %s before %s %s", a.Model, a.WheelSize, b.Model, b.WheelSize)
		}
	}
	f572 := findRim(t, rims, "F 572", "29")
	want := model.RimComponent{ID: f572.ID, Brand: "DT Swiss", Model: "F 572", WheelSize: "29",
		HoleCounts: []int{28, 32}, ERDMM: 602, MaxTensionKgf: 122,
		Source: "https://www.dtswiss.com/en/components/rims-mtb/free-ride/f-572"}
	if !reflect.DeepEqual(f572, want) {
		t.Fatalf("F 572 29 = %+v, want %+v", f572, want)
	}
	if r := findRim(t, rims, "F 572", "27.5"); r.ERDMM != 564 {
		t.Fatalf("F 572 27.5 ERD = %v, want 564", r.ERDMM)
	}
	got, err := s.GetRim(ctx, f572.ID)
	if err != nil || !reflect.DeepEqual(got, f572) {
		t.Fatalf("GetRim = %+v, %v", got, err)
	}

	hubs, err := s.ListHubs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hubs) != len(hubSeeds()) {
		t.Fatalf("%d hubs, want %d", len(hubs), len(hubSeeds()))
	}
	fade := findHub(t, hubs, "Newmen", "Fade SP Front 15x110 Boost 6-bolt/CL")
	wantHub := model.HubComponent{ID: fade.ID, Brand: "Newmen", Model: "Fade SP Front 15x110 Boost 6-bolt/CL",
		Position: "front", Axle: "15x110 Boost", FlangeDistLeftMM: 29, FlangeDistRightMM: 34,
		FlangeDiamLeftMM: 38.5, FlangeDiamRightMM: 38.5, SpokeType: model.SpokeStraightPull,
		HoleCounts: []int{28}, Source: srcNewmenHubs}
	if !reflect.DeepEqual(fade, wantHub) {
		t.Fatalf("Fade SP front = %+v, want %+v", fade, wantHub)
	}
	if h := findHub(t, hubs, "DT Swiss", "240 Rear 12x148 Boost 6-bolt"); len(h.HoleCounts) != 0 || h.Position != "rear" {
		t.Fatalf("DT 240 rear = %+v", h)
	}
	gotHub, err := s.GetHub(ctx, fade.ID)
	if err != nil || !reflect.DeepEqual(gotHub, fade) {
		t.Fatalf("GetHub = %+v, %v", gotHub, err)
	}
	for name, err := range map[string]error{
		"rim": second(s.GetRim(ctx, 9999)),
		"hub": second(s.GetHub(ctx, 9999)),
	} {
		if !errors.Is(err, model.ErrNotFound) {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestSeedUpsertKeepsIDs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wheels.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	rims, err := s.ListRims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f572 := findRim(t, rims, "F 572", "29")
	if _, err := s.db.ExecContext(ctx, `UPDATE rim_components SET erd_mm = 1 WHERE id = ?`, f572.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	again, err := s.ListRims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, rims) {
		t.Fatalf("reseeding must restore data and keep IDs:\n%+v\n%+v", again, rims)
	}
}

func TestComponentDeletionDetachesProfiles(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	w := wheel("w")
	w.RimComponentID, w.HubComponentID = ptr(int64(1)), ptr(int64(1))
	w, err := s.CreateWheel(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DELETE FROM rim_components WHERE id = 1`, `DELETE FROM hub_components WHERE id = 1`} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetWheel(ctx, w.ID)
	if err != nil || got.RimComponentID != nil || got.HubComponentID != nil {
		t.Fatalf("profile after component deletion = %+v, %v", got, err)
	}
	if _, err := s.CreateWheel(ctx, w); err == nil {
		t.Fatal("unknown component IDs must violate the foreign key")
	}
}

func TestCorruptComponentRows(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	for _, q := range []string{`UPDATE rim_components SET hole_counts = '28,x'`, `UPDATE hub_components SET hole_counts = 'x'`} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	for name, err := range map[string]error{
		"list rims": second(s.ListRims(ctx)),
		"get rim":   second(s.GetRim(ctx, 1)),
		"list hubs": second(s.ListHubs(ctx)),
		"get hub":   second(s.GetHub(ctx, 1)),
	} {
		if err == nil || errors.Is(err, model.ErrNotFound) {
			t.Errorf("%s on corrupt hole counts: %v", name, err)
		}
	}
}

// stage01Schema is the schema written by stage-01 builds (no user_version).
const stage01Schema = schemaV1 + `
INSERT INTO wheel_setups (name, spokes_count, erd_mm, cross_pattern, flange_dist_left_mm,
	flange_dist_right_mm, start_reference, view_point, counting_direction)
	VALUES ('legacy', 4, 600, 0, 35, 20, 'Valve', 'Drive Side', 'CCW');
INSERT INTO sessions (wheel_id, created_at) VALUES (1, '2026-01-02 03:04:05+00:00');
INSERT INTO measurements (session_id, spoke_index, side, tension_kgf) VALUES (1, 1, 'L', 100);
`

func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateFromStage01(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stage01.db")
	db := rawDB(t, path)
	if _, err := db.ExecContext(ctx, stage01Schema); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil || version != len(migrations()) {
		t.Fatalf("user_version = %d, %v", version, err)
	}
	w, err := s.GetWheel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := model.WheelSetup{ID: 1, Name: "legacy", SpokesCount: 4, ERDMM: 600, FlangeDistLeftMM: 35,
		FlangeDistRightMM: 20, StartReference: "Valve", ViewPoint: "Drive Side", CountingDirection: "CCW",
		SpokeType: model.SpokeJBend}
	if !reflect.DeepEqual(w, want) {
		t.Fatalf("migrated wheel = %+v, want %+v", w, want)
	}
	sess, err := s.GetSession(ctx, 1)
	if err != nil || len(sess.Measurements) != 1 || sess.Measurements[0].TensionReading != nil ||
		sess.Measurements[0].TensionKgf != 100 {
		t.Fatalf("migrated session = %+v, %v", sess, err)
	}
}

func TestMigrateErrors(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "future.db")
	db := rawDB(t, path)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := migrate(ctx, db, []string{"CREATE TABLE a (x);", "NOT SQL"}); err == nil {
		t.Fatal("a broken migration must fail")
	}
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
		t.Fatalf("user_version after failed step 2 = %d, %v; want 1", version, err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("a database from a newer build must be rejected")
	}
}

func TestSeedComponentsErrors(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	tests := map[string]struct {
		rims []model.RimComponent
		hubs []model.HubComponent
	}{
		"rim": {rims: []model.RimComponent{{Brand: "X", Model: "Y", ERDMM: math.NaN()}}},
		"hub": {hubs: []model.HubComponent{{Brand: "X", Model: "Y", Position: "middle", SpokeType: model.SpokeJBend}}},
	}
	for name, tc := range tests {
		if err := seedComponents(ctx, s.db, tc.rims, tc.hubs); err == nil {
			t.Errorf("invalid %s seed must fail", name)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := seedComponents(cancelled, s.db, nil, nil); err == nil {
		t.Error("cancelled context must fail")
	}
}
