package excel

import (
	"fmt"
	"sort"
	"time"

	"github.com/derekprior/rbrl/internal/config"
	"github.com/derekprior/rbrl/internal/schedule"
	"github.com/xuri/excelize/v2"
)

// Generate creates an Excel workbook with the master schedule and per-team sheets.
func Generate(cfg *config.Config, result *schedule.Result, slots []schedule.Slot, blackouts []schedule.BlackoutSlot) (*excelize.File, error) {
	f := excelize.NewFile()

	if err := writeMasterSheet(f, cfg, result, slots, blackouts); err != nil {
		return nil, fmt.Errorf("writing master sheet: %w", err)
	}

	if err := writeTeamSheets(f, cfg, result); err != nil {
		return nil, fmt.Errorf("writing team sheets: %w", err)
	}

	f.DeleteSheet("Sheet1")
	return f, nil
}

func writeMasterSheet(f *excelize.File, cfg *config.Config, result *schedule.Result, slots []schedule.Slot, blackouts []schedule.BlackoutSlot) error {
	sheet := "Master Schedule"
	f.NewSheet(sheet)

	headers := []string{"Date", "Day", "Time", "Field", "Home", "Away", "Game", "Notes"}
	for i, h := range headers {
		cell := cellRef(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Style for headers
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if headerStyle != 0 {
		for i := range headers {
			f.SetCellStyle(sheet, cellRef(i+1, 1), cellRef(i+1, 1), headerStyle)
		}
	}

	// Style for blackout rows
	blackoutStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#D9D9D9"}},
		Font: &excelize.Font{Color: "#808080", Italic: true},
	})

	// Build assignment lookup: slot -> assignment
	type slotKey struct {
		date  time.Time
		time  string
		field string
	}
	assignmentMap := make(map[slotKey]schedule.Assignment)
	for _, a := range result.Assignments {
		sk := slotKey{a.Slot.Date, a.Slot.Time, a.Slot.Field}
		assignmentMap[sk] = a
	}

	// Build blackout lookup: slot -> reason
	blackoutMap := make(map[slotKey]string)
	for _, b := range blackouts {
		sk := slotKey{b.Date, b.Time, b.Field}
		blackoutMap[sk] = b.Reason
	}

	// Merge all slots and blackout slots into a unified row list
	type rowData struct {
		date    time.Time
		time    string
		field   string
		isBlack bool
		reason  string
		assign  *schedule.Assignment
	}

	var rows []rowData

	// Available slots
	for _, s := range slots {
		sk := slotKey{s.Date, s.Time, s.Field}
		rd := rowData{date: s.Date, time: s.Time, field: s.Field}
		if a, ok := assignmentMap[sk]; ok {
			rd.assign = &a
		}
		rows = append(rows, rd)
	}

	// Blackout slots
	for _, b := range blackouts {
		rows = append(rows, rowData{
			date:    b.Date,
			time:    b.Time,
			field:   b.Field,
			isBlack: true,
			reason:  b.Reason,
		})
	}

	// Sort by date, time, field
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].date.Equal(rows[j].date) {
			return rows[i].date.Before(rows[j].date)
		}
		if rows[i].time != rows[j].time {
			return rows[i].time < rows[j].time
		}
		return rows[i].field < rows[j].field
	})

	for i, rd := range rows {
		row := i + 2 // 1-indexed, skip header
		f.SetCellValue(sheet, cellRef(1, row), rd.date.Format("01/02/2006"))
		f.SetCellValue(sheet, cellRef(2, row), rd.date.Format("Mon"))
		f.SetCellValue(sheet, cellRef(3, row), rd.time)
		f.SetCellValue(sheet, cellRef(4, row), rd.field)

		if rd.isBlack {
			f.SetCellValue(sheet, cellRef(8, row), rd.reason)
			if blackoutStyle != 0 {
				for col := 1; col <= len(headers); col++ {
					f.SetCellStyle(sheet, cellRef(col, row), cellRef(col, row), blackoutStyle)
				}
			}
		} else if rd.assign != nil {
			f.SetCellValue(sheet, cellRef(5, row), rd.assign.Game.Home)
			f.SetCellValue(sheet, cellRef(6, row), rd.assign.Game.Away)
			f.SetCellValue(sheet, cellRef(7, row), rd.assign.Game.Label)
		}
	}

	// Set column widths
	widths := map[string]float64{"A": 12, "B": 6, "C": 8, "D": 22, "E": 12, "F": 12, "G": 10, "H": 24}
	for col, w := range widths {
		f.SetColWidth(sheet, col, col, w)
	}

	return nil
}

func writeTeamSheets(f *excelize.File, cfg *config.Config, result *schedule.Result) error {
	for _, team := range cfg.AllTeams() {
		sheet := team
		f.NewSheet(sheet)

		headers := []string{"Date", "Day", "Time", "Field", "Opponent", "Home/Away", "Game"}
		for i, h := range headers {
			f.SetCellValue(sheet, cellRef(i+1, 1), h)
		}

		headerStyle, _ := f.NewStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
			Alignment: &excelize.Alignment{Horizontal: "center"},
		})
		if headerStyle != 0 {
			for i := range headers {
				f.SetCellStyle(sheet, cellRef(i+1, 1), cellRef(i+1, 1), headerStyle)
			}
		}

		// Collect and sort this team's games
		type teamGame struct {
			date     time.Time
			time     string
			field    string
			opponent string
			homeAway string
			label    string
		}
		var games []teamGame
		for _, a := range result.Assignments {
			if a.Game.Home == team {
				games = append(games, teamGame{
					date: a.Slot.Date, time: a.Slot.Time, field: a.Slot.Field,
					opponent: a.Game.Away, homeAway: "Home", label: a.Game.Label,
				})
			} else if a.Game.Away == team {
				games = append(games, teamGame{
					date: a.Slot.Date, time: a.Slot.Time, field: a.Slot.Field,
					opponent: a.Game.Home, homeAway: "Away", label: a.Game.Label,
				})
			}
		}
		sort.Slice(games, func(i, j int) bool {
			if !games[i].date.Equal(games[j].date) {
				return games[i].date.Before(games[j].date)
			}
			return games[i].time < games[j].time
		})

		for i, g := range games {
			row := i + 2
			f.SetCellValue(sheet, cellRef(1, row), g.date.Format("01/02/2006"))
			f.SetCellValue(sheet, cellRef(2, row), g.date.Format("Mon"))
			f.SetCellValue(sheet, cellRef(3, row), g.time)
			f.SetCellValue(sheet, cellRef(4, row), g.field)
			f.SetCellValue(sheet, cellRef(5, row), g.opponent)
			f.SetCellValue(sheet, cellRef(6, row), g.homeAway)
			f.SetCellValue(sheet, cellRef(7, row), g.label)
		}

		// Set column widths
		widths := map[string]float64{"A": 12, "B": 6, "C": 8, "D": 22, "E": 12, "F": 10, "G": 10}
		for col, w := range widths {
			f.SetColWidth(sheet, col, col, w)
		}
	}

	return nil
}

func cellRef(col, row int) string {
	return fmt.Sprintf("%s%d", colLetter(col), row)
}

func colLetter(col int) string {
	result := ""
	for col > 0 {
		col--
		result = string(rune('A'+col%26)) + result
		col /= 26
	}
	return result
}
