# Stage 02: Tensiometers, component library, force-based physics

Handoff document for agents continuing development. Read [stage_01.md](stage_01.md) first: its conventions, layout and quirks still apply unless overridden here.

| Item    | Value                                                     |
|---------|-----------------------------------------------------------|
| Branch  | `dev_step_01`                                             |
| Commit  | "Add tensiometers, component library and force model" (find it with `git log -- dev_stages/stage_02.md`) |
| Date    | 2026-11-12                                                |
| Status  | All stage scope done; gofmt, vet, golangci-lint, Go and JS tests green; browser E2E checked |

## 1. Scope delivered

| User request                                                        | Where                                                  |
|---------------------------------------------------------------------|--------------------------------------------------------|
| Tensiometer "masks": enter raw readings instead of kgf, ZTTO TC‑02 first (the user's conversion table) | `internal/tensiometer/{tensiometer,tc02,tm1}.go`, `model.ResolveTensions`, `web/static/js/tensiometer.js` |
| Library of popular rims and hubs (feature spec "component library + physics") | `internal/store/{seed,components}.go`, `/api/components/*`, `web/static/js/components.js` |
| DT Swiss F 572 28H/32H rims                                         | seeded for 27.5″ (ERD 564) and 29″ (ERD 602), `hole_counts [28, 32]` |
| Newmen Fade straight-pull front hub, 15x110 Boost, 6-bolt           | seeded as "Fade SP Front 15x110 Boost 6-bolt/CL" (28H, PCD 38.5/38.5, flanges 29/34) |
| Physics from the feature spec: spoke geometry, Hooke's law, rim offset, tension ratio, max-tension warning | `internal/solver/geometry.go`, `influence.go`, `internal/api/sessions.go` |

The original spec inputs (the TC‑02 table and `feature_spec_component_library_physics.md`) were attachments in the user's Downloads folder. They are not in the repo; their content is encoded in the code and tests listed above.

## 2. Decisions

The user did not review these; they were made autonomously (autopilot). Revisit them freely if the user disagrees.

1. **Force model replaces the stage-01 displacement model.** One nipple turn changes tension by `ΔT = SpokeRigidityKgf · ThreadPitchMM / L` (Hooke). The rim moves by its compliance times the force component: lateral `±ΔT·sin α·C_lat`, radial `−ΔT·radial·C_rad`. This is the only way spoke length, PCD, rim offset and bracing angle can influence the plan. Constants were **recalibrated** (§4.3) so that all stage-01 regression tests still pass.
2. **Profiles copy component data.** `rim_component_id` / `hub_component_id` are references only. ERD, flanges, PCD, spoke type and max tension live in the profile and stay editable. The solver never reads the library. IDs are validated on profile create/update, not on solve or geometry. Deleting a component sets the reference to `NULL` (`ON DELETE SET NULL`).
3. **Library seeded by upsert on every start** (`ON CONFLICT … DO UPDATE`), keyed by `(brand, model[, wheel_size])`. Data fixes ship with new builds, and IDs stay stable.
4. **Versioned migrations** via `PRAGMA user_version` (stage-01 backlog item 6). `schemaV1` is the stage-01 schema and `schemaV2` adds the library, the new profile columns and `measurements.tension_reading`. A stage-01 DB upgrades in place (`TestMigrateFromStage01`).
5. **Readings are resolved server-side once, at record time.** `createSession` and stateless solve convert `tension_reading` to `tension_kgf`. Stored sessions keep both values and are **not** re-resolved if the profile's tensiometer or gauge changes later.
6. **Out-of-range readings are rejected** (400), never extrapolated. Calibration curves are non-linear at the ends.
7. **Warning semantics:** `tension_warning = max_tension_kgf > 0 && predicted ≥ 0.95·max`. It is computed for every predicted row and for each action (the spoke's predicted tension after the plan). Unmeasured spokes (0 kgf) never warn. The optimizer does **not** enforce the limit yet (backlog 1).
8. **Frontend reading policy:** rows hold `tension_kgf` (always kgf; used by the radar and for display) and `tension_reading` (raw, sent only when non-null). An unconvertible reading sets `tension_kgf = null` and marks the cell invalid. Removing the tensiometer drops readings but keeps the last kgf values.
9. **Data honesty:** only manufacturer-published values are seeded. Newmen rims are skipped because no ERD is published. DT Swiss hubs have no published hole counts (`hole_counts` empty = unknown, no compatibility warning).

## 3. Data notes for the user's build

- **The Newmen Fade SP front Boost hub is 28H only.** F 572 28H fits. **F 572 32H needs a different hub** (e.g. Newmen Fade J-bend front 28/32H, or a DT 240/350 SP front). The UI shows the note *"Hub … has no 32H version (28H)."*
- Colour (silver/black) does not change geometry, so there is one hub row.
- DT Swiss 1200 N rim limit ≈ **122 kgf** (retailer data, r2-bike.com). It is set for F 572, EX 511, XM 481 and XR 391; FR 541 and XM 421 have 0 (unknown).
- Reference result, F 572 29″ + Fade SP front, 28H 2x: spoke length L 290.8 / R 291.4 mm, bracing 5.72° / 6.70°, ΔT 24.8 / 24.7 kgf per turn, expected tension ratio L/R **1.17**.
- TC‑02 gauge for typical butted 2.0‑1.8‑2.0 spokes: readings 1.7–3.4 mm; 3.0 mm ≈ 115 kgf.

Sources (also stored per row in `source`): DT Swiss product pages and spoke calculator, and the Newmen `Hub_Flange_Dimensions.pdf`. The TM‑1 table comes from Park Tool's conversion table PDF.

## 4. Changes by layer

### 4.1 `internal/tensiometer` (new, pure, 100%)

- `Model{ID, Name, ReadingUnit, Note, Gauges}` and `Gauge{ID, Label, Points}`. `Catalog()` rebuilds the catalog on each call, so there is no shared mutable state.
- `Gauge.Kgf(reading)`: linear interpolation; `ErrOutOfRange` outside the table. `Lookup(model, gauge)` returns `ErrUnknown` for an unknown model or gauge.
- `ztto-tc-02` (unit mm): gauges `1.8-1.5-1.8`, `1.8-1.6-1.8`, `2.0-1.7-1.8`, `2.0-1.8-2.0`, `1.8`, `2.0`, `2.3`.
- `park-tool-tm-1` (unit: scale): round steel gauges `1.4` … `2.6`.
- To add a tensiometer, write a new file returning `Model` and append it to `Catalog()`. `TestCatalogTablesAreMonotonic` guards every table.

### 4.2 `internal/model`

- `WheelSetup` gained these fields:
  - `rim_component_id` and `hub_component_id` (`*int64`, null allowed);
  - `rim_asymmetry_offset_mm`;
  - `flange_diam_left_mm` and `flange_diam_right_mm`;
  - `spoke_type` (`J-Bend` | `Straightpull`; case-insensitive, defaults to J-Bend);
  - `max_tension_kgf`;
  - `tensiometer`, `spoke_gauge_left` and `spoke_gauge_right`.
- `Normalize` clears the gauges when there is no tensiometer. `Validate` checks the tensiometer and gauges against the catalog.
- `Measurement.tension_reading` (`*float64`) and `ResolveTensions(w, ms)`.
- `RimComponent` and `HubComponent` entities.

### 4.3 `internal/solver`

- `geometry.go` (new):
  - `SpokeGeometry(w, p) → Geometry{Left, Right, TensionRatio}`.
  - Per side: `FlangeOffsetMM`, `SpokeLengthMM` (J-bend −1.2 mm), `BracingAngleDeg` and `TensionPerTurnKgf`.
  - Formula: `L = √(D² + F² + R² − 2DR·cos θ)` with `θ = cross·720°/N`.
- `Wheel` gained `RimOffsetMM`, `FlangeDiamLeftMM`, `FlangeDiamRightMM` and `SpokeType`.
  - Effective flanges are `F_L = flange_L − offset` and `F_R = flange_R + offset`. Each must be in `(0, ERD/2)`, and the PCD in `[0, ERD/2)`.
  - PCD 0 reproduces the stage-01 radial-spoke geometry.
- The influence matrix uses per-spoke `TensionPerTurnKgf` (tension matrix) and the force-model gains above.
- Default parameters, recalibrated:

  | Param             | Stage 01 | Stage 02 | Note |
  |-------------------|----------|----------|------|
  | TensionPerTurnKgf | 25       | removed  | now derived: `SpokeRigidityKgf·pitch/L` |
  | SpokeRigidityKgf  | n/a      | 16000    | ≈ 25 kgf/turn on a 290 mm spoke |
  | LateralCompliance | 0.25     | 0.3      | mm per kgf of lateral force |
  | RadialCompliance  | 0.5      | 0.012    | mm per kgf of radial force; the rim is far stiffer radially |
  | others            | unchanged | unchanged | decay 120/50 mm, coupling 0.15, cross-talk 0.1, weights 1/1/1e-4/1e-3 |

- New tests:
  - `geometry_test.go`: lengths against the classic spoke formula, asymmetry, front-disc tension ratio, Hooke's law, effective flanges in the influence matrix.
  - All stage-01 solver regression tests pass unchanged in intent.

### 4.4 `internal/store`

- Migrations: `migrate()` runs each pending `schemaVn` in its own transaction and sets `user_version`.
- `seed.go`: rim and hub seed data plus `seedComponents` (upsert).
- `components.go`: `ListRims`, `GetRim`, `ListHubs` and `GetHub`. `hole_counts` is stored as CSV text.
- Wheels and measurements read and write the new columns.
- Coverage: 93.0%. The remaining gaps are driver error branches (see stage 01).

### 4.5 `internal/api`

- New routes:
  - `GET /api/components/rims`, `GET /api/components/rims/{id}`;
  - `GET /api/components/hubs`, `GET /api/components/hubs/{id}`;
  - `GET /api/tensiometers`;
  - `POST /api/geometry`.
- The `Repository` interface was extended with the four component methods.
- Changes to `sessions.go`:
  - `checkInput` is shared by `resolveInput` (new data: validate + resolve readings) and `validateInput` (stored data: validate only).
  - `run` computes the geometry **before** `Solve`.
  - The solve response adds `geometry` and `max_tension_kgf`. Actions add `predicted_tension_kgf` and `tension_warning`; predicted rows add `tension_warning`.
- `wheels.go`: component IDs must exist (400 `field: rim_component_id` / `hub_component_id`).
- Tests: `components_test.go` (library, catalog, geometry, readings, warnings). The fake repo in `api_test.go` implements the new methods with injectable errors.

### 4.6 Frontend (`web/static`)

| Module            | Change |
|-------------------|--------|
| `tensiometer.js` (new) | `createConverter(catalog, wheel)` → `{label, gauges, convert(side, reading)}` or null; `applyReadings`, `tensionPatch`, `readingPreview`; option renderers |
| `components.js` (new)  | brand/model pickers, `rimPatch`/`hubPatch` (fit `spokes_count` to the drilling), `compatibilityNotes`, `renderGeometry` panel |
| `grid.js`         | `columnsFor(conv)` remaps the tension column to `tension_reading`; `≈ kgf` preview span; side change re-converts; paste goes through remapped columns |
| `paste.js`        | `applyPaste(rows, block, row, col, columns = COLUMNS)` |
| `measurements.js` | rows carry `tension_reading`; `toMeasurements` sends it only when non-null |
| `results.js`      | per-action ⚠️ *Approaching Max Rim Tension* and a summary alert |
| `api.js`          | `listRims`, `listHubs`, `listTensiometers`, `geometry` |
| `setup.js`        | new fields; component IDs read as null when empty |
| `main.js`         | loads the library at startup; live geometry preview (sequence-guarded); converter rebuilt on profile change; tension header shows the tensiometer unit |
| `index.html`, `app.css` | library fieldset, new fields, geometry panel, warning styles; grid columns use `minmax(0, 1fr)` so long selects do not overflow the card |

## 5. Quality gates

| Check     | Command                                                     | State                |
|-----------|-------------------------------------------------------------|----------------------|
| Format    | `gofmt -l .`                                                | clean                |
| Vet       | `go vet ./...`                                              | clean                |
| Lint      | `golangci-lint run ./...`                                   | 0 issues             |
| Go tests  | `go test -cover ./...`                                      | pass                 |
| JS tests  | `cd web && node --test --experimental-test-coverage test/`  | 75/75, 100% lines/branches/functions |

| Package / area                                  | Coverage |
|-------------------------------------------------|----------|
| solver, tensiometer, model, api, web (embed)    | 100%     |
| store                                           | 93.0%    |
| cmd                                             | 76.6%    |

### Browser E2E check (done with Playwright against `go run`)

1. Pick DT Swiss → F 572 29″ and Newmen → Fade SP Front. The form is filled with 28H, ERD 602, flanges 29/34, PCD 38.5, Straightpull, max 122. The geometry panel shows ratio 1.17.
2. Choosing 32 spokes shows the 28H-only note.
3. Pick ZTTO TC‑02 with gauge 2.0‑1.8‑2.0. The tension header changes to "ZTTO TC-02 reading, mm"; 3.0 shows "≈ 115 kgf", and 9 shows "outside 1.7–3.4".
4. Save the profile and solve. Actions with ⚠️ warnings appear, and wobble drops 0.60 → 0.15 mm.
5. Reload and load the session. Readings, gauges and the library picks are restored. There are no console errors.

Remember: static files are embedded at build time, so restart `go run` after editing `web/static`.

## 6. Gaps and backlog

Stage-01 backlog items 1–5, 7 and 8 remain open. Item 6 (versioned migrations) is done.

1. **Max-tension constraint in the optimizer.** It currently only warns. Add an inequality constraint or penalty so the plan never exceeds `max_tension_kgf`. The E2E dataset produced +1/2 turn → 126 kgf on a 122 kgf rim.
2. **Target L/R tension ratio.** `Geometry.TensionRatio` is computed but not used. Balance absolute tension so that `T_L/T_R → TensionRatio` (relates to stage-01 backlog 4).
3. **Straight-pull specifics:** the spoke-hole offset from the flange plane and the spoke head angle are not modelled. Bladed and aero spokes need separate tensiometer gauges.
4. **More data:**
   - Newmen rims (need published ERDs);
   - DT Swiss hub hole counts;
   - rear-hub PCD verification against physical measurements;
   - custom, user-defined tensiometer tables (API + UI), maybe a CRUD library.
5. **Re-resolve tool:** an explicit "recompute kgf from readings" action for stored sessions after a gauge correction (deliberately not automatic, decision 5).
6. **Validate recalibrated constants** (C_lat, C_rad, rigidity) on real before/after datasets from the user's F 572 build.

## 7. How to verify quickly

```powershell
gofmt -l .; go vet ./...; golangci-lint run ./...; go test -cover ./...
cd web; node --test --experimental-test-coverage test/
go run ./cmd/wheel-tuner -addr 127.0.0.1:8099 -db $env:TEMP\wt.db   # http://127.0.0.1:8099/
curl.exe -s http://127.0.0.1:8099/api/components/hubs | ConvertFrom-Json | Select-Object -First 1
```
