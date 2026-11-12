# Stage 03: Paired straight-pull geometry, tension limit, split UI, microservices, CI/CD

Handoff document for agents continuing development. Read [stage_01.md](stage_01.md) and [stage_02.md](stage_02.md) first; their conventions, layout and quirks still apply unless overridden here.

| Item    | Value                                                     |
|---------|-----------------------------------------------------------|
| Branch  | `dev_step_01`                                             |
| Commit  | "Stage 03: paired SP geometry, tension limit, split tables, containers, CI" (find it with `git log -- dev_stages/stage_03.md`) |
| Date    | 2026-11-12                                                |
| Status  | All stage scope done; gofmt, vet, golangci-lint, Go and JS tests green; browser E2E checked. Docker images **not built locally** (no Docker on the dev machine) — the CI `smoke` job is their first real build. |

## 1. Scope delivered

| User request | Where |
|--------------|-------|
| Fix the Newmen Fade data: the user laces **304 mm** spokes where **303 mm** are needed for 3x — verify | paired straight-pull model (`Wheel.PairedSpokes`, `spanDeg`), `hub_components.paired_spokes`, Fade SP 32H rows; §3 |
| Split the measurement grid into **Left** (= rotor side, front and rear) and **Right** tables | `index.html` `.grid-split`, `grid.js` `renderGrid(tbody, rows, side, conv)`, `measurements.js` side helpers |
| Bulk input of a list `[3.15,3.16,3.05,…]` instead of typing every cell | `web/static/js/bulk.js` (`parseList`, `applyList`), one `form.bulk` per side |
| Page reload must keep the cache and the calculation state | `web/static/js/persist.js` (`localStorage`), `main.js` restore/subscribe |
| Frontend and backend as independent containerised microservices | `Dockerfile` (api), `web/Dockerfile` + `web/nginx/default.conf.template` (web), `docker-compose.yml`, `-web=false` flag |
| GitHub Actions CI; images pushed to quay.io on merge to `main` | `.github/workflows/ci.yml` jobs `smoke`, `images` and `publish` |
| Desired tension limit in kgf **or** in TC‑02 mm scale | `WheelSetup.tension_limit` + `tension_limit_unit`, `model.TensionLimitsKgf`, solver caps (`limitSystem`, `capTurn`, `enforceLimits`) |

## 2. Decisions

Made autonomously (autopilot); revisit freely if the user disagrees.

