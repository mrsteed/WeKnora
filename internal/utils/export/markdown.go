package export

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var titleHeadingPattern = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

// MarkdownToStyledHTML converts Markdown content to a complete styled HTML document.
// Uses goldmark with extensions for tables, strikethrough, code highlighting, etc.
func MarkdownToStyledHTML(markdown string) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // GitHub Flavored Markdown (tables, strikethrough, autolinks, task lists)
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithFormatOptions(),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("goldmark conversion failed: %w", err)
	}

	rawHTML := buf.String()
	documentTitle := deriveDocumentTitle(markdown)
	escapedDocumentTitle := stdhtml.EscapeString(documentTitle)

	// Wrap with full HTML document including styles for good PDF/print rendering
	styledHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <style>
    :root {
      color-scheme: light;
      --page-background: #f5f7fb;
      --paper-background: #ffffff;
      --paper-border: rgba(15, 23, 42, 0.08);
      --text-primary: #0f172a;
      --text-secondary: #475569;
      --heading-accent: #1d4ed8;
      --code-background: #0f172a;
      --code-foreground: #e2e8f0;
      --border-subtle: #dbe3ee;
      --quote-border: #93c5fd;
      --table-header: #eaf2ff;
      --table-stripe: #f8fbff;
    }

    @page {
      size: A4;
      margin: 12mm;
    }

    * {
      box-sizing: border-box;
    }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial,
                   "Noto Sans", "PingFang SC", "Microsoft YaHei", "Hiragino Sans GB", sans-serif;
      line-height: 1.85;
      margin: 0;
      background: linear-gradient(180deg, #eef4ff 0%%, var(--page-background) 26%%, #ffffff 100%%);
      color: var(--text-primary);
      font-size: 14px;
      -webkit-print-color-adjust: exact;
      print-color-adjust: exact;
    }

    .export-page {
      min-height: 100vh;
      padding: 28px;
    }

    .export-paper {
      max-width: 860px;
      margin: 0 auto;
      background: var(--paper-background);
      border: 1px solid var(--paper-border);
      border-radius: 24px;
      box-shadow: 0 20px 48px rgba(15, 23, 42, 0.08);
      overflow: hidden;
    }

    .export-header {
      padding: 28px 36px 22px;
      background:
        radial-gradient(circle at top left, rgba(59, 130, 246, 0.18), transparent 40%%),
        linear-gradient(135deg, #eff6ff 0%%, #f8fafc 46%%, #ffffff 100%%);
      border-bottom: 1px solid var(--border-subtle);
    }

    .export-title {
      margin: 0;
      font-size: 28px;
      line-height: 1.25;
      font-weight: 800;
    }

    .export-content {
      padding: 30px 36px 40px;
    }

    h1, h2, h3, h4, h5, h6 {
      color: var(--text-primary);
      line-height: 1.35;
      page-break-after: avoid;
    }
    h1 { font-size: 2em; border-bottom: 1px solid var(--border-subtle); padding-bottom: 0.3em; margin-top: 0; margin-bottom: 0.8em; }
    h2 { font-size: 1.5em; border-bottom: 1px solid var(--border-subtle); padding-bottom: 0.3em; margin-top: 1.8em; margin-bottom: 0.8em; }
    h3 { font-size: 1.25em; margin-top: 1.2em; margin-bottom: 0.6em; }
    h4, h5, h6 { margin-top: 1.2em; margin-bottom: 0.6em; }

    p { margin: 0.8em 0; }

    code {
      background: #eaf2ff;
      padding: 0.2em 0.4em;
      border-radius: 6px;
      font-size: 0.9em;
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, Courier, monospace;
    }
    pre {
      background: var(--code-background);
      color: var(--code-foreground);
      padding: 18px 20px;
      border-radius: 14px;
      overflow-x: auto;
      line-height: 1.5;
      border: 1px solid rgba(15, 23, 42, 0.18);
      page-break-inside: avoid;
    }
    pre code {
      background: none;
      padding: 0;
      font-size: 0.85em;
      color: inherit;
    }

    blockquote {
      border-left: 4px solid var(--quote-border);
      margin: 1em 0;
      padding: 0.5em 1em;
      color: var(--text-secondary);
      background: linear-gradient(90deg, rgba(147, 197, 253, 0.14), rgba(255, 255, 255, 0.7));
      border-radius: 0 12px 12px 0;
    }
    blockquote p { margin: 0.4em 0; }

    table {
      border-collapse: collapse;
      width: 100%%;
      margin: 1em 0;
      font-size: 0.9em;
      page-break-inside: auto;
    }
    th, td {
      border: 1px solid var(--border-subtle);
      padding: 10px 14px;
      text-align: left;
    }
    th {
      background: var(--table-header);
      font-weight: 600;
    }
    tr:nth-child(2n) { background: var(--table-stripe); }

    ul, ol { padding-left: 2em; margin: 0.5em 0; }
    li { margin: 0.25em 0; }

    a { color: var(--heading-accent); text-decoration: none; }
    a:hover { text-decoration: underline; }

    img {
      max-width: 100%%;
      height: auto;
      display: block;
      margin: 1.2em auto;
      border-radius: 14px;
      box-shadow: 0 10px 24px rgba(15, 23, 42, 0.12);
    }

    hr {
      border: none;
      border-top: 1px solid var(--border-subtle);
      margin: 1.5em 0;
    }

    .task-list-item { list-style-type: none; }
    .task-list-item input { margin-right: 0.5em; }

    @media print {
      body { background: #ffffff; }
      .export-page { padding: 0; }
      .export-paper { max-width: none; border: none; border-radius: 0; box-shadow: none; }
      .export-header { padding: 0 0 20px; background: none; }
      .export-content { padding: 0; }
      pre { white-space: pre-wrap; word-wrap: break-word; }
      a { color: var(--text-primary); }
    }
  </style>
</head>
<body>
  <div class="export-page">
    <main class="export-paper">
      <header class="export-header">
        <h1 class="export-title">%s</h1>
      </header>
      <article class="export-content markdown-body">
%s
      </article>
    </main>
  </div>
</body>
</html>`, escapedDocumentTitle, escapedDocumentTitle, rawHTML)

	return styledHTML, nil
}

// MarkdownToHTML converts Markdown to raw HTML (without styling wrapper).
func MarkdownToHTML(markdown string) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("goldmark conversion failed: %w", err)
	}

	return buf.String(), nil
}

func deriveDocumentTitle(markdown string) string {
	matches := titleHeadingPattern.FindStringSubmatch(markdown)
	if len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		if title != "" {
			return title
		}
	}

	return "对话导出"
}
