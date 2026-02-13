package validator

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/xuri/excelize/v2"
)

// Violation represents a constraint violation found during validation.
type Violation struct {
	Row     int
	Type    string // "error" or "warning"
	Message string
	Days    int // for rematch violations: days between games (0 = not applicable)
}

// Validate reads a schedule Excel file and checks it against the config rules.
func Validate(cfg *config.Config, path string) ([]Violation, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	assignments, err := readAssignments(f)
	if err != nil {
		return nil, fmt.Errorf("reading assignments: %w", err)
	}

	var violations []Violation

	// Check hard constraints
	violations = append(violations, checkMaxGamesPerDay(cfg, assignments)...)
	violations = append(violations, checkConsecutiveDays(cfg, assignments)...)
	violations = append(violations, checkMaxGamesPerWeek(cfg, assignments)...)
	violations = append(violations, checkMaxGamesPerTimeslot(cfg, assignments)...)

	// Check soft constraints
	violations = append(violations, checkRematchProximity(cfg, assignments)...)
	violations = append(violations, check3In4Days(cfg, assignments)...)
	violations = append(violations, checkSundayBalance(cfg, assignments)...)

	// Check overflow usage
	violations = append(violations, checkOverflowUsage(cfg, f, assignments)...)

	// Check game completeness
	violations = append(violations, checkGameCompleteness(cfg, assignments)...)

	return violations, nil
}

type parsedGame struct {
	Row   int
	Date  time.Time
	Time  string
	Field string
	Home  string
	Away  string
}

func readAssignments(f *excelize.File) ([]parsedGame, error) {
	rows, err := f.GetRows("Master Schedule")
	if err != nil {
		return nil, fmt.Errorf("reading Master Schedule: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("Master Schedule is empty")
	}

	// Header row determines field columns (index 3+)
	header := rows[0]
	type fieldCol struct {
		index int
		name  string
	}
	var fieldCols []fieldCol
	for i := 3; i < len(header); i++ {
		fieldCols = append(fieldCols, fieldCol{i, header[i]})
	}

	var games []parsedGame
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 3 || row[0] == "" {
			continue
		}

		date, err := time.Parse("01/02/2006", row[0])
		if err != nil {
			continue
		}
		timeStr := row[2]

		for _, fc := range fieldCols {
			if fc.index >= len(row) || row[fc.index] == "" {
				continue
			}
			cell := row[fc.index]
			away, home, ok := parseGameCell(cell)
			if !ok {
				continue // blackout/reservation text, not a game
			}
			games = append(games, parsedGame{
				Row:   i + 1,
				Date:  date,
				Time:  timeStr,
				Field: fc.name,
				Home:  home,
				Away:  away,
			})
		}
	}

	return games, nil
}

// parseGameCell parses "Away @ Home" and returns (away, home, true).
// Returns ("", "", false) if the cell doesn't match the game format.
func parseGameCell(cell string) (away, home string, ok bool) {
	for i := 0; i < len(cell)-2; i++ {
		if cell[i] == ' ' && cell[i+1] == '@' && cell[i+2] == ' ' {
			return cell[:i], cell[i+3:], true
		}
	}
	return "", "", false
}

func checkMaxGamesPerDay(cfg *config.Config, games []parsedGame) []Violation {
	type teamDay struct {
		team string
		date time.Time
	}
	counts := make(map[teamDay][]int)
	for _, g := range games {
		counts[teamDay{g.Home, g.Date}] = append(counts[teamDay{g.Home, g.Date}], g.Row)
		counts[teamDay{g.Away, g.Date}] = append(counts[teamDay{g.Away, g.Date}], g.Row)
	}

	var violations []Violation
	for td, rows := range counts {
		if len(rows) > cfg.Rules.MaxGamesPerDayPerTeam {
			violations = append(violations, Violation{
				Row:     rows[1],
				Type:    "error",
				Message: fmt.Sprintf("%s plays %d games on %s (max %d)", td.team, len(rows), td.date.Format("01/02"), cfg.Rules.MaxGamesPerDayPerTeam),
			})
		}
	}
	return violations
}

