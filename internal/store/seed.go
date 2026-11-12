package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bakhterets/wheel-tuner/internal/model"
)

// Sources of the seeded component data.
const (
	srcDTSwissCalc = "DT Swiss spoke calculator, https://spokes-calculator.dtswiss.com/en/data/calculator"
	srcNewmenHubs  = "Newmen Hub_Flange_Dimensions.pdf, https://www.newmen-components.de/upload/_temp/gallery_87d8bc7d-01fb-4701-ab7a-c1051d746d2b/Hub_Flange_Dimensions.pdf"
)

// dtRimMaxTensionKgf is DT Swiss' 1200 N MTB rim limit as published by
// retailers (r2-bike.com) for the EX 511, XM 481, XR 391 and F 572.
const dtRimMaxTensionKgf = 122

// rimSeeds returns the rim library. ERDs are DT Swiss' published values;
// 28h and 32h drillings share one ERD. All are symmetric rims.
func rimSeeds() []model.RimComponent {
	type rim struct {
		model         string
		erd275, erd29 float64
		maxKgf        float64
		page          string
	}
	rims := []rim{
		{"F 572", 564, 602, dtRimMaxTensionKgf, "https://www.dtswiss.com/en/components/rims-mtb/free-ride/f-572"},
		{"EX 511", 563, 601, dtRimMaxTensionKgf, "https://www.dtswiss.com/en/components/rims-mtb/enduro/ex-511"},
		{"XM 481", 563, 601, dtRimMaxTensionKgf, "https://www.dtswiss.com/en/components/rims-mtb/all-mountain/xm-481"},
		{"FR 541", 565, 603, 0, "https://www.dtswiss.com/en/components/rims-mtb/free-ride/fr-541"},
		{"XM 421", 564, 602, 0, "https://www.dtswiss.com/en/components/rims-mtb/all-mountain/xm-421"},
		{"XR 391", 569, 607, dtRimMaxTensionKgf, srcDTSwissCalc},
	}
	out := make([]model.RimComponent, 0, 2*len(rims))
	for _, r := range rims {
		for _, size := range [...]struct {
			name string
			erd  float64
		}{{"27.5", r.erd275}, {"29", r.erd29}} {
			out = append(out, model.RimComponent{
				Brand: "DT Swiss", Model: r.model, WheelSize: size.name, HoleCounts: []int{28, 32},
				ERDMM: size.erd, MaxTensionKgf: r.maxKgf, Source: r.page,
			})
		}
	}
	return out
}

