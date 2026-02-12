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

	// Set default font for the workbook
	f.SetDefaultFont("Arial")

	if _, err := writeMasterSheet(f, cfg, result, slots, blackouts); err != nil {
		return nil, fmt.Errorf("writing master sheet: %w", err)
	}

	// Build game entries from assignments for team sheets
	var fieldNames []string
	for _, field := range cfg.Fields {
		fieldNames = append(fieldNames, field.Name)
	}
	var games []gameEntry
	for _, a := range result.Assignments {
		games = append(games, gameEntry{
			Date:  a.Slot.Date,
			Time:  a.Slot.Time,
			Field: fieldColumnName(a.Slot.Field, fieldNames),
			Home:  a.Game.Home,
			Away:  a.Game.Away,
		})
	}

	if err := writeTeamSheets(f, cfg, games); err != nil {
		return nil, fmt.Errorf("writing team sheets: %w", err)
	}

	f.DeleteSheet("Sheet1")
	return f, nil
}

// UpdateTeamSheets reads the master schedule from an existing xlsx file,
// regenerates all per-team sheets with static values, and saves the file.
func UpdateTeamSheets(path string, cfg *config.Config) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	games, err := readGamesFromMaster(f)
	if err != nil {
		return err
	}

	// Delete existing team sheets
	for _, team := range cfg.AllTeams() {
		f.DeleteSheet(team)
	}

	if err := writeTeamSheets(f, cfg, games); err != nil {
		return err
	}

	return f.SaveAs(path)
}

func fieldColumnName(name string, allNames []string) string {
	first := name
	for i, c := range name {
		if c == ' ' {
			first = name[:i]
			break
		}
	}
	// Check if first word is unique
	count := 0
	for _, n := range allNames {
		word := n
		for i, c := range n {
			if c == ' ' {
				word = n[:i]
				break
			}
		}
		if word == first {
			count++
		}
	}
	if count > 1 {
		return name
	}
	return first
}

func writeMasterSheet(f *excelize.File, cfg *config.Config, result *schedule.Result, slots []schedule.Slot, blackouts []schedule.BlackoutSlot) (int, error) {
	sheet := "Master Schedule"
	f.NewSheet(sheet)

	// Build field column names
	var fieldNames []string
	for _, field := range cfg.Fields {
		fieldNames = append(fieldNames, field.Name)
	}
	fieldCols := make([]string, len(fieldNames))
	for i, name := range fieldNames {
		fieldCols[i] = fieldColumnName(name, fieldNames)
	}

	// Headers: Date, Day, Time, <field1>, <field2>, ...
	headers := []string{"Date", "Day", "Time"}
	headers = append(headers, fieldCols...)
	for i, h := range headers {
		f.SetCellValue(sheet, cellRef(i+1, 1), h)
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 16, Family: "Arial"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if headerStyle != 0 {
		for i := range headers {
			f.SetCellStyle(sheet, cellRef(i+1, 1), cellRef(i+1, 1), headerStyle)
		}
	}

	cellStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 16, Family: "Arial"},
	})

	fieldCellStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 16, Family: "Arial"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	// Build field name -> column index (0-based into field list)
	fieldIndex := make(map[string]int)
	for i, name := range fieldNames {
		fieldIndex[name] = i
	}

	// Build assignment lookup: (date, time, field) -> assignment
	type slotKey struct {
		date  time.Time
		time  string
		field string
	}
	assignmentMap := make(map[slotKey]schedule.Assignment)
	for _, a := range result.Assignments {
		assignmentMap[slotKey{a.Slot.Date, a.Slot.Time, a.Slot.Field}] = a
	}

	// Build blackout lookup: (date, time, field) -> reason
	blackoutMap := make(map[slotKey]string)
	for _, b := range blackouts {
		blackoutMap[slotKey{b.Date, b.Time, b.Field}] = b.Reason
	}

	// Collect all unique (date, time) pairs from both slots and blackouts
	type timeSlot struct {
		date time.Time
		time string
	}
	seen := make(map[timeSlot]bool)
	var timeSlots []timeSlot
	for _, s := range slots {
		ts := timeSlot{s.Date, s.Time}
		if !seen[ts] {
			seen[ts] = true
			timeSlots = append(timeSlots, ts)
		}
	}
	for _, b := range blackouts {
		ts := timeSlot{b.Date, b.Time}
		if !seen[ts] {
			seen[ts] = true
			timeSlots = append(timeSlots, ts)
		}
	}

	sort.Slice(timeSlots, func(i, j int) bool {
		if !timeSlots[i].date.Equal(timeSlots[j].date) {
			return timeSlots[i].date.Before(timeSlots[j].date)
		}
		return timeSlots[i].time < timeSlots[j].time
	})

	for i, ts := range timeSlots {
		row := i + 2
		f.SetCellValue(sheet, cellRef(1, row), ts.date.Format("01/02/2006"))
		f.SetCellValue(sheet, cellRef(2, row), ts.date.Format("Mon"))
		f.SetCellValue(sheet, cellRef(3, row), ts.time)

		for fi, fname := range fieldNames {
			col := fi + 4 // 1-indexed, after Date/Day/Time
			sk := slotKey{ts.date, ts.time, fname}

			if a, ok := assignmentMap[sk]; ok {
				f.SetCellValue(sheet, cellRef(col, row), fmt.Sprintf("%s @ %s", a.Game.Away, a.Game.Home))
			} else if reason, ok := blackoutMap[sk]; ok {
				f.SetCellValue(sheet, cellRef(col, row), reason)
			}
		}

		if cellStyle != 0 {
			for col := 1; col <= 3; col++ {
				f.SetCellStyle(sheet, cellRef(col, row), cellRef(col, row), cellStyle)
			}
			for col := 4; col <= len(headers); col++ {
				f.SetCellStyle(sheet, cellRef(col, row), cellRef(col, row), fieldCellStyle)
			}
		}
	}

	// Set column widths (sized for Arial 16)
	f.SetColWidth(sheet, "A", "A", 18)
	f.SetColWidth(sheet, "B", "B", 8)
	f.SetColWidth(sheet, "C", "C", 10)
	for i := range fieldNames {
		col := colLetter(i + 4)
		f.SetColWidth(sheet, col, col, 30)
	}

	// Conditional formatting: non-game cells in field columns get light red
	lastRow := len(timeSlots) + 1
	redFill, _ := f.NewConditionalStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFC7CE"}},
		Font: &excelize.Font{Size: 16, Family: "Arial"},
	})
	for i := range fieldNames {
		col := colLetter(i + 4)
		cellRange := fmt.Sprintf("%s2:%s%d", col, col, lastRow)
		topCell := fmt.Sprintf("%s2", col)
		formula := fmt.Sprintf(`AND(%s<>"",ISERROR(FIND(" @ ",%s)))`, topCell, topCell)
		f.SetConditionalFormat(sheet, cellRange, []excelize.ConditionalFormatOptions{
			{
				Type:     "formula",
				Criteria: formula,
				Format:   &redFill,
			},
		})
	}

	return lastRow, nil
}

