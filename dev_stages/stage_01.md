# Stage 01: Initial implementation (spec milestones 1–5)

Handoff document for agents continuing development. Read it together with [spec.md](../spec.md) (requirements, do not modify) and [README.md](../README.md) (user-facing docs).

| Item    | Value                                                     |
|---------|-----------------------------------------------------------|
| Branch  | `dev_step_01`                                             |
| Commit  | `805579c`, "Add wheel truing solver service" (root commit, 57 files) |
| Date    | 2026-11-12                                                |
| Status  | All 5 milestones of spec §10 done; tests, vet, gofmt, golangci-lint green |

## 1. Scope delivered

| Spec §10 milestone                                | Where                                                        |
|---------------------------------------------------|--------------------------------------------------------------|
| 1. Go project, CI, golangci-lint, wheel CRUD      | `go.mod`, `.github/workflows/ci.yml`, `.golangci.yml`, `internal/store`, `internal/api/wheels.go` |
| 2. Vanilla JS grid for data entry                 | `web/static/js/{grid,paste,measurements}.js`                 |
| 3. Solver math engine + table-driven tests        | `internal/solver/{influence,wheel,matrix,params}.go` + tests |
| 4. Optimization connected to HTTP API             | `internal/solver/{solver,lsq,turns}.go`, `internal/api/sessions.go` |
| 5. Visualizations (wheel map, tension radar)      | `web/static/js/viz/{geometry,wheel,radar}.js`                |

## 2. Decisions made with the user

These are fixed. Do not revisit them without asking the user.

1. **SQLite driver:** `modernc.org/sqlite` (pure Go, no CGO). This deviates from the spec's `mattn/go-sqlite3` and is documented in the README. The version is pinned to **v1.58.0**, the newest release that still supports `go 1.25`. Newer versions force `go 1.26` and a toolchain download.
2. **Inter-spoke peak semantics = "value_at_half":** a value entered with `*_peak_between_next` is the peak at position `i.5`, not at spoke `i`. See §5.3.
3. **Git:** commit to the current branch. Never add a `Co-authored-by: Copilot` trailer.
4. **Language:** the user writes in Russian. Code, comments and repo docs are in English.

## 3. Repository layout

```
cmd/wheel-tuner/        main.go: run(ctx, args, stderr, ready) is testable; main() only wraps it
internal/solver/        PURE math. No imports of api/store/model. Stdlib only.
  wheel.go              Wheel, Measurement, Side, Direction, Validate, normalize (sorting + checks)
  influence.go          BuildInfluence, deviation matrices, virtual i.5 rows, tension matrix, crossPartners
  params.go             Params, Weights, DefaultParams (empirical constants), Params.Validate
  solver.go             Solve → Result{Actions, Turns, RawTurns, Before, After, Predicted, Iterations}
  lsq.go                weighted least squares: normal equations + conjugate gradient, ridge, fixed vars
  turns.go              QuantizeTurns, FormatTurns, Action.Instruction
  matrix.go             minimal dense Matrix
internal/model/         persisted entities + validation (WheelSetup, Session, Measurement, ValidationError, ErrNotFound)
internal/store/         SQLite repository: schema in store.go (CREATE IF NOT EXISTS), wheels.go, sessions.go
internal/api/           server.go (routes, Repository interface, options), http.go (JSON, errors, middleware),
                        wheels.go (CRUD, prepareWheel, toSolverWheel), sessions.go (DTOs, sessions, solve)
web/embed.go            //go:embed static → Static() (fs.FS, error)
web/static/             index.html, css/app.css, js/*.js (ES modules), js/viz/*.js
web/test/               node:test suites + fakedom.mjs (minimal DOM fake, no npm deps)
```

Dependency direction: `cmd → api → {model, store, solver}`, `store → model`. `solver` depends on nothing internal (spec §9).

## 4. Backend details

### 4.1 HTTP API (`internal/api`)

Routes are registered in [server.go](../internal/api/server.go). The full table and example payloads are in the README.

- `GET /healthz` pings the DB through `Repository.Ping`.
- `GET|POST /api/wheels`, `GET|PUT|DELETE /api/wheels/{id}`
- `GET|POST /api/wheels/{id}/sessions`
- `GET|DELETE /api/sessions/{id}`, `POST /api/sessions/{id}/solve`
- `POST /api/solve` (stateless: `{"wheel": {...}, "measurements": [...]}`)
- `GET /` serves the static files, if configured via `WithStatic`

Error mapping in `fail()` ([http.go](../internal/api/http.go)):

