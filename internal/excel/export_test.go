package excel

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/schedule"
)

func exportTestData() *config.Config {
	cfg, _ := testData()
	cfg.Fields = []config.Field{
		{Name: "Field A", Address: "123 Main St, Reading, MA 01867"},
		{Name: "Field B", Address: "456 Oak Ave, Reading, MA 01867"},
	}
	cfg.GameChanger = config.GameChanger{
		TeamNames: map[string]string{
			"Angels": "RBRL Angels",
			"Astros": "Astros",
			"Cubs":   "Cubs",
			"Padres": "RBRL Padres",
		},
	}
	return cfg
}

func generateScheduleFile(t *testing.T, cfg *config.Config) string {
	t.Helper()
	_, result := testData()
	slots := schedule.GenerateSlots(cfg)
	blackouts := schedule.GenerateBlackoutSlots(cfg)

	genFile, err := Generate(cfg, result, slots, blackouts)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	path := t.TempDir() + "/schedule.xlsx"
	if err := genFile.SaveAs(path); err != nil {
		t.Fatalf("SaveAs error: %v", err)
	}
	return path
}

func TestExport(t *testing.T) {
	cfg := exportTestData()
	schedulePath := generateScheduleFile(t, cfg)

	var buf bytes.Buffer
	if err := Export(cfg, schedulePath, &buf); err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("reading CSV: %v", err)
	}

	t.Run("headers", func(t *testing.T) {
		want := []string{"date", "time", "home", "away", "location", "duration"}
		for i, h := range want {
			if records[0][i] != h {
				t.Errorf("header %d = %q, want %q", i, records[0][i], h)
			}
		}
	})

	t.Run("game count", func(t *testing.T) {
		// Header + 2 games
		if len(records) != 3 {
			t.Errorf("row count = %d, want 3", len(records))
		}
	})

	t.Run("date format", func(t *testing.T) {
		if records[1][0] != "04/25/2026" {
			t.Errorf("date = %q, want 04/25/2026", records[1][0])
		}
	})

	t.Run("time format", func(t *testing.T) {
		if records[1][1] != "12:30 PM" {
			t.Errorf("time = %q, want 12:30 PM", records[1][1])
		}
	})

	t.Run("gamechanger team names", func(t *testing.T) {
		// Game 1: Cubs @ Angels at Field A
		if records[1][2] != "RBRL Angels" {
			t.Errorf("home = %q, want RBRL Angels", records[1][2])
		}
		if records[1][3] != "Cubs" {
			t.Errorf("away = %q, want Cubs", records[1][3])
		}

		// Game 2: Padres @ Astros at Field B
		if records[2][2] != "Astros" {
			t.Errorf("home = %q, want Astros", records[2][2])
		}
		if records[2][3] != "RBRL Padres" {
			t.Errorf("away = %q, want RBRL Padres", records[2][3])
		}
	})

	t.Run("location is field address", func(t *testing.T) {
		if records[1][4] != "123 Main St, Reading, MA 01867" {
			t.Errorf("location = %q, want field address", records[1][4])
		}
	})

	t.Run("duration is 120", func(t *testing.T) {
		if records[1][5] != "120" {
			t.Errorf("duration = %q, want 120", records[1][5])
		}
	})
}

func TestConvertTo12Hour(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"17:45", "5:45 PM"},
		{"12:30", "12:30 PM"},
		{"14:45", "2:45 PM"},
		{"17:00", "5:00 PM"},
		{"00:00", "12:00 AM"},
		{"09:30", "9:30 AM"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertTo12Hour(tt.input)
			if got != tt.want {
				t.Errorf("convertTo12Hour(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
