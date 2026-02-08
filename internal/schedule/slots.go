package schedule

import (
	"sort"
	"time"

	"github.com/derekprior/rbrl/internal/config"
)

// Slot represents an available game slot: a date, time, and field.
type Slot struct {
	Date  time.Time
	Time  string // "17:45", "12:30", etc.
	Field string
}

// BlackoutSlot represents a slot that is unavailable with a reason.
type BlackoutSlot struct {
	Date   time.Time
	Time   string
	Field  string
	Reason string
}

// GenerateSlots builds all available (date, time, field) tuples for the season,
// excluding blackout dates and field reservations.
func GenerateSlots(cfg *config.Config) []Slot {
	blackoutDates := make(map[time.Time]bool)
	for _, b := range cfg.Season.BlackoutDates {
		blackoutDates[b.Date.Time] = true
	}

	holidayDates := make(map[time.Time]bool)
	for _, h := range cfg.TimeSlots.HolidayDates {
		holidayDates[h.Time] = true
	}

	// Build reservation lookup: field+date+time -> true
	type resKey struct {
		field string
		date  time.Time
		time  string
	}
	reservations := make(map[resKey]bool)
	for _, f := range cfg.Fields {
		for _, r := range f.Reservations {
			for _, t := range r.Times {
				reservations[resKey{f.Name, r.Date.Time, t}] = true
			}
		}
	}

	var slots []Slot
	d := cfg.Season.StartDate.Time
	for !d.After(cfg.Season.EndDate.Time) {
		if blackoutDates[d] {
			d = d.AddDate(0, 0, 1)
			continue
		}

		times := timesForDay(d, holidayDates, cfg.TimeSlots)

		for _, t := range times {
			for _, f := range cfg.Fields {
				if reservations[resKey{f.Name, d, t}] {
					continue
				}
				slots = append(slots, Slot{Date: d, Time: t, Field: f.Name})
			}
		}

		d = d.AddDate(0, 0, 1)
	}

	sort.Slice(slots, func(i, j int) bool {
		if !slots[i].Date.Equal(slots[j].Date) {
			return slots[i].Date.Before(slots[j].Date)
		}
		if slots[i].Time != slots[j].Time {
			return slots[i].Time < slots[j].Time
		}
		return slots[i].Field < slots[j].Field
	})

	return slots
}

// GenerateBlackoutSlots returns all slots that are blacked out (season-wide
// blackouts and field reservations) for display on the master sheet.
func GenerateBlackoutSlots(cfg *config.Config) []BlackoutSlot {
	holidayDates := make(map[time.Time]bool)
	for _, h := range cfg.TimeSlots.HolidayDates {
		holidayDates[h.Time] = true
	}

	var blackouts []BlackoutSlot

	// Season-wide blackout dates
	for _, b := range cfg.Season.BlackoutDates {
		times := timesForDay(b.Date.Time, holidayDates, cfg.TimeSlots)
		for _, t := range times {
			for _, f := range cfg.Fields {
				blackouts = append(blackouts, BlackoutSlot{
					Date:   b.Date.Time,
					Time:   t,
					Field:  f.Name,
					Reason: b.Reason,
				})
			}
		}
	}

	// Field reservations
	for _, f := range cfg.Fields {
		for _, r := range f.Reservations {
			for _, t := range r.Times {
				blackouts = append(blackouts, BlackoutSlot{
					Date:   r.Date.Time,
					Time:   t,
					Field:  f.Name,
					Reason: r.Reason,
				})
			}
		}
	}

	sort.Slice(blackouts, func(i, j int) bool {
		if !blackouts[i].Date.Equal(blackouts[j].Date) {
			return blackouts[i].Date.Before(blackouts[j].Date)
		}
		if blackouts[i].Time != blackouts[j].Time {
			return blackouts[i].Time < blackouts[j].Time
		}
		return blackouts[i].Field < blackouts[j].Field
	})

	return blackouts
}

func timesForDay(d time.Time, holidays map[time.Time]bool, ts config.TimeSlots) []string {
	if holidays[d] {
		return ts.Sunday
	}
	switch d.Weekday() {
	case time.Saturday:
		return ts.Saturday
	case time.Sunday:
		return ts.Sunday
	default:
		return ts.Weekday
	}
}
