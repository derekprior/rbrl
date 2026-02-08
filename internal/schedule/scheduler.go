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

// TeamMetrics holds per-team schedule statistics.
type TeamMetrics struct {
	Games      int
	Saturday   int
	Sunday     int
	Violations []string
}

// Result is the output of the scheduling process.
type Result struct {
	Assignments []Assignment
	Warnings    []string
	TeamGames   map[string]int // games scheduled per team
	TeamMetrics map[string]*TeamMetrics
}

// Schedule assigns games to slots respecting constraints.
// On failure, returns a partial Result with the best attempt alongside the error.
func Schedule(cfg *config.Config, slots []Slot, games []strategy.Game) (*Result, error) {
	s := newScheduler(cfg, slots, games)
	if err := s.run(); err != nil {
		warnings, metrics := s.buildMetrics()
		return &Result{
			Assignments: s.assignments,
			Warnings:    warnings,
			TeamGames:   s.teamGames,
			TeamMetrics: metrics,
		}, err
	}
	warnings, metrics := s.buildMetrics()
	return &Result{
		Assignments: s.assignments,
		Warnings:    warnings,
		TeamGames:   s.teamGames,
		TeamMetrics: metrics,
	}, nil
}

// rejectionReason categorizes why a slot was rejected for a game.
type rejectionReason int

const (
	rejectSlotUsed rejectionReason = iota
	rejectTimeslotCap
	rejectDoublePlay
	rejectConsecutiveDays
	rejectMaxWeekGames
	reject3In4Days
)

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

	// diagnostics for failure reporting
	rejections  map[rejectionReason]int
	unscheduled []strategy.Game
	stuckOnGame *strategy.Game
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
		rejections:  make(map[rejectionReason]int),
	}
}

func (s *scheduler) run() error {
	rng := rand.New(rand.NewSource(42))

	bestResult := (*scheduler)(nil)
	bestScore := math.MaxFloat64
	var bestFailure *scheduler

	for attempt := range 50 {
		candidate := newScheduler(s.cfg, s.slots, s.games)
		shuffled := make([]strategy.Game, len(s.games))
		copy(shuffled, s.games)
		rng = rand.New(rand.NewSource(int64(42 + attempt)))
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		if candidate.trySchedule(shuffled, rng) {
			score := candidate.softScore()
			if score < bestScore {
				bestScore = score
				bestResult = candidate
			}
		} else {
			// Track the attempt that scheduled the most games
			if bestFailure == nil || len(candidate.assignments) > len(bestFailure.assignments) {
				bestFailure = candidate
			}
		}
	}

	if bestResult == nil {
		// Copy best failure state so caller can access partial results
		if bestFailure != nil {
			s.assignments = bestFailure.assignments
			s.usedSlots = bestFailure.usedSlots
			s.teamDates = bestFailure.teamDates
			s.teamGames = bestFailure.teamGames
			s.slotTimeCnt = bestFailure.slotTimeCnt
			s.matchupDate = bestFailure.matchupDate
		}
		return s.buildFailureError(bestFailure)
	}

	s.assignments = bestResult.assignments
	s.usedSlots = bestResult.usedSlots
	s.teamDates = bestResult.teamDates
	s.teamGames = bestResult.teamGames
	s.slotTimeCnt = bestResult.slotTimeCnt
	s.matchupDate = bestResult.matchupDate
	return nil
}

func (s *scheduler) buildFailureError(best *scheduler) error {
	msg := fmt.Sprintf("could not schedule all %d games into %d available slots", len(s.games), len(s.slots))

	if best == nil {
		return fmt.Errorf("%s", msg)
	}

	msg += fmt.Sprintf("\n\nBest attempt: scheduled %d of %d games (%d unscheduled)",
		len(best.assignments), len(s.games), len(best.unscheduled))

	if best.stuckOnGame != nil {
		msg += fmt.Sprintf("\nFirst game that failed: %s vs %s",
			best.stuckOnGame.Home, best.stuckOnGame.Away)
	}

	msg += "\n\nUnscheduled games:"
	for _, g := range best.unscheduled {
		msg += fmt.Sprintf("\n  • %s vs %s", g.Home, g.Away)
	}

	return fmt.Errorf("%s", msg)
}

