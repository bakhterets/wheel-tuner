package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bakhterets/wheel-tuner/internal/model"
)

const (
	rimColumns = `id, brand, model, wheel_size, hole_counts, erd_mm, asymmetry_offset_mm,
		max_tension_kgf, source`
	hubColumns = `id, brand, model, position, axle, flange_dist_left_mm, flange_dist_right_mm,
		flange_diam_left_mm, flange_diam_right_mm, spoke_type, paired_spokes, hole_counts, source`
)

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}

func splitInts(s string) ([]int, error) {
	out := []int{}
	if s == "" {
		return out, nil
	}
	for _, f := range strings.Split(s, ",") {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("hole counts %q: %w", s, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func scanRim(r scanner) (model.RimComponent, error) {
	var (
		c     model.RimComponent
		holes string
	)
	err := r.Scan(&c.ID, &c.Brand, &c.Model, &c.WheelSize, &holes, &c.ERDMM, &c.AsymmetryOffsetMM,
		&c.MaxTensionKgf, &c.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return c, model.ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.HoleCounts, err = splitInts(holes)
	return c, err
}

func scanHub(r scanner) (model.HubComponent, error) {
	var (
		c     model.HubComponent
		holes string
	)
	err := r.Scan(&c.ID, &c.Brand, &c.Model, &c.Position, &c.Axle, &c.FlangeDistLeftMM,
		&c.FlangeDistRightMM, &c.FlangeDiamLeftMM, &c.FlangeDiamRightMM, &c.SpokeType, &c.PairedSpokes, &holes, &c.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return c, model.ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.HoleCounts, err = splitInts(holes)
	return c, err
}

// listRows runs query and scans every row with scan.
func listRows[T any](ctx context.Context, db *sql.DB, query string, scan func(scanner) (T, error)) ([]T, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListRims returns the rim library ordered by brand, model and wheel size.
func (s *Store) ListRims(ctx context.Context) ([]model.RimComponent, error) {
	rims, err := listRows(ctx, s.db, `SELECT `+rimColumns+` FROM rim_components
		ORDER BY brand, model, wheel_size, id`, scanRim)
	if err != nil {
		return nil, fmt.Errorf("list rims: %w", err)
	}
	return rims, nil
}

// GetRim returns the library rim with the given ID.
func (s *Store) GetRim(ctx context.Context, id int64) (model.RimComponent, error) {
	c, err := scanRim(s.db.QueryRowContext(ctx, `SELECT `+rimColumns+` FROM rim_components WHERE id = ?`, id))
	if err != nil {
		return model.RimComponent{}, fmt.Errorf("get rim %d: %w", id, err)
	}
	return c, nil
}

// ListHubs returns the hub library ordered by brand, position and model.
func (s *Store) ListHubs(ctx context.Context) ([]model.HubComponent, error) {
	hubs, err := listRows(ctx, s.db, `SELECT `+hubColumns+` FROM hub_components
		ORDER BY brand, position, model, id`, scanHub)
	if err != nil {
		return nil, fmt.Errorf("list hubs: %w", err)
	}
	return hubs, nil
}

// GetHub returns the library hub with the given ID.
func (s *Store) GetHub(ctx context.Context, id int64) (model.HubComponent, error) {
	c, err := scanHub(s.db.QueryRowContext(ctx, `SELECT `+hubColumns+` FROM hub_components WHERE id = ?`, id))
	if err != nil {
		return model.HubComponent{}, fmt.Errorf("get hub %d: %w", id, err)
	}
	return c, nil
}