1. **Paired straight-pull model.** Hubs such as Newmen Fade SP and DT 240 SP seat spokes in pairs, so each spoke's hub end is rotated a further half flange-hole pitch (`360°/N`) compared with a classic hub. The crossing angle becomes `θ = cross·720°/N + 360°/N` (only when `cross > 0`; radial lacing is unaffected). The flag lives on the hub component (`paired_spokes`) and is **copied** into the profile like the rest of the geometry (stage-02 decision 2). Paired requires `Straightpull`; otherwise 400 `field: paired_spokes`. The UI forces it off for J-bend.
2. **Rim max tension is now a hard cap**, not only a warning (closes stage-02 backlog 1). The ≥ 95% warning stays.
3. **Desired limit.** `tension_limit` (0 = none) with `tension_limit_unit` `kgf` (default) or `reading`. A reading is converted **per side** with that side's gauge (`TensionLimitsKgf`), because L and R may use different spokes. Effective cap per side = the lower positive of the desired limit and `max_tension_kgf`. Errors: a reading limit without a tensiometer → `tension_limit_unit`; a reading outside the gauge's calibrated range → `tension_limit` (never extrapolated, same rule as measurements).
4. **Solver enforcement** (`internal/solver/solver.go`): an active-set penalty. Each measured spoke gets a zero-weight row `T_i + ΔT_i(x) → cap_i`; rows of spokes predicted above their cap are switched on (`limitWeight`), rows of spokes that fell below their cap are switched off again (the two-sided penalty would otherwise pull them **up** to the cap), and the system is re-solved until the active set is stable (bounded to 2n+1 passes). During successive quantization `capTurn` lowers a rounded turn by one step if it would leave that spoke over its cap, or if rounding up would push an already fixed spoke over its cap through cross-talk; `enforceLimits` finally loosens any spoke still over (rounding, skipped micro-turns). Unmeasured spokes (0 kgf) are never capped. A spoke already above its cap is loosened.
5. **Frontend rows stay one global array** indexed by spoke number; the two tables are filtered views. The per-row side select was replaced by one toggle *Spoke #1 is on Left/Right*; sides alternate from it (`assignSides`). Rows keep their global number, so the solver API is unchanged.
6. **Bulk list format.** Brackets optional. Separators: `,` or whitespace; if the text contains `;`, only `;` separates (so `3,15; 3,16` works with decimal commas). An empty item skips that spoke. More values than the side has spokes, an invalid number, or a negative tension → error message, nothing changed. Fewer values fill the first spokes only. A kgf value clears the row's `tension_reading`.
7. **Persistence** in `localStorage` under `wheel-tuner:state` (version 1): wheel form, rows and the last result. Written on every change, but only **after** the initial restore finished (`persisting` flag), so a failed load never overwrites saved state. `loadState` validates and sanitises rows (`restoreRow`) and drops a malformed result. If the saved profile id no longer exists, the form falls back to a new profile with the saved values.
8. **Microservices.** The same Go binary serves API only with `-web=false`. The web image is plain nginx serving the static files and reverse-proxying `/api/` and `/healthz` to `BACKEND_URL`, so the browser stays same-origin (no CORS, the CSP is unchanged). The single-binary mode with the embedded UI is kept for local use.
9. **Images:** api = `golang:1.25-bookworm` builder, `CGO_ENABLED=0`, runtime `gcr.io/distroless/static-debian12:nonroot` (uid 65532), volume `/data`. web = `nginxinc/nginx-unprivileged:1.27-alpine`, port 8080. No Docker `HEALTHCHECK` (distroless has no shell); use orchestrator `httpGet` probes.
10. **CI publishing.** Images are built on every run, so Dockerfile breakage shows up in PRs. The `build-images` matrix (branches and PRs) only builds and has no credentials; the `images` job turns it into one stable required check. The `publish` job runs only on a push to `main` and uses the protected GitHub environment `quay`, whose deployment-branch rule allows `main` only. The repo is public, so the quay.io credentials are environment secrets: no other branch, PR or fork can read them. Tags: `sha-<short>` and `latest`. Registry namespace: variable `QUAY_NAMESPACE`, else the GitHub owner.

## 3. Data notes for the user's build

- **Correction to stage 02:** the Newmen Fade SP front hub is **not** 28H only. Newmen also lists a 32H version with different flanges. Library rows:

  | Model | Holes | PCD L/R, mm | Flange L/R, mm | Paired |
  |-------|-------|-------------|----------------|--------|
  | Fade SP Front 15x110 Boost 6-bolt/CL | 28 | 38.5 / 38.5 | 29.0 / 34.0 | yes |
  | Fade SP Front 15x110 Boost 6-bolt 32h | 32 | 50.5 / 50.5 | 28.5 / 37.1 | yes |

  Rear SP rows (28H and 32H, 148 and 157) are paired as well; all J-bend Fade rows are not.
- **304 mm vs 303 mm check**, DT Swiss F 572 29″ (ERD 602) + Fade SP front 28H, 3x:

  | Model | θ | L, mm | R, mm |
  |-------|---|-------|-------|
  | classic (stage 02) | 77.14° | 298.7 | 299.2 |
  | paired (stage 03)  | 90.00° | **303.0** | **303.5** |

  The paired result matches the 303 mm the user got from the calculator. 304 mm spokes are +1.0 / +0.5 mm: fine (usual tolerance ±1 mm with 12 mm nipples; the spoke end should sit flush with or just below the nipple head). Asserted in `internal/api/components_test.go` (`TestGeometryEndpoint`, 303.006 / 303.525) and `internal/solver/geometry_test.go`.