| Error                      | Status | Body                         |
|----------------------------|--------|------------------------------|
| `*model.ValidationError`   | 400    | `{"error", "field"}`         |
| decode error               | 400    | `{"error"}`                  |
| `model.ErrNotFound`        | 404    | `{"error"}`                  |
| `solver.ErrInvalidInput`   | 422    | `{"error"}`                  |
| anything else              | 500    | `"internal error"`, logged   |

Other behavior:
- `decode()` limits bodies to 1 MB, rejects unknown fields, and rejects trailing data.
- Middleware: slog request logging, panic recovery (re-panics `http.ErrAbortHandler`), and security headers. The CSP is `default-src 'self'`, so the frontend must not use inline `<script>`, inline `style=""` attributes, or `on*=` handlers. SVG presentation attributes and CSSOM via JS are allowed.
- `writeJSON` is a `Server` method that logs encode failures; it never silently ignores errors.
- The solve pipeline:
  1. `model.WheelSetup.Normalize()` → `Validate()`
  2. `toSolverWheel`: infallible, with the precondition that the model is valid
  3. `solver.Wheel.Validate()`
  4. `model.ValidateMeasurements`
  5. `solver.Solve`
  `validateInput` re-validates the **stored** wheel, so a corrupt DB row yields 400, not 500.
- `solver.ErrNoConvergence` (e.g. `MaxIterations=1`) → 500.

### 4.2 Persistence (`internal/store`)

- The schema is identical to spec §6 plus CHECK constraints, `UNIQUE(session_id, spoke_index)`, `ON DELETE CASCADE`, and an index on `sessions.wheel_id`.
- The DSN sets `_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)`.
- `CreateSession` writes the session and its measurements in one transaction.
- `WithClock` option for deterministic `created_at` in tests.

### 4.3 Entrypoint (`cmd/wheel-tuner`)

- Flags: `-addr` (default `:8080`), `-db` (`wheel-tuner.db`), `-log-level` (slog `TextVar`).
- Server timeouts are set. The listener is created via `net.ListenConfig`, and `ready(addr)` is called for tests.
- Graceful shutdown on SIGINT/SIGTERM with a 10 s timeout. `BaseContext` is intentionally **not** tied to the signal context, so in-flight requests are not cancelled.

## 5. Solver model (the core: change with care)

### 5.1 Conventions

- **Lateral `+`** is toward the right (drive-side) flange. Tightening an `R` spoke moves the rim `+` (`Side.pull()`: R=+1, L=−1).
- **Radial `+`** is a high spot (hop). Tightening reduces the radius (negative radial gain). Radial errors and matrix columns are **centered** (`centerRadial`), so a uniform radius offset is not runout.
- **Sides:** `L` = left = rotor = non-drive side; `R` = right = drive side. The view point labels map in `model.viewPointSide`: "rotor side" / "non-drive side" / "left" → L, and "drive side" / "non-rotor side" / "right" → R.
- **Positions:** 1-based spoke indices in counting direction from the reference mark. Spacing along the rim is `π·ERD/N` (`Wheel.spacingMM`).
- **Validation (solver `normalize`):** every spoke must be measured exactly once, and **both sides must be non-empty**. For a crossed lacing, each flange needs an even count `> 2·cross−1`. The hub offset of the cross pattern is also checked.

### 5.2 Influence matrix (`influence.go`)

- Bracing angle: `β = atan2(flange, ERD/2)`.
- Lateral gain: `pull · pitch · LateralCompliance / sin β`. Radial gain: `−pitch · RadialCompliance · cos β`.
- Rim attenuation: `exp(−d · spacing / decayMM)`, where `d` is the circular index distance.
- Tension matrix:
  - diagonal: `TensionPerTurnKgf`;
  - neighbours: `±TensionPerTurnKgf · NeighborCoupling · decay` (same flange −, opposite flange +);
  - crossing spokes on the same flange: `+TensionPerTurnKgf · CrossTalk`.
- `crossPartners`: on each flange, leading spokes at flange position `m` cross trailing spokes `m+1, m+3, …, m+2k−1`. The pattern mirrors with the counting direction and the view side (`Wheel.rotationSense`).

### 5.3 Inter-spoke peaks (value_at_half)

- For each flagged spoke `i`, `deviationMatrix` adds a virtual row `A[i.5] = (A[i] + A[i+1]) / 2` (spec §5), so the row count grows by the number of flags.
- `deviationErrors`: the virtual row gets the entered value. The spoke-`i` row gets the mean of its neighbours' values (superseded: interpolated towards the peak, see stage_03 review fixes).
- `valueRows`: maps each spoke to the row its entered value refers to (the virtual row wins). `Predicted` reports values at that row.

### 5.4 Optimization (`solver.go`, `lsq.go`)

