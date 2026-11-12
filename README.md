# Wheel Tuner

A bicycle wheel truing solver. You enter the spoke tensions, the lateral (wobble) and the radial (hop) runout. The service returns a step-by-step plan in fractional nipple turns (`+1/4`, `-1/2`, …) that removes runout and evens out tension. See [spec.md](spec.md) for the requirements.

- **Backend:** Go 1.25, `net/http` (no frameworks), `log/slog`, SQLite.
- **Frontend:** HTML5 + vanilla JS (ES modules) + CSS, SVG visualizations. Embedded in the binary via `go:embed`.

## Quick start

```sh
go run ./cmd/wheel-tuner                      # http://localhost:8080
go run ./cmd/wheel-tuner -addr 127.0.0.1:9000 -db /tmp/wheels.db -log-level debug
```

| Flag         | Default          | Purpose                                 |
|--------------|------------------|-----------------------------------------|
| `-addr`      | `:8080`          | listen address                          |
| `-db`        | `wheel-tuner.db` | SQLite file (created automatically)     |
| `-log-level` | `info`           | `debug`, `info`, `warn`, `error`        |

Shutdown is graceful on SIGINT/SIGTERM: in-flight requests get up to 10 s to finish.

## Development

```sh
make test       # go test -race ./... + JS tests
make lint       # golangci-lint (config in .golangci.yml)
make cover      # Go coverage report
make build      # bin/wheel-tuner
```

The JS tests need Node ≥ 20 and no dependencies: `cd web && node --test --experimental-test-coverage test/`. The DOM is replaced by a minimal fake ([web/test/fakedom.mjs](web/test/fakedom.mjs)).

CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs gofmt, vet, golangci-lint, the Go tests with `-race`, and the JS tests.

## Layout

| Path                | Contents                                                                 |
|---------------------|--------------------------------------------------------------------------|
| `cmd/wheel-tuner`   | entry point: flags, logger, HTTP server, graceful shutdown               |
| `internal/solver`   | pure math: influence matrix, LSQ solver, quantization into turns         |
| `internal/model`    | domain entities and input validation                                     |
| `internal/store`    | SQLite repository (migrations, CRUD, transactions)                       |
| `internal/api`      | HTTP handlers, DTOs, middleware (logging, recover, security headers)     |
| `web`               | static frontend + `embed.go`                                             |

## HTTP API

Errors come as `{"error": "...", "field": "..."}`. Codes: `400` invalid input, `404` not found, `422` unsolvable problem, `500` internal error. Request bodies are limited to 1 MB, and unknown fields are rejected.

| Method   | Path                          | Description                                          |
|----------|-------------------------------|------------------------------------------------------|
| `GET`    | `/healthz`                    | health check                                         |
| `GET`    | `/api/wheels`                 | list of profiles                                     |
| `POST`   | `/api/wheels`                 | create a profile → `201`                             |
| `GET`    | `/api/wheels/{id}`            | get a profile                                        |
| `PUT`    | `/api/wheels/{id}`            | update a profile                                     |
| `DELETE` | `/api/wheels/{id}`            | delete a profile and its sessions → `204`            |
| `GET`    | `/api/wheels/{id}/sessions`   | session history of a profile                         |
| `POST`   | `/api/wheels/{id}/sessions`   | save measurements as a new session → `201`           |
| `GET`    | `/api/sessions/{id}`          | session with its measurements                        |
| `DELETE` | `/api/sessions/{id}`          | delete a session → `204`                             |
| `POST`   | `/api/sessions/{id}/solve`    | solve a saved session                                |
| `POST`   | `/api/solve`                  | stateless solve: `{"wheel": {...}, "measurements": [...]}` |

Profile:

```json
{
  "name": "Front 32H 3x",
  "spokes_count": 32,
  "erd_mm": 600,
  "cross_pattern": 3,
  "flange_dist_left_mm": 35,
  "flange_dist_right_mm": 20,
  "start_reference": "Rim Seam",
  "view_point": "Rotor Side",
  "counting_direction": "CCW"
}
```

Session measurements: `POST /api/wheels/1/sessions` with `{"measurements": [...]}`. Each element looks like this:

```json
{
  "spoke_index": 9, "side": "L", "tension_kgf": 115,
  "lateral_dev_mm": 0.76, "lat_peak_between_next": true,
  "radial_dev_mm": 0, "rad_peak_between_next": false
}
```

The solve response (abridged):

