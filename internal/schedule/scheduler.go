package schedule

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/strategy"
)

// Assignment pairs a game with a slot.
type Assignment struct {
	Game strategy.Game
	Slot Slot
}

// Result is the output of the scheduling process.
type Result struct {
	Assignments []Assignment
	Warnings    []string
}

// Schedule assigns games to slots respecting constraints.
func Schedule(cfg *config.Config, slots []Slot, games []strategy.Game) (*Result, error) {
	s := newScheduler(cfg, slots, games)
	if err := s.run(); err != nil {
		return nil, err
	}
	return &Result{
		Assignments: s.assignments,
		Warnings:    s.collectWarnings(),
	}, nil
}

type scheduler struct {
	cfg   *config.Config
	slots []Slot
	games []strategy.Game

	assignments []Assignment
	usedSlots   map[slotKey]bool
	teamDates   map[string][]time.Time   // team -> sorted game dates
	teamGames   map[string]int           // team -> total games scheduled
	slotTimeCnt map[timeKey]int          // (date, time) -> games in that timeslot
	matchupDate map[matchupKey]time.Time // normalized pair -> last date played
}

type slotKey struct {
	date  time.Time
	time  string
	field string
}

type timeKey struct {
	date time.Time
	time string
}

type matchupKey struct {
	a, b string
}

func normalizeMatchup(a, b string) matchupKey {
	if a > b {
		a, b = b, a
	}
	return matchupKey{a, b}
}

func newScheduler(cfg *config.Config, slots []Slot, games []strategy.Game) *scheduler {
	return &scheduler{
		cfg:         cfg,
		slots:       slots,
		games:       games,
		usedSlots:   make(map[slotKey]bool),
		teamDates:   make(map[string][]time.Time),
		teamGames:   make(map[string]int),
		slotTimeCnt: make(map[timeKey]int),
		matchupDate: make(map[matchupKey]time.Time),
	}
}

func (s *scheduler) run() error {
	// Shuffle games to avoid bias in ordering, then sort by scheduling
	// difficulty (teams with fewer remaining options first).
	rng := rand.New(rand.NewSource(42))

	// Multiple attempts with different shuffles
	bestResult := (*scheduler)(nil)
	bestScore := math.MaxFloat64

	for attempt := range 10 {
		candidate := newScheduler(s.cfg, s.slots, s.games)
		shuffled := make([]strategy.Game, len(s.games))
		copy(shuffled, s.games)
		rng = rand.New(rand.NewSource(int64(42 + attempt)))
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		if candidate.trySchedule(shuffled) {
			score := candidate.softScore()
			if score < bestScore {
				bestScore = score
				bestResult = candidate
			}
		}
	}

	if bestResult == nil {
		return fmt.Errorf("could not schedule all %d games into %d available slots", len(s.games), len(s.slots))
	}

	s.assignments = bestResult.assignments
	s.usedSlots = bestResult.usedSlots
	s.teamDates = bestResult.teamDates
	s.teamGames = bestResult.teamGames
	s.slotTimeCnt = bestResult.slotTimeCnt
	s.matchupDate = bestResult.matchupDate
	return nil
}

func (s *scheduler) trySchedule(games []strategy.Game) bool {
	for _, game := range games {
		if !s.assignGame(game) {
			return false
		}
	}
	return true
}

func (s *scheduler) assignGame(game strategy.Game) bool {
	bestSlot := -1
	bestScore := math.MaxFloat64

	for i, slot := range s.slots {
		sk := slotKey{slot.Date, slot.Time, slot.Field}
		if s.usedSlots[sk] {
			continue
		}

		if !s.hardConstraintsMet(game, slot) {
			continue
		}

		score := s.scoreSlot(game, slot)
		if score < bestScore {
			bestScore = score
			bestSlot = i
		}
	}

	if bestSlot < 0 {
		return false
	}

	s.assign(game, s.slots[bestSlot])
	return true
}

