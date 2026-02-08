package schedule

import (
	"testing"
	"time"

	"github.com/derekprior/rbrl/internal/config"
)

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func date(y, m, d int) config.Date {
	return config.Date{Time: time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)}
}

func datePtr(y, m, d int) *config.Date {
	dt := date(y, m, d)
	return &dt
}

func testConfig() *config.Config {
	return &config.Config{
		Season: config.Season{
			StartDate: date(2026, 4, 25), // Saturday
			EndDate:   date(2026, 5, 31), // Sunday
			BlackoutDates: []config.BlackoutDate{
				{Date: date(2026, 5, 10), Reason: "Mother's Day"},
				{Date: date(2026, 5, 23), Reason: "Memorial Day Weekend"},
				{Date: date(2026, 5, 24), Reason: "Memorial Day Weekend"},
				{Date: date(2026, 5, 25), Reason: "Memorial Day"},
			},
		},
		Fields: []config.Field{
			{
				Name: "Moscariello Ballpark",
				Reservations: []config.Reservation{
					{
						Date:   datePtr(2026, 5, 15),
						Times:  []string{"17:45"},
						Reason: "Varsity",
					},
					{
						StartDate: datePtr(2026, 5, 18),
						EndDate:   datePtr(2026, 5, 20),
						Reason:    "Tournament",
					},
				},
			},
			{
				Name: "Symonds Field",
				Reservations: []config.Reservation{
					{
						Date:   datePtr(2026, 5, 2),
						Reason: "Reserved",
					},
				},
			},
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
	}
}

func TestGenerateSlots(t *testing.T) {
	cfg := testConfig()
	slots := GenerateSlots(cfg)

	t.Run("no slots on blackout dates", func(t *testing.T) {
		blackouts := map[time.Time]bool{
			mustDate("2026-05-10"): true,
			mustDate("2026-05-23"): true,
			mustDate("2026-05-24"): true,
			mustDate("2026-05-25"): true,
		}
		for _, s := range slots {
			if blackouts[s.Date] {
				t.Errorf("found slot on blackout date %s", s.Date.Format("2006-01-02"))
			}
		}
	})

	t.Run("weekday slots use weekday times", func(t *testing.T) {
		// Monday April 27
		mon := mustDate("2026-04-27")
		var monSlots []Slot
		for _, s := range slots {
			if s.Date.Equal(mon) {
				monSlots = append(monSlots, s)
			}
		}
		// 3 fields × 1 time = 3 slots
		if len(monSlots) != 3 {
			t.Errorf("Monday slots = %d, want 3", len(monSlots))
		}
		for _, s := range monSlots {
			if s.Time != "17:45" {
				t.Errorf("Monday slot time = %q, want 17:45", s.Time)
			}
		}
	})

	t.Run("saturday slots use saturday times", func(t *testing.T) {
		// Saturday April 25 (opening day)
		sat := mustDate("2026-04-25")
		var satSlots []Slot
		for _, s := range slots {
			if s.Date.Equal(sat) {
				satSlots = append(satSlots, s)
			}
		}
		// 3 fields × 3 times = 9 slots
		if len(satSlots) != 9 {
			t.Errorf("Saturday slots = %d, want 9", len(satSlots))
		}
	})

	t.Run("sunday slots use sunday times", func(t *testing.T) {
		// Sunday April 26
		sun := mustDate("2026-04-26")
		var sunSlots []Slot
		for _, s := range slots {
			if s.Date.Equal(sun) {
				sunSlots = append(sunSlots, s)
			}
		}
		// 3 fields × 1 time = 3 slots
		if len(sunSlots) != 3 {
			t.Errorf("Sunday slots = %d, want 3", len(sunSlots))
		}
		for _, s := range sunSlots {
			if s.Time != "17:00" {
				t.Errorf("Sunday slot time = %q, want 17:00", s.Time)
			}
		}
	})

	t.Run("full-day reservation removes all slots for that field", func(t *testing.T) {
		// May 2 is a Saturday. Symonds has a full-day reservation (no times specified).
		sat := mustDate("2026-05-02")
		var satSymonds []Slot
		for _, s := range slots {
			if s.Date.Equal(sat) && s.Field == "Symonds Field" {
				satSymonds = append(satSymonds, s)
			}
		}
		if len(satSymonds) != 0 {
			t.Errorf("expected no Symonds slots on 5/2, got %d", len(satSymonds))
		}
		// Other fields should still have Saturday slots
		var satOther []Slot
		for _, s := range slots {
			if s.Date.Equal(sat) && s.Field != "Symonds Field" {
				satOther = append(satOther, s)
			}
		}
		if len(satOther) != 6 { // 2 fields × 3 Saturday times
			t.Errorf("other field slots on 5/2 = %d, want 6", len(satOther))
		}
	})

	t.Run("date range reservation removes all days in range", func(t *testing.T) {
		// May 18-20 (Mon-Wed) Moscariello has a Tournament reservation
		for _, ds := range []string{"2026-05-18", "2026-05-19", "2026-05-20"} {
			d := mustDate(ds)
			var moscSlots []Slot
			for _, s := range slots {
				if s.Date.Equal(d) && s.Field == "Moscariello Ballpark" {
					moscSlots = append(moscSlots, s)
				}
			}
			if len(moscSlots) != 0 {
				t.Errorf("expected no Moscariello slots on %s, got %d", ds, len(moscSlots))
			}
		}
	})

	t.Run("field reservations remove specific slots", func(t *testing.T) {
		// May 15 is a Friday. Moscariello at 17:45 is reserved.
		fri := mustDate("2026-05-15")
		var friMosc []Slot
		for _, s := range slots {
			if s.Date.Equal(fri) && s.Field == "Moscariello Ballpark" {
				friMosc = append(friMosc, s)
			}
		}
		if len(friMosc) != 0 {
			t.Errorf("expected no Moscariello slots on 5/15, got %d", len(friMosc))
		}
		// Other fields should still have slots
		var friOther []Slot
		for _, s := range slots {
			if s.Date.Equal(fri) && s.Field != "Moscariello Ballpark" {
				friOther = append(friOther, s)
			}
		}
		if len(friOther) != 2 {
			t.Errorf("other field slots on 5/15 = %d, want 2", len(friOther))
		}
	})

	t.Run("slots are sorted by date then time then field", func(t *testing.T) {
		for i := 1; i < len(slots); i++ {
			prev, curr := slots[i-1], slots[i]
			if curr.Date.Before(prev.Date) {
				t.Errorf("slot %d date %s before slot %d date %s",
					i, curr.Date.Format("2006-01-02"), i-1, prev.Date.Format("2006-01-02"))
			}
			if curr.Date.Equal(prev.Date) && curr.Time < prev.Time {
				t.Errorf("slot %d time %s before slot %d time %s on %s",
					i, curr.Time, i-1, prev.Time, curr.Date.Format("2006-01-02"))
			}
		}
	})
}

func TestGenerateBlackoutSlots(t *testing.T) {
	cfg := testConfig()
	blackouts := GenerateBlackoutSlots(cfg)

	t.Run("blackout slots exist for blackout dates", func(t *testing.T) {
		dates := make(map[time.Time]bool)
		for _, b := range blackouts {
			dates[b.Date] = true
		}
		if !dates[mustDate("2026-05-10")] {
			t.Error("missing blackout slots for Mother's Day")
		}
		if !dates[mustDate("2026-05-23")] {
			t.Error("missing blackout slots for Memorial Day Weekend Saturday")
		}
	})

	t.Run("blackout slots include reason", func(t *testing.T) {
		for _, b := range blackouts {
			if b.Reason == "" {
				t.Errorf("blackout slot on %s has no reason", b.Date.Format("2006-01-02"))
			}
		}
	})

	t.Run("reservation slots included as blackouts", func(t *testing.T) {
		found := false
		for _, b := range blackouts {
			if b.Date.Equal(mustDate("2026-05-15")) && b.Field == "Moscariello Ballpark" {
				found = true
				if b.Reason != "Varsity" {
					t.Errorf("reservation reason = %q, want Varsity", b.Reason)
				}
			}
		}
		if !found {
			t.Error("missing reservation blackout for Moscariello on 5/15")
		}
	})
}
