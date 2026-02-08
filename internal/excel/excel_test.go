package excel

import (
	"strings"
	"testing"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/schedule"
	"github.com/derekprior/rbrl/internal/strategy"
	"github.com/xuri/excelize/v2"
)

func date(y, m, d int) config.Date {
	return config.Date{Time: time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)}
}

func testData() (*config.Config, *schedule.Result) {
	cfg := &config.Config{
		Season: config.Season{
			StartDate: date(2026, 4, 25),
			EndDate:   date(2026, 5, 31),
			BlackoutDates: []config.BlackoutDate{
				{Date: date(2026, 5, 10), Reason: "Mother's Day"},
			},
		},
		Divisions: []config.Division{
			{Name: "American", Teams: []string{"Angels", "Astros"}},
			{Name: "National", Teams: []string{"Cubs", "Padres"}},
		},
		Fields: []config.Field{
			{Name: "Field A"},
			{Name: "Field B"},
		},
		TimeSlots: config.TimeSlots{
			Weekday:  []string{"17:45"},
			Saturday: []string{"12:30"},
			Sunday:   []string{"17:00"},
		},
		Rules: config.Rules{
			MaxGamesPerDayPerTeam: 1,
			MaxConsecutiveDays:    2,
			MaxGamesPerWeek:       3,
			MaxGamesPerTimeslot:   2,
		},
	}

	result := &schedule.Result{
		Assignments: []schedule.Assignment{
			{
				Game: strategy.Game{Home: "Angels", Away: "Cubs", Label: "Game 1"},
				Slot: schedule.Slot{Date: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Time: "12:30", Field: "Field A"},
			},
			{
				Game: strategy.Game{Home: "Astros", Away: "Padres", Label: "Game 2"},
				Slot: schedule.Slot{Date: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Time: "12:30", Field: "Field B"},
			},
		},
		Warnings: []string{"test warning"},
	}

	return cfg, result
}

func TestGenerateWorkbook(t *testing.T) {
	cfg, result := testData()
	slots := schedule.GenerateSlots(cfg)
	blackouts := schedule.GenerateBlackoutSlots(cfg)

	f, err := Generate(cfg, result, slots, blackouts)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	t.Run("has Master Schedule sheet", func(t *testing.T) {
		idx, err := f.GetSheetIndex("Master Schedule")
		if err != nil {
			t.Fatalf("GetSheetIndex error: %v", err)
		}
		if idx < 0 {
			t.Error("Master Schedule sheet not found")
		}
	})

	t.Run("master sheet has headers", func(t *testing.T) {
		val, _ := f.GetCellValue("Master Schedule", "A1")
		if val != "Date" {
			t.Errorf("A1 = %q, want Date", val)
		}
		// Field columns start at D — "Field" isn't unique so full names used
		val, _ = f.GetCellValue("Master Schedule", "D1")
		if val != "Field A" {
			t.Errorf("D1 = %q, want Field A", val)
		}
		val, _ = f.GetCellValue("Master Schedule", "E1")
		if val != "Field B" {
			t.Errorf("E1 = %q, want Field B", val)
		}
	})

	t.Run("master sheet has game rows", func(t *testing.T) {
		found := false
		rows, _ := f.GetRows("Master Schedule")
		for _, row := range rows[1:] {
			for i := 3; i < len(row); i++ {
				if row[i] == "Cubs @ Angels" {
					found = true
					break
				}
			}
		}
		if !found {
			t.Error("Cubs @ Angels game not found in master sheet")
		}
	})

	t.Run("master sheet has blackout rows", func(t *testing.T) {
		found := false
		rows, _ := f.GetRows("Master Schedule")
		for _, row := range rows[1:] {
			for i := 3; i < len(row); i++ {
				if row[i] == "Mother's Day" {
					found = true
					break
				}
			}
		}
		if !found {
			t.Error("Mother's Day blackout not found in master sheet")
		}
	})

	t.Run("has per-team sheets", func(t *testing.T) {
		for _, team := range []string{"Angels", "Astros", "Cubs", "Padres"} {
			idx, err := f.GetSheetIndex(team)
			if err != nil {
				t.Fatalf("GetSheetIndex error: %v", err)
			}
			if idx < 0 {
				t.Errorf("sheet for %s not found", team)
			}
		}
	})

	t.Run("team sheet has formula", func(t *testing.T) {
		formula, _ := f.GetCellFormula("Angels", "A2")
		if formula == "" {
			t.Error("Angels sheet A2 should have a formula")
		}
		if !strings.Contains(formula, "FILTER") || !strings.Contains(formula, "Angels") {
			t.Errorf("formula should reference FILTER and team name, got: %s", formula)
		}
	})

	t.Run("default Sheet1 removed", func(t *testing.T) {
		idx, _ := f.GetSheetIndex("Sheet1")
		if idx >= 0 {
			t.Error("Sheet1 should be removed")
		}
	})
}

func TestWriteAndRead(t *testing.T) {
	cfg, result := testData()
	slots := schedule.GenerateSlots(cfg)
	blackouts := schedule.GenerateBlackoutSlots(cfg)

	f, err := Generate(cfg, result, slots, blackouts)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	path := t.TempDir() + "/test.xlsx"
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs error: %v", err)
	}

	// Verify we can read it back
	f2, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile error: %v", err)
	}
	defer f2.Close()

	val, _ := f2.GetCellValue("Master Schedule", "A1")
	if val != "Date" {
		t.Errorf("re-read A1 = %q, want Date", val)
	}
}
