package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Date is a wrapper around time.Time for YAML date parsing.
type Date struct {
	Time time.Time
}

func (d *Date) UnmarshalYAML(value *yaml.Node) error {
	t, err := time.Parse("2006-01-02", value.Value)
	if err != nil {
		return fmt.Errorf("invalid date %q: %w", value.Value, err)
	}
	d.Time = t
	return nil
}

type BlackoutDate struct {
	Date   Date   `yaml:"date"`
	Reason string `yaml:"reason"`
}

type Season struct {
	StartDate     Date           `yaml:"start_date"`
	EndDate       Date           `yaml:"end_date"`
	BlackoutDates []BlackoutDate `yaml:"blackout_dates"`
}

type Reservation struct {
	Date      *Date    `yaml:"date"`
	StartDate *Date    `yaml:"start_date"`
	EndDate   *Date    `yaml:"end_date"`
	Times     []string `yaml:"times"`
	Reason    string   `yaml:"reason"`
}

// Dates returns all dates covered by this reservation.
// Supports single date (date:) or range (start_date:/end_date:).
func (r *Reservation) Dates() []time.Time {
	if r.StartDate != nil && r.EndDate != nil {
		var dates []time.Time
		d := r.StartDate.Time
		for !d.After(r.EndDate.Time) {
			dates = append(dates, d)
			d = d.AddDate(0, 0, 1)
		}
		return dates
	}
	if r.Date != nil {
		return []time.Time{r.Date.Time}
	}
	return nil
}

type Field struct {
	Name         string        `yaml:"name"`
	Reservations []Reservation `yaml:"reservations"`
}

type Division struct {
	Name  string   `yaml:"name"`
	Teams []string `yaml:"teams"`
}

type TimeSlots struct {
	Weekday      []string `yaml:"weekday"`
	Saturday     []string `yaml:"saturday"`
	Sunday       []string `yaml:"sunday"`
	HolidayDates []Date   `yaml:"holiday_dates"`
}

type Rules struct {
	MaxGamesPerDayPerTeam int `yaml:"max_games_per_day_per_team"`
	MaxConsecutiveDays    int `yaml:"max_consecutive_days"`
	MaxGamesPerWeek       int `yaml:"max_games_per_week"`
	MaxGamesPerTimeslot   int `yaml:"max_games_per_timeslot"`
}

type Guidelines struct {
	Avoid3In4Days             bool `yaml:"avoid_3_in_4_days"`
	MinDaysBetweenSameMatchup int  `yaml:"min_days_between_same_matchup"`
	BalanceSundayGames        bool `yaml:"balance_sunday_games"`
	BalancePace               bool `yaml:"balance_pace"`
}

type Config struct {
	Season     Season     `yaml:"season"`
	Divisions  []Division `yaml:"divisions"`
	Fields     []Field    `yaml:"fields"`
	TimeSlots  TimeSlots  `yaml:"time_slots"`
	Strategy   string     `yaml:"strategy"`
	Rules      Rules      `yaml:"rules"`
	Guidelines Guidelines `yaml:"guidelines"`
}

// AllTeams returns all team names across all divisions.
func (c *Config) AllTeams() []string {
	var teams []string
	for _, d := range c.Divisions {
		teams = append(teams, d.Teams...)
	}
	return teams
}

// LoadFromBytes parses YAML bytes into a Config and validates it.
func LoadFromBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadFromFile reads and parses a YAML config file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return LoadFromBytes(data)
}

func (c *Config) validate() error {
	if !c.Season.EndDate.Time.After(c.Season.StartDate.Time) {
		return fmt.Errorf("end date %s must be after start date %s",
			c.Season.EndDate.Time.Format("2006-01-02"),
			c.Season.StartDate.Time.Format("2006-01-02"))
	}

	if len(c.Divisions) == 0 {
		return fmt.Errorf("at least one division is required")
	}

	if len(c.Fields) == 0 {
		return fmt.Errorf("at least one field is required")
	}

	// Check for duplicate team names
	seen := make(map[string]string)
	for _, div := range c.Divisions {
		if len(div.Teams) == 0 {
			return fmt.Errorf("division %q has no teams", div.Name)
		}
		for _, team := range div.Teams {
			if prevDiv, ok := seen[team]; ok {
				return fmt.Errorf("team %q appears in both %q and %q divisions", team, prevDiv, div.Name)
			}
			seen[team] = div.Name
		}
	}

	// Validate reservations
	for _, f := range c.Fields {
		for _, r := range f.Reservations {
			hasDate := r.Date != nil
			hasRange := r.StartDate != nil || r.EndDate != nil
			if !hasDate && !hasRange {
				return fmt.Errorf("field %q: reservation must have either 'date' or 'start_date'/'end_date'", f.Name)
			}
			if hasDate && hasRange {
				return fmt.Errorf("field %q: reservation cannot have both 'date' and 'start_date'/'end_date'", f.Name)
			}
			if hasRange && (r.StartDate == nil || r.EndDate == nil) {
				return fmt.Errorf("field %q: reservation with date range must have both 'start_date' and 'end_date'", f.Name)
			}
			if hasRange && !r.EndDate.Time.After(r.StartDate.Time) && r.EndDate.Time != r.StartDate.Time {
				return fmt.Errorf("field %q: reservation end_date must be on or after start_date", f.Name)
			}
		}
	}

	return nil
}
