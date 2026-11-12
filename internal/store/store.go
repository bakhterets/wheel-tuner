// Package store persists wheel profiles, truing sessions and measurements in
// SQLite (pure-Go driver modernc.org/sqlite, no CGO required).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bakhterets/wheel-tuner/internal/model"

	// Registers the "sqlite" database/sql driver.
	_ "modernc.org/sqlite"
)

// migrations are applied in order; PRAGMA user_version records how many ran.
// Version 1 is the stage-01 schema: its IF NOT EXISTS clauses make it a no-op
// on databases created before versioning was introduced.
func migrations() []string {
	return []string{schemaV1, schemaV2}
}

const schemaV1 = `
CREATE TABLE IF NOT EXISTS wheel_setups (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	name                 TEXT    NOT NULL,
	spokes_count         INTEGER NOT NULL,
	erd_mm               REAL    NOT NULL,
	cross_pattern        INTEGER NOT NULL,
	flange_dist_left_mm  REAL    NOT NULL,
	flange_dist_right_mm REAL    NOT NULL,
	start_reference      TEXT    NOT NULL DEFAULT 'Rim Seam',
	view_point           TEXT    NOT NULL DEFAULT 'Rotor Side',
	counting_direction   TEXT    NOT NULL DEFAULT 'CW' CHECK (counting_direction IN ('CW', 'CCW'))
);

CREATE TABLE IF NOT EXISTS sessions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	wheel_id   INTEGER  NOT NULL REFERENCES wheel_setups(id) ON DELETE CASCADE,
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_wheel ON sessions(wheel_id);

CREATE TABLE IF NOT EXISTS measurements (
	id                    INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id            INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	spoke_index           INTEGER NOT NULL CHECK (spoke_index >= 1),
	side                  TEXT    NOT NULL CHECK (side IN ('L', 'R')),
	tension_kgf           REAL    NOT NULL DEFAULT 0,
	lateral_dev_mm        REAL    NOT NULL DEFAULT 0,
	lat_peak_between_next BOOLEAN NOT NULL DEFAULT 0,
	radial_dev_mm         REAL    NOT NULL DEFAULT 0,
	rad_peak_between_next BOOLEAN NOT NULL DEFAULT 0,
	UNIQUE (session_id, spoke_index)
);
`

// schemaV2 adds the component library, rim/hub geometry and tensiometer
// settings of a profile, and raw tensiometer readings. hole_counts is a
// comma-separated list ("28,32").
const schemaV2 = `
CREATE TABLE rim_components (
	id                  INTEGER PRIMARY KEY AUTOINCREMENT,
	brand               TEXT NOT NULL,
	model               TEXT NOT NULL,
	wheel_size          TEXT NOT NULL DEFAULT '',
	hole_counts         TEXT NOT NULL DEFAULT '',
	erd_mm              REAL NOT NULL,
	asymmetry_offset_mm REAL NOT NULL DEFAULT 0,
	max_tension_kgf     REAL NOT NULL DEFAULT 0,
	source              TEXT NOT NULL DEFAULT '',
	UNIQUE (brand, model, wheel_size)
);

CREATE TABLE hub_components (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	brand                TEXT NOT NULL,
	model                TEXT NOT NULL,
	position             TEXT NOT NULL CHECK (position IN ('front', 'rear')),
	axle                 TEXT NOT NULL DEFAULT '',
	flange_dist_left_mm  REAL NOT NULL,
	flange_dist_right_mm REAL NOT NULL,
	flange_diam_left_mm  REAL NOT NULL DEFAULT 0,
	flange_diam_right_mm REAL NOT NULL DEFAULT 0,
	spoke_type           TEXT NOT NULL DEFAULT 'J-Bend' CHECK (spoke_type IN ('J-Bend', 'Straightpull')),
	hole_counts          TEXT NOT NULL DEFAULT '',
	source               TEXT NOT NULL DEFAULT '',
	UNIQUE (brand, model)
);

ALTER TABLE wheel_setups ADD COLUMN rim_component_id INTEGER REFERENCES rim_components(id) ON DELETE SET NULL;
ALTER TABLE wheel_setups ADD COLUMN hub_component_id INTEGER REFERENCES hub_components(id) ON DELETE SET NULL;
ALTER TABLE wheel_setups ADD COLUMN rim_asymmetry_offset_mm REAL NOT NULL DEFAULT 0;
ALTER TABLE wheel_setups ADD COLUMN flange_diam_left_mm REAL NOT NULL DEFAULT 0;
ALTER TABLE wheel_setups ADD COLUMN flange_diam_right_mm REAL NOT NULL DEFAULT 0;
ALTER TABLE wheel_setups ADD COLUMN spoke_type TEXT NOT NULL DEFAULT 'J-Bend';
ALTER TABLE wheel_setups ADD COLUMN max_tension_kgf REAL NOT NULL DEFAULT 0;
ALTER TABLE wheel_setups ADD COLUMN tensiometer TEXT NOT NULL DEFAULT '';
ALTER TABLE wheel_setups ADD COLUMN spoke_gauge_left TEXT NOT NULL DEFAULT '';
ALTER TABLE wheel_setups ADD COLUMN spoke_gauge_right TEXT NOT NULL DEFAULT '';

ALTER TABLE measurements ADD COLUMN tension_reading REAL;
`

// Store is a SQLite-backed repository. It is safe for concurrent use.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Option configures a Store.
type Option func(*Store)

// WithClock overrides the clock used for session timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *Store) { s.now = now }
}

// Open opens (creating if needed) the database at path, migrates its schema
// and refreshes the component library. An empty path or ":memory:" opens a
// private in-memory database.
func Open(ctx context.Context, path string, opts ...Option) (*Store, error) {
	if path == "" {
		path = ":memory:"
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite serialises writers anyway; a single long-lived connection also
	// keeps in-memory databases alive.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	if err := migrate(ctx, db, migrations()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := seedComponents(ctx, db, rimSeeds(), hubSeeds()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	s := &Store{db: db, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// migrate applies the pending steps, each in its own transaction.
func migrate(ctx context.Context, db *sql.DB, steps []string) error {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(steps) {
		return fmt.Errorf("schema version %d is newer than this build supports (%d)", version, len(steps))
	}
	for v := version; v < len(steps); v++ {
		if err := migrateStep(ctx, db, v+1, steps[v]); err != nil {
			return fmt.Errorf("migrate schema to version %d: %w", v+1, err)
		}
	}
	return nil
}

func migrateStep(ctx context.Context, db *sql.DB, version int, ddl string) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx, &err)
	// PRAGMA does not accept bound parameters; version is an int.
	if _, err := tx.ExecContext(ctx, ddl+fmt.Sprintf("\nPRAGMA user_version = %d;", version)); err != nil {
		return err
	}
	return tx.Commit()
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Ping verifies the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func rollback(tx *sql.Tx, err *error) {
	if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
		*err = errors.Join(*err, rbErr)
	}
}

func affectedOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}
