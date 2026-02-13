package config

import (
	"testing"
	"time"
)

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

const testConfigYAML = `
season:
  start_date: "2026-04-25"
  end_date: "2026-05-31"
  blackout_dates:
    - date: "2026-05-10"
      reason: "Mother's Day"
    - date: "2026-05-25"
      reason: "Memorial Day"

divisions:
  - name: American
    teams: [Angels, Astros, Athletics, Mariners, Royals]
  - name: National
    teams: [Cubs, Padres, Phillies, Pirates, Marlins]

fields:
  - name: Moscariello Ballpark
    reservations:
      - date: "2026-05-15"
        times: ["17:45"]
        reason: "Varsity"
  - name: Symonds Field
  - name: Washington Park

time_slots:
  weekday: ["17:45"]
  saturday: ["12:30", "14:45", "17:00"]
  sunday: ["17:00"]
  holiday_dates:
    - "2026-05-25"

strategy: division_weighted

rules:
  max_games_per_day_per_team: 1
  max_consecutive_days: 2
  max_games_per_week: 3
  max_games_per_timeslot: 2

guidelines:
  min_days_between_same_matchup: 14
  balance_sunday_games: true
  balance_pace: true
`

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadFromBytes([]byte(testConfigYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("season dates", func(t *testing.T) {
		if cfg.Season.StartDate.Time != mustDate("2026-04-25") {
			t.Errorf("start date = %v, want 2026-04-25", cfg.Season.StartDate.Time)
		}
		if cfg.Season.EndDate.Time != mustDate("2026-05-31") {
			t.Errorf("end date = %v, want 2026-05-31", cfg.Season.EndDate.Time)
		}
	})

	t.Run("blackout dates", func(t *testing.T) {
		if len(cfg.Season.BlackoutDates) != 2 {
			t.Fatalf("blackout dates = %d, want 2", len(cfg.Season.BlackoutDates))
		}
		if cfg.Season.BlackoutDates[0].Reason != "Mother's Day" {
			t.Errorf("reason = %q, want %q", cfg.Season.BlackoutDates[0].Reason, "Mother's Day")
		}
	})

	t.Run("divisions", func(t *testing.T) {
		if len(cfg.Divisions) != 2 {
			t.Fatalf("divisions = %d, want 2", len(cfg.Divisions))
		}
		if len(cfg.Divisions[0].Teams) != 5 {
			t.Errorf("American teams = %d, want 5", len(cfg.Divisions[0].Teams))
		}
		if cfg.Divisions[0].Name != "American" {
			t.Errorf("division name = %q, want %q", cfg.Divisions[0].Name, "American")
		}
	})

	t.Run("fields", func(t *testing.T) {
		if len(cfg.Fields) != 3 {
			t.Fatalf("fields = %d, want 3", len(cfg.Fields))
		}
		if len(cfg.Fields[0].Reservations) != 1 {
			t.Fatalf("reservations = %d, want 1", len(cfg.Fields[0].Reservations))
		}
		r := cfg.Fields[0].Reservations[0]
		if r.Reason != "Varsity" {
			t.Errorf("reservation reason = %q, want %q", r.Reason, "Varsity")
		}
		if len(r.Times) != 1 || r.Times[0] != "17:45" {
			t.Errorf("reservation times = %v, want [17:45]", r.Times)
		}
	})

	t.Run("time slots", func(t *testing.T) {
		if len(cfg.TimeSlots.Weekday) != 1 || cfg.TimeSlots.Weekday[0] != "17:45" {
			t.Errorf("weekday slots = %v, want [17:45]", cfg.TimeSlots.Weekday)
		}
		if len(cfg.TimeSlots.Saturday) != 3 {
			t.Errorf("saturday slots = %d, want 3", len(cfg.TimeSlots.Saturday))
		}
		if len(cfg.TimeSlots.Sunday) != 1 || cfg.TimeSlots.Sunday[0] != "17:00" {
			t.Errorf("sunday slots = %v, want [17:00]", cfg.TimeSlots.Sunday)
		}
		if len(cfg.TimeSlots.HolidayDates) != 1 {
			t.Errorf("holiday dates = %d, want 1", len(cfg.TimeSlots.HolidayDates))
		}
	})

	t.Run("strategy", func(t *testing.T) {
		if cfg.Strategy != "division_weighted" {
			t.Errorf("strategy = %q, want %q", cfg.Strategy, "division_weighted")
		}
	})

	t.Run("rules", func(t *testing.T) {
		if cfg.Rules.MaxGamesPerDayPerTeam != 1 {
			t.Errorf("max games/day/team = %d, want 1", cfg.Rules.MaxGamesPerDayPerTeam)
		}
		if cfg.Rules.MaxConsecutiveDays != 2 {
			t.Errorf("max consecutive days = %d, want 2", cfg.Rules.MaxConsecutiveDays)
		}
		if cfg.Rules.MaxGamesPerWeek != 3 {
			t.Errorf("max games/week = %d, want 3", cfg.Rules.MaxGamesPerWeek)
		}
		if cfg.Rules.MaxGamesPerTimeslot != 2 {
			t.Errorf("max games/timeslot = %d, want 2", cfg.Rules.MaxGamesPerTimeslot)
		}
	})

	t.Run("guidelines", func(t *testing.T) {
		if cfg.Guidelines.MinDaysBetweenSameMatchup != 14 {
			t.Errorf("min days between rematch = %d, want 14", cfg.Guidelines.MinDaysBetweenSameMatchup)
		}
		if !cfg.Guidelines.BalanceSundayGames {
			t.Error("balance_sunday_games should be true")
		}
		if !cfg.Guidelines.BalancePace {
			t.Error("balance_pace should be true")
		}
	})
}

func TestLoadConfigValidation(t *testing.T) {
	t.Run("end before start", func(t *testing.T) {
		yaml := `
season:
  start_date: "2026-06-01"
  end_date: "2026-05-01"
divisions:
  - name: A
    teams: [T1, T2]
fields:
  - name: F1
time_slots:
  weekday: ["17:45"]
strategy: division_weighted
rules:
  max_games_per_day_per_team: 1
  max_consecutive_days: 2
  max_games_per_week: 3
  max_games_per_timeslot: 2
`
		_, err := LoadFromBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for end date before start date")
		}
	})

	t.Run("no divisions", func(t *testing.T) {
		yaml := `
season:
  start_date: "2026-04-25"
  end_date: "2026-05-31"
divisions: []
fields:
  - name: F1
time_slots:
  weekday: ["17:45"]
strategy: division_weighted
rules:
  max_games_per_day_per_team: 1
  max_consecutive_days: 2
  max_games_per_week: 3
  max_games_per_timeslot: 2
`
		_, err := LoadFromBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for no divisions")
		}
	})

	t.Run("no fields", func(t *testing.T) {
		yaml := `
season:
  start_date: "2026-04-25"
  end_date: "2026-05-31"
divisions:
  - name: A
    teams: [T1, T2]
fields: []
time_slots:
  weekday: ["17:45"]
strategy: division_weighted
rules:
  max_games_per_day_per_team: 1
  max_consecutive_days: 2
  max_games_per_week: 3
  max_games_per_timeslot: 2
`
		_, err := LoadFromBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for no fields")
		}
	})

	t.Run("duplicate team names", func(t *testing.T) {
		yaml := `
season:
  start_date: "2026-04-25"
  end_date: "2026-05-31"
divisions:
  - name: A
    teams: [Angels, Astros]
  - name: B
    teams: [Angels, Cubs]
fields:
  - name: F1
time_slots:
  weekday: ["17:45"]
strategy: division_weighted
rules:
  max_games_per_day_per_team: 1
  max_consecutive_days: 2
  max_games_per_week: 3
  max_games_per_timeslot: 2
`
		_, err := LoadFromBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for duplicate team name")
		}
	})
}

func TestAllTeams(t *testing.T) {
	cfg, err := LoadFromBytes([]byte(testConfigYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	teams := cfg.AllTeams()
	if len(teams) != 10 {
		t.Errorf("AllTeams() = %d teams, want 10", len(teams))
	}
}
