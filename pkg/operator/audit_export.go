package operator

import (
	"os"
	"time"

	"github.com/xuri/excelize/v2"
)

var auditExcelHeaders = []string{
	"Timestamp", "Action", "Target", "UID", "Name",
	"Change Kind", "Change Field", "Diff", "Old Value", "New Value",
	"Error",
}

func changeDiff(c Change) string {
	switch {
	case c.Old == "" && c.New != "":
		return "added"
	case c.Old != "" && c.New == "":
		return "removed"
	default:
		return "modified"
	}
}

func ExportToExcel(file *os.File, entries []AuditEntry) error {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"

	for i, h := range auditExcelHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	row := 2
	for _, e := range entries {
		var errStr string
		if e.Error != nil {
			errStr = e.Error.Error()
		}

		base := []any{
			e.Timestamp.Format(time.RFC3339),
			string(e.Action),
			e.Target,
			e.UID,
			e.Name,
		}

		if len(e.Changes) == 0 {
			values := append(base, "", "", "", "", "", errStr)
			for i, v := range values {
				cell, _ := excelize.CoordinatesToCellName(i+1, row)
				f.SetCellValue(sheet, cell, v)
			}
			row++
			continue
		}

		for _, c := range e.Changes {
			values := append(base, c.Kind, c.Field, changeDiff(c), c.Old, c.New, errStr)
			for i, v := range values {
				cell, _ := excelize.CoordinatesToCellName(i+1, row)
				f.SetCellValue(sheet, cell, v)
			}
			row++
		}
	}

	_, err := f.WriteTo(file)
	return err
}
