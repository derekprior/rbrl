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
	StartDate       Date           `yaml:"start_date"`
	EndDate         Date           `yaml:"end_date"`
	OverflowEndDate *Date          `yaml:"overflow_end_date"`
	BlackoutDates   []BlackoutDate `yaml:"blackout_dates"`
}

type Reservation struct {
	Date      *Date    `yaml:"date"`
	StartDate *Date    `yaml:"start_date"`
	EndDate   *Date    `yaml:"end_date"`
	Times     []string `yaml:"times"`
	GameTime  string   `yaml:"game_time"`
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
	ShortName    string        `yaml:"short_name"`
	Address      string        `yaml:"address"`
	Reservations []Reservation `yaml:"reservations"`
}

type Division struct {
	Name  string   `yaml:"name"`
	Teams []string `yaml:"teams"`
}

type TimeSlots struct {
	Weekday        []string `yaml:"weekday"`
	Saturday       []string `yaml:"saturday"`
	Sunday         []string `yaml:"sunday"`
	MakeupWeekday  []string `yaml:"makeup_weekday"`
	MakeupSaturday []string `yaml:"makeup_saturday"`
	MakeupSunday   []string `yaml:"makeup_sunday"`
	HolidayDates   []Date   `yaml:"holiday_dates"`
}

// Rules holds hard-constraint values. Populated from Constraints during
// config load; test code may also build this struct directly.
type Rules struct {
	MaxGamesPerDayPerTeam int
	MaxConsecutiveDays    int
	MaxGamesPerWeek       int
	MaxGamesPerTimeslot   int
	Max3In4Days           bool
}

// Guidelines holds soft-constraint values. Populated from Constraints during
// config load; test code may also build this struct directly.
type Guidelines struct {
	MinDaysBetweenSameMatchup int
	BalanceSundayGames        bool
	BalancePace               bool
}

// Severity controls how a constraint violation is reported.
type Severity string

const (
	SeverityRule      Severity = "rule"
	SeverityGuideline Severity = "guideline"
	SeverityDisabled  Severity = "disabled"
)

// Mode identifies a phase of operation that can carry its own per-constraint
// severity.
type Mode string

const (
	ModePreseason  Mode = "preseason"
	ModeReschedule Mode = "reschedule"
)

// Constraint is one entry in the unified constraints block.
//
// The YAML form is:
//
//	max_3_in_4_days:
//	  value: true
//	  mode:
//	    preseason: rule
//	    reschedule: guideline
//
// `mode` accepts either a single severity string (applied to every mode) or
// a per-mode map. Modes not listed default to SeverityRule.
type Constraint struct {
	// Value is the constraint's threshold (int) or enable flag (bool).
	// Stored as `any` because constraints have heterogeneous value types.
	Value any
	// Mode maps each Mode to its Severity. nil/empty means rule everywhere.
	Mode map[Mode]Severity
}

// constraintRaw mirrors the YAML shape so we can custom-unmarshal Mode.
type constraintRaw struct {
	Value any       `yaml:"value"`
	Mode  yaml.Node `yaml:"mode"`
}

func (c *Constraint) UnmarshalYAML(value *yaml.Node) error {
	var raw constraintRaw
	if err := value.Decode(&raw); err != nil {
		return err
	}
	c.Value = raw.Value

	switch raw.Mode.Kind {
	case 0:
		// `mode:` omitted entirely.
		c.Mode = nil
	case yaml.ScalarNode:
		s := Severity(raw.Mode.Value)
		c.Mode = map[Mode]Severity{
			ModePreseason:  s,
			ModeReschedule: s,
		}
	case yaml.MappingNode:
		m := make(map[Mode]Severity)
		if err := raw.Mode.Decode(&m); err != nil {
			return fmt.Errorf("constraint mode: %w", err)
		}
		c.Mode = m
	default:
		return fmt.Errorf("constraint mode: expected string or map")
	}
	return nil
}

// constraintKind describes the expected value type of a constraint.
type constraintKind int

const (
	kindInt constraintKind = iota
	kindBool
)

// constraintMeta describes one well-known constraint.
type constraintMeta struct {
	kind     constraintKind
	physical bool     // true => severity must be SeverityRule in every mode
	defaultSeverity Severity // severity when not configured (back-compat)
}