- TC‑02 limit examples (gauge 2.0‑1.8‑2.0): reading 2.8 → 102 kgf, 2.85 → 105 kgf; gauge 1.8: 2.8 → 100 kgf, 3.4 → 148 kgf.

## 4. Changes by layer

### 4.1 `internal/solver`

- `Wheel` gained `PairedSpokes`, `TensionLimitLeftKgf`, `TensionLimitRightKgf`; `Validate` rejects paired J-bend and negative limits.
- `spanDeg()` adds the pair offset; `tensionLimit(side)` returns the side cap, which the API has already combined with the rim max (`model.TensionLimitsKgf` → `minLimit`).
- `solver.go`: `limitRows`, `limitSystem`, `updateLimits`, `solveLimited`, `capTurn`, `enforceLimits` (§2.4).
- Tests: paired lengths in `geometry_test.go`; `TestSolveTensionLimit`, `TestSolveNearLimitStillImproves`, `TestCapTurn` in `solver_test.go`; paired validation in `wheel_test.go`. All earlier regression tests unchanged.

### 4.2 `internal/model`

- `WheelSetup`: `paired_spokes`, `tension_limit`, `tension_limit_unit` (`LimitUnitKgf`/`LimitUnitReading`, normalised to lower case, default `kgf`).
- `TensionLimitsKgf() (left, right, err)`; `HubComponent.paired_spokes`.

### 4.3 `internal/store`

- `schemaV3`: `hub_components.paired_spokes`, `wheel_setups.paired_spokes`, `tension_limit`, `tension_limit_unit`. Stage-02 DBs upgrade in place; legacy rows get unit `kgf`.
- Seeds: paired flag on SP rows; new Fade SP 32H front and rear rows.

### 4.4 `internal/api`

- `toSolverWheel` passes paired and per-side limits (validation errors surface as 400 on create/update, solve and `/api/geometry`).
- Solve response adds `tension_limit_left_kgf` / `tension_limit_right_kgf` (effective caps, 0 = none).
- Tests: `TestTensionLimit` and paired cases in `TestGeometryEndpoint` (`components_test.go`).

### 4.5 `cmd/wheel-tuner`

- Flag `-web` (default `true`); `false` registers no static handler (`/` → 404). `TestRunAPIOnly`; tests share a `start(t, args...)` helper.

### 4.6 Frontend (`web/static`)

| Module | Change |
|--------|--------|
| `bulk.js` (new) | `parseList(text)`, `applyList(rows, side, key, text) → {rows, error}` |
| `persist.js` (new) | `STORAGE_KEY`, `saveState(storage, state) → bool`, `loadState(storage)` with validation |
| `measurements.js` | `defaultSide`, `emptyRow(index, first)`, `firstSide`, `assignSides`, `sideIndexes`; `resizeRows` keeps the alternation |
| `grid.js` | rewritten for per-side tables: `GRID_COLUMNS` (no side column), `renderGrid(tbody, rows, side, conv)`, `bindGrid(table, {side,…})`, `pasteSide` (paste walks down the same side), `hasInvalidCells(root)` |
| `results.js` | `limitText(result)` → `<p class="limits">` |
| `components.js` | `hubPatch` sets `paired_spokes` |
| `setup.js` | new fields; checkbox handling; paired forced off for J-bend |
| `main.js` | two tables, bulk forms, first-side toggle, `localStorage` restore/save |
| `index.html`, `app.css` | paired checkbox, *Desired max tension* + *Limit unit*, split tables with side colours, bulk forms, limits line |

JS tests: `bulk_persist.test.mjs` (new); `render.test.mjs`, `library.test.mjs`, `measurements.test.mjs` updated.

### 4.7 Containers and CI