func (s *scheduler) assign(game strategy.Game, slot Slot) {
	s.assignments = append(s.assignments, Assignment{Game: game, Slot: slot})
	sk := slotKey{slot.Date, slot.Time, slot.Field}
	s.usedSlots[sk] = true
	s.slotTimeCnt[timeKey{slot.Date, slot.Time}]++

	s.teamDates[game.Home] = insertSorted(s.teamDates[game.Home], slot.Date)
	s.teamDates[game.Away] = insertSorted(s.teamDates[game.Away], slot.Date)
	s.teamGames[game.Home]++
	s.teamGames[game.Away]++

	mk := normalizeMatchup(game.Home, game.Away)
	s.matchupDate[mk] = slot.Date
}

func (s *scheduler) hardConstraintsMet(game strategy.Game, slot Slot) bool {
	// Max games per timeslot
	tk := timeKey{slot.Date, slot.Time}
	if s.slotTimeCnt[tk] >= s.cfg.Rules.MaxGamesPerTimeslot {
		return false
	}

	// No team plays twice in one day
	for _, team := range []string{game.Home, game.Away} {
		for _, d := range s.teamDates[team] {
			if d.Equal(slot.Date) {
				return false
			}
		}
	}

	// No team plays 3 consecutive days
	for _, team := range []string{game.Home, game.Away} {
		if s.wouldMakeConsecutive(team, slot.Date, s.cfg.Rules.MaxConsecutiveDays) {
			return false
		}
	}

	// Max games per week
	for _, team := range []string{game.Home, game.Away} {
		_, week := slot.Date.ISOWeek()
		count := 0
		for _, d := range s.teamDates[team] {
			_, w := d.ISOWeek()
			if w == week {
				count++
			}
		}
		if count >= s.cfg.Rules.MaxGamesPerWeek {
			return false
		}
	}

	return true
}

func (s *scheduler) wouldMakeConsecutive(team string, newDate time.Time, maxConsec int) bool {
	dates := s.teamDates[team]
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

	consecutive := 1
	for i := 1; i < len(all); i++ {
		if all[i].Sub(all[i-1]) == 24*time.Hour {
			consecutive++
			if consecutive > maxConsec {
				return true
			}
		} else {
			consecutive = 1
		}
	}
	return false
}

// scoreSlot returns a lower score for more desirable slots (soft constraints).
func (s *scheduler) scoreSlot(game strategy.Game, slot Slot) float64 {
	score := 0.0

	// Prefer spreading games evenly (balance pace)
	if s.cfg.Guidelines.BalancePace {
		homeGames := s.teamGames[game.Home]
		awayGames := s.teamGames[game.Away]
		avgGames := 0.0
		if len(s.teamGames) > 0 {
			total := 0
			for _, c := range s.teamGames {
				total += c
			}
			avgGames = float64(total) / float64(len(s.cfg.AllTeams()))
		}
		// Penalize scheduling teams that are ahead of average
		score += math.Abs(float64(homeGames)-avgGames) * 2
		score += math.Abs(float64(awayGames)-avgGames) * 2
	}

	// Avoid rematches too soon
	mk := normalizeMatchup(game.Home, game.Away)
	if lastDate, ok := s.matchupDate[mk]; ok {
		daysBetween := slot.Date.Sub(lastDate).Hours() / 24
		minDays := float64(s.cfg.Guidelines.MinDaysBetweenSameMatchup)
		if daysBetween < minDays {
			score += (minDays - daysBetween) * 5
		}
	}

	// Avoid 3 games in 4 days
	if s.cfg.Guidelines.Avoid3In4Days {
		for _, team := range []string{game.Home, game.Away} {
			if s.gamesInWindow(team, slot.Date, 4) >= 2 {
				score += 20
			}
		}
	}

	// Balance Sunday games
	if s.cfg.Guidelines.BalanceSundayGames && slot.Date.Weekday() == time.Sunday {
		for _, team := range []string{game.Home, game.Away} {
			sunCount := s.sundayGames(team)
			score += float64(sunCount) * 10
		}
	}

	// Prefer earlier dates slightly (spread across season)
	dayNum := slot.Date.Sub(s.cfg.Season.StartDate.Time).Hours() / 24
	score += dayNum * 0.1

	return score
}

