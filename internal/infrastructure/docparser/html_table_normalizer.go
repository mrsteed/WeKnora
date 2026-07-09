package docparser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	xhtml "golang.org/x/net/html"
)

var (
	// htmlTableBlockPattern matches a single (non-nested) <table>...</table>
	// block, the form OCR/layout engines such as PaddleOCR-VL emit tables in.
	htmlTableBlockPattern = regexp.MustCompile(`(?is)<table\b[^>]*>.*?</table>`)

	// htmlLayoutAttrPattern matches presentational HTML attributes that carry
	// no semantic value (text-align styles, CSS classes, sizing). Structural
	// attributes like rowspan/colspan are intentionally excluded.
	htmlLayoutAttrPattern = regexp.MustCompile(
		`(?is)\s+(?:style|class|align|valign|width|height|bgcolor)\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)`,
	)

	// htmlSpanAttrPattern detects rowspan/colspan so we can expand merged cells
	// into a plain grid before serializing back to Markdown.
	htmlSpanAttrPattern = regexp.MustCompile(`(?i)\b(?:row|col)span\b`)

	// markdownTableSeparatorPattern matches the |---|---| delimiter row that a
	// valid GFM table must contain.
	markdownTableSeparatorPattern = regexp.MustCompile(`(?m)^\s*\|?\s*:?-+:?\s*(?:\|\s*:?-+:?\s*)+\|?\s*$`)
)

// NormalizeHTMLTables rewrites inline HTML <table> blocks embedded in OCR
// markdown output. PaddleOCR-VL emits tables as HTML with per-cell text-align
// styles, which (1) waste tokens on layout markup and (2) are not recognized
// by the chunker's table-protection logic, so large tables get split mid-row.
//
// Each table block is converted to a GFM Markdown table when possible. Tables
// that contain merged cells are expanded into a rectangular grid so Pandoc and
// downstream Markdown tooling can still render them as standard tables.
func NormalizeHTMLTables(md string) string {
	if !strings.Contains(strings.ToLower(md), "<table") {
		return md
	}

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
		),
	)

	return htmlTableBlockPattern.ReplaceAllStringFunc(md, func(block string) string {
		converted := convertHTMLTableBlock(conv, block)
		if converted == "" {
			return stripHTMLLayoutAttrs(block)
		}
		// Pad with blank lines so the Markdown table is a standalone block that
		// the chunker recognizes (and protects) as a table.
		return "\n\n" + converted + "\n\n"
	})
}

func normalizeHTMLTables(md string) string {
	return NormalizeHTMLTables(md)
}

func convertHTMLTableBlock(conv *converter.Converter, block string) string {
	if !htmlSpanAttrPattern.MatchString(block) {
		if converted := convertTableBlockWithConverter(conv, block); converted != "" {
			return converted
		}
	}

	if converted, err := expandHTMLTableBlockToMarkdown(block); err == nil {
		return converted
	}

	stripped := stripHTMLLayoutAttrs(block)
	if converted := convertTableBlockWithConverter(conv, stripped); converted != "" {
		return converted
	}

	return ""
}

func convertTableBlockWithConverter(conv *converter.Converter, block string) string {
	converted, err := conv.ConvertString(block)
	if err != nil {
		return ""
	}

	converted = strings.TrimSpace(converted)
	if converted == "" || !markdownTableSeparatorPattern.MatchString(converted) {
		return ""
	}

	return converted
}

func expandHTMLTableBlockToMarkdown(block string) (string, error) {
	node, err := xhtml.Parse(strings.NewReader(block))
	if err != nil {
		return "", fmt.Errorf("parse html table block: %w", err)
	}

	tableNode := findFirstElement(node, "table")
	if tableNode == nil {
		return "", fmt.Errorf("html table block does not contain a table element")
	}

	rows, err := extractTableRows(tableNode)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("html table block does not contain rows")
	}

	return renderMarkdownTable(rows), nil
}

