// Package reschedule evaluates candidate slots for moving one or more
// already-scheduled games. It does not modify any files; callers display
// the results and decide what to do.
package reschedule

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/schedule"
)

// Game is a directional matchup as it appears in the schedule.
type Game struct {
	Away string
	Home string
}

func (g Game) String() string { return g.Away + " @ " + g.Home }

// Assignment pairs a game with its slot in the existing schedule.
type Assignment struct {
	Game Game
	Slot schedule.Slot
}

// Status is the verdict for a candidate slot.
type Status int

const (
	StatusOK Status = iota
	StatusWarn
	StatusFail
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusWarn:
		return "WARN"
	case StatusFail:
		return "FAIL"
	}
	return "?"
}

// Candidate is one slot evaluated against the rules and guidelines.
type Candidate struct {
	Slot        schedule.Slot
	Status      Status
	HardReasons []string // rule violations
	SoftReasons []string // guideline violations
}

// LoadAssignments reads the Master Schedule sheet from an xlsx file. The cfg
// is used to resolve abbreviated field column headers (e.g., "Washington")
// back to their canonical configuration names (e.g., "Washington Park") so
// slot identifiers match those produced by schedule.GenerateSlots.
func LoadAssignments(path string, cfg *config.Config) ([]Assignment, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	rows, err := f.GetRows("Master Schedule")
	if err != nil {
		return nil, fmt.Errorf("reading Master Schedule: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Master Schedule is empty")
	}

	header := rows[0]
	var fieldNames []string
	for i := 3; i < len(header); i++ {
		fieldNames = append(fieldNames, resolveFieldName(header[i], cfg))
	}

	var games []Assignment
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
		t := row[2]
		for fi, fname := range fieldNames {
			colIdx := fi + 3
			if colIdx >= len(row) {
				continue
			}
			cell := row[colIdx]
			if cell == "" {
				continue
			}
			away, home, ok := parseGameCell(cell)
			if !ok {
				continue
			}
			games = append(games, Assignment{
				Game: Game{Away: away, Home: home},
				Slot: schedule.Slot{Date: date, Time: t, Field: fname},
			})
		}
	}
	return games, nil
}

func parseGameCell(cell string) (away, home string, ok bool) {
	idx := strings.Index(cell, " @ ")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(cell[:idx]), strings.TrimSpace(cell[idx+3:]), true
}

// resolveFieldName maps an Excel column header (full name or short_name)
// back to the canonical field name from the config. Falls back to the
// header itself if no match is found.
func resolveFieldName(header string, cfg *config.Config) string {
	for _, field := range cfg.Fields {
		if field.Name == header || (field.ShortName != "" && field.ShortName == header) {
			return field.Name
		}
	}
	return header
}

// FindGame locates the assignment matching spec ("Away @ Home"). Match is
// case-insensitive on team names. Multiple matches (e.g., a matchup played
// twice in the season) returns an error listing the existing dates so the
// caller can pin down the intended one.
func FindGame(assignments []Assignment, spec string) (int, error) {
	away, home, ok := parseGameCell(spec)
	if !ok {
		return -1, fmt.Errorf("game %q: expected format \"Away @ Home\"", spec)
	}

	var matches []int
	for i, a := range assignments {
		if strings.EqualFold(a.Game.Away, away) && strings.EqualFold(a.Game.Home, home) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return -1, fmt.Errorf("game %q not found in schedule", spec)
	case 1:
		return matches[0], nil
	default:
		var dates []string
		for _, idx := range matches {
			dates = append(dates, assignments[idx].Slot.Date.Format("01/02"))
		}
		return -1, fmt.Errorf(
			"game %q matches %d entries on %s; the reschedule command requires unique matchups",
			spec, len(matches), strings.Join(dates, ", "))
	}
}

// CandidateSlots returns every open (unused) slot in the season, including
// the overflow period and any makeup-only timeslots from the config (which
// the initial scheduler does not use). Sorted chronologically.
func CandidateSlots(cfg *config.Config, existing []Assignment) []schedule.Slot {
	cfgCopy := *cfg
	cfgCopy.TimeSlots = cfg.TimeSlots
	cfgCopy.TimeSlots.Weekday = mergeSlots(cfg.TimeSlots.Weekday, cfg.TimeSlots.MakeupWeekday)
	cfgCopy.TimeSlots.Saturday = mergeSlots(cfg.TimeSlots.Saturday, cfg.TimeSlots.MakeupSaturday)
	cfgCopy.TimeSlots.Sunday = mergeSlots(cfg.TimeSlots.Sunday, cfg.TimeSlots.MakeupSunday)

	all := append([]schedule.Slot{}, schedule.GenerateSlots(&cfgCopy)...)
	all = append(all, schedule.GenerateOverflowSlots(&cfgCopy)...)

	used := make(map[string]bool, len(existing))
	for _, a := range existing {
		used[slotID(a.Slot)] = true
	}

	sortSlotsChrono(all)

	// Field choice doesn't affect rule evaluation, so collapse to one
	// representative slot per (date, time). When only one field is open,
	// keep its name; when multiple are open, label the field "either".
	type dtKey struct {
		date string
		time string
	}
	type entry struct {
		index  int
		fields []string
	}
	openByKey := make(map[dtKey]*entry)
	var order []dtKey

	for _, s := range all {
		if used[slotID(s)] {
			continue
		}
		k := dtKey{s.Date.Format("2006-01-02"), s.Time}
		e, ok := openByKey[k]
		if !ok {
			e = &entry{index: len(order)}
			openByKey[k] = e
			order = append(order, k)
		}
		e.fields = append(e.fields, s.Field)
	}

	open := make([]schedule.Slot, 0, len(order))
	for _, k := range order {
		date, _ := time.Parse("2006-01-02", k.date)
		field := openByKey[k].fields[0]
		if len(openByKey[k].fields) > 1 {
			field = "Either"
		}
		open = append(open, schedule.Slot{Date: date, Time: k.time, Field: field})
	}
	return open
}