Cost: `W_L‖lat‖² + W_R‖rad‖² + W_T‖ΔT‖² + W_X‖x‖²`.
- `ΔT` is the deviation from the per-side mean.
- Spokes with `tension_kgf == 0` count as unmeasured: weight 0, and they are excluded from the means and metrics.

Algorithm:
1. Weighted normal equations solved with CG.
2. Successive quantization: take the free variable with the largest `|x|` and round it to the `TurnStep` (1/4) grid; drop it if under `MinTurn` (1/8). Fix it and re-solve the remaining variables. Stop at the first variable that rounds to 0 and zero out the rest.
3. `RawTurns` holds the first, unquantized solution.

### 5.5 Default parameters: calibrated at the end of this stage

| Param               | Value   | Note                                                    |
|---------------------|---------|---------------------------------------------------------|
| ThreadPitchMM       | 0.45    | 56 TPI                                                  |
| TensionPerTurnKgf   | 25      |                                                         |
| LateralCompliance   | 0.25    |                                                         |
| RadialCompliance    | 0.5     |                                                         |
| **LateralDecayMM**  | **120** | was 60                                                  |
| **RadialDecayMM**   | **50**  | was 90; radial influence must be *narrower* than lateral |
| NeighborCoupling    | 0.15    |                                                         |
| CrossTalk           | 0.1     |                                                         |
| TurnStep / MinTurn  | 0.25 / 0.125 | spec §7.4                                          |
| Weights L / R / **T** / X | 1 / 1 / **1e-4** / 1e-3 | T was 0.01                  |

Why the change: with the old values, a realistic dataset (wobble 0.76 mm, one +20% tension outlier) improved wobble by only 8%. An isolated 0.4 mm hop produced **zero** actions, because fixing the hop created more wobble than it removed and the tension term outweighed the geometry. The new values give wobble −67% on the same data, and a lone hop is fixed by +1/4 turn on the three spokes at the high spot.

Regression tests that guard this (both fail with the old constants):
- `TestSolveGeometryDominatesTensionOutlier`
- `TestSolveIsolatedHopTightensHighSpot`

All constants are **empirical guesses**, not yet validated on real wheels. Any change must keep `go test ./internal/solver` green; add a regression test for the scenario that motivated it.

## 6. Frontend (`web/static`)

- **Modules:**

  | Module           | Role                                                                        |
  |------------------|-----------------------------------------------------------------------------|
  | `main.js`        | wiring: store, API client, render subscription. Browser-only, not unit-tested |
  | `state.js`       | `createStore`                                                               |
  | `api.js`         | `createClient(fetchImpl, base)`, `ApiError`                                 |
  | `setup.js`       | profile form; browser-only                                                  |
  | `measurements.js`| row model, `toMeasurements`, `rowsFromMeasurements`                         |
  | `grid.js`        | `renderGrid`, `bindGrid`                                                    |
  | `paste.js`       | Excel TSV parsing, `applyPaste`                                             |
  | `results.js`     | action list and metrics table                                               |
  | `dom.js`         | `h()` / `s()` element builders                                              |
  | `viz/geometry.js`| pure math for the visualizations                                            |
  | `viz/wheel.js`   | SVG wheel map                                                               |
  | `viz/radar.js`   | SVG tension radar                                                           |

- **Grid:**
  - Typing updates the store **without re-rendering**, to keep the caret. `gridVersion` is bumped only when rows are replaced (paste, resize, session load, clear), and only then is the grid re-rendered.
- **Paste:**
  - Anchored at the target cell's `data-row` / `data-col`. Column 0 (the index) is ignored and header rows are skipped.
  - Accepts decimal commas.
  - Side aliases: `l/left/nds/rotor` → L and `r/right/ds/drive` → R.
  - Flag truthy values: `1, x, y, yes, true, ✓, ✔, да, +`.
- **Wheel map:**
  - Angle 0 is at 12 o'clock and follows the counting direction; the reference mark sits at the top.
  - Spoke colors: blue = tighten, red = loosen. Near-side spokes are solid, far-side spokes dashed.
  - Rings: the outer ring shows wobble zones, the inner ring hop zones. A zone is drawn where `|v| ≥ max(0.1 mm, 0.3·peak)`.
- **Radar:**
  - Plots tension relative to each side's mean, with a ±50% plotted span and a ±20% band. Outliers get bigger dots.
  - Predicted tensions are drawn as a dashed polygon, only for measured spokes.

## 7. Quality gates and tooling