func checkConsecutiveDays(cfg *config.Config, games []parsedGame) []Violation {
	teamDates := buildTeamDates(games)
	var violations []Violation

	for team, dates := range teamDates {
		consecutive := 1
		for i := 1; i < len(dates); i++ {
			if dates[i].Sub(dates[i-1]) == 24*time.Hour {
				consecutive++
				if consecutive > cfg.Rules.MaxConsecutiveDays {
					violations = append(violations, Violation{
						Type: "error",
						Message: fmt.Sprintf("%s plays %d consecutive days ending %s",
							team, consecutive, dates[i].Format("01/02")),
					})
				}
			} else {
				consecutive = 1
			}
		}
	}
	return violations
}

func checkMaxGamesPerWeek(cfg *config.Config, games []parsedGame) []Violation {
	teamDates := buildTeamDates(games)
	var violations []Violation

	for team, dates := range teamDates {
		weeks := make(map[int]int)
		for _, d := range dates {
			_, w := d.ISOWeek()
			weeks[w]++
		}
		for w, count := range weeks {
			if count > cfg.Rules.MaxGamesPerWeek {
				violations = append(violations, Violation{
					Type:    "error",
					Message: fmt.Sprintf("%s plays %d games in week %d (max %d)", team, count, w, cfg.Rules.MaxGamesPerWeek),
				})
			}
		}
	}
	return violations
}

func checkMaxGamesPerTimeslot(cfg *config.Config, games []parsedGame) []Violation {
	type slotKey struct {
		date time.Time
		time string
	}
	counts := make(map[slotKey]int)
	for _, g := range games {
		counts[slotKey{g.Date, g.Time}]++
	}

	var violations []Violation
	for sk, count := range counts {
		if count > cfg.Rules.MaxGamesPerTimeslot {
			violations = append(violations, Violation{
				Type:    "error",
				Message: fmt.Sprintf("%d games at %s %s (max %d)", count, sk.date.Format("01/02"), sk.time, cfg.Rules.MaxGamesPerTimeslot),
			})
		}
	}
	return violations
}

func checkRematchProximity(cfg *config.Config, games []parsedGame) []Violation {
	if cfg.Guidelines.MinDaysBetweenSameMatchup <= 0 {
		return nil
	}

	type matchup struct{ a, b string }
	matchDates := make(map[matchup][]time.Time)
	for _, g := range games {
		a, b := g.Home, g.Away
		if a > b {
			a, b = b, a
		}
		matchDates[matchup{a, b}] = append(matchDates[matchup{a, b}], g.Date)
	}

	var violations []Violation
	for mk, dates := range matchDates {
		sortDates(dates)
		for i := 1; i < len(dates); i++ {
			days := int(dates[i].Sub(dates[i-1]).Hours() / 24)
			if days < cfg.Guidelines.MinDaysBetweenSameMatchup {
				violations = append(violations, Violation{
					Type: "warning",
					Days: days,
					Message: fmt.Sprintf("%s vs %s rematch after %d days (min %d): %s and %s",
						mk.a, mk.b, days, cfg.Guidelines.MinDaysBetweenSameMatchup,
						dates[i-1].Format("01/02"), dates[i].Format("01/02")),
				})
			}
		}
	}
	// Sort by severity: fewest days (worst) first
	sort.Slice(violations, func(i, j int) bool {
		return violations[i].Days < violations[j].Days
	})
	return violations
}

func check3In4Days(cfg *config.Config, games []parsedGame) []Violation {
	if !cfg.Rules.Max3In4Days {
		return nil
	}

	teamDates := buildTeamDates(games)
	var violations []Violation

	for team, dates := range teamDates {
		for i := 2; i < len(dates); i++ {
			if dates[i].Sub(dates[i-2]).Hours()/24 <= 3 {
				violations = append(violations, Violation{
					Type: "error",
					Message: fmt.Sprintf("%s plays 3 games in 4 days: %s, %s, %s",
						team, dates[i-2].Format("01/02"), dates[i-1].Format("01/02"), dates[i].Format("01/02")),
				})
			}
		}
	}
	return violations
}

