# Wheel Tuner

A bicycle wheel truing solver. You enter the spoke tensions, the lateral (wobble) and the radial (hop) runout. The service returns a step-by-step plan in fractional nipple turns (`+1/4`, `-1/2`, …) that removes runout and evens out tension. See [spec.md](spec.md) for the requirements.

Tensions can be entered in kgf or as raw tensiometer readings (ZTTO TC‑02, Park Tool TM‑1), which are converted with the manufacturer's calibration table. A built-in component library (DT Swiss rims, Newmen and DT Swiss hubs) fills the wheel geometry, and the solver derives spoke lengths, bracing angles and the expected left/right tension ratio from it. Development history: [dev_stages/](dev_stages/).

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
| `-web`       | `true`           | serve the embedded UI; `false` = API only (microservice mode) |

Shutdown is graceful on SIGINT/SIGTERM: in-flight requests get up to 10 s to finish.

## Containers (microservices)

Backend and frontend are built and run as two independent images:

| Image | Dockerfile | Contents | Port |
|-------|------------|----------|------|
| `wheel-tuner-api` | [Dockerfile](Dockerfile) | static Go binary on `distroless/static:nonroot`, run with `-web=false`, SQLite in the `/data` volume | 8080 |
| `wheel-tuner-web` | [web/Dockerfile](web/Dockerfile) | `nginx-unprivileged` serving `web/static`; proxies `/api/` and `/healthz` to `BACKEND_URL` | 8080 |

```sh
docker compose up -d --build     # UI on http://localhost:8080, API reachable only via the web proxy
docker compose down
```

- `BACKEND_URL` (web image, default `http://api:8080`, **no trailing slash**) points nginx at the API; it is substituted from [web/nginx/default.conf.template](web/nginx/default.conf.template) at start-up. The web container is stateless and scales independently.
- Probes: web — `GET /livez` (nginx itself), `GET /healthz` (proxied API health); api — `GET /healthz`. The distroless image has no shell, so use orchestrator probes (`httpGet` in Kubernetes) rather than a Docker `HEALTHCHECK`.
- The API runs as uid 65532; mount a writable volume at `/data`. SQLite allows **one** API replica per volume.
- Without containers: `make run-api` (API on `:8081`, no UI) or the default single binary with the embedded UI.

## CI/CD and quay.io

[.github/workflows/ci.yml](.github/workflows/ci.yml) runs on every push and pull request:

1. `go` — gofmt, vet, golangci-lint, `go test -race` with coverage.
2. `web` — JS tests with coverage.
3. `smoke` — `docker compose up --build`, then checks `/livez`, `/healthz`, `/`, `/api/tensiometers` and `/api/components/hubs` through the web proxy.
4. `build-images` (branches and pull requests) — builds both images (matrix `api`, `web`) without pushing. It has no access to registry credentials. The `images` job aggregates the matrix into one stable check name for the ruleset (matrix jobs report as `build image (…)`).
5. `publish` (push to `main` only, i.e. after a merge) — builds and pushes `quay.io/<namespace>/wheel-tuner-{api,web}` with tags `sha-<short>` and `latest`. It runs in the protected `quay` environment.

Repository setup. The repo is public, so the credentials are **environment secrets**, not repository secrets:

1. On quay.io, create the empty repositories `wheel-tuner-api` and `wheel-tuner-web` and a robot account with *Write* on both, and nothing else.
2. In GitHub, go to Settings → Environments → **New environment** and name it `quay`.
3. Under **Deployment branches and tags**, choose *Selected branches and tags* and add `main`. No other branch, pull request or fork can then use the environment or its secrets. Optionally, *Required reviewers* adds a manual approval before every push.
4. In the `quay` environment, add these:

| Kind | Name | Value |
|------|------|-------|
| Environment secret | `QUAY_USERNAME` | robot account, e.g. `bakhterets+github_ci` |
| Environment secret | `QUAY_PASSWORD` | robot account token |
| Variable (optional) | `QUAY_NAMESPACE` | quay.io organisation/user; defaults to the GitHub owner |

5. Recommended: protect `main` (Settings → Rules → require a pull request and passing checks). Then only reviewed code reaches the job that holds the secrets.

