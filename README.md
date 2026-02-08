# RBRL — Reading Babe Ruth League Schedule Generator

A command-line tool for generating and validating baseball schedules for the
Reading Babe Ruth League. Compiles to a single binary with zero runtime
dependencies.

## Features

- **Auto-generates** a complete season schedule respecting all league rules
- **Outputs Excel workbook** with a master schedule and per-team sheets
- **Validates** manually-edited schedules and reports constraint violations
- **Pluggable scheduling strategies** (currently: division-weighted)
- **Configurable** via a single YAML file — teams, fields, dates, blackouts, rules

## Installation

Requires [Go](https://go.dev/dl/) 1.21+.

```sh
go build -o rbrl ./cmd/rbrl/
```

Or install directly:

```sh
go install github.com/derekprior/rbrl/cmd/rbrl@latest
```

## Usage

### Generate a schedule

```sh
rbrl generate config.yaml -o schedule.xlsx
```

This reads the config, generates all matchups, assigns them to available
timeslots respecting constraints, and writes an Excel workbook.

### Validate a schedule

After manually editing the Excel file (e.g., rescheduling rainouts), validate it:

```sh
rbrl validate config.yaml schedule.xlsx
```

This reads the master sheet back and checks all constraints, reporting errors
(hard constraint violations) and warnings (soft constraint violations).

## Configuration

All season parameters are defined in a YAML config file. See
[`config.yaml`](config.yaml) for a complete example.

### Key sections

- **season** — Start/end dates and league-wide blackout dates (e.g., Mother's
  Day, Memorial Day Weekend)
- **divisions** — Division names and team lists
- **fields** — Field names and per-field reservations (date, time, reason) for
  conflicts like high school baseball
- **time_slots** — Game times by day type (weekday, Saturday, Sunday) and
  holiday dates treated as Sundays
- **strategy** — Scheduling strategy name (`division_weighted`: intra-division
  2x, inter-division 1x)
- **rules** — Constraint configuration

### Rules

**Hard constraints** (never violated):
- `max_games_per_day_per_team` — No team plays more than N games in a day
- `max_consecutive_days` — No team plays more than N consecutive days
- `max_games_per_week` — No team plays more than N games per ISO week
- `max_games_per_timeslot` — Max simultaneous games (umpire availability)
- `max_3_in_4_days` — No team plays 3 games in any 4-day window

**Soft constraints** (preferred; violations reported as warnings):
- `min_days_between_same_matchup` — Prefer spacing out rematches
- `balance_sunday_games` — Spread Sunday games evenly across teams
- `balance_pace` — Keep teams roughly even in games played throughout the season

## Excel Output

### Master Schedule sheet

Every timeslot in the season appears as a row:
- **Scheduled games** show Home, Away, and Game label
- **Blacked-out slots** are greyed out with the reason (e.g., "Mother's Day",
  "Varsity")
- **Open slots** are empty — available for makeup scheduling

### Per-team sheets

Each team gets its own sheet showing just their games, sorted by date. Useful
for distributing to coaches for review.

## Development

```sh
# Run all tests
go test ./...

# Build
go build -o rbrl ./cmd/rbrl/

# Generate a schedule
./rbrl generate config.yaml -o schedule.xlsx
```

## Project Structure

```
cmd/rbrl/           CLI entry point (cobra commands)
internal/
  config/           YAML config parsing and validation
  strategy/         Pluggable matchup generation
  schedule/         Timeslot generation and constraint-based scheduler
  excel/            Excel workbook generation
  validator/        Schedule validation (reads Excel, checks rules)
```

## License

MIT
