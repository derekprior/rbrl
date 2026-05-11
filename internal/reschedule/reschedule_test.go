package reschedule

import (
	"strings"
	"testing"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/schedule"
)

func mustDate(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func testConfig() *config.Config {
	return &config.Config{
		Season: config.Season{
			StartDate: config.Date{Time: mustDate("2026-04-27")}, // Monday
			EndDate:   config.Date{Time: mustDate("2026-05-10")}, // 2 weeks
		},
		Divisions: []config.Division{
			{Name: "A", Teams: []string{"Angels", "Astros", "Athletics", "Mariners"}},
		},
		Fields: []config.Field{
			{Name: "Field1"},
			{Name: "Field2"},
		},
		TimeSlots: config.TimeSlots{
			Weekday:  []string{"17:45"},
			Saturday: []string{"12:30", "15:00"},
			Sunday:   []string{"17:00"},
		},
		Rules: config.Rules{
			MaxGamesPerDayPerTeam: 1,
			MaxConsecutiveDays:    2,
			MaxGamesPerWeek:       3,
			MaxGamesPerTimeslot:   2,
			Max3In4Days:           true,
		},
		Guidelines: config.Guidelines{
			MinDaysBetweenSameMatchup: 10,
			BalanceSundayGames:        true,
		},
	}
}

func TestParseGameCell(t *testing.T) {
	tests := []struct {
		in              string
		away, home      string
		ok              bool
	}{
		{"Astros @ Angels", "Astros", "Angels", true},
		{"  Astros @ Angels  ", "Astros", "Angels", true},
		{"BLACKED OUT", "", "", false},
		{"Reservation: JV", "", "", false},
	}
	for _, tc := range tests {
		away, home, ok := parseGameCell(tc.in)
		if ok != tc.ok || away != tc.away || home != tc.home {
			t.Errorf("parseGameCell(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, away, home, ok, tc.away, tc.home, tc.ok)
		}
	}
}

func TestFindGame(t *testing.T) {
	assignments := []Assignment{
		{Game: Game{Away: "Astros", Home: "Angels"}, Slot: schedule.Slot{Date: mustDate("2026-04-27")}},
		{Game: Game{Away: "Cubs", Home: "Padres"}, Slot: schedule.Slot{Date: mustDate("2026-04-28")}},
		{Game: Game{Away: "Astros", Home: "Angels"}, Slot: schedule.Slot{Date: mustDate("2026-05-04")}},
	}

	idx, err := FindGame(assignments, "Cubs @ Padres")
	if err != nil || idx != 1 {
		t.Errorf("expected idx=1 nil err, got idx=%d err=%v", idx, err)
	}

	if _, err := FindGame(assignments, "Astros @ Angels"); err == nil {
		t.Error("expected ambiguity error for duplicate matchup")
	} else if !strings.Contains(err.Error(), "matches 2 entries") {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := FindGame(assignments, "Royals @ Mariners"); err == nil {
		t.Error("expected not-found error")
	}

	if _, err := FindGame(assignments, "garbage"); err == nil {
		t.Error("expected format error")
	}

	idx, err = FindGame(assignments, "cubs @ padres")
	if err != nil || idx != 1 {
		t.Errorf("expected case-insensitive match, got idx=%d err=%v", idx, err)
	}
}

func TestEvaluate_FiltersAlreadyPlayingDates(t *testing.T) {
	cfg := testConfig()
	game := Game{Away: "Astros", Home: "Angels"}

	// Angels already plays on 04/27 → that date should be filtered out
	// entirely (not returned as a FAIL candidate).
	others := []Assignment{
		{Game: Game{Away: "Mariners", Home: "Angels"}, Slot: schedule.Slot{Date: mustDate("2026-04-27"), Time: "17:45", Field: "Field1"}},
	}
	candidates := []schedule.Slot{
		{Date: mustDate("2026-04-27"), Time: "17:45", Field: "Field1"},
		{Date: mustDate("2026-04-28"), Time: "17:45", Field: "Field1"},
	}

	got := Evaluate(cfg, others, game, candidates)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate (04/27 filtered out), got %d", len(got))
	}
	if !got[0].Slot.Date.Equal(mustDate("2026-04-28")) {
		t.Errorf("expected 04/28 to remain, got %s", got[0].Slot.Date.Format("01/02"))
	}
	if got[0].Status != StatusOK {
		t.Errorf("04/28 should be OK, got %s with reasons %v %v", got[0].Status, got[0].HardReasons, got[0].SoftReasons)
	}
}

func TestEvaluate_TimeslotCap(t *testing.T) {
	cfg := testConfig()
	game := Game{Away: "Mariners", Home: "Athletics"}

	others := []Assignment{
		{Game: Game{Away: "Astros", Home: "Angels"}, Slot: schedule.Slot{Date: mustDate("2026-05-02"), Time: "12:30", Field: "Field1"}},
		{Game: Game{Away: "Cubs", Home: "Padres"}, Slot: schedule.Slot{Date: mustDate("2026-05-02"), Time: "12:30", Field: "Field2"}},
	}
	candidates := []schedule.Slot{
		{Date: mustDate("2026-05-02"), Time: "12:30", Field: "Field3"},
	}
	got := Evaluate(cfg, others, game, candidates)
	if len(got) != 0 {
		t.Errorf("expected slot at timeslot cap to be filtered out, got %d candidates", len(got))
	}
}

func TestEvaluate_RematchSoftWarning(t *testing.T) {
	cfg := testConfig()
	game := Game{Away: "Astros", Home: "Angels"}

	others := []Assignment{
		{Game: Game{Away: "Astros", Home: "Angels"}, Slot: schedule.Slot{Date: mustDate("2026-04-27"), Time: "17:45", Field: "Field1"}},
	}
	candidates := []schedule.Slot{
		{Date: mustDate("2026-04-30"), Time: "17:45", Field: "Field1"}, // 3 days later
	}
	got := Evaluate(cfg, others, game, candidates)
	if got[0].Status != StatusWarn {
		t.Fatalf("expected WARN for early rematch, got %s (hard=%v soft=%v)",
			got[0].Status, got[0].HardReasons, got[0].SoftReasons)
	}
	if len(got[0].SoftReasons) == 0 || !strings.Contains(got[0].SoftReasons[0], "rematch") {
		t.Errorf("expected rematch warning, got %v", got[0].SoftReasons)
	}
}

// withConstraintMode returns cfg with a single constraint's reschedule-mode
// severity overridden, leaving every other constraint at its default.
func withConstraintMode(cfg *config.Config, name string, sev config.Severity) *config.Config {
	if cfg.Constraints == nil {
		cfg.Constraints = make(map[string]config.Constraint)
	}
	cfg.Constraints[name] = config.Constraint{
		Value: 1, // value is irrelevant; routing reads severity only
		Mode:  map[config.Mode]config.Severity{config.ModeReschedule: sev},
	}
	return cfg
}

func TestEvaluate_3In4DowngradedToGuideline(t *testing.T) {
	cfg := testConfig()
	withConstraintMode(cfg, "max_3_in_4_days", config.SeverityGuideline)
	game := Game{Away: "Astros", Home: "Angels"}

	// Astros already played on 04/27 and 04/29. A 3rd game on 04/30 would
	// trigger the 3-in-4 rule.
	others := []Assignment{
		{Game: Game{Away: "Astros", Home: "Mariners"}, Slot: schedule.Slot{Date: mustDate("2026-04-27"), Time: "17:45", Field: "Field1"}},
		{Game: Game{Away: "Astros", Home: "Athletics"}, Slot: schedule.Slot{Date: mustDate("2026-04-29"), Time: "17:45", Field: "Field1"}},
	}
	candidates := []schedule.Slot{
		{Date: mustDate("2026-04-30"), Time: "17:45", Field: "Field1"},
	}
	got := Evaluate(cfg, others, game, candidates)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	if got[0].Status != StatusWarn {
		t.Fatalf("expected WARN under guideline, got %s (hard=%v soft=%v)",
			got[0].Status, got[0].HardReasons, got[0].SoftReasons)
	}
	if len(got[0].SoftReasons) == 0 || !strings.Contains(strings.Join(got[0].SoftReasons, "|"), "3 games in 4 days") {
		t.Errorf("expected 3-in-4 soft reason, got %v", got[0].SoftReasons)
	}
	if len(got[0].HardReasons) != 0 {
		t.Errorf("expected no hard reasons, got %v", got[0].HardReasons)
	}
}

func TestEvaluate_3In4StaysHardByDefault(t *testing.T) {
	cfg := testConfig() // no constraints map -> registry default (rule)
	game := Game{Away: "Astros", Home: "Angels"}

	others := []Assignment{
		{Game: Game{Away: "Astros", Home: "Mariners"}, Slot: schedule.Slot{Date: mustDate("2026-04-27"), Time: "17:45", Field: "Field1"}},
		{Game: Game{Away: "Astros", Home: "Athletics"}, Slot: schedule.Slot{Date: mustDate("2026-04-29"), Time: "17:45", Field: "Field1"}},
	}
	candidates := []schedule.Slot{
		{Date: mustDate("2026-04-30"), Time: "17:45", Field: "Field1"},
	}
	got := Evaluate(cfg, others, game, candidates)
	if got[0].Status != StatusFail {
		t.Fatalf("expected FAIL by default, got %s (hard=%v soft=%v)",
			got[0].Status, got[0].HardReasons, got[0].SoftReasons)
	}
}

func TestEvaluate_RematchDisabled(t *testing.T) {
	cfg := testConfig()
	withConstraintMode(cfg, "min_days_between_same_matchup", config.SeverityDisabled)
	game := Game{Away: "Astros", Home: "Angels"}

	others := []Assignment{
		{Game: Game{Away: "Astros", Home: "Angels"}, Slot: schedule.Slot{Date: mustDate("2026-04-27"), Time: "17:45", Field: "Field1"}},
	}
	candidates := []schedule.Slot{
		{Date: mustDate("2026-04-30"), Time: "17:45", Field: "Field1"},
	}
	got := Evaluate(cfg, others, game, candidates)
	if got[0].Status != StatusOK {
		t.Fatalf("expected OK with rematch disabled, got %s (hard=%v soft=%v)",
			got[0].Status, got[0].HardReasons, got[0].SoftReasons)
	}
}

func TestEvaluate_MaxConsecutiveStaysRule(t *testing.T) {
	cfg := testConfig()
	game := Game{Away: "Astros", Home: "Angels"}

	// Astros played 04/27 and 04/28. A game on 04/29 would be 3 in a row.
	others := []Assignment{
		{Game: Game{Away: "Astros", Home: "Mariners"}, Slot: schedule.Slot{Date: mustDate("2026-04-27"), Time: "17:45", Field: "Field1"}},
		{Game: Game{Away: "Astros", Home: "Athletics"}, Slot: schedule.Slot{Date: mustDate("2026-04-28"), Time: "17:45", Field: "Field1"}},
	}
	candidates := []schedule.Slot{
		{Date: mustDate("2026-04-29"), Time: "17:45", Field: "Field1"},
	}
	got := Evaluate(cfg, others, game, candidates)
	if got[0].Status != StatusFail {
		t.Fatalf("expected FAIL for consecutive-days violation, got %s",
			got[0].Status)
	}
}

func TestSlotsAfter(t *testing.T) {
	slots := []schedule.Slot{
		{Date: mustDate("2026-04-27"), Time: "17:45", Field: "F1"},
		{Date: mustDate("2026-04-28"), Time: "17:45", Field: "F1"},
		{Date: mustDate("2026-04-29"), Time: "17:45", Field: "F1"},
	}
	got := SlotsAfter(slots, mustDate("2026-04-28"))
	if len(got) != 1 {
		t.Fatalf("expected 1 slot strictly after 04/28, got %d", len(got))
	}
	if !got[0].Date.Equal(mustDate("2026-04-29")) {
		t.Errorf("expected 04/29, got %s", got[0].Date.Format("2006-01-02"))
	}
}

func TestCandidateSlots_DedupesByDateTime(t *testing.T) {
	cfg := testConfig()
	pool := CandidateSlots(cfg, nil)

	seen := make(map[string]int)
	for _, s := range pool {
		key := s.Date.Format("2006-01-02") + "|" + s.Time
		seen[key]++
	}
	for key, n := range seen {
		if n > 1 {
			t.Errorf("(date,time) %s appears %d times; expected 1 (field should be deduped)", key, n)
		}
	}
}

func TestCandidateSlots_IncludesMakeupSlots(t *testing.T) {
	cfg := testConfig()
	cfg.TimeSlots.MakeupSunday = []string{"14:45"}

	pool := CandidateSlots(cfg, nil)

	found := false
	for _, s := range pool {
		if s.Date.Weekday() == time.Sunday && s.Time == "14:45" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Sunday 14:45 makeup slot to be available for rescheduling")
	}
}

func TestCandidateSlots_FieldLabeling(t *testing.T) {
	cfg := testConfig()

	// Reserve Field2 on a particular Tuesday so only Field1 is open at 17:45.
	tuesday := mustDate("2026-04-28")
	existing := []Assignment{
		{Game: Game{Away: "Astros", Home: "Angels"},
			Slot: schedule.Slot{Date: tuesday, Time: "17:45", Field: "Field2"}},
	}

	pool := CandidateSlots(cfg, existing)

	var tueLabel, otherLabel string
	for _, s := range pool {
		if s.Time != "17:45" {
			continue
		}
		if s.Date.Equal(tuesday) {
			tueLabel = s.Field
		} else if otherLabel == "" {
			otherLabel = s.Field
		}
	}
	if tueLabel != "Field1" {
		t.Errorf("Tuesday 17:45 should show Field1 (only one open), got %q", tueLabel)
	}
	if otherLabel != "Either" {
		t.Errorf("a slot with both fields open should show \"Either\", got %q", otherLabel)
	}
}

func TestMergeSlots(t *testing.T) {
	got := mergeSlots([]string{"17:00"}, []string{"17:00", "14:45"})
	if len(got) != 2 || got[0] != "17:00" || got[1] != "14:45" {
		t.Errorf("expected [17:00 14:45], got %v", got)
	}
	got = mergeSlots([]string{"a", "b"}, []string{"c"})
	if len(got) != 3 || got[2] != "c" {
		t.Errorf("expected append, got %v", got)
	}
	got = mergeSlots([]string{"a"}, nil)
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("expected unchanged copy, got %v", got)
	}
}

func TestConsecRun(t *testing.T) {
	dates := []time.Time{
		mustDate("2026-04-27"),
		mustDate("2026-04-29"),
	}
	if got := consecRun(dates, mustDate("2026-04-28")); got != 3 {
		t.Errorf("inserting 04/28 between 04/27 and 04/29 → run %d, want 3", got)
	}
	if got := consecRun(dates, mustDate("2026-05-05")); got != 1 {
		t.Errorf("isolated insert → run %d, want 1", got)
	}
}

func TestResolveFieldName(t *testing.T) {
	cfg := &config.Config{
		Fields: []config.Field{
			{Name: "Washington Park", ShortName: "Washington"},
			{Name: "Symonds Field", ShortName: "Symonds"},
			{Name: "Moscariello Ballpark"}, // no short_name → header must use full name
		},
	}
	cases := map[string]string{
		"Washington":           "Washington Park",
		"Symonds":              "Symonds Field",
		"Moscariello Ballpark": "Moscariello Ballpark",
		"Washington Park":      "Washington Park",
		"Unknown":              "Unknown",
	}
	for in, want := range cases {
		if got := resolveFieldName(in, cfg); got != want {
			t.Errorf("resolveFieldName(%q) = %q, want %q", in, got, want)
		}
	}
}
