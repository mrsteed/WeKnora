package export

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

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

	// Wrap with full HTML document including styles for good PDF/print rendering
	styledHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
    /* Base styles */
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial,
                   "Noto Sans", "PingFang SC", "Microsoft YaHei", "Hiragino Sans GB", sans-serif;
      line-height: 1.8;
      padding: 40px;
      max-width: 800px;
      margin: 0 auto;
      color: #24292e;
      font-size: 14px;
    }

    /* Headings */
    h1 { font-size: 2em; border-bottom: 1px solid #eaecef; padding-bottom: 0.3em; margin-top: 1.5em; margin-bottom: 0.8em; }
    h2 { font-size: 1.5em; border-bottom: 1px solid #eaecef; padding-bottom: 0.3em; margin-top: 1.5em; margin-bottom: 0.8em; }
    h3 { font-size: 1.25em; margin-top: 1.2em; margin-bottom: 0.6em; }
    h4, h5, h6 { margin-top: 1.2em; margin-bottom: 0.6em; }

    /* Paragraphs */
    p { margin: 0.8em 0; }

    /* Code */
    code {
      background: #f6f8fa;
      padding: 0.2em 0.4em;
      border-radius: 3px;
      font-size: 0.9em;
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, Courier, monospace;
    }
    pre {
      background: #f6f8fa;
      padding: 16px;
      border-radius: 6px;
      overflow-x: auto;
      line-height: 1.5;
      border: 1px solid #e1e4e8;
    }
    pre code {
      background: none;
      padding: 0;
      font-size: 0.85em;
    }

    /* Blockquote */
    blockquote {
      border-left: 4px solid #dfe2e5;
      margin: 1em 0;
      padding: 0.5em 1em;
      color: #6a737d;
      background: #f6f8fa;
    }
    blockquote p { margin: 0.4em 0; }

    /* Tables */
    table {
      border-collapse: collapse;
      width: 100%%;
      margin: 1em 0;
      font-size: 0.9em;
    }
    th, td {
      border: 1px solid #dfe2e5;
      padding: 8px 13px;
      text-align: left;
    }
    th {
      background: #f6f8fa;
      font-weight: 600;
    }
    tr:nth-child(2n) { background: #f6f8fa; }

    /* Lists */
    ul, ol { padding-left: 2em; margin: 0.5em 0; }
    li { margin: 0.25em 0; }

    /* Links */
    a { color: #0366d6; text-decoration: none; }
    a:hover { text-decoration: underline; }

    /* Images */
    img { max-width: 100%%; height: auto; }

    /* Horizontal rule */
    hr {
      border: none;
      border-top: 1px solid #eaecef;
      margin: 1.5em 0;
    }

    /* Task list */
    .task-list-item { list-style-type: none; }
    .task-list-item input { margin-right: 0.5em; }

    /* Print styles */
    @media print {
      body { padding: 0; max-width: none; }
      pre { white-space: pre-wrap; word-wrap: break-word; }
      a { color: #24292e; }
    }
  </style>
</head>
<body>
%s
</body>
</html>`, rawHTML)

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
