# Pre-PR Review Remediation Spec

## Context

Read [spec.md](../spec.md), [README.md](../README.md), and [stage_01.md](stage_01.md)–[stage_03.md](stage_03.md) first.
Follow all repository quality gates and mandatory rules:
- No `_` error ignores (including tests).
- No global mutable state.
- `internal/solver` and `internal/tensiometer` must remain 100% pure Go (stdlib only).
- 100% test coverage must be preserved across `internal/solver`, `internal/tensiometer`, `internal/model`, `internal/api`, `web`, and all tested JS modules (`web/test/*.test.mjs`).
- `gofmt -l .`, `go vet ./...`, and `golangci-lint run ./...` must report zero issues.

---

## Tasks

### 1. Add Missing `run-api` Target to `Makefile`

**Problem:**
`Makefile` declares `run-api` in `.PHONY` (line 1) and `README.md` (line 43) documents `make run-api` (API on `:8081`, no UI), but the target recipe is missing from `Makefile`.

**Fix:**
Add the `run-api` target to `Makefile`:

```makefile
run-api:
	go run ./cmd/wheel-tuner -addr :8081 -db wheel-tuner.db -web=false
```

---

### 2. Add `Unwrap()` to `statusRecorder` in `internal/api/http.go`

**Problem:**
`statusRecorder` wraps `http.ResponseWriter` without implementing `Unwrap() http.ResponseWriter`, hiding underlying interfaces (`http.Flusher`, `io.ReaderFrom`, etc.) from `http.ResponseController` and `http.FileServerFS`.

**Fix:**
1. In `internal/api/http.go`, add:
   ```go
   func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
   ```
2. Add a unit test assertion in `internal/api/api_test.go` (e.g., via `http.NewResponseController(w)` inside a handler or directly testing `statusRecorder.Unwrap()`) to keep `internal/api` statement coverage at 100%.

---

### 3. Clear `tension_kgf` When Pasting Empty Reading Cells in Tensiometer Mode

**Problem:**
In `web/static/js/tensiometer.js`, `applyReadings(rows, conv)` skips rows where `r.tension_reading === null`:
```js
if (r.tension_reading === null || r.tension_reading === undefined) {
  return r;
}
```
When a tensiometer is active (`conv !== null`) and the user pastes an empty cell (`""` -> `tension_reading: null`) over a row that previously had a valid reading and converted `tension_kgf`, `applyPaste` sets `row.tension_reading = null`, and `applyReadings` leaves the stale `row.tension_kgf` intact instead of clearing it to `null`.

**Fix:**
1. In `web/static/js/paste.js` (`applyPaste`) or `web/static/js/tensiometer.js` (`applyReadings`), ensure that when `conv !== null` and `tension_reading` is cleared (`null`), `tension_kgf` is also set to `null` (or in `applyPaste`, when writing `col.key === 'tension_reading'`, also set `row.tension_kgf = null`, and when writing `col.key === 'tension_kgf'`, also set `row.tension_reading = null`).
2. Add a regression test in `web/test/paste.test.mjs` or `web/test/render.test.mjs` verifying that pasting an empty cell over an existing tensiometer reading clears both `tension_reading` and `tension_kgf`.

---

### 4. Center Tension Matrix Columns Per Side in `internal/solver/solver.go`

**Problem:**
In `internal/solver/solver.go`:
- `tensionErrors` computes initial tension deviations relative to the initial per-side mean: $e_{T,i} = T_{0,i} - \bar{T}_{0,\text{side}(i)}$.
- However, the tension block of the system matrix $A$ (`inf.Tension`) is stacked uncentered. As a result, $e_T + A_T x$ penalizes deviations from the **initial** side mean $\bar{T}_{0,\text{side}}$ rather than the **post-adjustment** side mean $\bar{T}_{\text{side}}(x)$.
- Consequently, uniform or net tightening of a flange (e.g., when fixing a hop) is penalized by the tension cost term even when post-adjustment tension spread is zero, and `Metrics.Cost` measures tension residuals against the initial mean while `Metrics.TensionSpreadLeftKgf` / `TensionSpreadRightKgf` measure standard deviation around the new mean.

