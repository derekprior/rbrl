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

func main() {
	rootCmd := &cobra.Command{
		Use:   "rbrl",
		Short: "Reading Babe Ruth League schedule generator",
	}

	var outputFile string
	generateCmd := &cobra.Command{
		Use:   "generate <config.yaml>",
		Short: "Generate a schedule from a config file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(args[0], outputFile)
		},
	}
	generateCmd.Flags().StringVarP(&outputFile, "output", "o", "schedule.xlsx", "Output Excel file path")

	validateCmd := &cobra.Command{
		Use:   "validate <config.yaml> <schedule.xlsx>",
		Short: "Validate a schedule against config rules",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(args[0], args[1])
		},
	}

	rootCmd.AddCommand(generateCmd, validateCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

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

	if len(result.Warnings) > 0 {
		fmt.Printf("\n⚠ %d warnings:\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("  • %s\n", w)
		}
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
			fmt.Printf("✗ ERROR: %s\n", v.Message)
		case "warning":
			warnings++
			fmt.Printf("⚠ WARNING: %s\n", v.Message)
		}
	}

	fmt.Printf("\nValidation complete: %d errors, %d warnings\n", errors, warnings)

	if errors > 0 {
		return fmt.Errorf("%d constraint violations found", errors)
	}
	return nil
}
