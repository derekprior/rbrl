package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/excel"
	"github.com/derekprior/rbrl/internal/schedule"
	"github.com/derekprior/rbrl/internal/strategy"
	"github.com/derekprior/rbrl/internal/validator"
)

const defaultConfigFile = "config.yaml"

func resolveConfigPath(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if _, err := os.Stat(defaultConfigFile); err == nil {
		return defaultConfigFile, nil
	}
	return "", fmt.Errorf("no config file found. Either create %s in the current directory or pass the path as an argument", defaultConfigFile)
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "rbrl",
		Short: "Reading Babe Ruth League schedule generator",
	}

	var outputFile string
	generateCmd := &cobra.Command{
		Use:          "generate [config.yaml]",
		Short:        "Generate a schedule from a config file",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := resolveConfigPath(args)
			if err != nil {
				return err
			}
			return runGenerate(configPath, outputFile)
		},
	}
	generateCmd.Flags().StringVarP(&outputFile, "output", "o", "schedule.xlsx", "Output Excel file path")

	validateCmd := &cobra.Command{
		Use:          "validate [config.yaml] <schedule.xlsx>",
		Short:        "Validate a schedule against config rules",
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				return runValidate(args[0], args[1])
			}
			configPath, err := resolveConfigPath(nil)
			if err != nil {
				return err
			}
			return runValidate(configPath, args[0])
		},
	}

	var initOutputPath string
	initCmd := &cobra.Command{
		Use:          "init",
		Short:        "Create a starter config.yaml in the current directory",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(initOutputPath)
		},
	}
	initCmd.Flags().StringVarP(&initOutputPath, "output", "o", defaultConfigFile, "Output path for the config file")

	rootCmd.AddCommand(generateCmd, validateCmd, initCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runInit(outputPath string) error {
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("%s already exists; remove it first or use -o to write elsewhere", outputPath)
	}

	if err := os.WriteFile(outputPath, []byte(configTemplate), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("✓ Created %s\n", outputPath)
	return nil
}

const configTemplate = `# RBRL Season Configuration
# ========================
# This file defines the parameters for generating a baseball schedule.

# Season defines the date range for the regular season.
season:
  start_date: "2026-04-25"
  end_date: "2026-05-31"

  # Blackout dates are full days where no games will be scheduled on any field.
  # Common examples: holidays, town events, etc.
  blackout_dates:
    - date: "2026-05-10"
      reason: "Mother's Day"
    - date: "2026-05-23"
      reason: "Memorial Day Weekend"
    - date: "2026-05-24"
      reason: "Memorial Day Weekend"
    - date: "2026-05-25"
      reason: "Memorial Day"

# Divisions and their teams. The number of divisions and teams per division
# can vary. Team names must be unique across all divisions.
divisions:
  - name: American
    teams: [Angels, Astros, Orioles, Mariners, Royals]
  - name: National
    teams: [Cubs, Padres, Phillies, Pirates, Rockies]

# Fields available for scheduling. Each field can have reservations that block
# it for specific dates or date ranges.
#
# Reservations block a field for a given date or date range.
# If 'times' is omitted or empty, the field is blocked for the full day.
# If 'times' is provided, only those specific time slots are blocked.
#
# Single date reservation (full day):
#   - date: "2026-05-04"
#     reason: "Freshman"
#
# Single date, specific times only:
#   - date: "2026-05-04"
#     times: ["17:45"]
#     reason: "Freshman"
#
# Date range reservation (blocks every day in the range):
#   - start_date: "2026-04-25"
#     end_date: "2026-05-31"
#     reason: "Reserved"
fields:
  - name: Moscariello Ballpark
    reservations:
      - start_date: "2026-04-25"
        end_date: "2026-05-31"
        reason: "Reserved"
  - name: Symonds Field
    reservations:
      - date: "2026-05-04"
        reason: "Freshman"
      - date: "2026-05-05"
        reason: "Freshman"
      - date: "2026-05-06"
        reason: "Freshman"
      - date: "2026-05-13"
        reason: "Freshman"
      - date: "2026-05-22"
        reason: "Freshman"
  - name: Washington Park
    reservations:
      - date: "2026-04-29"
        reason: "JV"
      - date: "2026-05-01"
        reason: "JV"
      - date: "2026-05-11"
        reason: "JV"
      - date: "2026-05-12"
        reason: "JV"

# Time slots define when games can be played on each type of day.
# Times use 24-hour format (e.g., "17:45" = 5:45 PM).
time_slots:
  weekday: ["17:45"]
  saturday: ["12:30", "14:45", "17:00"]
  sunday: ["17:00"]

  # Holiday dates are treated as Sundays for scheduling purposes.
  # Use this for holidays that fall on weekdays but have Sunday-style availability.
  holiday_dates:
    - "2026-05-25"

# Strategy determines how matchups are generated.
# "division_weighted" plays each intra-division opponent twice and each
# inter-division opponent once, with balanced home/away assignments.
strategy: division_weighted

# Rules are hard constraints. A schedule that violates these is invalid.
rules:
  max_games_per_day_per_team: 1    # No team plays more than once per day
  max_consecutive_days: 2          # No team plays 3+ days in a row
  max_games_per_week: 3            # Max games per team per calendar week
  max_games_per_timeslot: 2        # Max simultaneous games (limited by umpire crews)

# Guidelines are soft constraints. The scheduler tries to honor them but
# violations are reported as warnings, not errors. This allows manual edits
# that intentionally break guidelines when needed.
guidelines:
  avoid_3_in_4_days: true                # Try not to schedule 3 games in any 4-day window
  min_days_between_same_matchup: 14      # Minimum days before two teams play again
  balance_sunday_games: true             # Spread Sunday games evenly across teams
  balance_pace: true                     # Keep games-played roughly equal across teams
`

func runGenerate(configPath, outputPath string) error {
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	strat, err := strategy.Get(cfg.Strategy)
	if err != nil {
		return err
	}

	games := strat.GenerateMatchups(cfg.Divisions)
	slots := schedule.GenerateSlots(cfg)
	blackouts := schedule.GenerateBlackoutSlots(cfg)

	fmt.Printf("Scheduling %d games into %d available slots...\n", len(games), len(slots))

	result, err := schedule.Schedule(cfg, slots, games)
	if err != nil {
		return fmt.Errorf("scheduling: %w", err)
	}

	fmt.Printf("✓ All %d games scheduled\n", len(result.Assignments))

	fmt.Println("\nPer Team Metrics:")
	fmt.Printf("  %-15s %6s %4s %4s %s\n", "Team", "Games", "Sat", "Sun", "Violations")
	for _, team := range cfg.AllTeams() {
		m := result.TeamMetrics[team]
		fmt.Printf("  %-15s %6d %4d %4d %d\n", team, m.Games, m.Saturday, m.Sunday, len(m.Violations))
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\nGuideline violations (%d):\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	} else {
		fmt.Println("\n✓ No guideline violations")
	}

	f, err := excel.Generate(cfg, result, slots, blackouts)
	if err != nil {
		return fmt.Errorf("generating Excel: %w", err)
	}

	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("saving file: %w", err)
	}

	fmt.Printf("\n✓ Schedule saved to %s\n", outputPath)
	return nil
}

func runValidate(configPath, schedulePath string) error {
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	violations, err := validator.Validate(cfg, schedulePath)
	if err != nil {
		return fmt.Errorf("validating: %w", err)
	}

	errors := 0
	warnings := 0
	for _, v := range violations {
		switch v.Type {
		case "error":
			errors++
			fmt.Printf("✗ Rule violation: %s\n", v.Message)
		case "warning":
			warnings++
			fmt.Printf("⚠ Guideline violation: %s\n", v.Message)
		}
	}

	fmt.Printf("\nValidation complete: %d rule violations, %d guideline violations\n", errors, warnings)

	if errors > 0 {
		return fmt.Errorf("%d constraint violations found", errors)
	}
	return nil
}
