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

func resolveConfigPath(configFlag string) (string, error) {
	if configFlag != "" {
		return configFlag, nil
	}
	if _, err := os.Stat(defaultConfigFile); err == nil {
		return defaultConfigFile, nil
	}
	return "", fmt.Errorf("no config file found. Either create %s in the current directory or pass --config", defaultConfigFile)
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "rbrl",
		Short: "Reading Babe Ruth League schedule generator",
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

	scheduleCmd := &cobra.Command{
		Use:   "schedule",
		Short: "Generate and validate schedules",
	}

	var configFile string
	scheduleCmd.PersistentFlags().StringVar(&configFile, "config", "", "Path to config file (default: config.yaml in current directory)")

	var outputFile string
	generateCmd := &cobra.Command{
		Use:          "generate",
		Short:        "Generate a schedule from a config file",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := resolveConfigPath(configFile)
			if err != nil {
				return err
			}
			return runGenerate(configPath, outputFile)
		},
	}
	generateCmd.Flags().StringVarP(&outputFile, "output", "o", "schedule.xlsx", "Output Excel file path")

	validateCmd := &cobra.Command{
		Use:          "validate <schedule.xlsx>",
		Short:        "Validate a schedule against config rules",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := resolveConfigPath(configFile)
			if err != nil {
				return err
			}
			return runValidate(configPath, args[0])
		},
	}

	scheduleCmd.AddCommand(generateCmd, validateCmd)
	rootCmd.AddCommand(initCmd, scheduleCmd)
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

  # Overflow period: games that can't fit in the regular season may be
  # scheduled between end_date and overflow_end_date as a last resort.
  # The scheduler minimizes overflow usage, preferring fewer and earlier days.
  overflow_end_date: "2026-06-05"

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
    teams: [Angels, Astros, Athletics, Mariners, Royals]
  - name: National
    teams: [Cubs, Padres, Phillies, Pirates, Marlins]

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
  max_3_in_4_days: true            # No team plays 3 games in any 4-day window

# Guidelines are soft constraints. The scheduler tries to honor them but
# violations are reported as warnings, not errors. This allows manual edits
# that intentionally break guidelines when needed.
guidelines:
  min_days_between_same_matchup: 10      # Minimum days before two teams play again
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
	overflowSlots := schedule.GenerateOverflowSlots(cfg)
	blackouts := schedule.GenerateBlackoutSlots(cfg)

	totalSlots := len(slots) + len(overflowSlots)
	if len(overflowSlots) > 0 {
		fmt.Printf("Scheduling %d games into %d available slots (%d regular + %d overflow)...\n",
			len(games), totalSlots, len(slots), len(overflowSlots))
	} else {
		fmt.Printf("Scheduling %d games into %d available slots...\n", len(games), len(slots))
	}

	result, schedErr := schedule.Schedule(cfg, slots, overflowSlots, games)

	if schedErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", schedErr)
		fmt.Fprintf(os.Stderr, "\nGenerating partial schedule...\n")
	} else {
		fmt.Printf("✓ All %d games scheduled\n", len(result.Assignments))
	}

	fmt.Println("\nPer Team Metrics:")
	fmt.Printf("  %-15s %6s %4s %4s\n", "Team", "Games", "Sat", "Sun")
	for _, team := range cfg.AllTeams() {
		m := result.TeamMetrics[team]
		fmt.Printf("  %-15s %6d %4d %4d\n", team, m.Games, m.Saturday, m.Sunday)
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\nGuideline violations (%d):\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	} else {
		fmt.Println("\n✓ No guideline violations")
	}

	allSlots := append(slots, overflowSlots...)
	f, err := excel.Generate(cfg, result, allSlots, blackouts)
	if err != nil {
		return fmt.Errorf("generating Excel: %w", err)
	}

	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("saving file: %w", err)
	}

	fmt.Printf("\n✓ Schedule saved to %s\n", outputPath)
	if schedErr != nil {
		return fmt.Errorf("schedule is incomplete: %d of %d games scheduled", len(result.Assignments), len(games))
	}
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

	// Regenerate team sheets from master schedule
	if err := excel.UpdateTeamSheets(schedulePath, cfg); err != nil {
		return fmt.Errorf("updating team sheets: %w", err)
	}
	fmt.Printf("✓ Team sheets updated in %s\n", schedulePath)

	if errors > 0 {
		return fmt.Errorf("%d constraint violations found", errors)
	}
	return nil
}