func (s *scheduler) gamesInWindow(team string, center time.Time, windowDays int) int {
	count := 0
	start := center.AddDate(0, 0, -(windowDays - 1))
	end := center.AddDate(0, 0, windowDays-1)
	for _, d := range s.teamDates[team] {
		if !d.Before(start) && !d.After(end) {
			count++
		}
	}
	return count
}

func (s *scheduler) sundayGames(team string) int {
	count := 0
	for _, d := range s.teamDates[team] {
		if d.Weekday() == time.Sunday {
			count++
		}
	}
	return count
}

func (s *scheduler) softScore() float64 {
	score := 0.0

	// Pace imbalance
	if len(s.teamGames) > 0 {
		max, min := 0, math.MaxInt
		for _, c := range s.teamGames {
			if c > max {
				max = c
			}
			if c < min {
				min = c
			}
		}
		score += float64(max - min)
	}

	// 3-in-4-days violations
	for _, team := range s.cfg.AllTeams() {
		dates := s.teamDates[team]
		for i := 2; i < len(dates); i++ {
			if dates[i].Sub(dates[i-2]).Hours()/24 <= 3 {
				score += 10
			}
		}
	}

	// Rematch proximity
	for _, a := range s.assignments {
		mk := normalizeMatchup(a.Game.Home, a.Game.Away)
		_ = mk // already tracked
	}

	return score
}

func (s *scheduler) collectWarnings() []string {
	var warnings []string

	// Check 3-in-4-days
	for _, team := range s.cfg.AllTeams() {
		dates := s.teamDates[team]
		for i := 2; i < len(dates); i++ {
			if dates[i].Sub(dates[i-2]).Hours()/24 <= 3 {
				warnings = append(warnings, fmt.Sprintf(
					"%s plays 3 games in 4 days: %s, %s, %s",
					team,
					dates[i-2].Format("01/02"),
					dates[i-1].Format("01/02"),
					dates[i].Format("01/02")))
			}
		}
	}

	// Check rematch proximity
	type matchDates struct {
		first, second time.Time
	}
	matchups := make(map[matchupKey][]time.Time)
	for _, a := range s.assignments {
		mk := normalizeMatchup(a.Game.Home, a.Game.Away)
		matchups[mk] = append(matchups[mk], a.Slot.Date)
	}
	for mk, dates := range matchups {
		sortDatesInPlace(dates)
		for i := 1; i < len(dates); i++ {
			daysBetween := dates[i].Sub(dates[i-1]).Hours() / 24
			if daysBetween < float64(s.cfg.Guidelines.MinDaysBetweenSameMatchup) {
				warnings = append(warnings, fmt.Sprintf(
					"%s vs %s rematch after only %.0f days (min %d): %s and %s",
					mk.a, mk.b, daysBetween, s.cfg.Guidelines.MinDaysBetweenSameMatchup,
					dates[i-1].Format("01/02"), dates[i].Format("01/02")))
			}
		}
	}

	// Sunday balance
	sundayCounts := make(map[string]int)
	for _, team := range s.cfg.AllTeams() {
		sundayCounts[team] = s.sundayGames(team)
	}
	maxSun, minSun := 0, math.MaxInt
	for _, c := range sundayCounts {
		if c > maxSun {
			maxSun = c
		}
		if c < minSun {
			minSun = c
		}
	}
	if maxSun-minSun > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"Sunday game imbalance: min %d, max %d across teams", minSun, maxSun))
	}

	return warnings
}

func insertSorted(dates []time.Time, d time.Time) []time.Time {
	i := 0
	for i < len(dates) && dates[i].Before(d) {
		i++
	}
	dates = append(dates, time.Time{})
	copy(dates[i+1:], dates[i:])
	dates[i] = d
	return dates
}

func sortDatesInPlace(dates []time.Time) {
	for i := 1; i < len(dates); i++ {
		for j := i; j > 0 && dates[j].Before(dates[j-1]); j-- {
			dates[j], dates[j-1] = dates[j-1], dates[j]
		}
	}
}