// hubSeeds returns the hub library. Left is the disc side of front hubs and
// the non-drive side of rear hubs; flange diameters are pitch circles.
// Hole counts are listed only where the manufacturer publishes them.
func hubSeeds() []model.HubComponent {
	const (
		jb = model.SpokeJBend
		sp = model.SpokeStraightPull
	)
	hub := func(brand, name, pos, axle, spoke string, holes []int, pcdL, pcdR, fL, fR float64, src string) model.HubComponent {
		return model.HubComponent{
			Brand: brand, Model: name, Position: pos, Axle: axle, SpokeType: spoke, HoleCounts: holes,
			FlangeDiamLeftMM: pcdL, FlangeDiamRightMM: pcdR, FlangeDistLeftMM: fL, FlangeDistRightMM: fR, Source: src,
		}
	}
	newmen := func(name, pos, axle, spoke string, holes []int, pcdL, pcdR, fL, fR float64) model.HubComponent {
		return hub("Newmen", name, pos, axle, spoke, holes, pcdL, pcdR, fL, fR, srcNewmenHubs)
	}
	dt := func(name, pos, axle, spoke string, pcdL, pcdR, fL, fR float64) model.HubComponent {
		return hub("DT Swiss", name, pos, axle, spoke, nil, pcdL, pcdR, fL, fR, srcDTSwissCalc)
	}
	return []model.HubComponent{
		// The straight-pull Fade front hub is sold in 28h only; the colour
		// (silver, black, ...) does not change its geometry.
		newmen("Fade SP Front 15x110 Boost 6-bolt/CL", "front", "15x110 Boost", sp, []int{28}, 38.5, 38.5, 29.0, 34.0),
		newmen("Fade Front 15/20x110 Boost 6-bolt", "front", "15x110 Boost", jb, []int{28, 32}, 54.8, 37.8, 27.5, 39.0),
		newmen("Fade Front 15x110 Boost CL", "front", "15x110 Boost", jb, []int{28, 32}, 56.6, 56.6, 26.3, 36.0),
		newmen("Fade UL Front 15x110 Boost", "front", "15x110 Boost", jb, nil, 53.9, 37.8, 27.1, 39.4),
		newmen("Fade Rear 12x148 Boost", "rear", "12x148 Boost", jb, []int{28, 32}, 54.8, 54.8, 36.8, 23.3),
		newmen("Fade SP Rear 12x148 Boost", "rear", "12x148 Boost", sp, []int{28}, 39.2, 55.3, 36.9, 23.1),
		newmen("Fade UL Rear 12x148 Boost", "rear", "12x148 Boost", jb, nil, 53.8, 48.6, 36.7, 23.7),
		newmen("Fade Rear 12x157", "rear", "12x157", jb, nil, 54.8, 54.8, 41.3, 27.8),
		newmen("Fade SP Rear 12x157", "rear", "12x157", sp, nil, 39.2, 55.3, 41.4, 27.6),

		dt("350 Front 15x110 Boost 6-bolt", "front", "15x110 Boost", jb, 60, 44, 27.4, 40),
		dt("350 Front 15x110 Boost CL", "front", "15x110 Boost", jb, 44, 44, 27.4, 40),
		dt("350 SP Front 15x110 Boost 6-bolt", "front", "15x110 Boost", sp, 40.8, 36, 28, 40.2),
		dt("350 SP Front 15x110 Boost CL", "front", "15x110 Boost", sp, 40.8, 36, 27.9, 40),
		dt("240 Front 15x110 Boost 6-bolt", "front", "15x110 Boost", jb, 57.8, 49.4, 27.3, 41),
		dt("240 Front 15x110 Boost CL", "front", "15x110 Boost", jb, 44.4, 44.4, 27.6, 41.2),
		dt("240 SP Front 15x110 Boost 6-bolt/CL", "front", "15x110 Boost", sp, 35.5, 35.5, 26.5, 39.7),
		dt("180 SP Front 15x110 Boost CL", "front", "15x110 Boost", sp, 34.5, 34, 26.4, 39.8),

		dt("350 Front 15x100 6-bolt", "front", "15x100", jb, 60, 44, 22.4, 35),
		dt("350 Front 15x100 CL", "front", "15x100", jb, 44, 44, 22.4, 35),
		dt("350 SP Front 15x100 6-bolt", "front", "15x100", sp, 40.8, 36, 23, 35.2),
		dt("350 SP Front 15x100 CL", "front", "15x100", sp, 40.8, 36, 23, 35),
		dt("240 Front 15x100 6-bolt", "front", "15x100", jb, 57.8, 49.4, 22.3, 36),
		dt("240 Front 15x100 CL", "front", "15x100", jb, 44.4, 44.4, 22.6, 36.2),
		dt("240 SP Front 15x100 6-bolt/CL", "front", "15x100", sp, 35.5, 35.5, 21.6, 34.8),
		dt("180 Front 15x100 CL", "front", "15x100", jb, 44, 37.5, 22.5, 36),

		dt("350 Rear 12x148 Boost 6-bolt", "rear", "12x148 Boost", jb, 60, 50.5, 36.6, 23.4),
		dt("350 Rear 12x148 Boost CL", "rear", "12x148 Boost", jb, 47, 47, 36, 23.2),
		dt("350 SP Rear 12x148 Boost 6-bolt", "rear", "12x148 Boost", sp, 39, 48.7, 37.1, 21.4),
		dt("350 SP Rear 12x148 Boost CL", "rear", "12x148 Boost", sp, 39, 48.7, 38.6, 21.4),
		dt("240 Rear 12x148 Boost 6-bolt", "rear", "12x148 Boost", jb, 57.9, 50.4, 37.1, 22.6),
		dt("240 Rear 12x148 Boost CL", "rear", "12x148 Boost", jb, 50.4, 50.4, 37.3, 22.6),
		dt("240 SP Rear 12x148 Boost 6-bolt/CL", "rear", "12x148 Boost", sp, 35.9, 42.8, 35.8, 21.5),
		dt("180 SP Rear 12x148 Boost CL", "rear", "12x148 Boost", sp, 34.5, 41.3, 35.7, 21.5),

		dt("350 Rear 12x142 6-bolt", "rear", "12x142", jb, 60, 50.5, 33.6, 20.4),
		dt("350 Rear 12x142 CL", "rear", "12x142", jb, 47, 47, 33, 20.2),
		dt("350 SP Rear 12x142 6-bolt", "rear", "12x142", sp, 39, 48.7, 34.1, 18.4),
		dt("350 SP Rear 12x142 CL", "rear", "12x142", sp, 39, 48.7, 35.8, 18.4),
		dt("240 Rear 12x142 6-bolt", "rear", "12x142", jb, 57, 49.9, 34.1, 19.6),
		dt("240 Rear 12x142 CL", "rear", "12x142", jb, 50.4, 50.4, 34.3, 19.6),
		dt("240 SP Rear 12x142 6-bolt/CL", "rear", "12x142", sp, 35.9, 42.8, 32.8, 18.5),
		dt("180 Rear 12x142 CL", "rear", "12x142", jb, 44, 47.5, 33.2, 19.5),
	}
}

