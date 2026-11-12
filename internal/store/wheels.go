package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bakhterets/wheel-tuner/internal/model"
)

const wheelColumns = `id, name, spokes_count, erd_mm, cross_pattern, flange_dist_left_mm,
	flange_dist_right_mm, start_reference, view_point, counting_direction, rim_component_id,
	hub_component_id, rim_asymmetry_offset_mm, flange_diam_left_mm, flange_diam_right_mm, spoke_type,
	max_tension_kgf, tensiometer, spoke_gauge_left, spoke_gauge_right, paired_spokes, tension_limit,
	tension_limit_unit`

type scanner interface{ Scan(dest ...any) error }

func scanWheel(r scanner) (model.WheelSetup, error) {
	var (
		w        model.WheelSetup
		rim, hub sql.NullInt64
	)
	err := r.Scan(&w.ID, &w.Name, &w.SpokesCount, &w.ERDMM, &w.CrossPattern, &w.FlangeDistLeftMM,
		&w.FlangeDistRightMM, &w.StartReference, &w.ViewPoint, &w.CountingDirection, &rim, &hub,
		&w.RimAsymmetryOffsetMM, &w.FlangeDiamLeftMM, &w.FlangeDiamRightMM, &w.SpokeType,
		&w.MaxTensionKgf, &w.Tensiometer, &w.SpokeGaugeLeft, &w.SpokeGaugeRight, &w.PairedSpokes,
		&w.TensionLimit, &w.TensionLimitUnit)
	if errors.Is(err, sql.ErrNoRows) {
		return w, model.ErrNotFound
	}
	if rim.Valid {
		w.RimComponentID = &rim.Int64
	}
	if hub.Valid {
		w.HubComponentID = &hub.Int64
	}
	return w, err
}

// wheelArgs returns the column values of w in wheelColumns order, without id.
func wheelArgs(w model.WheelSetup) []any {
	return []any{w.Name, w.SpokesCount, w.ERDMM, w.CrossPattern, w.FlangeDistLeftMM, w.FlangeDistRightMM,
		w.StartReference, w.ViewPoint, w.CountingDirection, w.RimComponentID, w.HubComponentID,
		w.RimAsymmetryOffsetMM, w.FlangeDiamLeftMM, w.FlangeDiamRightMM, w.SpokeType,
		w.MaxTensionKgf, w.Tensiometer, w.SpokeGaugeLeft, w.SpokeGaugeRight, w.PairedSpokes,
		w.TensionLimit, w.TensionLimitUnit}
}

// CreateWheel inserts a profile and returns it with its new ID.
func (s *Store) CreateWheel(ctx context.Context, w model.WheelSetup) (model.WheelSetup, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO wheel_setups (name, spokes_count, erd_mm,
		cross_pattern, flange_dist_left_mm, flange_dist_right_mm, start_reference, view_point,
		counting_direction, rim_component_id, hub_component_id, rim_asymmetry_offset_mm,
		flange_diam_left_mm, flange_diam_right_mm, spoke_type, max_tension_kgf, tensiometer,
		spoke_gauge_left, spoke_gauge_right, paired_spokes, tension_limit, tension_limit_unit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wheelArgs(w)...)
	if err != nil {
		return model.WheelSetup{}, fmt.Errorf("insert wheel: %w", err)
	}
	if w.ID, err = res.LastInsertId(); err != nil {
		return model.WheelSetup{}, fmt.Errorf("insert wheel: %w", err)
	}
	return w, nil
}

// GetWheel returns the profile with the given ID.
func (s *Store) GetWheel(ctx context.Context, id int64) (model.WheelSetup, error) {
	w, err := scanWheel(s.db.QueryRowContext(ctx,
		`SELECT `+wheelColumns+` FROM wheel_setups WHERE id = ?`, id))
	if err != nil {
		return model.WheelSetup{}, fmt.Errorf("get wheel %d: %w", id, err)
	}
	return w, nil
}

// ListWheels returns all profiles ordered by name.
func (s *Store) ListWheels(ctx context.Context) ([]model.WheelSetup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+wheelColumns+` FROM wheel_setups ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list wheels: %w", err)
	}
	defer rows.Close()
	wheels := []model.WheelSetup{}
	for rows.Next() {
		w, err := scanWheel(rows)
		if err != nil {
			return nil, fmt.Errorf("list wheels: %w", err)
		}
		wheels = append(wheels, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list wheels: %w", err)
	}
	return wheels, nil
}

// UpdateWheel replaces all fields of an existing profile.
func (s *Store) UpdateWheel(ctx context.Context, w model.WheelSetup) error {
	err := affectedOne(s.db.ExecContext(ctx, `UPDATE wheel_setups SET name = ?, spokes_count = ?,
		erd_mm = ?, cross_pattern = ?, flange_dist_left_mm = ?, flange_dist_right_mm = ?,
		start_reference = ?, view_point = ?, counting_direction = ?, rim_component_id = ?,
		hub_component_id = ?, rim_asymmetry_offset_mm = ?, flange_diam_left_mm = ?,
		flange_diam_right_mm = ?, spoke_type = ?, max_tension_kgf = ?, tensiometer = ?,
		spoke_gauge_left = ?, spoke_gauge_right = ?, paired_spokes = ?, tension_limit = ?,
		tension_limit_unit = ? WHERE id = ?`,
		append(wheelArgs(w), w.ID)...))
	if err != nil {
		return fmt.Errorf("update wheel %d: %w", w.ID, err)
	}
	return nil
}

// DeleteWheel removes a profile together with its sessions and measurements.
func (s *Store) DeleteWheel(ctx context.Context, id int64) error {
	if err := affectedOne(s.db.ExecContext(ctx, `DELETE FROM wheel_setups WHERE id = ?`, id)); err != nil {
		return fmt.Errorf("delete wheel %d: %w", id, err)
	}
	return nil
}