| File | Purpose |
|------|---------|
| `Dockerfile` | api image (§2.9); CMD `-addr :8080 -db /data/wheel-tuner.db -web=false` |
| `web/Dockerfile` | web image; `ENV BACKEND_URL=http://api:8080` |
| `web/nginx/default.conf.template` | listen 8080; security headers on `/`; `/api/` and `= /healthz` → `${BACKEND_URL}`; `= /livez` → 200. `BACKEND_URL` must have **no trailing slash** (URI passed through unchanged). envsubst only replaces defined variables, so `$host` etc. survive |
| `docker-compose.yml` | `api` (volume `wheel-data`) + `web` (port 8080); images `quay.io/${QUAY_NAMESPACE:-bakhterets}/wheel-tuner-{api,web}:${TAG:-latest}` |
| `deploy/podman/wheel-tuner.yaml` | Podman (`podman kube play`): one pod; the containers share `localhost`, so the API is overridden to `127.0.0.1:8081` and `BACKEND_URL=http://127.0.0.1:8081`; PVC → named volume `wheel-tuner-data`; images `localhost/wheel-tuner-{api,web}:latest` |
| `scripts/podman.sh` | Linux only: `up [--pull]` (build or pull+retag, `kube play --replace`, wait for `/healthz`), `down [--volumes]`, `logs`, `status`; `PORT` override via `kube play --publish` |
| `scripts/smoke.sh` | curl probes through the proxy, shared by the `smoke` and `podman` CI jobs |
| `.gitattributes` | LF for `*.sh`, Dockerfiles, YAML, nginx template (Windows checkouts) |
| `.dockerignore`, `web/.dockerignore` | keep contexts small (no tests, DBs, `.git`) |
| `.github/workflows/ci.yml` | `go`, `web`, then `smoke` (compose up + `scripts/smoke.sh`) and `podman` (rootless Podman: pod manifest via `scripts/podman.sh`, restart on another port with the kept volume), then `build-images` (build only, branches/PRs; aggregated into the single required check `images`, because matrix jobs report as `build image (…)`) or `publish` (push on `main`, environment `quay`) |
| `.github/workflows/automerge.yml` | label `automerge` on a PR into `main` → `gh pr merge --auto --squash`; removing it → `--disable-auto`. Uses the repo secret `AUTOMERGE_TOKEN` (a fine-grained PAT): merges made with `GITHUB_TOKEN` do not trigger `publish` |
| `Makefile` | `run-api`, `images`, `compose-up`, `compose-down`, `podman-up`, `podman-down` |

**Repository setup needed once:**

1. On quay.io, create `wheel-tuner-api` and `wheel-tuner-web` as empty repositories, with no Dockerfile or GitHub build trigger.
2. Create a robot account with *Write* on those two repositories only.
3. In GitHub, create the environment `quay` (Settings → Environments). Set *Deployment branches* to `main` only; required reviewers are optional.
4. In that environment, add the secrets `QUAY_USERNAME` (e.g. `bakhterets+github_ci`) and `QUAY_PASSWORD` (the robot token), plus the optional variable `QUAY_NAMESPACE`.
5. Do not add these as repository secrets.

## 5. Quality gates

| Check     | Command                                                     | State                |
|-----------|-------------------------------------------------------------|----------------------|
| Format    | `gofmt -l .`                                                | clean                |
| Vet       | `go vet ./...`                                              | clean                |
| Lint      | `golangci-lint run ./...`                                   | 0 issues             |
| Go tests  | `go test -cover ./...`                                      | pass                 |
| JS tests  | `cd web && node --test --experimental-test-coverage 'test/*.test.mjs'`  | 84/84, 100% lines/branches/functions |
| Workflow YAML | parsed with js-yaml                                     | valid                |

| Package / area                                  | Coverage |
|-------------------------------------------------|----------|
| solver, tensiometer, model, api, web (embed)    | 100%     |
| store                                           | 93.0%    |
| cmd                                             | 78.4%    |

### Browser E2E check (Playwright against `go run`)

1. DT Swiss F 572 29″ + Newmen Fade SP Front 28H, 3x → *Paired SP heads* ticked automatically; geometry panel **303.0 / 303.5 mm**; 14 rows per table.
2. TC‑02, gauge 2.0‑1.8‑2.0, desired limit 2.85 *reading* → save.
3. Bulk fill: L and R tension lists, L lateral list; 15 values for 14 spokes → *"15 values for 14 R-side spokes."* Converted tension shown as `≈ 102 kgf`.
4. Solve → *Tension cap (desired limit / rim max): Left ≤ 105 kgf · Right ≤ 105 kgf*, 10 actions.
5. Reload → form (name, limit, unit, paired), rows and result restored.
6. *Spoke #1 is on: Right* → spoke 1 moves to the right table; survives reload.
7. Paste of three lines into R2 lateral fills R2, R4, R6 (same side). No console errors.