func (s *scheduler) trySchedule(games []strategy.Game, rng *rand.Rand) bool {
	remaining := make([]strategy.Game, len(games))
	copy(remaining, games)

	// Phase 1: Schedule Saturdays — all teams play every Saturday
	remaining = s.scheduleSaturdays(remaining, rng)

	// Phase 2: Schedule Sundays — balanced across teams
	remaining = s.scheduleSundays(remaining, rng)

	// Phase 3: Fill remaining games into weekday slots
	// Sort by difficulty: games with fewer available slots go first
	s.sortByDifficulty(remaining)

	remaining = s.scheduleWithBacktracking(remaining)
	s.unscheduled = remaining

	return len(s.unscheduled) == 0
}

// scheduleWithBacktracking tries to place all games, displacing existing
// assignments when a game can't be placed directly.
func (s *scheduler) scheduleWithBacktracking(games []strategy.Game) []strategy.Game {
	var unscheduled []strategy.Game

	for _, game := range games {
		if s.assignGame(game) {
			continue
		}

		// Can't place directly — try displacing an existing assignment
		if s.tryDisplace(game) {
			continue
		}

		if s.stuckOnGame == nil {
			s.stuckOnGame = &game
		}
		unscheduled = append(unscheduled, game)
	}

	return unscheduled
}

// tryDisplace attempts to place a game by removing a conflicting assignment
// and re-placing the displaced game elsewhere, up to maxDepth levels deep.
func (s *scheduler) tryDisplace(game strategy.Game) bool {
	return s.tryDisplaceAtDepth(game, 3)
}

func (s *scheduler) tryDisplaceAtDepth(game strategy.Game, depth int) bool {
	if depth <= 0 {
		return false
	}

	for _, slot := range s.slots {
		sk := slotKey{slot.Date, slot.Time, slot.Field}

		if !s.usedSlots[sk] {
			continue
		}

		victimIdx := -1
		for i, a := range s.assignments {
			if a.Slot.Date.Equal(slot.Date) && a.Slot.Time == slot.Time && a.Slot.Field == slot.Field {
				victimIdx = i
				break
			}
		}
		if victimIdx < 0 {
			continue
		}

		victim := s.unassign(victimIdx)

		if _, ok := s.hardConstraintCheck(game, slot); ok {
			s.assign(game, slot)
			if s.assignGame(victim.Game) {
				return true
			}
			if s.tryDisplaceAtDepth(victim.Game, depth-1) {
				return true
			}
			for i, a := range s.assignments {
				if a.Slot.Date.Equal(slot.Date) && a.Slot.Time == slot.Time && a.Slot.Field == slot.Field {
					s.unassign(i)
					break
				}
			}
		}

		s.assign(victim.Game, victim.Slot)
	}

	return false
}

// sortByDifficulty orders games so those with fewer available slots come first.
func (s *scheduler) sortByDifficulty(games []strategy.Game) {
	counts := make([]int, len(games))
	for i, game := range games {
		counts[i] = s.countAvailableSlots(game)
	}
	// Simple insertion sort (small N)
	for i := 1; i < len(games); i++ {
		for j := i; j > 0 && counts[j] < counts[j-1]; j-- {
			games[j], games[j-1] = games[j-1], games[j]
			counts[j], counts[j-1] = counts[j-1], counts[j]
		}
	}
}

// countAvailableSlots returns how many slots a game could currently be assigned to.
func (s *scheduler) countAvailableSlots(game strategy.Game) int {
	count := 0
	for _, slot := range s.slots {
		sk := slotKey{slot.Date, slot.Time, slot.Field}
		if s.usedSlots[sk] {
			continue
		}
		if _, ok := s.hardConstraintCheck(game, slot); !ok {
			continue
		}
		count++
	}
	return count
}

// scheduleSaturdays assigns games to Saturday slots so every team plays each Saturday.
func (s *scheduler) scheduleSaturdays(games []strategy.Game, rng *rand.Rand) []strategy.Game {
	teams := s.cfg.AllTeams()
	saturdays := s.slotDates(time.Saturday)

	scheduled := make(map[int]bool) // index into games

	for _, sat := range saturdays {
		satSlots := s.slotsForDate(sat)
		if len(satSlots) == 0 {
			continue
		}

		// Find a perfect matching: 5 games covering all teams
		match := s.findPerfectMatch(games, scheduled, teams, rng)
		if match == nil {
			continue
		}

		for _, gi := range match {
			game := games[gi]
			// Find the best Saturday slot for this game
			bestSlot := -1
			bestScore := math.MaxFloat64
			for _, si := range satSlots {
				slot := s.slots[si]
				sk := slotKey{slot.Date, slot.Time, slot.Field}
				if s.usedSlots[sk] {
					continue
				}
				if _, ok := s.hardConstraintCheck(game, slot); !ok {
					continue
				}
				score := s.scoreSlot(game, slot)
				if score < bestScore {
					bestScore = score
					bestSlot = si
				}
			}
			if bestSlot >= 0 {
				s.assign(game, s.slots[bestSlot])
				scheduled[gi] = true
			}
		}
	}

	// Return unscheduled games
	var remaining []strategy.Game
	for i, g := range games {
		if !scheduled[i] {
			remaining = append(remaining, g)
		}
	}
	return remaining
}