// mergeSlots returns a copy of regular with each makeup time appended that
// is not already present.
func mergeSlots(regular, makeup []string) []string {
	out := append([]string{}, regular...)
	seen := make(map[string]bool, len(regular))
	for _, t := range regular {
		seen[t] = true
	}
	for _, t := range makeup {
		if !seen[t] {
			out = append(out, t)
			seen[t] = true
		}
	}
	return out
}

// Evaluate scores each candidate slot for placing `game` into the schedule
// where `others` are the existing assignments excluding the game being moved.
// Slots with no rule violations are OK; with only guideline violations are
// WARN; with any rule violation are FAIL.
func Evaluate(cfg *config.Config, others []Assignment, game Game, candidates []schedule.Slot) []Candidate {
	teamDates := make(map[string][]time.Time)
	timeCount := make(map[timeKey]int)
	matchDates := make(map[matchKey][]time.Time)
	sundayCount := make(map[string]int)
	for _, a := range others {
		teamDates[a.Game.Home] = append(teamDates[a.Game.Home], a.Slot.Date)
		teamDates[a.Game.Away] = append(teamDates[a.Game.Away], a.Slot.Date)
		timeCount[timeKey{a.Slot.Date, a.Slot.Time}]++
		mk := normMatch(a.Game.Home, a.Game.Away)
		matchDates[mk] = append(matchDates[mk], a.Slot.Date)
		if a.Slot.Date.Weekday() == time.Sunday {
			sundayCount[a.Game.Home]++
			sundayCount[a.Game.Away]++
		}
	}
	for k := range teamDates {
		sortDates(teamDates[k])
	}

	out := make([]Candidate, 0, len(candidates))
	for _, slot := range candidates {
		// Skip slots already at the timeslot cap — they aren't really
		// available, so don't clutter the output with them.
		if cfg.Rules.MaxGamesPerTimeslot > 0 &&
			timeCount[timeKey{slot.Date, slot.Time}] >= cfg.Rules.MaxGamesPerTimeslot {
			continue
		}

		// Skip dates where either team is already scheduled — there is no
		// way to place this game on such a date, so the slot isn't a real
		// candidate.
		alreadyPlaying := false
		for _, team := range []string{game.Home, game.Away} {
			for _, d := range teamDates[team] {
				if d.Equal(slot.Date) {
					alreadyPlaying = true
					break
				}
			}
			if alreadyPlaying {
				break
			}
		}
		if alreadyPlaying {
			continue
		}

		c := Candidate{Slot: slot}

		// addReason routes a violation message through the configured
		// severity for `name` in reschedule mode.
		addReason := func(name, msg string) {
			switch cfg.Severity(name, config.ModeReschedule) {
			case config.SeverityRule:
				c.HardReasons = append(c.HardReasons, msg)
			case config.SeverityGuideline:
				c.SoftReasons = append(c.SoftReasons, msg)
			case config.SeverityDisabled:
				// skip
			}
		}
		enabled := func(name string) bool {
			return cfg.Severity(name, config.ModeReschedule) != config.SeverityDisabled
		}

		for _, team := range []string{game.Home, game.Away} {
			if cfg.Rules.MaxConsecutiveDays > 0 && enabled("max_consecutive_days") {
				if run := consecRun(teamDates[team], slot.Date); run > cfg.Rules.MaxConsecutiveDays {
					addReason("max_consecutive_days", fmt.Sprintf(
						"%s would play %d consecutive days (max %d)",
						team, run, cfg.Rules.MaxConsecutiveDays))
				}
			}

			if cfg.Rules.MaxGamesPerWeek > 0 && enabled("max_games_per_week") {
				slotYear, slotWeek := slot.Date.ISOWeek()
				count := 1
				for _, d := range teamDates[team] {
					y, w := d.ISOWeek()
					if y == slotYear && w == slotWeek {
						count++
					}
				}
				if count > cfg.Rules.MaxGamesPerWeek {
					addReason("max_games_per_week", fmt.Sprintf(
						"%s would play %d games in week %d (max %d)",
						team, count, slotWeek, cfg.Rules.MaxGamesPerWeek))
				}
			}

			if cfg.Rules.Max3In4Days && enabled("max_3_in_4_days") {
				if windowedCount(teamDates[team], slot.Date, 4) >= 2 {
					addReason("max_3_in_4_days", fmt.Sprintf(
						"%s would play 3 games in 4 days", team))
				}
			}
		}

		mk := normMatch(game.Home, game.Away)
		if cfg.Guidelines.MinDaysBetweenSameMatchup > 0 && enabled("min_days_between_same_matchup") {
			if dates := matchDates[mk]; len(dates) > 0 {
				nearest := -1
				for _, d := range dates {
					diff := int(slot.Date.Sub(d).Hours() / 24)
					if diff < 0 {
						diff = -diff
					}
					if nearest < 0 || diff < nearest {
						nearest = diff
					}
				}
				if nearest >= 0 && nearest < cfg.Guidelines.MinDaysBetweenSameMatchup {
					addReason("min_days_between_same_matchup", fmt.Sprintf(
						"%s vs %s rematch within %d days (min %d)",
						mk.a, mk.b, nearest, cfg.Guidelines.MinDaysBetweenSameMatchup))
				}
			}
		}

		if cfg.Guidelines.BalanceSundayGames && slot.Date.Weekday() == time.Sunday && enabled("balance_sunday_games") {
			_, maxSun := minMaxSunday(cfg, sundayCount)
			for _, team := range []string{game.Home, game.Away} {
				if sundayCount[team]+1 > maxSun && sundayCount[team]+1 > 1 {
					addReason("balance_sunday_games", fmt.Sprintf(
						"%s would have %d Sunday games (current max: %d)",
						team, sundayCount[team]+1, maxSun))
					break
				}
			}
		}

		switch {
		case len(c.HardReasons) > 0:
			c.Status = StatusFail
		case len(c.SoftReasons) > 0:
			c.Status = StatusWarn
		default:
			c.Status = StatusOK
		}
		out = append(out, c)
	}
	return out
}

