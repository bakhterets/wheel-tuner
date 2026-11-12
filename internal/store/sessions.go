package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bakhterets/wheel-tuner/internal/model"
)

// CreateSession stores a truing session with its measurements atomically.
func (s *Store) CreateSession(ctx context.Context, wheelID int64, ms []model.Measurement) (sess model.Session, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sess, fmt.Errorf("create session: %w", err)
	}
	defer rollback(tx, &err)

	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM wheel_setups WHERE id = ?`, wheelID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = model.ErrNotFound
		}
		return sess, fmt.Errorf("create session for wheel %d: %w", wheelID, err)
	}
	sess = model.Session{WheelID: wheelID, CreatedAt: s.now().UTC().Truncate(time.Second)}
	res, err := tx.ExecContext(ctx, `INSERT INTO sessions (wheel_id, created_at) VALUES (?, ?)`,
		sess.WheelID, sess.CreatedAt)
	if err != nil {
		return sess, fmt.Errorf("insert session: %w", err)
	}
	if sess.ID, err = res.LastInsertId(); err != nil {
		return sess, fmt.Errorf("insert session: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO measurements (session_id, spoke_index, side,
		tension_kgf, tension_reading, lateral_dev_mm, lat_peak_between_next, radial_dev_mm,
		rad_peak_between_next) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return sess, fmt.Errorf("prepare measurement insert: %w", err)
	}
	defer stmt.Close()
	sess.Measurements = make([]model.Measurement, len(ms))
	for i, m := range ms {
		m.SessionID = sess.ID
		res, err := stmt.ExecContext(ctx, m.SessionID, m.SpokeIndex, m.Side, m.TensionKgf, m.TensionReading,
			m.LateralDevMM, m.LatPeakBetweenNext, m.RadialDevMM, m.RadPeakBetweenNext)
		if err != nil {
			return sess, fmt.Errorf("insert measurement %d: %w", m.SpokeIndex, err)
		}
		if m.ID, err = res.LastInsertId(); err != nil {
			return sess, fmt.Errorf("insert measurement %d: %w", m.SpokeIndex, err)
		}
		sess.Measurements[i] = m
	}
	if err := tx.Commit(); err != nil {
		return sess, fmt.Errorf("commit session: %w", err)
	}
	return sess, nil
}

// GetSession returns a session with its measurements ordered by spoke index.
func (s *Store) GetSession(ctx context.Context, id int64) (model.Session, error) {
	var sess model.Session
	err := s.db.QueryRowContext(ctx, `SELECT id, wheel_id, created_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.WheelID, &sess.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = model.ErrNotFound
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("get session %d: %w", id, err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, session_id, spoke_index, side, tension_kgf,
		tension_reading, lateral_dev_mm, lat_peak_between_next, radial_dev_mm, rad_peak_between_next
		FROM measurements WHERE session_id = ? ORDER BY spoke_index`, id)
	if err != nil {
		return model.Session{}, fmt.Errorf("get measurements of session %d: %w", id, err)
	}
	defer rows.Close()
	sess.Measurements = []model.Measurement{}
	for rows.Next() {
		var (
			m       model.Measurement
			reading sql.NullFloat64
		)
		if err := rows.Scan(&m.ID, &m.SessionID, &m.SpokeIndex, &m.Side, &m.TensionKgf, &reading,
			&m.LateralDevMM, &m.LatPeakBetweenNext, &m.RadialDevMM, &m.RadPeakBetweenNext); err != nil {
			return model.Session{}, fmt.Errorf("scan measurement: %w", err)
		}
		if reading.Valid {
			m.TensionReading = &reading.Float64
		}
		sess.Measurements = append(sess.Measurements, m)
	}
	if err := rows.Err(); err != nil {
		return model.Session{}, fmt.Errorf("get measurements of session %d: %w", id, err)
	}
	return sess, nil
}

// ListSessions returns the sessions of a wheel (newest first) without measurements.
func (s *Store) ListSessions(ctx context.Context, wheelID int64) ([]model.Session, error) {
	if _, err := s.GetWheel(ctx, wheelID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, wheel_id, created_at FROM sessions
		WHERE wheel_id = ? ORDER BY created_at DESC, id DESC`, wheelID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	sessions := []model.Session{}
	for rows.Next() {
		var sess model.Session
		if err := rows.Scan(&sess.ID, &sess.WheelID, &sess.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

// DeleteSession removes a session and its measurements.
func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	if err := affectedOne(s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)); err != nil {
		return fmt.Errorf("delete session %d: %w", id, err)
	}
	return nil
}
