package excel

import (
	"fmt"
	"sort"
	"strings"
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

	lastMasterRow, err := writeMasterSheet(f, cfg, result, slots, blackouts)
	if err != nil {
		return nil, fmt.Errorf("writing master sheet: %w", err)
	}

	if err := writeTeamSheets(f, cfg, lastMasterRow); err != nil {
		return nil, fmt.Errorf("writing team sheets: %w", err)
	}

	f.DeleteSheet("Sheet1")
	return f, nil
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

func writeTeamSheets(f *excelize.File, cfg *config.Config, lastMasterRow int) error {
	masterSheet := "Master Schedule"

	var fieldNames []string
	for _, field := range cfg.Fields {
		fieldNames = append(fieldNames, field.Name)
	}

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

		formula := buildTeamFormula(team, masterSheet, fieldNames, lastMasterRow)
		f.SetCellFormula(sheet, "A2", formula)

		// Set column style for data area (Arial 16)
		cellStyle, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Size: 16, Family: "Arial"},
		})
		if cellStyle != 0 {
			lastCol := colLetter(len(headers))
			f.SetColStyle(sheet, fmt.Sprintf("A:%s", lastCol), cellStyle)
		}

		// Set column widths
		widths := map[string]float64{"A": 18, "B": 8, "C": 10, "D": 28, "E": 16, "F": 14, "G": 28}
		for col, w := range widths {
			f.SetColWidth(sheet, col, col, w)
		}
	}

	return nil
}

// buildTeamFormula creates a LET/FILTER/HSTACK formula that derives
// a team's schedule from the Master Schedule sheet.
// Requires Excel 365 or Excel 2021+ for dynamic array support.
func buildTeamFormula(team, masterSheet string, fieldNames []string, lastRow int) string {
	ms := fmt.Sprintf("'%s'", masterSheet)
	colRange := func(col string) string {
		return fmt.Sprintf("%s!%s$2:%s$%d", ms, col, col, lastRow)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf(`team,"%s"`, team))
	parts = append(parts, fmt.Sprintf("d,%s", colRange("A")))
	parts = append(parts, fmt.Sprintf("dy,%s", colRange("B")))
	parts = append(parts, fmt.Sprintf("tm,%s", colRange("C")))

	// Column variables and match variables for each field
	for i := range fieldNames {
		col := colLetter(i + 4)
		parts = append(parts, fmt.Sprintf("c%d,%s", i+1, colRange(col)))
	}
	for i := range fieldNames {
		parts = append(parts, fmt.Sprintf("m%d,ISNUMBER(SEARCH(team,c%d))", i+1, i+1))
	}

	// found = any field column contains the team
	matchExprs := make([]string, len(fieldNames))
	for i := range fieldNames {
		matchExprs[i] = fmt.Sprintf("m%d", i+1)
	}
	parts = append(parts, fmt.Sprintf("found,(%s)>0", strings.Join(matchExprs, "+")))

	// game = cell text from the matching field column
	gameExpr := `""`
	for i := len(fieldNames) - 1; i >= 0; i-- {
		gameExpr = fmt.Sprintf("IF(m%d,c%d,%s)", i+1, i+1, gameExpr)
	}
	parts = append(parts, fmt.Sprintf("game,%s", gameExpr))

	// field = column header name from the matching field
	fieldExpr := `""`
	for i := len(fieldNames) - 1; i >= 0; i-- {
		colName := fieldColumnName(fieldNames[i], fieldNames)
		fieldExpr = fmt.Sprintf(`IF(m%d,"%s",%s)`, i+1, colName, fieldExpr)
	}
	parts = append(parts, fmt.Sprintf("field,%s", fieldExpr))

	// opponent and home/away parsed from "Away @ Home" format
	parts = append(parts, `opp,IFERROR(IF(LEFT(game,FIND(" @ ",game)-1)=team,MID(game,FIND(" @ ",game)+3,100),LEFT(game,FIND(" @ ",game)-1)),"")`)
	parts = append(parts, `ha,IFERROR(IF(LEFT(game,FIND(" @ ",game)-1)=team,"Away","Home"),"")`)

	// Final: FILTER + HSTACK to produce Date, Day, Time, Field, Opponent, Home/Away, Game
	parts = append(parts, `FILTER(HSTACK(d,dy,tm,field,opp,ha,game),found,"No games scheduled")`)

	return "LET(" + strings.Join(parts, ",") + ")"
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