// findPerfectMatch finds len(teams)/2 games from the pool that cover all teams.
func (s *scheduler) findPerfectMatch(games []strategy.Game, used map[int]bool, teams []string, rng *rand.Rand) []int {
	needed := len(teams) / 2

	// Shuffle indices for randomness
	indices := make([]int, 0, len(games))
	for i := range games {
		if !used[i] {
			indices = append(indices, i)
		}
	}
	rng.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})

	teamUsed := make(map[string]bool)
	var match []int

	for _, i := range indices {
		g := games[i]
		if teamUsed[g.Home] || teamUsed[g.Away] {
			continue
		}
		match = append(match, i)
		teamUsed[g.Home] = true
		teamUsed[g.Away] = true
		if len(match) == needed {
			return match
		}
	}

	return nil // couldn't find a perfect match
}

// scheduleSundays assigns games to Sunday slots, balancing across teams.
func (s *scheduler) scheduleSundays(games []strategy.Game, rng *rand.Rand) []strategy.Game {
	sundays := s.slotDates(time.Sunday)
	scheduled := make(map[int]bool)

	for _, sun := range sundays {
		sunSlots := s.slotsForDate(sun)
		if len(sunSlots) == 0 {
			continue
		}

		// Determine how many games we can play this Sunday
		availableSlots := 0
		for _, si := range sunSlots {
			sk := slotKey{s.slots[si].Date, s.slots[si].Time, s.slots[si].Field}
			if !s.usedSlots[sk] {
				availableSlots++
			}
		}
		maxGames := availableSlots
		if maxGames > s.cfg.Rules.MaxGamesPerTimeslot {
			maxGames = s.cfg.Rules.MaxGamesPerTimeslot
		}

		// Pick games favoring teams with fewer Sunday games
		for gamesScheduled := 0; gamesScheduled < maxGames; gamesScheduled++ {
			bestGame := -1
			bestSlot := -1
			bestScore := math.MaxFloat64

			for i, game := range games {
				if scheduled[i] {
					continue
				}

				sunHome := s.sundayGames(game.Home)
				sunAway := s.sundayGames(game.Away)
				gameScore := float64(sunHome+sunAway) * 10

				for _, si := range sunSlots {
					slot := s.slots[si]
					sk := slotKey{slot.Date, slot.Time, slot.Field}
					if s.usedSlots[sk] {
						continue
					}
					if _, ok := s.hardConstraintCheck(game, slot); !ok {
						continue
					}
					score := gameScore + s.scoreSlot(game, slot)
					if score < bestScore {
						bestScore = score
						bestGame = i
						bestSlot = si
					}
				}
			}

			if bestGame < 0 {
				break
			}

			s.assign(games[bestGame], s.slots[bestSlot])
			scheduled[bestGame] = true
		}
	}

	var remaining []strategy.Game
	for i, g := range games {
		if !scheduled[i] {
			remaining = append(remaining, g)
		}
	}
	return remaining
}

// slotDates returns sorted unique dates for a given weekday across available slots.
func (s *scheduler) slotDates(day time.Weekday) []time.Time {
	seen := make(map[time.Time]bool)
	var dates []time.Time
	for _, slot := range s.slots {
		if slot.Date.Weekday() == day && !seen[slot.Date] {
			seen[slot.Date] = true
			dates = append(dates, slot.Date)
		}
	}
	sortDatesInPlace(dates)
	return dates
}