func checkSundayBalance(cfg *config.Config, games []parsedGame) []Violation {
	if !cfg.Guidelines.BalanceSundayGames {
		return nil
	}

	counts := make(map[string]int)
	for _, team := range cfg.AllTeams() {
		counts[team] = 0
	}
	for _, g := range games {
		if g.Date.Weekday() == time.Sunday {
			counts[g.Home]++
			counts[g.Away]++
		}
	}

	maxSun, minSun := 0, math.MaxInt
	for _, c := range counts {
		if c > maxSun {
			maxSun = c
		}
		if c < minSun {
			minSun = c
		}
	}
	if maxSun-minSun > 1 {
		return []Violation{{
			Type:    "warning",
			Message: fmt.Sprintf("Sunday game imbalance: min %d, max %d across teams", minSun, maxSun),
		}}
	}
	return nil
}

func checkGameCompleteness(cfg *config.Config, games []parsedGame) []Violation {
	counts := make(map[string]int)
	for _, g := range games {
		counts[g.Home]++
		counts[g.Away]++
	}

	var violations []Violation
	for _, team := range cfg.AllTeams() {
		if counts[team] == 0 {
			violations = append(violations, Violation{
				Type:    "error",
				Message: fmt.Sprintf("%s has no games scheduled", team),
			})
		}
	}
	return violations
}

func buildTeamDates(games []parsedGame) map[string][]time.Time {
	m := make(map[string][]time.Time)
	for _, g := range games {
		m[g.Home] = append(m[g.Home], g.Date)
		m[g.Away] = append(m[g.Away], g.Date)
	}
	for team := range m {
		sortDates(m[team])
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

// checkOverflowUsage warns when games are scheduled in the overflow period
// but open slots exist in the regular season.
func checkOverflowUsage(cfg *config.Config, f *excelize.File, games []parsedGame) []Violation {
	if cfg.Season.OverflowEndDate == nil {
		return nil
	}

	endDate := cfg.Season.EndDate.Time

	var overflowGames int
	for _, g := range games {
		if g.Date.After(endDate) {
			overflowGames++
		}
	}
	if overflowGames == 0 {
		return nil
	}

	openSlots := countOpenRegularSlots(cfg, f, games)
	if openSlots == 0 {
		return nil
	}

	return []Violation{{
		Type: "warning",
		Message: fmt.Sprintf(
			"%d game(s) in overflow period could potentially fit in %d open regular-season slot(s)",
			overflowGames, openSlots),
	}}
}

// countOpenRegularSlots counts empty, non-blackout field cells in the master
// sheet for rows with dates on or before the season end date.
func countOpenRegularSlots(cfg *config.Config, f *excelize.File, games []parsedGame) int {
	rows, err := f.GetRows("Master Schedule")
	if err != nil || len(rows) == 0 {
		return 0
	}

	header := rows[0]
	numFields := len(header) - 3 // columns after Date, Day, Time
	if numFields <= 0 {
		return 0
	}

	endDate := cfg.Season.EndDate.Time

	// Build a set of (date, time, field-column-index) that have games
	type slotKey struct {
		date  time.Time
		time  string
		field int // field column index (0-based)
	}
	gameSlots := make(map[slotKey]bool)
	for _, g := range games {
		for fi := 0; fi < numFields; fi++ {
			if fi+3 < len(header) && header[fi+3] == g.Field {
				gameSlots[slotKey{g.Date, g.Time, fi}] = true
			}
		}
	}

	open := 0
	for i, row := range rows {
		if i == 0 || len(row) < 3 || row[0] == "" {
			continue
		}
		date, err := time.Parse("01/02/2006", row[0])
		if err != nil || date.After(endDate) {
			continue
		}

		for fi := 0; fi < numFields; fi++ {
			colIdx := fi + 3
			cell := ""
			if colIdx < len(row) {
				cell = row[colIdx]
			}
			if cell == "" {
				open++
			}
		}
	}
	return open
}