type gameEntry struct {
	Date  time.Time
	Time  string
	Field string
	Home  string
	Away  string
}

func writeTeamSheets(f *excelize.File, cfg *config.Config, games []gameEntry) error {
	// Sort games by date then time
	sort.Slice(games, func(i, j int) bool {
		if !games[i].Date.Equal(games[j].Date) {
			return games[i].Date.Before(games[j].Date)
		}
		return games[i].Time < games[j].Time
	})

	for _, team := range cfg.AllTeams() {
		sheet := team
		f.NewSheet(sheet)

		headers := []string{"Date", "Day", "Time", "Field", "Opponent", "Home/Away", "Game"}
		for i, h := range headers {
			f.SetCellValue(sheet, cellRef(i+1, 1), h)
		}

		headerStyle, _ := f.NewStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 16, Family: "Arial"},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
			Alignment: &excelize.Alignment{Horizontal: "center"},
		})
		if headerStyle != 0 {
			for i := range headers {
				f.SetCellStyle(sheet, cellRef(i+1, 1), cellRef(i+1, 1), headerStyle)
			}
		}

		cellStyle, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Size: 16, Family: "Arial"},
		})

		row := 2
		for _, g := range games {
			if g.Home != team && g.Away != team {
				continue
			}

			opponent := g.Away
			ha := "Home"
			if g.Away == team {
				opponent = g.Home
				ha = "Away"
			}

			f.SetCellValue(sheet, cellRef(1, row), g.Date.Format("01/02/2006"))
			f.SetCellValue(sheet, cellRef(2, row), g.Date.Format("Mon"))
			f.SetCellValue(sheet, cellRef(3, row), g.Time)
			f.SetCellValue(sheet, cellRef(4, row), g.Field)
			f.SetCellValue(sheet, cellRef(5, row), opponent)
			f.SetCellValue(sheet, cellRef(6, row), ha)
			f.SetCellValue(sheet, cellRef(7, row), fmt.Sprintf("%s @ %s", g.Away, g.Home))

			if cellStyle != 0 {
				for col := 1; col <= 7; col++ {
					f.SetCellStyle(sheet, cellRef(col, row), cellRef(col, row), cellStyle)
				}
			}
			row++
		}

		// Set column widths
		widths := map[string]float64{"A": 18, "B": 8, "C": 10, "D": 28, "E": 16, "F": 14, "G": 28}
		for col, w := range widths {
			f.SetColWidth(sheet, col, col, w)
		}
	}

	return nil
}

func readGamesFromMaster(f *excelize.File) ([]gameEntry, error) {
	rows, err := f.GetRows("Master Schedule")
	if err != nil {
		return nil, fmt.Errorf("reading Master Schedule: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("Master Schedule is empty")
	}

	header := rows[0]
	var games []gameEntry
	for i, row := range rows {
		if i == 0 || len(row) < 3 || row[0] == "" {
			continue
		}
		date, err := time.Parse("01/02/2006", row[0])
		if err != nil {
			continue
		}
		for fi := 3; fi < len(header) && fi < len(row); fi++ {
			if row[fi] == "" {
				continue
			}
			away, home, ok := parseGameCell(row[fi])
			if !ok {
				continue
			}
			games = append(games, gameEntry{
				Date:  date,
				Time:  row[2],
				Field: header[fi],
				Home:  home,
				Away:  away,
			})
		}
	}
	return games, nil
}

func parseGameCell(cell string) (away, home string, ok bool) {
	for i := 0; i < len(cell)-2; i++ {
		if cell[i] == ' ' && cell[i+1] == '@' && cell[i+2] == ' ' {
			return cell[:i], cell[i+3:], true
		}
	}
	return "", "", false
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
