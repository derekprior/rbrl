# Copilot Instructions for RBRL

## Project Overview

This is a Go CLI application for generating and validating baseball schedules
for the Reading Babe Ruth League (RBRL). It reads a YAML config file and
produces an Excel workbook with a master schedule and per-team sheets.

## Architecture

- **`cmd/rbrl/`** — CLI entry point using cobra. Two commands: `generate` and `validate`.
- **`internal/config/`** — YAML config parsing. Custom `Date` type for YAML unmarshaling.
- **`internal/strategy/`** — Pluggable matchup generation via the `Strategy` interface. Currently implements `DivisionWeighted` (intra-division 2x, inter-division 1x).
- **`internal/schedule/`** — Two key pieces:
  - `slots.go` — Generates available timeslots from config (date × field × time, minus blackouts/reservations)
  - `scheduler.go` — Constraint-based scheduler that assigns games to slots. Uses greedy assignment with scoring heuristics and multiple random attempts to find the best solution.
- **`internal/excel/`** — Generates Excel workbook using excelize. Master sheet shows all slots (games, blackouts, open). Per-team sheets show filtered view.
- **`internal/validator/`** — Reads an Excel schedule back and checks all hard/soft constraints, reporting violations.

## Scheduling Constraints

Hard constraints (must satisfy):
- Max 1 game per day per team
- Max 2 consecutive days playing
- Max 3 games per week per team
- Max 2 games per timeslot (umpire limit)

Soft constraints (preferred, warned if violated):
- Avoid 3 games in 4 days
- Avoid rematches within 14 days
- Balance Sunday games across teams
- Balance pace of play across teams

## Development Conventions

- **TDD**: Tests are written before implementation. All packages have test files.
- **Table-driven tests**: Use Go subtests (`t.Run`) for organized test cases.
- **No external test frameworks**: Use stdlib `testing` package only.
- **Config-driven**: All season parameters come from `config.yaml`. No hardcoded values.
- **Compiler warnings as errors**: CI runs `go vet` and treats warnings as failures.

## Excel Generation

Team sheets are statically generated with values computed from the master
schedule (not formulas). This avoids excelize limitations with dynamic array
formulas (LET, FILTER, HSTACK don't serialize correctly for spilling).

- **`generate`** writes both the master schedule and team sheets.
- **`validate`** re-reads the master schedule and regenerates team sheets,
  so manual edits to the master sheet are reflected without re-generating.
- **Build with `make`**: Use `make build` (not `go build` directly) to
  ensure `go vet` runs first.

## Key Types

- `config.Config` — Top-level config struct parsed from YAML
- `config.Date` — Custom date type wrapping `time.Time` with YAML support
- `strategy.Game` — A matchup with Home, Away, and Label
- `schedule.Slot` — An available (date, time, field) tuple
- `schedule.Assignment` — A Game assigned to a Slot
- `schedule.Result` — All assignments plus warnings
- `validator.Violation` — A constraint violation with type ("error"/"warning") and message