```json
{
  "session_id": 1,
  "orientation": "Start at Rim Seam. View from Rotor Side. Count CCW.",
  "actions": [
    {"spoke_index": 9, "side": "L", "turns": 0.25, "fraction": "+1/4",
     "direction": "tighten", "instruction": "Spoke L9: +1/4 turn (Tighten)"}
  ],
  "turns": [0, 0, ...], "raw_turns": [0.01, -0.03, ...],
  "before": {"max_lateral_mm": 0.76, "max_radial_mm": 0.4, "tension_spread_left_kgf": 6.05, "tension_spread_right_kgf": 0, "cost": 2.17},
  "after":  {"max_lateral_mm": 0.25, "max_radial_mm": 0.36, "...": "..."},
  "predicted": [{"spoke_index": 1, "side": "L", "tension_kgf": 115, "lateral_dev_mm": 0.01, "radial_dev_mm": -0.01}],
  "iterations": 12
}
```

## Model and conventions

**Signs.** Lateral `+` means toward the right (drive-side) flange. Radial `+` is a high spot (hop) relative to the mean radius; a uniform offset is ignored. Tightening a spoke pulls the rim toward its own flange and reduces the radius. Sides: `L` is the left / rotor / non-drive side, `R` is the right / drive side. The view point (`Rotor Side`, `Drive Side`, `Left`, `Right`, …) and the counting direction (`CW`/`CCW`) only affect the crossing topology and the instructions shown on screen.

**Influence matrix** (`internal/solver/influence.go`):
- The bracing angle comes from the flange distances and the ERD. It splits a spoke's pull into lateral and radial components. Lateral gain ∝ `1/sin β` and radial gain ∝ `cos β`.
- Influence decays exponentially along the rim arc. The arc length per spoke is `π·ERD/N`. Lateral influence spreads further (120 mm) than radial (50 mm).
- Tension: the spoke being turned gains `25 kgf/turn`. Neighbours on the same flange lose a little, those on the opposite flange gain a little (`NeighborCoupling`). Spokes that cross it on the same flange gain a share as well (`CrossTalk`, per the cross pattern and counting direction).

**Inter-spoke peak** (`*_peak_between_next`). The value entered for spoke *i* is taken as the peak at point *i.5*. A virtual row `A[i.5][j] = (A[i][j] + A[i+1][j]) / 2` is added. The spoke-*i* row itself gets the mean of its neighbours.

**Cost function**: `W_L·‖lat‖² + W_R·‖rad‖² + W_T·‖ΔT‖² + W_X·‖x‖²`. The defaults are `1, 1, 1e-4, 1e-3`: 1 mm of runout weighs the same as 100 kgf of deviation from the side's mean tension. Geometry therefore comes first, while tension breaks ties and pulls in gross outliers. Spokes with no tension reading (`0`) are excluded from the tension term. The solver uses normal equations with conjugate gradients. Turns are then quantized successively: the spoke with the largest turn is rounded to the nearest 1/4, fixed, and the rest is re-solved. Turns under 1/8 are dropped.

Truing is iterative: apply the plan, re-measure, and solve again in a new session. All constants (`solver.DefaultParams`) are empirical values for steel spokes and an alloy rim.

**Validation.** The spoke count must be one of 20/24/28/32/36. ERD, cross pattern and flange distances must lie in physical ranges, and the cross pattern must fit the hub. Measurements must cover every spoke exactly once. Both flanges must have spokes (an all-L or all-R wheel is rejected). For a crossed lacing, each flange needs an even number of spokes, more than `2·cross−1`. Details are in `internal/solver/wheel.go` and `internal/model`.

## Deviations from the spec

- **SQLite driver:** [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (pure Go) instead of `mattn/go-sqlite3`. It needs no CGO or C toolchain, cross-compiles to a single static binary, and the SQL schema is identical.

## Test coverage

| Package           | Coverage |
|-------------------|----------|
| `internal/solver` | 100%     |
| `internal/model`  | 100%     |
| `internal/api`    | 100%     |
| `web` (embed)     | 100%     |
| `internal/store`  | ~89%     |
| `cmd/wheel-tuner` | ~77%     |
| JS modules        | 100% lines/branches |

Uncovered: SQLite driver error branches (`rows.Err`, `Scan`, `Commit` failures that cannot be reproduced without fault injection into the driver), and `main()` itself (all logic lives in `run()`, which is tested). `main.js` and `setup.js` are browser wiring, checked manually and end to end.