Pitfall found during E2E: a browser that cached the old `main.js` threw `renderGrid … null`; the real cause was an unapplied edit. If you see it, hard-reload and re-check the `ui` refs in `main.js`.

### Pre-PR review fixes ([review.md](review.md))

1. `make run-api` target added (documented but missing).
2. `statusRecorder.Unwrap()` so `http.ResponseController` (flush, deadlines) reaches the real writer (`TestStatusRecorderUnwrap`).
3. Pasting into the tension column keeps both fields consistent: a pasted reading, even an empty one, clears the converted `tension_kgf` (re-derived by `applyReadings`); a pasted kgf clears `tension_reading`.
4. Tension rows are centred per side (`centerTension`), so `ΔT` is measured against the post-adjustment side mean and uniform side tightening is free in the tension term. Absolute tensions and caps use the uncentred matrix (`problem.tension`).
5. `capTurn` also steps down when rounding up would push an already fixed spoke over its cap.
6. `deviationErrors`: the spoke row under a flagged peak is interpolated towards the peak instead of ignoring it.
7. Found while verifying 4–5 (pre-existing): an active cap row stayed active after its spoke dropped below the cap and pulled the tension **up**, which made a near-cap wheel worse than no action (cost 1.70 → 4.47 at HEAD). `updateLimits` now also deactivates such rows; the same case now ends at cost 0.47 (`TestSolveNearLimitStillImproves`). `TestTensionWarnings` covers both a binding 112 kgf and the 122 kgf DT rim limit.

## 6. Gaps and backlog

Stage-01 backlog 1–5, 7, 8 and stage-02 backlog 2–6 remain open. Stage-02 item 1 (max tension in the optimizer) is done.

1. **Newmen "offset spoke hole"** value from `Hub_Flange_Dimensions.pdf` is not modelled: its sign/reference convention is undocumented. If confirmed, add it as a per-flange tangential offset (it would shift θ slightly and change lengths by < 0.5 mm).
2. **Docker images are unverified locally.** First real build is the CI `smoke` job; check its logs on the first push. Consider pinning base images by digest and adding Trivy/Grype scanning plus SBOM/provenance (`docker/build-push-action` `provenance`/`sbom`).
3. **Kubernetes manifests / Helm chart** (Deployments with `httpGet` probes: web `/livez`, api `/healthz`; a PVC for `/data`; the api must stay a single replica while on SQLite). Moving to Postgres would allow horizontal scaling of the api.
4. **Persistence is per browser** (`localStorage`). Cross-device continuity would need a server-side "draft" entity.
5. **Bulk list for the peak flags** (`*_peak_between_next`) and a "copy column as list" export are not implemented.
6. **Limit UX:** show the converted kgf next to a reading limit in the form (currently only in the results line).
7. **Quantization near a cap:** with ≈2 kgf headroom and ≈6 kgf per quarter turn, `capTurn` can refuse a key tightening (e.g. L9) before the headroom-creating loosens are fixed. Deferring capped spokes in the rounding order (or a small branch-and-bound over the last steps) could recover more runout.

## 7. How to verify quickly

```powershell
gofmt -l .; go vet ./...; golangci-lint run ./...; go test -cover ./...
cd web; node --test --experimental-test-coverage 'test/*.test.mjs'; cd ..
go run ./cmd/wheel-tuner -addr 127.0.0.1:8099 -db $env:TEMP\wt.db          # UI + API
go run ./cmd/wheel-tuner -addr 127.0.0.1:8081 -web=false -db $env:TEMP\wt.db # API only
docker compose up -d --build   # where Docker is available; UI on http://localhost:8080
```