GitHub never passes secrets to workflows triggered by pull requests from forks, and masks them in logs. The workflow does not use `pull_request_target`; keep it that way.

### Auto-merge by label

[.github/workflows/automerge.yml](.github/workflows/automerge.yml): adding the label **`automerge`** to a PR into `main` enables GitHub auto-merge (squash). The PR then merges by itself once the required checks pass. Removing the label disables auto-merge. Fork PRs and PRs into other branches are ignored.

Setup:

1. Settings → General → Pull Requests: enable **Allow auto-merge**.
2. `main` needs required status checks (`go`, `web`, `smoke`, `images`), otherwise there is nothing to wait for.
3. Issues → Labels → **New label** `automerge`.
4. Create a fine-grained PAT (Settings → Developer settings → Fine-grained tokens):
   - *Only select repositories* → `wheel-tuner`;
   - permissions *Contents: Read and write* and *Pull requests: Read and write*;
   - an expiry date.
5. Store the PAT as the repository secret `AUTOMERGE_TOKEN`.

A merge made with `GITHUB_TOKEN` would not trigger the push workflow on `main`, so `publish` would never run. That is why the workflow needs a PAT (or a GitHub App token).

## Development

```sh
make test       # go test -race ./... + JS tests
make lint       # golangci-lint (config in .golangci.yml)
make cover      # Go coverage report
make build      # bin/wheel-tuner
```

The JS tests need Node ≥ 22 and no dependencies: `cd web && node --test --experimental-test-coverage 'test/*.test.mjs'`. The DOM is replaced by a minimal fake ([web/test/fakedom.mjs](web/test/fakedom.mjs)).

CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) is described in [CI/CD and quay.io](#cicd-and-quayio).

## Layout

| Path                | Contents                                                                 |
|---------------------|--------------------------------------------------------------------------|
| `cmd/wheel-tuner`   | entry point: flags, logger, HTTP server, graceful shutdown               |
| `internal/solver`   | pure math: spoke geometry, influence matrix, LSQ solver, quantization    |
| `internal/tensiometer` | tensiometer calibration tables and reading → kgf conversion           |
| `internal/model`    | domain entities and input validation                                     |
| `internal/store`    | SQLite repository (migrations, component seeds, CRUD, transactions)      |
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
| `POST`   | `/api/geometry`               | spoke geometry of an (unsaved) profile               |
| `GET`    | `/api/components/rims`        | rim library                                          |
| `GET`    | `/api/components/rims/{id}`   | one rim                                              |
| `GET`    | `/api/components/hubs`        | hub library                                          |
| `GET`    | `/api/components/hubs/{id}`   | one hub                                              |
| `GET`    | `/api/tensiometers`           | tensiometers with their calibration tables           |

Profile (the first nine fields are required; the rest are optional and shown with example values):

```json
{
  "name": "Front F 572 / Fade SP",
  "spokes_count": 28,
  "erd_mm": 602,
  "cross_pattern": 2,
  "flange_dist_left_mm": 29,
  "flange_dist_right_mm": 34,
  "start_reference": "Rim Seam",
  "view_point": "Rotor Side",
  "counting_direction": "CW",

  "rim_component_id": 2,
  "hub_component_id": 1,
  "rim_asymmetry_offset_mm": 0,
  "flange_diam_left_mm": 38.5,
  "flange_diam_right_mm": 38.5,
  "spoke_type": "Straightpull",
  "paired_spokes": true,
  "max_tension_kgf": 122,
  "tension_limit": 2.85,
  "tension_limit_unit": "reading",
  "tensiometer": "ztto-tc-02",
  "spoke_gauge_left": "2.0-1.8-2.0",
  "spoke_gauge_right": "2.0-1.8-2.0"
}
```

- `rim_component_id` / `hub_component_id` reference the library (`null` = custom). The profile keeps its **own copy** of the geometry, so library picks are only a starting point and every value stays editable. The IDs are checked to exist on create and update.
- `rim_asymmetry_offset_mm`: offset of the spoke bed for asymmetric rims. `+` shifts it toward the left flange, which shortens the left flange distance and lengthens the right one.
- `flange_diam_*_mm`: hub flange pitch-circle diameter (PCD). `0` models the spoke as starting at the hub axis (the stage 01 behaviour).
- `spoke_type`: `J-Bend` (default) or `Straightpull`.
- `paired_spokes`: straight-pull hubs that seat spokes in pairs (Newmen Fade SP, DT Swiss 240 SP …). Only valid with `Straightpull`; see *Spoke geometry*.
- `max_tension_kgf`: the rim's tension limit (`0` = unknown). It is a **hard cap**: the solver never plans a turn that would push a predicted tension above it.
- `tension_limit` + `tension_limit_unit`: the builder's desired maximum tension (`0` = none). The unit is `kgf` (default) or `reading`. A reading is a tensiometer value (e.g. TC‑02 mm) and is converted per side with that side's spoke gauge, so L and R may get different kgf caps. `reading` requires a tensiometer, and the value must lie in the calibrated range. The effective cap per side is the lower of the positive values `tension_limit` and `max_tension_kgf`.
- `tensiometer` + `spoke_gauge_*`: when set, measurements may carry `tension_reading` instead of `tension_kgf`. The gauge IDs come from `GET /api/tensiometers`.

Session measurements: `POST /api/wheels/1/sessions` with `{"measurements": [...]}`. Each element looks like this:

```json
{
  "spoke_index": 9, "side": "L", "tension_kgf": 115,
  "lateral_dev_mm": 0.76, "lat_peak_between_next": true,
  "radial_dev_mm": 0, "rad_peak_between_next": false
}
```

With a tensiometer in the profile, send `"tension_reading": 3.0` instead. The server converts it to kgf by linear interpolation in the calibration table of that side's spoke gauge. A reading outside the table returns `400`, because extrapolating a non-linear calibration curve would give misleading tensions. The stored session keeps both the reading and the converted kgf, which is fixed at record time.

The solve response (abridged):

```json
{
  "session_id": 1,
  "orientation": "Start at Rim Seam. View from Rotor Side. Count CW.",
  "actions": [
    {"spoke_index": 5, "side": "L", "turns": 0.5, "fraction": "+1/2",
     "direction": "tighten", "instruction": "Spoke L5: +1/2 turn (Tighten)",
     "predicted_tension_kgf": 126, "tension_warning": true}
  ],
  "turns": [0, 0, ...], "raw_turns": [0.01, -0.03, ...],
  "before": {"max_lateral_mm": 0.6, "max_radial_mm": 0, "tension_spread_left_kgf": 2.39, "tension_spread_right_kgf": 0.9, "cost": 1.2},
  "after":  {"max_lateral_mm": 0.15, "max_radial_mm": 0.12, "...": "..."},
  "predicted": [{"spoke_index": 1, "side": "L", "tension_kgf": 115, "lateral_dev_mm": 0.01, "radial_dev_mm": -0.01, "tension_warning": false}],
  "geometry": {
    "left":  {"flange_offset_mm": 29, "spoke_length_mm": 290.8, "bracing_angle_deg": 5.72, "tension_per_turn_kgf": 24.8},
    "right": {"flange_offset_mm": 34, "spoke_length_mm": 291.4, "bracing_angle_deg": 6.70, "tension_per_turn_kgf": 24.7},
    "tension_ratio": 1.17
  },
  "max_tension_kgf": 122,
  "tension_limit_left_kgf": 105, "tension_limit_right_kgf": 105,
  "iterations": 12
}
```

`tension_limit_*_kgf` are the effective per-side caps that the solver enforced (`0` = none). `tension_warning` ("Approaching Max Rim Tension") is set when `max_tension_kgf > 0` and the predicted tension is at least 95% of it. `POST /api/geometry` accepts a profile body and returns the `geometry` object alone. The UI uses it for a live preview.

Component library (`GET /api/components/hubs`, abridged):

```json
[{"id": 1, "brand": "Newmen", "model": "Fade SP Front 15x110 Boost 6-bolt/CL", "position": "front",
  "axle": "15x110 Boost", "flange_dist_left_mm": 29, "flange_dist_right_mm": 34,
  "flange_diam_left_mm": 38.5, "flange_diam_right_mm": 38.5, "spoke_type": "Straightpull",
  "hole_counts": [28], "source": "https://www.newmen-components.de/..."}]
```

Rims carry `brand`, `model`, `wheel_size`, `hole_counts`, `erd_mm`, `asymmetry_offset_mm`, `max_tension_kgf` and `source`. The library is upserted on every start: data fixes ship with new builds, and row IDs stay stable.

| Seeded data | Contents | Source |
|-------------|----------|--------|
| Rims | DT Swiss F 572, EX 511, XM 481, FR 541, XM 421, XR 391 in 27.5″ and 29″, 28/32H | DT Swiss product pages / spoke calculator; 1200 N ≈ 122 kgf limit from retailer data |
| Hubs | Newmen Fade (SP/J-bend, front/rear, Boost/157), DT Swiss 180/240/350 (SP/J-bend, 6-bolt/CL, Boost and non-Boost) | Newmen *Hub_Flange_Dimensions.pdf*, DT Swiss spoke calculator |
| Tensiometers | ZTTO TC‑02 (reading in mm, 7 spoke gauges incl. butted), Park Tool TM‑1 (scale reading, round steel 1.4–2.6 mm) | manufacturer conversion tables |

The Newmen Fade SP front hub exists in **28H and 32H** with different flange geometry (28H: PCD 38.5 mm, flange offsets 29.0/34.0 mm; 32H: PCD 50.5 mm, 28.5/37.1 mm, per the Newmen PDF), so the library has one row per drilling. The library notes warn when a hub and a rim have different hole counts.

## Model and conventions

**Signs.** Lateral `+` means toward the right (drive-side) flange. Radial `+` is a high spot (hop) relative to the mean radius; a uniform offset is ignored. Tightening a spoke pulls the rim toward its own flange and reduces the radius. Sides: `L` is the left / rotor / non-drive side, `R` is the right / drive side. The view point (`Rotor Side`, `Drive Side`, `Left`, `Right`, …) and the counting direction (`CW`/`CCW`) only affect the crossing topology and the instructions shown on screen.

**Spoke geometry** (`internal/solver/geometry.go`). For each flange, the effective flange distance is `F_L = flange_left − rim_offset` and `F_R = flange_right + rim_offset`. The hub end of a crossed spoke is rotated by `θ = cross·720°/N`. **Paired straight-pull hubs** (`paired_spokes`) seat two spokes side by side, so each spoke's hub end is rotated a further half flange-hole pitch: `θ = cross·720°/N + 360°/N`. Example: DT Swiss F 572 29″ (ERD 602) + Fade SP front 28H, 3x → L 303.0 / R 303.5 mm (without the pair offset the result would be ≈ 298.7 / 299.2 mm, ~4 mm too short). Spoke length is `L = √(D² + F² + R² − 2·D·R·cos θ)`, with `D = PCD/2` and `R = ERD/2`; J-bend lengths subtract the 1.2 mm flange-hole radius. The bracing angle is `α = asin(F/L)`. A true wheel balances the lateral components, `T_L·sin α_L = T_R·sin α_R`, so the expected tension ratio is `T_L/T_R = sin α_R / sin α_L`.

**Influence matrix** (`internal/solver/influence.go`), a force model:
- One nipple turn adds `ΔT = SpokeRigidity·pitch / L` (Hooke's law; `16000 kgf · 0.45 mm / 290 mm ≈ 25 kgf/turn`).
- The rim deflects through its compliance. Lateral gain is `±ΔT·sin α·C_lat`, toward the spoke's own flange. Radial gain is `−ΔT·(radial share)·C_rad`. Defaults are `C_lat = 0.3`, `C_rad = 0.012` mm/kgf. A spoke on the narrow, steep flange therefore moves the rim less laterally than one on the wide flange.
- Influence decays exponentially along the rim arc. The arc length per spoke is `π·ERD/N`. Lateral influence spreads further (120 mm) than radial (50 mm).
- Tension: neighbours on the same flange lose a little, those on the opposite flange gain a little (`NeighborCoupling`). Spokes that cross it on the same flange gain a share as well (`CrossTalk`, per the cross pattern and counting direction).

**Inter-spoke peak** (`*_peak_between_next`). The value entered for spoke *i* is taken as the peak at point *i.5*. A virtual row `A[i.5][j] = (A[i][j] + A[i+1][j]) / 2` is added. The spoke-*i* row itself gets the mean of its neighbours.

**Cost function**: `W_L·‖lat‖² + W_R·‖rad‖² + W_T·‖ΔT‖² + W_X·‖x‖²`. The defaults are `1, 1, 1e-4, 1e-3`: 1 mm of runout weighs the same as 100 kgf of deviation from the side's mean tension. Geometry therefore comes first, while tension breaks ties and pulls in gross outliers. Spokes with no tension reading (`0`) are excluded from the tension term. The solver uses normal equations with conjugate gradients. Turns are then quantized successively: the spoke with the largest turn is rounded to the nearest 1/4, fixed, and the rest is re-solved. Turns under 1/8 are dropped.

Truing is iterative: apply the plan, re-measure, and solve again in a new session. All constants (`solver.DefaultParams`) are empirical values for steel spokes and an alloy rim.

**Validation.** The spoke count must be one of 20/24/28/32/36. ERD, cross pattern, flange distances (after the rim offset) and flange diameters (`[0, ERD/2)`) must lie in physical ranges, and the cross pattern must fit the hub. Measurements must cover every spoke exactly once. Both flanges must have spokes (an all-L or all-R wheel is rejected). For a crossed lacing, each flange needs an even number of spokes, more than `2·cross−1`. A tensiometer needs a known gauge on both sides, and readings need a tensiometer. Details are in `internal/solver/wheel.go` and `internal/model`.

## UI

- **Component library:** brand/model pickers for the rim and the hub fill the profile fields, which stay editable. Notes flag hole-count mismatches, for example a 28H hub with a 32H rim. Picking a paired straight-pull hub ticks *Paired SP heads*.
- **Live geometry panel:** spoke lengths, flange offsets, bracing angles, ΔT per turn and the L/R tension ratio, recomputed as the fields change.
- **Tensiometer mode:** pick a tensiometer and per-side spoke gauges. The tension column then takes raw readings (also through clipboard paste) and shows the converted `≈ kgf` beside each cell. Out-of-range readings are marked invalid with the calibrated range.
- **Rim limit:** actions and predictions at ≥ 95% of the rim's max tension are flagged with ⚠️ *Approaching Max Rim Tension*.
- **Desired max tension:** a cap in kgf or in tensiometer units (e.g. TC‑02 reading 2.85). The results show the enforced per-side caps (*Tension cap (desired limit / rim max): Left ≤ 105 kgf · Right ≤ 105 kgf*).
- **Split measurement tables:** *Left side — rotor* and *Right side*, for both front and rear wheels. The *Spoke #1 is on* toggle sets which side spoke 1 belongs to; the sides alternate from there. Rows keep their global spoke number.
- **Bulk list input:** each side has a field for a list such as `[3.15,3.16,3.05,…]` (brackets optional; separators `,` or whitespace, or `;` when values use decimal commas) and a column picker (tension/reading, lateral, radial). Values fill that side's spokes in order; too many values or a non-number is rejected with a message. Clipboard paste from Excel also walks down the same side.
- **Persistence:** the profile form, the measurement grid and the last result are stored in `localStorage` (`wheel-tuner:state`) and restored after a page reload. Corrupt or foreign data is ignored, never overwritten before a successful restore.

## Deviations from the spec

- **SQLite driver:** [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (pure Go) instead of `mattn/go-sqlite3`. It needs no CGO or C toolchain, cross-compiles to a single static binary, and the SQL schema is identical.

## Test coverage

| Package                | Coverage |
|------------------------|----------|
| `internal/solver`      | 100%     |
| `internal/tensiometer` | 100%     |
| `internal/model`       | 100%     |
| `internal/api`         | 100%     |
| `web` (embed)          | 100%     |
| `internal/store`       | ~93%     |
| `cmd/wheel-tuner`      | ~78%     |
| JS modules             | 100% lines/branches/functions |

Uncovered: SQLite driver error branches (`rows.Err`, `Scan`, `Commit` failures that cannot be reproduced without fault injection into the driver), and `main()` itself (all logic lives in `run()`, which is tested). `main.js` and `setup.js` are browser wiring, checked manually and end to end.