**Fix:**
1. In `internal/solver/solver.go`, add a `centerTension(m *Matrix, ms []Measurement) *Matrix` function (analogous to `centerRadial`):
   - For each column $j \in [0, N)$, compute the mean of `m.At(i, j)` over measured spokes (`ms[i].TensionKgf > 0`) on the Left side and separately on the Right side.
   - For each measured spoke row $i$, subtract its side's column mean from `m.At(i, j)`. Unmeasured spoke rows (`ms[i].TensionKgf == 0`) have weight 0 and can be zeroed or left unchanged.
   - Note: `limitSystem` and `tensionAfter` must continue to use the **uncentered** tension matrix (`inf.Tension`) so predicted absolute spoke tensions $T_i(x) = T_{0,i} + (A_T x)_i$ and upper tension caps remain exact. Store `inf.Tension` on `problem` for `tensionAfter` and `limitSystem`.
2. Verify all solver tests in `internal/solver/solver_test.go` pass, and add a test confirming that uniform tightening on one side does not increase the tension cost term.

---

### 5. Enforce Cross-Spoke Tension Caps During `capTurn` in `internal/solver/solver.go`

**Problem:**
In `internal/solver/solver.go`:
```go
func (pr problem) capTurn(caps, x []float64, k int, q, step float64) float64
```
`capTurn` only checks whether rounding `x[k]` to `q` pushes spoke `k` itself above `caps[k]`. Because of positive `CrossTalk` (same flange crossing partners) and positive opposite-side `NeighborCoupling`, rounding `x[k]` up can also push an already-fixed spoke `i` above `caps[i]`, which then forces `enforceLimits` to alter spoke `i` after optimization without re-balancing runout.

**Fix:**
1. Check whether setting `x[k] = q` causes spoke `k` (`caps[k] > 0 && tension[k] > caps[k]+limitTolKgf`) **or** any already-fixed spoke `i` (`fixed[i] && caps[i] > 0 && tension[i] > caps[i]+limitTolKgf`) to exceed its cap when `q > cont` increases that spoke's tension.
2. Keep `enforceLimits` as the final safety net.
3. Maintain 100% coverage in `internal/solver`.

---

### 6. Improve Inter-Spoke Peak Estimation at the Flagged Spoke Row in `internal/solver/solver.go`

**Problem:**
In `deviationErrors(vals []float64, flags []bool)` (`internal/solver/solver.go`):
```go
prev, next := vals[(i-1+n)%n], vals[(i+1)%n]
out = append(out, (prev+next)/2, v)
```
When `flags[i]` is true, `v = vals[i]` is the peak at position $i + 0.5$. Setting the deviation at spoke $i$ to `(prev + next) / 2` ignores the peak `v` entirely: if a sharp bump sits at $i + 0.5$ while $i - 1$ and $i + 1$ are $0$, spoke $i$ (only $0.5$ spacing away from the peak) is assigned `0` instead of lying on the slope between $i - 1$ and $i + 0.5$. Furthermore, if `flags[i-1]` is also true, `vals[i-1]` is already at position $i - 0.5$ (distance $0.5$ from spoke $i$), whereas `vals[i]` is at $i + 0.5$ (also distance $0.5$ from spoke $i$).

**Fix:**
1. Compute the interpolated value at spoke $i$ using linear interpolation between the immediate left sample `(pos_left, val_left)` and the peak `(i + 0.5, v)`:
   - If `flags[(i-1+n)%n]` is true, the left sample is at $i - 0.5$ with value `prev`, equidistant ($0.5$) from spoke $i$ as $(i + 0.5, v)$, giving `(prev + v) / 2`.
   - If `flags[(i-1+n)%n]` is false, the left sample is at $i - 1$ with value `prev` (distance $1.0$ from $i$) and the right sample is at $i + 0.5$ with value `v` (distance $0.5$ from $i$), giving `(prev + 2*v) / 3`.
2. Update `TestDeviationErrors` in `internal/solver/solver_test.go` and verify `TestSolveInterSpokePeak` passes with improved or equal residual runout.

---

## Verification Checklist

Run all commands below and ensure zero errors and 100% coverage on core packages:

```powershell
gofmt -l .
go vet ./...
golangci-lint run ./...
go test "-coverprofile=coverage.out" ./...
go tool cover "-func=coverage.out"
cd web; node --test --experimental-test-coverage 'test/*.test.mjs'; cd ..
```

