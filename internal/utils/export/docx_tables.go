package export

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

	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
)

var (
	exportHTMLTableBlockPattern         = regexp.MustCompile(`(?is)<table\b[^>]*>.*?</table>`)
	exportFencedCodeBlockPattern        = regexp.MustCompile("(?s)(?:```|~~~)[^\n]*\n.*?(?:```|~~~)")
	exportMarkdownTableSeparatorPattern = regexp.MustCompile(`(?m)^\s*\|?\s*:?-+:?\s*(?:\|\s*:?-+:?\s*)*\|?\s*$`)
)

func normalizeHTMLTablesForDocxExport(markdown string) string {
	markdown = docparser.NormalizeHTMLTables(markdown)
	if !strings.Contains(strings.ToLower(markdown), "<table") {
		return markdown
	}

	locs := exportHTMLTableBlockPattern.FindAllStringIndex(markdown, -1)
	if len(locs) == 0 {
		return markdown
	}
	fences := exportFencedCodeBlockPattern.FindAllStringIndex(markdown, -1)
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
		),
	)

	var builder strings.Builder
	last := 0
	for _, loc := range locs {
		builder.WriteString(markdown[last:loc[0]])
		block := markdown[loc[0]:loc[1]]
		if exportIndexInsideSpans(loc[0], fences) {
			builder.WriteString(block)
			last = loc[1]
			continue
		}

		out := strings.TrimRight(builder.String(), "\n")
		builder.Reset()
		builder.WriteString(out)
		builder.WriteString("\n\n")
		builder.WriteString(normalizeDocxExportHTMLTable(conv, block))
		last = loc[1]
		for last < len(markdown) && markdown[last] == '\n' {
			last++
		}
		builder.WriteString("\n\n")
	}
	builder.WriteString(markdown[last:])
	return builder.String()
}

func normalizeDocxExportHTMLTable(conv *converter.Converter, block string) string {
	converted, err := conv.ConvertString(block)
	if err == nil {
		converted = strings.TrimSpace(converted)
		if converted != "" && exportMarkdownTableSeparatorPattern.MatchString(converted) {
			return exportUnescapeMarkdownImageSyntax(converted)
		}
	}

	converted, err = expandDocxExportHTMLTableToMarkdown(block)
	if err == nil && converted != "" {
		return converted
	}

	return strings.TrimSpace(block)
}

func exportIndexInsideSpans(pos int, spans [][]int) bool {
	for _, span := range spans {
		if pos >= span[0] && pos < span[1] {
			return true
		}
	}
	return false
}

func expandDocxExportHTMLTableToMarkdown(block string) (string, error) {
	node, err := xhtml.Parse(strings.NewReader(block))
	if err != nil {
		return "", fmt.Errorf("parse html table block: %w", err)
	}
	tableNode := exportFindFirstElement(node, "table")
	if tableNode == nil {
		return "", fmt.Errorf("html table block does not contain a table element")
	}
	rows, err := exportExtractTableRows(tableNode)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("html table block does not contain rows")
	}
	return renderDocxExportMarkdownTable(rows), nil
}

func exportFindFirstElement(node *xhtml.Node, tag string) *xhtml.Node {
	if node == nil {
		return nil
	}
	if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, tag) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := exportFindFirstElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func exportExtractTableRows(tableNode *xhtml.Node) ([][]string, error) {
	rows := make([][]string, 0)
	pendingRowspans := map[int]int{}
	maxCols := 0

	for rowNode := exportNextTableRow(tableNode.FirstChild); rowNode != nil; rowNode = exportNextTableRow(rowNode.NextSibling) {
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

		for cellNode := exportNextTableCell(rowNode.FirstChild); cellNode != nil; cellNode = exportNextTableCell(cellNode.NextSibling) {
			hasCell = true
			consumePending()

			colspan := exportParseSpanAttr(cellNode, "colspan")
			rowspan := exportParseSpanAttr(cellNode, "rowspan")
			if colspan < 1 {
				colspan = 1
			}
			if rowspan < 1 {
				rowspan = 1
			}

			row = append(row, exportExtractCellText(cellNode))
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

func exportNextTableRow(node *xhtml.Node) *xhtml.Node {
	for current := node; current != nil; current = current.NextSibling {
		if current.Type == xhtml.ElementNode && strings.EqualFold(current.Data, "tr") {
			return current
		}
		if current.Type == xhtml.ElementNode && (strings.EqualFold(current.Data, "thead") || strings.EqualFold(current.Data, "tbody") || strings.EqualFold(current.Data, "tfoot")) {
			if nested := exportNextTableRow(current.FirstChild); nested != nil {
				return nested
			}
		}
	}
	return nil
}

func exportNextTableCell(node *xhtml.Node) *xhtml.Node {
	for current := node; current != nil; current = current.NextSibling {
		if current.Type == xhtml.ElementNode && (strings.EqualFold(current.Data, "td") || strings.EqualFold(current.Data, "th")) {
			return current
		}
	}
	return nil
}

func exportParseSpanAttr(node *xhtml.Node, name string) int {
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

func exportExtractCellText(node *xhtml.Node) string {
	var builder strings.Builder
	exportCollectCellText(node, &builder)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func exportCollectCellText(node *xhtml.Node, builder *strings.Builder) {
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
			exportCollectCellText(child, builder)
			if strings.EqualFold(child.Data, "p") || strings.EqualFold(child.Data, "div") {
				builder.WriteString(" ")
			}
		}
	}
}

func renderDocxExportMarkdownTable(rows [][]string) string {
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
	builder.WriteString(renderDocxExportMarkdownTableRow(rows[0], columnCount))
	builder.WriteByte('\n')
	builder.WriteString(renderDocxExportMarkdownTableSeparator(columnCount))
	for _, row := range rows[1:] {
		builder.WriteByte('\n')
		builder.WriteString(renderDocxExportMarkdownTableRow(row, columnCount))
	}
	return builder.String()
}

func renderDocxExportMarkdownTableRow(row []string, columnCount int) string {
	var builder strings.Builder
	builder.WriteString("|")
	for index := 0; index < columnCount; index++ {
		cell := ""
		if index < len(row) {
			cell = strings.TrimSpace(row[index])
		}
		builder.WriteString(" ")
		builder.WriteString(exportUnescapeMarkdownImageSyntax(cell))
		builder.WriteString(" |")
	}
	return builder.String()
}

func renderDocxExportMarkdownTableSeparator(columnCount int) string {
	var builder strings.Builder
	builder.WriteString("|")
	for index := 0; index < columnCount; index++ {
		builder.WriteString(" --- |")
	}
	return builder.String()
}

func exportUnescapeMarkdownImageSyntax(value string) string {
	value = strings.ReplaceAll(value, `!\\[`, `![`)
	value = strings.ReplaceAll(value, `\\](`, `](`)
	return value
}