func findFirstElement(node *xhtml.Node, tag string) *xhtml.Node {
	if node == nil {
		return nil
	}
	if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, tag) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func extractTableRows(tableNode *xhtml.Node) ([][]string, error) {
	rows := make([][]string, 0)
	pendingRowspans := map[int]int{}
	maxCols := 0

	for rowNode := nextTableRow(tableNode.FirstChild); rowNode != nil; rowNode = nextTableRow(rowNode.NextSibling) {
		row := make([]string, 0)
		columnIndex := 0
		hasCell := false

		consumePending := func() {
			for remaining, ok := pendingRowspans[columnIndex]; ok && remaining > 0; remaining, ok = pendingRowspans[columnIndex] {
				row = append(row, "")
				remaining--
				if remaining == 0 {
					delete(pendingRowspans, columnIndex)
				} else {
					pendingRowspans[columnIndex] = remaining
				}
				columnIndex++
			}
		}

		consumePending()

		for cellNode := nextTableCell(rowNode.FirstChild); cellNode != nil; cellNode = nextTableCell(cellNode.NextSibling) {
			hasCell = true
			consumePending()

			colspan := parseSpanAttr(cellNode, "colspan")
			rowspan := parseSpanAttr(cellNode, "rowspan")
			if colspan < 1 {
				colspan = 1
			}
			if rowspan < 1 {
				rowspan = 1
			}

			row = append(row, extractCellText(cellNode))
			for i := 1; i < colspan; i++ {
				row = append(row, "")
			}

			if rowspan > 1 {
				for i := 0; i < colspan; i++ {
					pendingRowspans[columnIndex+i] = rowspan - 1
				}
			}

			columnIndex += colspan
		}

		consumePending()

		if !hasCell && len(row) == 0 {
			continue
		}

		if len(row) > maxCols {
			maxCols = len(row)
		}
		rows = append(rows, row)
	}

	if maxCols == 0 {
		return nil, nil
	}

	for index := range rows {
		for len(rows[index]) < maxCols {
			rows[index] = append(rows[index], "")
		}
	}

	return rows, nil
}

func nextTableRow(node *xhtml.Node) *xhtml.Node {
	for current := node; current != nil; current = current.NextSibling {
		if current.Type == xhtml.ElementNode && strings.EqualFold(current.Data, "tr") {
			return current
		}
		if current.Type == xhtml.ElementNode && (strings.EqualFold(current.Data, "thead") || strings.EqualFold(current.Data, "tbody") || strings.EqualFold(current.Data, "tfoot")) {
			if nested := nextTableRow(current.FirstChild); nested != nil {
				return nested
			}
		}
	}
	return nil
}

func nextTableCell(node *xhtml.Node) *xhtml.Node {
	for current := node; current != nil; current = current.NextSibling {
		if current.Type == xhtml.ElementNode && (strings.EqualFold(current.Data, "td") || strings.EqualFold(current.Data, "th")) {
			return current
		}
	}
	return nil
}

func parseSpanAttr(node *xhtml.Node, name string) int {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			value, err := strconv.Atoi(strings.TrimSpace(attr.Val))
			if err == nil && value > 0 {
				return value
			}
		}
	}
	return 1
}

func extractCellText(node *xhtml.Node) string {
	var builder strings.Builder
	collectCellText(node, &builder)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func collectCellText(node *xhtml.Node, builder *strings.Builder) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case xhtml.TextNode:
			builder.WriteString(child.Data)
		case xhtml.ElementNode:
			if strings.EqualFold(child.Data, "br") {
				builder.WriteString(" ")
				continue
			}
			if strings.EqualFold(child.Data, "img") {
				for _, attr := range child.Attr {
					if strings.EqualFold(attr.Key, "alt") && strings.TrimSpace(attr.Val) != "" {
						builder.WriteString(attr.Val)
						builder.WriteString(" ")
						break
					}
				}
				continue
			}
			collectCellText(child, builder)
			if strings.EqualFold(child.Data, "p") || strings.EqualFold(child.Data, "div") {
				builder.WriteString(" ")
			}
		}
	}
}

func renderMarkdownTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	columnCount := 0
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	if columnCount == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(renderMarkdownTableRow(rows[0], columnCount))
	builder.WriteByte('\n')
	builder.WriteString(renderMarkdownTableSeparator(columnCount))
	for _, row := range rows[1:] {
		builder.WriteByte('\n')
		builder.WriteString(renderMarkdownTableRow(row, columnCount))
	}

	return builder.String()
}

func renderMarkdownTableRow(row []string, columnCount int) string {
	var builder strings.Builder
	builder.WriteString("|")
	for index := 0; index < columnCount; index++ {
		cell := ""
		if index < len(row) {
			cell = escapeMarkdownTableCell(row[index])
		}
		builder.WriteString(" ")
		builder.WriteString(cell)
		builder.WriteString(" |")
	}
	return builder.String()
}

func renderMarkdownTableSeparator(columnCount int) string {
	var builder strings.Builder
	builder.WriteString("|")
	for index := 0; index < columnCount; index++ {
		builder.WriteString(" --- |")
	}
	return builder.String()
}

func escapeMarkdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

// stripHTMLLayoutAttrs removes presentational attributes from an HTML fragment
// while preserving structural attributes (rowspan/colspan) and text content.
func stripHTMLLayoutAttrs(html string) string {
	return htmlLayoutAttrPattern.ReplaceAllString(html, "")
}
