package validator

import (
	"testing"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/excel"
	"github.com/derekprior/rbrl/internal/schedule"
	"github.com/derekprior/rbrl/internal/strategy"
)

func date(y, m, d int) config.Date {
	return config.Date{Time: time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)}
}

func fullTestConfig() *config.Config {
	return &config.Config{
		Season: config.Season{
			StartDate: date(2026, 4, 25),
			EndDate:   date(2026, 5, 31),
			BlackoutDates: []config.BlackoutDate{
				{Date: date(2026, 5, 10), Reason: "Mother's Day"},
				{Date: date(2026, 5, 23), Reason: "Memorial Day Weekend"},
				{Date: date(2026, 5, 24), Reason: "Memorial Day Weekend"},
				{Date: date(2026, 5, 25), Reason: "Memorial Day"},
			},
		},
		Divisions: []config.Division{
			{Name: "American", Teams: []string{"Angels", "Astros", "Orioles", "Mariners", "Royals"}},
			{Name: "National", Teams: []string{"Cubs", "Padres", "Phillies", "Pirates", "Rockies"}},
		},
		Fields: []config.Field{
			{Name: "Moscariello Ballpark"},
			{Name: "Symonds Field"},
			{Name: "Washington Park"},
		},
		TimeSlots: config.TimeSlots{
			Weekday:  []string{"17:45"},
			Saturday: []string{"12:30", "14:45", "17:00"},
			Sunday:   []string{"17:00"},
			HolidayDates: []config.Date{
				date(2026, 5, 25),
			},
		},
		Strategy: "division_weighted",
		Rules: config.Rules{
			MaxGamesPerDayPerTeam:    1,
			MaxConsecutiveDays:       2,
			MaxGamesPerWeek:          3,
			MaxGamesPerTimeslot:      2,
			Avoid3In4Days:            true,
			MinDaysBetweenSameMatchup: 14,
			BalanceSundayGames:       true,
			BalancePace:              true,
		},
	}
}

func TestValidateGeneratedSchedule(t *testing.T) {
	cfg := fullTestConfig()
	slots := schedule.GenerateSlots(cfg)
	blackouts := schedule.GenerateBlackoutSlots(cfg)
	strat := &strategy.DivisionWeighted{}
	games := strat.GenerateMatchups(cfg.Divisions)

	result, err := schedule.Schedule(cfg, slots, games)
	if err != nil {
		t.Fatalf("Schedule() error: %v", err)
	}

	f, err := excel.Generate(cfg, result, slots, blackouts)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	path := t.TempDir() + "/schedule.xlsx"
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs error: %v", err)
	}

	violations, err := Validate(cfg, path)
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}

	t.Run("no hard constraint violations", func(t *testing.T) {
		for _, v := range violations {
			if v.Type == "error" {
				t.Errorf("hard violation: %s", v.Message)
			}
		}
	})

	t.Run("reports soft constraint warnings", func(t *testing.T) {
		warnings := 0
		for _, v := range violations {
			if v.Type == "warning" {
				warnings++
				t.Logf("WARNING: %s", v.Message)
			}
		}
		t.Logf("Total warnings: %d", warnings)
	})
}