// constraintRegistry is the closed set of constraint names accepted in YAML.
var constraintRegistry = map[string]constraintMeta{
	"max_games_per_day_per_team":    {kind: kindInt, physical: true, defaultSeverity: SeverityRule},
	"max_games_per_timeslot":        {kind: kindInt, physical: true, defaultSeverity: SeverityRule},
	"max_consecutive_days":          {kind: kindInt, defaultSeverity: SeverityRule},
	"max_games_per_week":            {kind: kindInt, defaultSeverity: SeverityRule},
	"max_3_in_4_days":               {kind: kindBool, defaultSeverity: SeverityRule},
	"min_days_between_same_matchup": {kind: kindInt, defaultSeverity: SeverityGuideline},
	"balance_sunday_games":          {kind: kindBool, defaultSeverity: SeverityGuideline},
	"balance_pace":                  {kind: kindBool, defaultSeverity: SeverityGuideline},
}

type GameChanger struct {
	TeamNames map[string]string `yaml:"team_names"`
}

type Config struct {
	Season      Season                `yaml:"season"`
	Divisions   []Division            `yaml:"divisions"`
	Fields      []Field               `yaml:"fields"`
	TimeSlots   TimeSlots             `yaml:"time_slots"`
	Strategy    string                `yaml:"strategy"`
	Constraints map[string]Constraint `yaml:"constraints"`
	GameChanger GameChanger           `yaml:"gamechanger"`

	// Rules and Guidelines are derived from Constraints during validation
	// (or set directly by test code). Reads of these fields throughout the
	// codebase remain unchanged.
	Rules      Rules      `yaml:"-"`
	Guidelines Guidelines `yaml:"-"`
}

// Severity returns the configured severity for the named constraint in the
// given mode. If the constraint has no per-mode entry, falls back to the
// constraint's registry default (or SeverityRule for unknown names).
func (c *Config) Severity(name string, mode Mode) Severity {
	if con, ok := c.Constraints[name]; ok {
		if s, ok := con.Mode[mode]; ok {
			return s
		}
	}
	if meta, ok := constraintRegistry[name]; ok {
		return meta.defaultSeverity
	}
	return SeverityRule
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

	if c.Season.OverflowEndDate != nil && !c.Season.OverflowEndDate.Time.After(c.Season.EndDate.Time) {
		return fmt.Errorf("overflow_end_date %s must be after end_date %s",
			c.Season.OverflowEndDate.Time.Format("2006-01-02"),
			c.Season.EndDate.Time.Format("2006-01-02"))
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
			if r.GameTime != "" && len(r.Times) > 0 {
				return fmt.Errorf("field %q: reservation cannot have both 'game_time' and 'times'", f.Name)
			}
			if r.GameTime != "" {
				if _, err := time.Parse("15:04", r.GameTime); err != nil {
					return fmt.Errorf("field %q: invalid game_time %q (expected HH:MM format)", f.Name, r.GameTime)
				}
			}
		}
	}

	// Validate field short_names: must be unique and must not collide with
	// any other field's full Name.
	shortByName := make(map[string]string)
	for _, f := range c.Fields {
		if f.ShortName == "" {
			continue
		}
		if prev, ok := shortByName[f.ShortName]; ok {
			return fmt.Errorf("field %q: short_name %q already used by field %q", f.Name, f.ShortName, prev)
		}
		shortByName[f.ShortName] = f.Name
		for _, other := range c.Fields {
			if other.Name == f.Name {
				continue
			}
			if other.Name == f.ShortName {
				return fmt.Errorf("field %q: short_name %q collides with another field's name", f.Name, f.ShortName)
			}
		}
	}

	// Validate makeup timeslots: HH:MM format and not duplicating a
	// regular slot for the same day-of-week.
	checkMakeup := func(day string, regular, makeup []string) error {
		regSet := make(map[string]bool, len(regular))
		for _, t := range regular {
			regSet[t] = true
		}
		seen := make(map[string]bool, len(makeup))
		for _, t := range makeup {
			if _, err := time.Parse("15:04", t); err != nil {
				return fmt.Errorf("time_slots.makeup_%s: invalid time %q (expected HH:MM)", day, t)
			}
			if regSet[t] {
				return fmt.Errorf("time_slots.makeup_%s: %q already listed in regular %s slots", day, t, day)
			}
			if seen[t] {
				return fmt.Errorf("time_slots.makeup_%s: duplicate time %q", day, t)
			}
			seen[t] = true
		}
		return nil
	}
	if err := checkMakeup("weekday", c.TimeSlots.Weekday, c.TimeSlots.MakeupWeekday); err != nil {
		return err
	}
	if err := checkMakeup("saturday", c.TimeSlots.Saturday, c.TimeSlots.MakeupSaturday); err != nil {
		return err
	}
	if err := checkMakeup("sunday", c.TimeSlots.Sunday, c.TimeSlots.MakeupSunday); err != nil {
		return err
	}

	// Default GameChanger team names to the config team name when not set
	if c.GameChanger.TeamNames == nil {
		c.GameChanger.TeamNames = make(map[string]string)
	}
	for _, div := range c.Divisions {
		for _, team := range div.Teams {
			if _, ok := c.GameChanger.TeamNames[team]; !ok {
				c.GameChanger.TeamNames[team] = team
			}
		}
	}

	if err := c.validateConstraints(); err != nil {
		return err
	}

	return nil
}