// slotsForDate returns indices of slots on a given date.
func (s *scheduler) slotsForDate(date time.Time) []int {
	var indices []int
	for i, slot := range s.slots {
		if slot.Date.Equal(date) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (s *scheduler) assignGame(game strategy.Game) bool {
	bestSlot := -1
	bestScore := math.MaxFloat64

	for i, slot := range s.slots {
		sk := slotKey{slot.Date, slot.Time, slot.Field}
		if s.usedSlots[sk] {
			s.rejections[rejectSlotUsed]++
			continue
		}

		if reason, ok := s.hardConstraintCheck(game, slot); !ok {
			s.rejections[reason]++
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

func (s *scheduler) unassign(idx int) Assignment {
	a := s.assignments[idx]
	s.assignments = append(s.assignments[:idx], s.assignments[idx+1:]...)

	sk := slotKey{a.Slot.Date, a.Slot.Time, a.Slot.Field}
	delete(s.usedSlots, sk)
	s.slotTimeCnt[timeKey{a.Slot.Date, a.Slot.Time}]--

	s.teamDates[a.Game.Home] = removeDate(s.teamDates[a.Game.Home], a.Slot.Date)
	s.teamDates[a.Game.Away] = removeDate(s.teamDates[a.Game.Away], a.Slot.Date)
	s.teamGames[a.Game.Home]--
	s.teamGames[a.Game.Away]--

	// Rebuild matchupDate for this pair from remaining assignments
	mk := normalizeMatchup(a.Game.Home, a.Game.Away)
	delete(s.matchupDate, mk)
	for _, other := range s.assignments {
		omk := normalizeMatchup(other.Game.Home, other.Game.Away)
		if omk == mk {
			if existing, ok := s.matchupDate[mk]; !ok || other.Slot.Date.After(existing) {
				s.matchupDate[mk] = other.Slot.Date
			}
		}
	}

	return a
}

func removeDate(dates []time.Time, d time.Time) []time.Time {
	for i, t := range dates {
		if t.Equal(d) {
			return append(dates[:i], dates[i+1:]...)
		}
	}
	return dates
}

func (s *scheduler) hardConstraintCheck(game strategy.Game, slot Slot) (rejectionReason, bool) {
	// Max games per timeslot
	tk := timeKey{slot.Date, slot.Time}
	if s.slotTimeCnt[tk] >= s.cfg.Rules.MaxGamesPerTimeslot {
		return rejectTimeslotCap, false
	}

	// No team plays twice in one day
	for _, team := range []string{game.Home, game.Away} {
		for _, d := range s.teamDates[team] {
			if d.Equal(slot.Date) {
				return rejectDoublePlay, false
			}
		}
	}

	// No team plays 3 consecutive days
	for _, team := range []string{game.Home, game.Away} {
		if s.wouldMakeConsecutive(team, slot.Date, s.cfg.Rules.MaxConsecutiveDays) {
			return rejectConsecutiveDays, false
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
			return rejectMaxWeekGames, false
		}
	}

	// No 3 games in 4 days
	if s.cfg.Rules.Max3In4Days {
		for _, team := range []string{game.Home, game.Away} {
			if s.gamesInWindow(team, slot.Date, 4) >= 2 {
				return reject3In4Days, false
			}
		}
	}

	return 0, true
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

func (s *scheduler) buildMetrics() ([]string, map[string]*TeamMetrics) {
	var warnings []string
	metrics := make(map[string]*TeamMetrics)

	// Initialize metrics for all teams
	for _, team := range s.cfg.AllTeams() {
		m := &TeamMetrics{Games: s.teamGames[team]}
		for _, d := range s.teamDates[team] {
			switch d.Weekday() {
			case time.Saturday:
				m.Saturday++
			case time.Sunday:
				m.Sunday++
			}
		}
		metrics[team] = m
	}

	// Check 3-in-4-days
	for _, team := range s.cfg.AllTeams() {
		dates := s.teamDates[team]
		for i := 2; i < len(dates); i++ {
			if dates[i].Sub(dates[i-2]).Hours()/24 <= 3 {
				w := fmt.Sprintf("%s plays 3 games in 4 days: %s, %s, %s",
					team,
					dates[i-2].Format("01/02"),
					dates[i-1].Format("01/02"),
					dates[i].Format("01/02"))
				warnings = append(warnings, w)
				metrics[team].Violations = append(metrics[team].Violations, w)
			}
		}
	}

	// Check rematch proximity
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
				w := fmt.Sprintf("%s vs %s rematch after only %.0f days (min %d): %s and %s",
					mk.a, mk.b, daysBetween, s.cfg.Guidelines.MinDaysBetweenSameMatchup,
					dates[i-1].Format("01/02"), dates[i].Format("01/02"))
				warnings = append(warnings, w)
				metrics[mk.a].Violations = append(metrics[mk.a].Violations, w)
				metrics[mk.b].Violations = append(metrics[mk.b].Violations, w)
			}
		}
	}

	// Sunday balance
	maxSun, minSun := 0, math.MaxInt
	for _, m := range metrics {
		if m.Sunday > maxSun {
			maxSun = m.Sunday
		}
		if m.Sunday < minSun {
			minSun = m.Sunday
		}
	}
	if maxSun-minSun > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"Sunday game imbalance: min %d, max %d across teams", minSun, maxSun))
	}

	return warnings, metrics
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
