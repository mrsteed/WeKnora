package docparser

import (
	"strings"
	"testing"
)

func TestNormalizeHTMLTables_ConvertsStyledTableToMarkdown(t *testing.T) {
	// Mirrors PaddleOCR-VL output: a data table where every cell carries a
	// text-align style that wastes tokens.
	input := `# 报告

<table><tr><td style="text-align:center;">指标</td><td style="text-align:center;">数值</td></tr>` +
		`<tr><td style="text-align:center;">营收</td><td style="text-align:right;">10亿</td></tr>` +
		`<tr><td style="text-align:center;">利润</td><td style="text-align:right;">2.3亿</td></tr></table>

结尾。`

	got := normalizeHTMLTables(input)

	if strings.Contains(got, "<table") {
		t.Fatalf("expected HTML table to be converted away, got:\n%s", got)
	}
	if strings.Contains(got, "text-align") {
		t.Fatalf("expected style attributes removed, got:\n%s", got)
	}
	if !markdownTableSeparatorPattern.MatchString(got) {
		t.Fatalf("expected a Markdown table separator row, got:\n%s", got)
	}
	for _, want := range []string{"指标", "数值", "营收", "10亿", "利润", "2.3亿"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected converted table to retain %q, got:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "# 报告") || !strings.Contains(got, "结尾。") {
		t.Fatalf("expected surrounding markdown to be preserved, got:\n%s", got)
	}
}

func TestNormalizeHTMLTables_StripsAttrsOnSpanTables(t *testing.T) {
	// 合并单元格会被展开成普通 Markdown 网格，保证后续 Pandoc/分块逻辑都能识别。
	input := `<table><tr><td colspan="2" style="text-align:center;" class="hdr">合计</td></tr>` +
		`<tr><td style="text-align:left;">A</td><td width="80">B</td></tr></table>`

	got := normalizeHTMLTables(input)

	if strings.Contains(got, "<table") {
		t.Fatalf("expected span table to be converted to markdown, got:\n%s", got)
	}
	if !markdownTableSeparatorPattern.MatchString(got) {
		t.Fatalf("expected markdown table separator row, got:\n%s", got)
	}
	for _, want := range []string{"合计", "A", "B"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected expanded markdown table to retain %q, got:\n%s", want, got)
		}
	}
}

func TestNormalizeHTMLTables_NoTableUnchanged(t *testing.T) {
	input := "# 标题\n\n普通段落，没有表格。\n\n| a | b |\n| --- | --- |\n| 1 | 2 |"
	if got := normalizeHTMLTables(input); got != input {
		t.Fatalf("expected content without HTML tables to be unchanged, got:\n%s", got)
	}
}
