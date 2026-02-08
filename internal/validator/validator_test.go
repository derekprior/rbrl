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
			MaxGamesPerDayPerTeam: 1,
			MaxConsecutiveDays:    2,
			MaxGamesPerWeek:       3,
			MaxGamesPerTimeslot:   2,
		},
		Guidelines: config.Guidelines{
			Avoid3In4Days:             true,
			MinDaysBetweenSameMatchup: 14,
			BalanceSundayGames:        true,
			BalancePace:               true,
		},
	}
}

func TestValidateGeneratedSchedule(t *testing.T) {
	cfg := fullTestConfig()
	slots := schedule.GenerateSlots(cfg)
	blackouts := schedule.GenerateBlackoutSlots(cfg)
	strat := &strategy.DivisionWeighted{}
	games := strat.GenerateMatchups(cfg.Divisions)

	result, err := schedule.Schedule(cfg, slots, nil, games)
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

func d(month, day int) time.Time {
	return time.Date(2026, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func defaultRules() config.Rules {
	return config.Rules{
		MaxGamesPerDayPerTeam: 1,
		MaxConsecutiveDays:    2,
		MaxGamesPerWeek:       3,
		MaxGamesPerTimeslot:   2,
	}
}

func defaultGuidelines() config.Guidelines {
	return config.Guidelines{
		Avoid3In4Days:             true,
		MinDaysBetweenSameMatchup: 14,
		BalanceSundayGames:        true,
		BalancePace:               true,
	}
}

func TestCheckMaxGamesPerDay(t *testing.T) {
	cfg := &config.Config{Rules: defaultRules()}

	t.Run("no violation when teams play once per day", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 1), Home: "Astros", Away: "Padres"},
		}
		v := checkMaxGamesPerDay(cfg, games)
		if len(v) != 0 {
			t.Errorf("expected 0 violations, got %d: %v", len(v), v)
		}
	})

	t.Run("violation when team plays twice in one day", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 1), Home: "Angels", Away: "Padres"},
		}
		v := checkMaxGamesPerDay(cfg, games)
		if len(v) == 0 {
			t.Error("expected violation for Angels playing twice on 5/1")
		}
		for _, vi := range v {
			if vi.Type != "error" {
				t.Errorf("expected error, got %s", vi.Type)
			}
		}
	})
}

func TestCheckConsecutiveDays(t *testing.T) {
	cfg := &config.Config{Rules: defaultRules()}

	t.Run("no violation for 2 consecutive days", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 2), Home: "Angels", Away: "Padres"},
		}
		v := checkConsecutiveDays(cfg, games)
		if len(v) != 0 {
			t.Errorf("expected 0 violations, got %d", len(v))
		}
	})

	t.Run("violation for 3 consecutive days", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 2), Home: "Angels", Away: "Padres"},
			{Row: 4, Date: d(5, 3), Home: "Angels", Away: "Astros"},
		}
		v := checkConsecutiveDays(cfg, games)
		if len(v) == 0 {
			t.Error("expected violation for 3 consecutive days")
		}
	})
}

func TestCheckMaxGamesPerWeek(t *testing.T) {
	cfg := &config.Config{Rules: defaultRules()}

	t.Run("no violation at 3 games in a week", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 4), Home: "Angels", Away: "Cubs"},   // Mon
			{Row: 3, Date: d(5, 6), Home: "Angels", Away: "Padres"}, // Wed
			{Row: 4, Date: d(5, 9), Home: "Angels", Away: "Astros"}, // Sat
		}
		v := checkMaxGamesPerWeek(cfg, games)
		if len(v) != 0 {
			t.Errorf("expected 0 violations, got %d", len(v))
		}
	})

	t.Run("violation at 4 games in a week", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 4), Home: "Angels", Away: "Cubs"},     // Mon
			{Row: 3, Date: d(5, 6), Home: "Angels", Away: "Padres"},   // Wed
			{Row: 4, Date: d(5, 8), Home: "Angels", Away: "Astros"},   // Fri
			{Row: 5, Date: d(5, 9), Home: "Angels", Away: "Mariners"}, // Sat
		}
		v := checkMaxGamesPerWeek(cfg, games)
		if len(v) == 0 {
			t.Error("expected violation for 4 games in one week")
		}
	})
}

func TestCheckMaxGamesPerTimeslot(t *testing.T) {
	cfg := &config.Config{Rules: defaultRules()}

	t.Run("no violation at 2 games in a timeslot", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Time: "17:45", Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 1), Time: "17:45", Home: "Astros", Away: "Padres"},
		}
		v := checkMaxGamesPerTimeslot(cfg, games)
		if len(v) != 0 {
			t.Errorf("expected 0 violations, got %d", len(v))
		}
	})

	t.Run("violation at 3 games in a timeslot", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Time: "17:45", Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 1), Time: "17:45", Home: "Astros", Away: "Padres"},
			{Row: 4, Date: d(5, 1), Time: "17:45", Home: "Orioles", Away: "Royals"},
		}
		v := checkMaxGamesPerTimeslot(cfg, games)
		if len(v) == 0 {
			t.Error("expected violation for 3 games in one timeslot")
		}
	})
}

func TestCheck3In4Days(t *testing.T) {
	cfg := &config.Config{Guidelines: config.Guidelines{Avoid3In4Days: true}}

	t.Run("no warning for 2 games in 4 days", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 4), Home: "Angels", Away: "Padres"},
		}
		v := check3In4Days(cfg, games)
		if len(v) != 0 {
			t.Errorf("expected 0 warnings, got %d", len(v))
		}
	})

	t.Run("warning for 3 games in 4 days", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 2), Home: "Angels", Away: "Padres"},
			{Row: 4, Date: d(5, 4), Home: "Angels", Away: "Astros"},
		}
		v := check3In4Days(cfg, games)
		if len(v) == 0 {
			t.Error("expected warning for 3 games in 4 days")
		}
		if v[0].Type != "warning" {
			t.Errorf("expected warning, got %s", v[0].Type)
		}
	})

	t.Run("skipped when guideline disabled", func(t *testing.T) {
		cfg2 := &config.Config{Guidelines: config.Guidelines{Avoid3In4Days: false}}
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 2), Home: "Angels", Away: "Padres"},
			{Row: 4, Date: d(5, 4), Home: "Angels", Away: "Astros"},
		}
		v := check3In4Days(cfg2, games)
		if len(v) != 0 {
			t.Errorf("expected 0 warnings when disabled, got %d", len(v))
		}
	})
}

func TestCheckRematchProximity(t *testing.T) {
	cfg := &config.Config{Guidelines: config.Guidelines{MinDaysBetweenSameMatchup: 14}}

	t.Run("no warning when rematches are spaced", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(4, 25), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 15), Home: "Cubs", Away: "Angels"},
		}
		v := checkRematchProximity(cfg, games)
		if len(v) != 0 {
			t.Errorf("expected 0 warnings, got %d", len(v))
		}
	})

	t.Run("warning when rematch is too soon", func(t *testing.T) {
		games := []parsedGame{
			{Row: 2, Date: d(5, 1), Home: "Angels", Away: "Cubs"},
			{Row: 3, Date: d(5, 8), Home: "Cubs", Away: "Angels"},
		}
		v := checkRematchProximity(cfg, games)
		if len(v) == 0 {
			t.Error("expected warning for rematch after 7 days")
		}
	})
}
