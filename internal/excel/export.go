package excel

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/xuri/excelize/v2"
)

// Export reads a schedule Excel file and writes a GameChanger-formatted CSV
// with columns: date, time, home, away, location, duration.
func Export(cfg *config.Config, schedulePath string, w io.Writer) error {
	sf, err := excelize.OpenFile(schedulePath)
	if err != nil {
		return fmt.Errorf("opening schedule: %w", err)
	}
	defer sf.Close()

	games, err := readGamesFromMaster(sf)
	if err != nil {
		return fmt.Errorf("reading schedule: %w", err)
	}

	sort.Slice(games, func(i, j int) bool {
		if !games[i].Date.Equal(games[j].Date) {
			return games[i].Date.Before(games[j].Date)
		}
		return games[i].Time < games[j].Time
	})

	fieldAddresses := buildFieldAddressMap(cfg)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{"date", "time", "home", "away", "location", "duration"})

	for _, g := range games {
		home := cfg.GameChanger.TeamNames[g.Home]
		away := cfg.GameChanger.TeamNames[g.Away]
		location := fieldAddresses[g.Field]
		timeStr := convertTo12Hour(g.Time)

		cw.Write([]string{
			g.Date.Format("01/02/2006"),
			timeStr,
			home,
			away,
			location,
			"120",
		})
	}

	return nil
}

// buildFieldAddressMap maps field column header names to their addresses.
func buildFieldAddressMap(cfg *config.Config) map[string]string {
	var fullNames []string
	for _, f := range cfg.Fields {
		fullNames = append(fullNames, f.Name)
	}

	m := make(map[string]string)
	for _, f := range cfg.Fields {
		colName := fieldColumnName(f.Name, fullNames)
		m[colName] = f.Address
	}
	return m
}

// convertTo12Hour converts a 24-hour time string (HH:MM) to 12-hour format.
func convertTo12Hour(t string) string {
	parsed, err := time.Parse("15:04", t)
	if err != nil {
		return t
	}
	return parsed.Format("3:04 PM")
}
