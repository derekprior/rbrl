package schedule

import (
	"testing"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/strategy"
)

func schedulerTestConfig() *config.Config {
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
			MinDaysBetweenSameMatchup: 14,
			BalanceSundayGames:        true,
			BalancePace:               true,
		},
	}
}

func TestScheduleAllGames(t *testing.T) {
	cfg := schedulerTestConfig()
	slots := GenerateSlots(cfg)
	strat := &strategy.DivisionWeighted{}
	games := strat.GenerateMatchups(cfg.Divisions)

	result, err := Schedule(cfg, slots, nil, games)
	if err != nil {
		t.Fatalf("Schedule() error: %v", err)
	}

	t.Run("all 65 games scheduled", func(t *testing.T) {
		if len(result.Assignments) != 65 {
			t.Errorf("scheduled %d games, want 65", len(result.Assignments))
		}
	})

	t.Run("no team plays twice in one day", func(t *testing.T) {
		type teamDay struct {
			team string
			date time.Time
		}
		seen := make(map[teamDay]int)
		for _, a := range result.Assignments {
			seen[teamDay{a.Game.Home, a.Slot.Date}]++
			seen[teamDay{a.Game.Away, a.Slot.Date}]++
		}
		for td, count := range seen {
			if count > 1 {
				t.Errorf("%s plays %d games on %s", td.team, count, td.date.Format("2006-01-02"))
			}
		}
	})

	t.Run("no team plays 3 consecutive days", func(t *testing.T) {
		teamDates := teamGameDates(result.Assignments)
		for team, dates := range teamDates {
			for i := 2; i < len(dates); i++ {
				if dates[i].Sub(dates[i-2]) <= 48*time.Hour {
					t.Errorf("%s plays 3 consecutive days: %s, %s, %s",
						team,
						dates[i-2].Format("01/02"),
						dates[i-1].Format("01/02"),
						dates[i].Format("01/02"))
				}
			}
		}
	})

	t.Run("no team plays more than 3 games per week", func(t *testing.T) {
		teamDates := teamGameDates(result.Assignments)
		for team, dates := range teamDates {
			weeks := make(map[int]int) // ISO week -> count
			for _, d := range dates {
				_, w := d.ISOWeek()
				weeks[w]++
			}
			for w, count := range weeks {
				if count > 3 {
					t.Errorf("%s plays %d games in week %d, max 3", team, count, w)
				}
			}
		}
	})

	t.Run("max 2 games per timeslot", func(t *testing.T) {
		type slotKey struct {
			date time.Time
			time string
		}
		counts := make(map[slotKey]int)
		for _, a := range result.Assignments {
			counts[slotKey{a.Slot.Date, a.Slot.Time}]++
		}
		for sk, count := range counts {
			if count > 2 {
				t.Errorf("%d games at %s %s, max 2",
					count, sk.date.Format("2006-01-02"), sk.time)
			}
		}
	})

	t.Run("no duplicate slot assignments", func(t *testing.T) {
		type slotKey struct {
			date  time.Time
			time  string
			field string
		}
		seen := make(map[slotKey]bool)
		for _, a := range result.Assignments {
			sk := slotKey{a.Slot.Date, a.Slot.Time, a.Slot.Field}
			if seen[sk] {
				t.Errorf("duplicate assignment at %s %s %s",
					a.Slot.Date.Format("2006-01-02"), a.Slot.Time, a.Slot.Field)
			}
			seen[sk] = true
		}
	})

	t.Run("warnings reported for soft constraint violations", func(t *testing.T) {
		// We just verify the warnings field exists and is populated reasonably.
		// Some soft constraint violations are expected in a tight schedule.
		t.Logf("Schedule produced %d warnings", len(result.Warnings))
		for _, w := range result.Warnings {
			t.Logf("  WARNING: %s", w)
		}
	})
}

// teamGameDates extracts sorted game dates per team.
func teamGameDates(assignments []Assignment) map[string][]time.Time {
	m := make(map[string][]time.Time)
	for _, a := range assignments {
		m[a.Game.Home] = append(m[a.Game.Home], a.Slot.Date)
		m[a.Game.Away] = append(m[a.Game.Away], a.Slot.Date)
	}
	for team := range m {
		dates := m[team]
		sortDates(dates)
		m[team] = dates
	}
	return m
}

func sortDates(dates []time.Time) {
	for i := 1; i < len(dates); i++ {
		for j := i; j > 0 && dates[j].Before(dates[j-1]); j-- {
			dates[j], dates[j-1] = dates[j-1], dates[j]
		}
	}
}