// seedComponents upserts the library on every start, so data fixes ship with
// new builds while row IDs (referenced by profiles) stay stable.
func seedComponents(ctx context.Context, db *sql.DB, rims []model.RimComponent, hubs []model.HubComponent) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("seed components: %w", err)
	}
	defer rollback(tx, &err)
	for _, r := range rims {
		if _, err := tx.ExecContext(ctx, `INSERT INTO rim_components (brand, model, wheel_size,
			hole_counts, erd_mm, asymmetry_offset_mm, max_tension_kgf, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (brand, model, wheel_size) DO UPDATE SET hole_counts = excluded.hole_counts,
			erd_mm = excluded.erd_mm, asymmetry_offset_mm = excluded.asymmetry_offset_mm,
			max_tension_kgf = excluded.max_tension_kgf, source = excluded.source`,
			r.Brand, r.Model, r.WheelSize, joinInts(r.HoleCounts), r.ERDMM, r.AsymmetryOffsetMM,
			r.MaxTensionKgf, r.Source); err != nil {
			return fmt.Errorf("seed rim %s %s: %w", r.Brand, r.Model, err)
		}
	}
	for _, h := range hubs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hub_components (brand, model, position, axle,
			flange_dist_left_mm, flange_dist_right_mm, flange_diam_left_mm, flange_diam_right_mm,
			spoke_type, hole_counts, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (brand, model) DO UPDATE SET position = excluded.position, axle = excluded.axle,
			flange_dist_left_mm = excluded.flange_dist_left_mm, flange_dist_right_mm = excluded.flange_dist_right_mm,
			flange_diam_left_mm = excluded.flange_diam_left_mm, flange_diam_right_mm = excluded.flange_diam_right_mm,
			spoke_type = excluded.spoke_type, hole_counts = excluded.hole_counts, source = excluded.source`,
			h.Brand, h.Model, h.Position, h.Axle, h.FlangeDistLeftMM, h.FlangeDistRightMM,
			h.FlangeDiamLeftMM, h.FlangeDiamRightMM, h.SpokeType, joinInts(h.HoleCounts), h.Source); err != nil {
			return fmt.Errorf("seed hub %s %s: %w", h.Brand, h.Model, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("seed components: %w", err)
	}
	return nil
}