// validateConstraints checks the constraints block and populates the legacy
// Rules / Guidelines structs from it.
func (c *Config) validateConstraints() error {
	if len(c.Constraints) == 0 {
		// No constraints block — leave Rules/Guidelines untouched (test
		// code may have set them directly).
		return nil
	}

	knownModes := map[Mode]bool{
		ModePreseason:  true,
		ModeReschedule: true,
	}
	knownSeverities := map[Severity]bool{
		SeverityRule:      true,
		SeverityGuideline: true,
		SeverityDisabled:  true,
	}

	intVal := func(name string, v any) (int, error) {
		switch x := v.(type) {
		case int:
			return x, nil
		case int64:
			return int(x), nil
		case float64:
			return int(x), nil
		default:
			return 0, fmt.Errorf("constraints.%s: value must be an integer", name)
		}
	}
	boolVal := func(name string, v any) (bool, error) {
		b, ok := v.(bool)
		if !ok {
			return false, fmt.Errorf("constraints.%s: value must be a boolean", name)
		}
		return b, nil
	}

	for name, con := range c.Constraints {
		meta, ok := constraintRegistry[name]
		if !ok {
			return fmt.Errorf("constraints: unknown constraint %q", name)
		}
		if con.Value == nil {
			return fmt.Errorf("constraints.%s: missing value", name)
		}
		switch meta.kind {
		case kindInt:
			if _, err := intVal(name, con.Value); err != nil {
				return err
			}
		case kindBool:
			if _, err := boolVal(name, con.Value); err != nil {
				return err
			}
		}
		for m, s := range con.Mode {
			if !knownModes[m] {
				return fmt.Errorf("constraints.%s: unknown mode %q", name, m)
			}
			if !knownSeverities[s] {
				return fmt.Errorf("constraints.%s.mode.%s: unknown severity %q", name, m, s)
			}
			if meta.physical && s != SeverityRule {
				return fmt.Errorf("constraints.%s: physical constraint must be %q in every mode (got %q in %q)",
					name, SeverityRule, s, m)
			}
		}
	}

	// Populate legacy Rules / Guidelines from constraints. Missing entries
	// leave the corresponding zero value in place.
	get := func(name string) (any, bool) {
		con, ok := c.Constraints[name]
		if !ok {
			return nil, false
		}
		return con.Value, true
	}
	if v, ok := get("max_games_per_day_per_team"); ok {
		c.Rules.MaxGamesPerDayPerTeam, _ = intVal("max_games_per_day_per_team", v)
	}
	if v, ok := get("max_consecutive_days"); ok {
		c.Rules.MaxConsecutiveDays, _ = intVal("max_consecutive_days", v)
	}
	if v, ok := get("max_games_per_week"); ok {
		c.Rules.MaxGamesPerWeek, _ = intVal("max_games_per_week", v)
	}
	if v, ok := get("max_games_per_timeslot"); ok {
		c.Rules.MaxGamesPerTimeslot, _ = intVal("max_games_per_timeslot", v)
	}
	if v, ok := get("max_3_in_4_days"); ok {
		c.Rules.Max3In4Days, _ = boolVal("max_3_in_4_days", v)
	}
	if v, ok := get("min_days_between_same_matchup"); ok {
		c.Guidelines.MinDaysBetweenSameMatchup, _ = intVal("min_days_between_same_matchup", v)
	}
	if v, ok := get("balance_sunday_games"); ok {
		c.Guidelines.BalanceSundayGames, _ = boolVal("balance_sunday_games", v)
	}
	if v, ok := get("balance_pace"); ok {
		c.Guidelines.BalancePace, _ = boolVal("balance_pace", v)
	}

	return nil
}
