package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bakhterets/wheel-tuner/internal/model"
)

var fixedNow = time.Date(2026, 11, 12, 16, 5, 1, 999, time.FixedZone("CET", 3600))

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), "", WithClock(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func wheel(name string) model.WheelSetup {
	return model.WheelSetup{
		Name:              name,
		SpokesCount:       4,
		ERDMM:             600,
		CrossPattern:      0,
		FlangeDistLeftMM:  35,
		FlangeDistRightMM: 20,
		StartReference:    "Rim Seam",
		ViewPoint:         "Rotor Side",
		CountingDirection: "CCW",
		SpokeType:         model.SpokeJBend,
	}
}

func measurements() []model.Measurement {
	return []model.Measurement{
		{SpokeIndex: 1, Side: "L", TensionKgf: 102, TensionReading: ptr(2.8), LateralDevMM: 0.5, LatPeakBetweenNext: true},
		{SpokeIndex: 2, Side: "R", TensionKgf: 120, RadialDevMM: -0.2, RadPeakBetweenNext: true},
		{SpokeIndex: 3, Side: "L", TensionKgf: 98},
		{SpokeIndex: 4, Side: "R", TensionKgf: 125},
	}
}

func TestWheelCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	list, err := s.ListWheels(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("empty list = %v, %v", list, err)
	}
	b, err := s.CreateWheel(ctx, wheel("b-rear"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateWheel(ctx, wheel("a-front"))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || b.ID == 0 || a.ID == b.ID {
		t.Fatalf("ids %d, %d", a.ID, b.ID)
	}
	got, err := s.GetWheel(ctx, b.ID)
	if err != nil || !reflect.DeepEqual(got, b) {
		t.Fatalf("GetWheel = %+v, %v; want %+v", got, err, b)
	}
	list, err = s.ListWheels(ctx)
	if err != nil || len(list) != 2 || !reflect.DeepEqual(list, []model.WheelSetup{a, b}) {
		t.Fatalf("ListWheels = %+v, %v", list, err)
	}

	b.Name, b.CrossPattern, b.CountingDirection = "b-rear-2", 3, "CW"
	b.RimComponentID, b.HubComponentID, b.RimAsymmetryOffsetMM = ptr(int64(1)), ptr(int64(2)), 2.5
	b.FlangeDiamLeftMM, b.FlangeDiamRightMM, b.SpokeType, b.MaxTensionKgf = 38.5, 38.5, model.SpokeStraightPull, 122
	b.Tensiometer, b.SpokeGaugeLeft, b.SpokeGaugeRight = "ztto-tc-02", "2.0-1.8-2.0", "1.8"
	if err := s.UpdateWheel(ctx, b); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetWheel(ctx, b.ID); err != nil || !reflect.DeepEqual(got, b) {
		t.Fatalf("after update %+v (err %v), want %+v", got, err, b)
	}

	if err := s.DeleteWheel(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"get":    second(s.GetWheel(ctx, b.ID)),
		"update": s.UpdateWheel(ctx, b),
		"delete": s.DeleteWheel(ctx, b.ID),
	} {
		if !errors.Is(err, model.ErrNotFound) {
			t.Errorf("%s missing wheel: %v", name, err)
		}
	}
}

func second[T any](_ T, err error) error { return err }

func ptr[T any](v T) *T { return &v }

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	w, err := s.CreateWheel(ctx, wheel("w"))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.CreateSession(ctx, w.ID, measurements())
	if err != nil {
		t.Fatal(err)
	}
	if want := fixedNow.UTC().Truncate(time.Second); !sess.CreatedAt.Equal(want) {
		t.Fatalf("created_at = %v, want %v", sess.CreatedAt, want)
	}
	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != sess.ID || got.WheelID != w.ID || !got.CreatedAt.Equal(sess.CreatedAt) {
		t.Fatalf("GetSession = %+v, want %+v", got, sess)
	}
	if len(got.Measurements) != 4 {
		t.Fatalf("got %d measurements", len(got.Measurements))
	}
	for i, m := range got.Measurements {
		if !reflect.DeepEqual(m, sess.Measurements[i]) || m.SessionID != sess.ID || m.ID == 0 {
			t.Fatalf("measurement %d = %+v, want %+v", i, m, sess.Measurements[i])
		}
	}

	older := sess
	newer, err := s.CreateSession(ctx, w.ID, measurements())
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListSessions(ctx, w.ID)
	if err != nil || len(list) != 2 || list[0].ID != newer.ID || list[1].ID != older.ID {
		t.Fatalf("ListSessions = %+v, %v", list, err)
	}
	if list[0].Measurements != nil {
		t.Fatal("ListSessions must not load measurements")
	}

	if err := s.DeleteSession(ctx, older.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, older.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("deleted session: %v", err)
	}
	if err := s.DeleteSession(ctx, older.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("delete missing session: %v", err)
	}

	// Deleting the wheel cascades to sessions and measurements.
	if err := s.DeleteWheel(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, newer.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("session survived wheel deletion: %v", err)
	}
	var orphans int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM measurements`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("orphan measurements = %d, %v", orphans, err)
	}
}

func TestSessionErrors(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.CreateSession(ctx, 42, measurements()); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("session for missing wheel: %v", err)
	}
	if _, err := s.ListSessions(ctx, 42); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("list for missing wheel: %v", err)
	}
	w, err := s.CreateWheel(ctx, wheel("w"))
	if err != nil {
		t.Fatal(err)
	}
	dup := measurements()
	dup[1].SpokeIndex = 1
	if _, err := s.CreateSession(ctx, w.ID, dup); err == nil {
		t.Fatal("duplicate spoke index must violate the unique constraint")
	}
	list, err := s.ListSessions(ctx, w.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("failed insert must roll back, got %+v, %v", list, err)
	}
	bad := measurements()
	bad[0].Side = "X"
	if _, err := s.CreateSession(ctx, w.ID, bad); err == nil {
		t.Fatal("invalid side must violate the check constraint")
	}
}

func TestClosedStoreErrors(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	checks := map[string]error{
		"ping":           s.Ping(ctx),
		"create wheel":   second(s.CreateWheel(ctx, wheel("w"))),
		"get wheel":      second(s.GetWheel(ctx, 1)),
		"list wheels":    second(s.ListWheels(ctx)),
		"update wheel":   s.UpdateWheel(ctx, wheel("w")),
		"delete wheel":   s.DeleteWheel(ctx, 1),
		"create session": second(s.CreateSession(ctx, 1, nil)),
		"get session":    second(s.GetSession(ctx, 1)),
		"list sessions":  second(s.ListSessions(ctx, 1)),
		"list rims":      second(s.ListRims(ctx)),
		"get rim":        second(s.GetRim(ctx, 1)),
		"list hubs":      second(s.ListHubs(ctx)),
		"get hub":        second(s.GetHub(ctx, 1)),
		"delete session": s.DeleteSession(ctx, 1),
	}
	for name, err := range checks {
		if err == nil || errors.Is(err, model.ErrNotFound) {
			t.Errorf("%s on closed store: %v", name, err)
		}
	}
}

func TestCorruptRowsSurfaceScanErrors(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	w, err := s.CreateWheel(ctx, wheel("w"))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.CreateSession(ctx, w.ID, measurements())
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE measurements SET spoke_index = 'abc' WHERE session_id = ? AND spoke_index = 4`, sess.ID)
	if _, err := s.GetSession(ctx, sess.ID); err == nil {
		t.Error("GetSession must fail on a corrupt measurement")
	}
	exec(`UPDATE sessions SET created_at = 'not a date' WHERE id = ?`, sess.ID)
	if _, err := s.ListSessions(ctx, w.ID); err == nil {
		t.Error("ListSessions must fail on a corrupt timestamp")
	}
	if _, err := s.GetSession(ctx, sess.ID); err == nil || errors.Is(err, model.ErrNotFound) {
		t.Errorf("GetSession on corrupt session: %v", err)
	}
	exec(`UPDATE wheel_setups SET spokes_count = 'many' WHERE id = ?`, w.ID)
	if _, err := s.ListWheels(ctx); err == nil {
		t.Error("ListWheels must fail on a corrupt wheel")
	}
}

func TestOpenFilePersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wheels.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.CreateWheel(ctx, wheel("persisted"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	if got, err := s.GetWheel(ctx, w.ID); err != nil || !reflect.DeepEqual(got, w) {
		t.Fatalf("reopened wheel = %+v, %v", got, err)
	}
}

func TestOpenErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := Open(ctx, filepath.Join(t.TempDir(), "missing", "dir", "x.db")); err == nil {
		t.Fatal("opening a file in a missing directory must fail")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Open(cancelled, ""); err == nil {
		t.Fatal("cancelled context must fail")
	}
}