// Placement is the chosen slot for a game.
type Placement struct {
	Game        Game
	Slot        schedule.Slot
	SoftReasons []string
}

// --- helpers ---

// SlotsAfter returns slots whose date is strictly after `after`.
func SlotsAfter(slots []schedule.Slot, after time.Time) []schedule.Slot {
	out := make([]schedule.Slot, 0, len(slots))
	for _, s := range slots {
		if s.Date.After(after) {
			out = append(out, s)
		}
	}
	return out
}

type timeKey struct {
	date time.Time
	t    string
}
type matchKey struct{ a, b string }

func normMatch(a, b string) matchKey {
	if a > b {
		a, b = b, a
	}
	return matchKey{a, b}
}

// consecRun returns the longest run of consecutive days that includes
// `newDate` when inserted into the (already sorted) `dates` list.
func consecRun(dates []time.Time, newDate time.Time) int {
	all := make([]time.Time, 0, len(dates)+1)
	inserted := false
	for _, d := range dates {
		if !inserted && newDate.Before(d) {
			all = append(all, newDate)
			inserted = true
		}
		all = append(all, d)
	}
	if !inserted {
		all = append(all, newDate)
	}
	worst := 1
	consec := 1
	for i := 1; i < len(all); i++ {
		if all[i].Sub(all[i-1]) == 24*time.Hour {
			consec++
			if consec > worst {
				worst = consec
			}
		} else {
			consec = 1
		}
	}
	return worst
}

func windowedCount(dates []time.Time, center time.Time, window int) int {
	start := center.AddDate(0, 0, -(window - 1))
	end := center.AddDate(0, 0, window-1)
	count := 0
	for _, d := range dates {
		if !d.Before(start) && !d.After(end) {
			count++
		}
	}
	return count
}

func minMaxSunday(cfg *config.Config, counts map[string]int) (minVal, maxVal int) {
	first := true
	for _, team := range cfg.AllTeams() {
		c := counts[team]
		if first {
			minVal, maxVal = c, c
			first = false
			continue
		}
		if c < minVal {
			minVal = c
		}
		if c > maxVal {
			maxVal = c
		}
	}
	return
}

func slotID(s schedule.Slot) string {
	return s.Date.Format("2006-01-02") + "|" + s.Time + "|" + s.Field
}

func sortSlotsChrono(slots []schedule.Slot) {
	sort.Slice(slots, func(i, j int) bool {
		if !slots[i].Date.Equal(slots[j].Date) {
			return slots[i].Date.Before(slots[j].Date)
		}
		if slots[i].Time != slots[j].Time {
			return slots[i].Time < slots[j].Time
		}
		return slots[i].Field < slots[j].Field
	})
}

func sortDates(dates []time.Time) {
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
}

func filterUsed(slots []schedule.Slot, used map[string]bool) []schedule.Slot {
	if len(used) == 0 {
		return slots
	}
	out := make([]schedule.Slot, 0, len(slots))
	for _, s := range slots {
		if !used[slotID(s)] {
			out = append(out, s)
		}
	}
	return out
}
