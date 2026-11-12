# Software Requirements Specification (SRS): Wheel Truing Solver

## 1. Project Overview

The goal of this project is to develop a mathematical solver and web application that assists bicycle mechanics in truing wheels. It takes input data (spoke tension, lateral deviation, radial deviation) and outputs a step-by-step algorithm in fractional nipple turns (e.g., +1/4, -1/2) to eliminate runout (wobble and hop) while maintaining optimal tension balance.

## 2. Technology Stack

* **Backend & Math Engine:** Go (Golang).
* **Solver Logic:** Least Squares Optimization or Gradient Descent (e.g., using `gonum` or a custom iterative solver).
* **Database:** SQLite3 (using `github.com/mattn/go-sqlite3`).
* **Frontend:** HTML5, Vanilla JavaScript, CSS (lightweight interface, SVG/Canvas for visualizations).

## 3. Physical Model & Wheel Parameters

The mathematical core (Influence Matrix) relies on physical characteristics to calculate how one spoke's tension affects the rest of the rim.

* **Spoke Count:** 20, 24, 28, 32, 36. Defines the angular step.
* **ERD (Effective Rim Diameter) in mm:** Determines the stiffness and the attenuation coefficient of the deformation across neighboring spokes.
* **Cross Pattern:** 0 (radial), 1, 2, 3, 4. Defines the physical crossing of spokes. The algorithm must account for "cross-talk" (tension transfer) between crossing pulling and pushing spokes on the same flange.
* **Hub Geometry:** Left and Right Flange Distance from the center (mm). Determines the bracing angle, which calculates the vector split between radial (hop) and lateral (wobble) pull.

## 4. Indexing, Direction, & Reference Point

To ensure the mathematical array maps perfectly to the physical wheel, the system must track indexing directions:

* **Start Reference:** Zero point (e.g., "Rim Seam" or "Valve Hole").
* **View Point:** The side the mechanic is looking at (e.g., "Rotor Side" / "Drive Side").
* **Counting Direction:** Clockwise (CW) or Counter-Clockwise (CCW).
* *Requirement:* The UI must display a clear header on the results page reminding the user of their chosen orientation (e.g., *"Start at Rim Seam. View from Rotor side. Count CCW."*).

## 5. Sub-segment Deviation (Inter-spoke Peaks)

Physical rim deformations often peak between spokes rather than exactly at a spoke hole.

* **Data Input:** Users can flag if the peak lateral or radial deviation is located *between* the current spoke (N) and the next spoke (N+1).
* **Math Engine interpolation:** If an error peak is marked as inter-spoke, the solver must generate a virtual interpolation point ($i.5$). The influence of any nipple $j$ on point $i.5$ is approximated as the average of its influence on point $i$ and point $i+1$: $A_{i.5, j} = (A_{i, j} + A_{i+1, j}) / 2$. This allows the optimizer to distribute the pulling force accurately between adjacent spokes.

## 6. Database Schema (SQLite)

**Table: `wheel_setups` (Profiles)**
* `id` (PK, INTEGER)
* `name` (TEXT)
* `spokes_count` (INTEGER)
* `erd_mm` (REAL)
* `cross_pattern` (INTEGER)
* `flange_dist_left_mm` (REAL)
* `flange_dist_right_mm` (REAL)
* `start_reference` (TEXT) - Default: "Rim Seam"
* `view_point` (TEXT) - Default: "Rotor Side"
* `counting_direction` (TEXT) - "CW" or "CCW"

**Table: `sessions` (Truing Sessions)**
* `id` (PK, INTEGER)
* `wheel_id` (FK, INTEGER)
* `created_at` (DATETIME)

**Table: `measurements` (State Array)**
* `id` (PK, INTEGER)
* `session_id` (FK, INTEGER)
* `spoke_index` (INTEGER) - 1 to N
* `side` (TEXT) - 'L' or 'R'
* `tension_kgf` (REAL)
* `lateral_dev_mm` (REAL)
* `lat_peak_between_next` (BOOLEAN)
* `radial_dev_mm` (REAL)
* `rad_peak_between_next` (BOOLEAN)

## 7. API and Backend Logic (`solver.go`)

1. **Influence Matrix (Jacobian) Generator:** Builds a matrix dynamically based on `wheel_setups` parameters. Accounts for attenuation based on ERD and cross-talk based on the Cross Pattern and counting direction.
2. **Cost Function:** $C = W_L(\Delta L)^2 + W_R(\Delta R)^2 + W_T(\Delta T)^2$
3. **Solver Execution:** Runs optimization to minimize the error vector $E$.
4. **Action Discretization:** The raw output vector must be mapped to practical mechanic fractions (e.g., `1/4`, `1/2`, `3/4`, `1` turn) and filtered to ignore micro-adjustments (< 1/8 turn).

## 8. Frontend UI Requirements

1. **Setup Panel:** Form to create/select a wheel profile, including Reference/Direction toggles.
2. **Input Grid:** An $N$-row table for Spoke Index, Side, Tension, Lateral/Radial Dev. Include a checkbox UI for the `peak_between_next` flags. Support Excel copy-paste.
3. **Action Output:** Clean, filtered list of instructions (e.g., "Spoke R14: +1/2 turn (Tighten)") with the orientation reminder.
4. **Visual Vector Model (Wheel Canvas):** A 2D vector graphic (SVG or Canvas) representing the physical wheel. 
    * It must visually highlight problem areas (wobble and hop zones).
    * It should color-code the specific spokes that require adjustment based on the solver's output (e.g., red for loosen, blue for tighten).
5. **Tension Distribution Radar Chart:** A polar/radar chart (similar to the Park Tool Tension App visualizer) to display the tension uniformity. It should map the relative tension of spokes, separated by Drive Side and Non-Drive Side, visually indicating spikes or drops relative to the average tension of that side.

## 9. Development Culture & Code Quality (Mandatory)

The codebase must strictly adhere to professional software engineering standards:

* **Idiomatic Go:** Code must follow effective Go practices. Use `gofmt` for formatting. Project structure should follow standard Go layout (e.g., separating `cmd/`, `internal/solver/`, `internal/api/`, etc.). Avoid global state and ensure proper error handling (no empty `_` ignores for errors).
* **Test Coverage:** The math engine and solver must be heavily unit-tested. 
    * Implement **Table-Driven Tests** for the Influence Matrix generator to verify cross-talk coefficients, interpolation points ($i.5$), and matrix dimensions.
    * Include integration tests for the optimization loop to prove it converges on known mock datasets.
* **Separation of Concerns:** Keep the mathematical solving logic completely decoupled from HTTP handlers and SQLite database layers. The `solver` package should be pure and accept interfaces/structs.
* **Frontend Best Practices:** Vanilla JS is preferred for simplicity, but it must be modular (using ES6 modules). Visualization logic (Canvas/SVG rendering and Radar Chart) should be separated from form handling and state management.

## 10. Development Milestones for Agent

1. Initialize Go project, setup CI/CD linters (golangci-lint), and write CRUD for wheel profiles.
2. Build the vanilla JS grid interface for data entry.
3. Develop the `solver.go` math engine and write comprehensive table-driven tests for it.
4. Implement the optimization algorithm and connect it to the HTTP API.
5. Develop the Frontend Visualizations (Vector wheel model and Tension Radar Chart).