| Check        | Command                                                         | State at commit      |
|--------------|-----------------------------------------------------------------|----------------------|
| Format       | `gofmt -l .` (must be empty), `golangci-lint fmt --diff`        | clean                |
| Vet          | `go vet ./...`                                                  | clean                |
| Lint         | `golangci-lint run ./...` (v2.12.2, config `.golangci.yml`)     | 0 issues             |
| Go tests     | `go test -cover ./...`                                          | pass                 |
| JS tests     | `cd web && node --test --experimental-test-coverage test/`      | 58/58, 100% lines/branches |

Coverage:

| Package / area                    | Coverage |
|-----------------------------------|----------|
| solver, model, api, web (embed)   | 100%     |
| store                             | 89.4%    |
| cmd                               | 76.6%    |

Linters enabled: bodyclose, errorlint, gocritic, gosec (G104 excluded; off for `_test.go`), misspell, nilerr, noctx, revive, rowserrcheck, sqlclosecheck, unconvert, unparam, usestdlibvars, plus the std-error-handling preset. Formatters: gofmt, goimports (local prefix `github.com/bakhterets/wheel-tuner`).

Mandatory code rules (spec §9 plus the decisions above):
- No `_` error ignores, **including tests**.
- No global state.
- `solver` stays pure.
- Table-driven tests.
- Visualization code stays separate from form and state code.

### Environment quirks (Windows dev machine)

- Keep `go.mod` at `go 1.25.0`. Bumping it, or running `go mod tidy` with a newer modernc, triggers a toolchain auto-download and breaks coverage tooling.
- PowerShell: quote flags with `=`, e.g. `go test "-coverprofile=cover.out" ./...` and `go tool cover "-func=cover.out"`.
- Files created by agents may get CRLF line endings; run `gofmt -w` afterwards. Git normalizes the non-Go files to LF.
- `make test` uses `go test -race`, which needs CGO (not available locally on Windows). It works in CI on Linux; run plain `go test ./...` locally.
- Node is not installed system-wide. A portable Node v26 was used from the agent session folder; CI uses Node 22. The JS tests have no npm dependencies.
- `go run` leaves the compiled child process holding the port after the shell is stopped. Free it by PID (`Get-NetTCPConnection -LocalPort <port>` → `Stop-Process -Id`), or run a built binary instead.

## 8. Known gaps and backlog for next stages

Ordered by suggested priority.

1. **Calibrate solver constants against real wheels.**
   - Collect measured before/after datasets and fit `Params`.
   - Consider exposing `Params` overrides via the API or profile, e.g. per rim type (carbon vs alloy) or spoke type (butted / bladed).
   - `WithSolverParams` already exists server-side.
2. **Hop handling is weak when mixed with wobble** (only −10% in the E2E dataset). Ideas:
   - pair-wise L/R tightening heuristics;
   - a separate radial-first pass;
   - non-uniform weights by measurement uncertainty.
3. **Iteration workflow in the UI:** "apply plan → re-measure" pre-fill, and a comparison of consecutive sessions (trend of max wobble/hop).
4. **Overall tension level target.** The solver currently balances only *relative* tension (deviation from the side mean). It does not reach a target absolute tension or a target L/R ratio.
5. **Coverage gaps:**
   - store: SQLite driver error branches (`rows.Err`, `Scan`, `Commit`) would need fault injection;
   - cmd: `main()` itself is not covered;
   - `main.js` and `setup.js` are only checked manually and end to end. A headless browser test (Playwright) would close this gap.
6. **Schema migrations:** there is only `CREATE TABLE IF NOT EXISTS`. Introduce versioned migrations (`PRAGMA user_version`) before any schema change.
7. **Ops:** a Dockerfile (distroless; a static binary is possible thanks to modernc). Split liveness and readiness if deployed to Kubernetes (today `/healthz` always pings the DB).
8. **Localization:** the UI is English-only.

## 9. How to verify a change quickly

```powershell
gofmt -l .; go vet ./...; golangci-lint run ./...; go test -cover ./...
cd web; node --test --experimental-test-coverage test/
go run ./cmd/wheel-tuner -addr 127.0.0.1:8099 -db $env:TEMP\wt.db   # then open http://127.0.0.1:8099/
```

Manual E2E check:
1. Create a profile (32H, 3x, ERD 600, flanges 35/20, CCW, Rotor Side).
2. Paste a TSV block into the grid and click Solve.
3. Expect: an action list, an orientation header, metrics with Δ%, the wheel map with colored spokes and zones, the radar, and no console errors.
4. Reload the page, pick the profile, load the saved session, and solve again.

## 10. Convention for stage documents

Each stage gets its own `dev_stages/stage_NN.md` in the same structure:
1. scope delivered
2. decisions with the user
3. changes, by layer
4. quality-gate state
5. gaps and backlog

Update the backlog of the previous stage by referencing it rather than editing history.
