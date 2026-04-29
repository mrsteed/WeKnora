package export

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

const (
	overviewSheetName    = "概览"
	defaultSheetMaxRunes = 31
)

type markdownTable struct {
	title string
	rows  [][]string
}

// markdownToXLSX renders a basic workbook representation for Markdown.
// The first sheet preserves the original narrative flow, while each detected
// GitHub-style table is written to an individual sheet so frontend code no
// longer needs to synthesize spreadsheets in the browser.
func markdownToXLSX(ctx context.Context, markdown string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	workbook := excelize.NewFile()
	defer func() { _ = workbook.Close() }()

	overviewRows, tables, err := parseMarkdownWorkbook(ctx, markdown)
	if err != nil {
		return nil, err
	}
	if err := workbook.SetSheetName("Sheet1", overviewSheetName); err != nil {
		return nil, fmt.Errorf("rename overview sheet: %w", err)
	}

	usedSheetNames := map[string]struct{}{overviewSheetName: {}}
	if err := writeSheetRows(ctx, workbook, overviewSheetName, overviewRows); err != nil {
		return nil, err
	}

	for index, table := range tables {
		sheetName := nextSheetName(table.title, index+1, usedSheetNames)
		if _, err := workbook.NewSheet(sheetName); err != nil {
			return nil, fmt.Errorf("create sheet %q: %w", sheetName, err)
		}
		if err := writeSheetRows(ctx, workbook, sheetName, table.rows); err != nil {
			return nil, err
		}
	}

	workbook.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := workbook.Write(&buf); err != nil {
		return nil, fmt.Errorf("write workbook: %w", err)
	}

	return buf.Bytes(), nil
}

func parseMarkdownWorkbook(ctx context.Context, markdown string) ([][]string, []markdownTable, error) {
	overviewRows := [][]string{{"内容"}}
	tables := make([]markdownTable, 0)
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	currentHeading := ""
	inFence := false

	for i := 0; i < len(lines); {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if isFenceLine(trimmed) {
			inFence = !inFence
			overviewRows = appendOverviewLine(overviewRows, line)
			i++
			continue
		}

		if !inFence && isMarkdownTableHeader(trimmed, lines, i) {
			rows, nextIndex := collectMarkdownTable(lines, i)
			title := currentHeading
			if title == "" {
				title = fmt.Sprintf("表格%d", len(tables)+1)
			}
			tables = append(tables, markdownTable{title: title, rows: rows})
			overviewRows = appendOverviewLine(overviewRows, fmt.Sprintf("[表格] %s（%d 行）", title, max(len(rows)-1, 0)))
			i = nextIndex
			continue
		}

		if title, ok := parseHeading(trimmed); ok {
			currentHeading = title
		}

		overviewRows = appendOverviewLine(overviewRows, line)
		i++
	}

	if len(overviewRows) == 1 {
		overviewRows = append(overviewRows, []string{""})
	}

	return overviewRows, tables, nil
}

func appendOverviewLine(rows [][]string, line string) [][]string {
	if line == "" {
		if len(rows) > 0 && rows[len(rows)-1][0] == "" {
			return rows
		}
		return append(rows, []string{""})
	}

	return append(rows, []string{line})
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func parseHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}

	title := strings.TrimSpace(strings.TrimLeft(line, "#"))
	if title == "" {
		return "", false
	}

	return title, true
}

func isMarkdownTableHeader(line string, lines []string, index int) bool {
	if index+1 >= len(lines) {
		return false
	}

	return isMarkdownTableRow(line) && isMarkdownTableSeparator(strings.TrimSpace(lines[index+1]))
}

func collectMarkdownTable(lines []string, start int) ([][]string, int) {
	rows := make([][]string, 0, 4)
	rows = append(rows, splitMarkdownTableRow(lines[start]))

	index := start + 2
	for index < len(lines) {
		trimmed := strings.TrimSpace(lines[index])
		if !isMarkdownTableRow(trimmed) {
			break
		}
		rows = append(rows, splitMarkdownTableRow(lines[index]))
		index++
	}

	return rows, index
}

func isMarkdownTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}

	return strings.Count(trimmed, "|") >= 2
}

func isMarkdownTableSeparator(line string) bool {
	if !isMarkdownTableRow(line) {
		return false
	}

	inner := strings.TrimSpace(line)
	inner = strings.TrimPrefix(inner, "|")
	inner = strings.TrimSuffix(inner, "|")
	parts := strings.Split(inner, "|")
	if len(parts) == 0 {
		return false
	}

	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell == "" {
			return false
		}
		for _, char := range cell {
			if char != '-' && char != ':' {
				return false
			}
		}
	}

	return true
}

func splitMarkdownTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	row := make([]string, 0, len(parts))
	for _, part := range parts {
		row = append(row, strings.TrimSpace(part))
	}
	return row
}

func writeSheetRows(ctx context.Context, workbook *excelize.File, sheetName string, rows [][]string) error {
	maxColumns := 1
	columnWidths := make([]float64, maxColumns)

	for rowIndex, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}

		if len(row) > maxColumns {
			columnWidths = append(columnWidths, make([]float64, len(row)-maxColumns)...)
			maxColumns = len(row)
		}

		for colIndex, value := range row {
			cellName, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				return fmt.Errorf("resolve cell for sheet %q: %w", sheetName, err)
			}
			if err := workbook.SetCellValue(sheetName, cellName, value); err != nil {
				return fmt.Errorf("set cell %s on sheet %q: %w", cellName, sheetName, err)
			}

			width := clampColumnWidth(float64(utf8.RuneCountInString(value) + 2))
			if width > columnWidths[colIndex] {
				columnWidths[colIndex] = width
			}
		}
	}

	for colIndex := range columnWidths {
		columnName, err := excelize.ColumnNumberToName(colIndex + 1)
		if err != nil {
			return fmt.Errorf("resolve column for sheet %q: %w", sheetName, err)
		}
		width := columnWidths[colIndex]
		if width == 0 {
			width = 14
		}
		if err := workbook.SetColWidth(sheetName, columnName, columnName, width); err != nil {
			return fmt.Errorf("set column width on sheet %q: %w", sheetName, err)
		}
	}

	return nil
}

func clampColumnWidth(width float64) float64 {
	if width < 10 {
		return 10
	}
	if width > 60 {
		return 60
	}
	return width
}

func nextSheetName(base string, index int, used map[string]struct{}) string {
	candidate := sanitizeSheetName(base)
	if candidate == "" {
		candidate = fmt.Sprintf("表格%d", index)
	}

	if _, exists := used[candidate]; !exists {
		used[candidate] = struct{}{}
		return candidate
	}

	for suffix := 1; ; suffix++ {
		variant := sanitizeSheetName(fmt.Sprintf("%s_%d", candidate, suffix))
		if variant == "" {
			variant = fmt.Sprintf("表格%d_%d", index, suffix)
		}
		if _, exists := used[variant]; !exists {
			used[variant] = struct{}{}
			return variant
		}
	}
}

func sanitizeSheetName(name string) string {
	replacer := strings.NewReplacer(":", "_", "\\", "_", "/", "_", "?", "_", "*", "_", "[", "_", "]", "_")
	cleaned := strings.TrimSpace(replacer.Replace(name))
	if cleaned == "" {
		return ""
	}

	runes := []rune(cleaned)
	if len(runes) > defaultSheetMaxRunes {
		cleaned = string(runes[:defaultSheetMaxRunes])
	}

	return cleaned
}

func max(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
