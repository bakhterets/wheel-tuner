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

const schema = `
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

// Open opens (creating if needed) the database at path and applies the
// schema. An empty path or ":memory:" opens a private in-memory database.
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
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return nil, errors.Join(fmt.Errorf("apply schema: %w", err), db.Close())
	}
	s := &Store{db: db, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s, nil
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
