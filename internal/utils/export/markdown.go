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
	xhtml "golang.org/x/net/html"
)

var titleHeadingPattern = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

type htmlDocument struct {
	DocumentTitle string
	HTML          string
}

type htmlDocumentOptions struct {
	DocumentTitle string
	Stylesheet    string
	KaTeXAssetDir string
	VisualTitle   bool
}

// MarkdownToStyledHTML converts Markdown content to a complete styled HTML document.
// Uses goldmark with extensions for tables, strikethrough, code highlighting, etc.
func MarkdownToStyledHTML(markdown string) (string, error) {
	stylesheet, err := readPDFStylesheet()
	if err != nil {
		return "", err
	}

	document, err := renderMarkdownDocument(markdown, htmlDocumentOptions{
		Stylesheet:  stylesheet,
		VisualTitle: true,
	})
	if err != nil {
		return "", err
	}

	return document.HTML, nil
}

// MarkdownToHTML converts Markdown to raw HTML (without styling wrapper).
//
// The export package renders user-submitted Markdown for direct download, so the
// output has to show through legitimate raw HTML like `<table>` blocks that
// GFM Markdown otherwise omits. The rendered HTML is therefore first passed
// through a narrow whitelist sanitizer that strips script, event handlers,
// remote/anchor URLs and dangerous attributes; only then is it emitted by
// Goldmark with raw-HTML pass-through enabled.
func MarkdownToHTML(markdown string) (string, error) {
	sanitized, err := sanitizeExportMarkdown(markdown)
	if err != nil {
		return "", fmt.Errorf("sanitize markdown for export: %w", err)
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
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
	if err := md.Convert([]byte(sanitized), &buf); err != nil {
		return "", fmt.Errorf("goldmark conversion failed: %w", err)
	}

	rendered, err := sanitizeExportRenderedHTML(buf.String())
	if err != nil {
		return "", fmt.Errorf("sanitize rendered export HTML: %w", err)
	}

	return rendered, nil
}

// exportAllowedRawTags enumerates the HTML nodes Goldmark may keep when
// sanitizing user-supplied Markdown before letting the renderer emit raw HTML
// (which is required for legitimate `<table>` blocks produced by upstream chat
// exports). Anything outside this set, including `script`, `iframe`, `style`,
// `object`, `embed`, `link`, `form`, and any tag that can carry JavaScript or
// remote state, is dropped together with its subtree.
var exportAllowedRawTags = map[string]struct{}{
	"html": {}, "head": {}, "body": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"p":      {},
	"br":     {},
	"hr":     {},
	"span":   {},
	"strong": {},
	"b":      {}, "em": {}, "i": {}, "u": {}, "s": {}, "del": {}, "ins": {}, "sup": {}, "sub": {}, "small": {},
	"ul":         {},
	"ol":         {},
	"li":         {},
	"blockquote": {},
	"pre":        {},
	"code":       {},
	"table":      {}, "thead": {}, "tbody": {}, "tfoot": {}, "tr": {}, "th": {}, "td": {},
	"figure": {}, "figcaption": {},
	"img": {}, "a": {},
}

// exportAllowedAttributes enumerates attributes that may remain after
// sanitization. All event handlers, remote URLs, and storage/api references
// outside `https`/`http` are rejected by sanitizeRawHTMLAttributes.
var exportAllowedAttributes = map[string]map[string]struct{}{
	"a": {
		"href":   {},
		"title":  {},
		"name":   {},
		"rel":    {},
		"target": {},
	},
	"img": {
		"src":    {},
		"alt":    {},
		"title":  {},
		"width":  {},
		"height": {},
	},
	"th": {
		"scope":   {},
		"colspan": {},
		"rowspan": {},
	},
	"td": {
		"colspan": {},
		"rowspan": {},
	},
	"span": {
		"class": {},
	},
	"p": {
		"class": {},
	},
	"blockquote": {
		"cite": {},
	},
	"code": {
		"class": {},
	},
	"pre": {
		"class": {},
	},
}

// sanitizeExportMarkdown applies the whitelist sanitizer to the Markdown source
// itself. Goldmark treats raw HTML inside Markdown as opaque text by default;
// to allow the renderer to keep table markup produced by upstream chat
// exports, we pre-sanitize the source so unsafe tags never reach the renderer.
//
// golang.org/x/net/html auto-wraps naked content in <html><head/><body>…
// nodes; our sanitizer emits those wrapper tags as opaque containers because
// the Markdown renderer expects only inner content (otherwise Goldmark treats
// the document as raw HTML and omits everything). We therefore drop the
// outer <html>, <head>, and <body> markers and keep only the inner tokens.
func sanitizeExportMarkdown(markdown string) (string, error) {
	bodyTokens, err := collectExportBody(markdown)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	builder.Grow(len(markdown))
	for _, token := range bodyTokens {
		builder.WriteString(token)
	}

	return builder.String(), nil
}

// sanitizeExportRenderedHTML re-applies the whitelist sanitizer to the HTML
// Goldmark produced after the Markdown pass. This guards against any tag that
// slipped through (for example malicious attributes injected into GFM table
// extension output) before the result is fed to Chromium or Pandoc.
func sanitizeExportRenderedHTML(htmlFragment string) (string, error) {
	tokens, err := collectExportBody(htmlFragment)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	builder.Grow(len(htmlFragment))
	for _, token := range tokens {
		builder.WriteString(token)
	}

	return builder.String(), nil
}

// collectExportBody parses the input as HTML and emits the sanitized tokens
// that live inside the document body. When the input has no <body> wrapper
// (legitimate Markdown source), the sanitizer collects whatever the parser
// produced without the surrounding <html>/<head>/<body> wrappers.
func collectExportBody(htmlFragment string) ([]string, error) {
	node, err := xhtml.Parse(strings.NewReader(htmlFragment))
	if err != nil {
		return nil, fmt.Errorf("parse HTML fragment: %w", err)
	}

	tokens := []string{}
	if err := walkExportDocument(node, &tokens); err != nil {
		return nil, err
	}

	return tokens, nil
}

// walkExportDocument walks an HTML document node and sanitizes either the
// inner body (if present) or the entire tree (if there is no body wrapper).
func walkExportDocument(node *xhtml.Node, tokens *[]string) error {
	if node == nil {
		return nil
	}

	bodyFound := false
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode && strings.EqualFold(child.Data, "body") {
			bodyFound = true
			if err := sanitizeExportNode(child, tokens); err != nil {
				return err
			}
			continue
		}

		if child.Type == xhtml.ElementNode && strings.EqualFold(child.Data, "head") {
			continue
		}

		if err := sanitizeExportNode(child, tokens); err != nil {
			return err
		}
	}

	if !bodyFound && node.Type != xhtml.DocumentNode {
		if err := sanitizeExportNode(node, tokens); err != nil {
			return err
		}
	}

	return nil
}

// tokenizeExportHTML sanitizes the given HTML fragment using the export
// whitelist. It returns safe token text that can be safely re-rendered by a
// downstream HTML engine. Whitelist entries are based on
// exportAllowedRawTags and exportAllowedAttributes.
func tokenizeExportHTML(fragment string) ([]string, error) {
	node, err := xhtml.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil, fmt.Errorf("parse HTML fragment: %w", err)
	}

	var tokens []string
	if err := sanitizeExportNode(node, &tokens); err != nil {
		return nil, err
	}

	return tokens, nil
}

// sanitizeExportNode walks an HTML node tree, allowing only tags from
// exportAllowedRawTags and only attributes from exportAllowedAttributes. The
// emitted node text is appended onto tokens so callers can reconstruct the
// fragment.
func sanitizeExportNode(node *xhtml.Node, tokens *[]string) error {
	if node == nil {
		return nil
	}

	switch node.Type {
	case xhtml.ErrorNode, xhtml.CommentNode, xhtml.DoctypeNode:
		return nil
	case xhtml.DocumentNode:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := sanitizeExportNode(child, tokens); err != nil {
				return err
			}
		}
		return nil
	case xhtml.TextNode:
		*tokens = append(*tokens, stdhtml.EscapeString(node.Data))
		return nil
	case xhtml.ElementNode:
		if !isAllowedExportTag(node.Data) {
			return dropExportChildren(node, tokens)
		}

		if isTransparentExportWrapper(node.Data) {
			return dropExportChildren(node, tokens)
		}

		*tokens = append(*tokens, "<"+node.Data)
		allowedAttrs := exportAllowedAttributes[node.Data]
		for _, attr := range node.Attr {
			if !isAllowedExportAttribute(node.Data, attr.Key, attr.Val, allowedAttrs) {
				continue
			}
			*tokens = append(*tokens, " "+attr.Key+"=\""+stdhtml.EscapeString(attr.Val)+"\"")
		}
		*tokens = append(*tokens, ">")

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := sanitizeExportNode(child, tokens); err != nil {
				return err
			}
		}

		if !isVoidExportTag(node.Data) {
			*tokens = append(*tokens, "</"+node.Data+">")
		}
		return nil
	}

	return nil
}

// isTransparentExportWrapper reports whether the given tag should be passed
// through without rendering its wrapper element. golang.org/x/net/html auto-
// wraps naked content with html/head/body, so we strip those wrappers and
// keep only their children.
func isTransparentExportWrapper(tag string) bool {
	switch strings.ToLower(tag) {
	case "html", "head", "body":
		return true
	}
	return false
}

// dropExportChildren serializes the children of node (recursively) as plain
// text, dropping the surrounding tag. We still pass the children through the
// sanitizer so any unsafe content is removed instead of rendered verbatim.
func dropExportChildren(node *xhtml.Node, tokens *[]string) error {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := sanitizeExportNode(child, tokens); err != nil {
			return err
		}
	}
	return nil
}

// isAllowedExportTag reports whether the given HTML tag may remain in the
// sanitized export output.
func isAllowedExportTag(tag string) bool {
	_, ok := exportAllowedRawTags[strings.ToLower(tag)]
	return ok
}

// isAllowedExportAttribute reports whether the given attribute is permitted on
// the tag. For `a[href]`, `img[src]` and `blockquote[cite]`, only `http` and
// `https` URLs are kept; any other scheme (including `javascript:`, `data:` or
// `minio:`) is stripped.
func isAllowedExportAttribute(tag string, key string, value string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return false
	}

	if _, ok := allowed[strings.ToLower(key)]; !ok {
		return false
	}

	switch strings.ToLower(tag) + "|" + strings.ToLower(key) {
	case "a|href", "img|src", "blockquote|cite":
		return isSafeExportURL(value)
	}

	return true
}

// isSafeExportURL ensures that an attribute value is either a relative URL, a
// fragment, an email-style value, or an absolute URL whose scheme is one of
// `http` / `https` / `mailto`. This blocks `javascript:`, `data:`, `vbscript:`,
// `file:` and other dangerous schemes that arbitrary Markdown could otherwise
// inject.
func isSafeExportURL(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}

	if strings.HasPrefix(trimmed, "#") {
		return true
	}

	lower := strings.ToLower(trimmed)
	for _, scheme := range []string{"javascript:", "data:", "vbscript:", "file:"} {
		if strings.HasPrefix(lower, scheme) {
			return false
		}
	}

	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") {
		return true
	}

	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") {
		return true
	}

	return false
}

// isVoidExportTag reports whether the HTML tag is a void element that does not
// require a closing tag in HTML output.
func isVoidExportTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "br", "hr", "img":
		return true
	}
	return false
}

func buildPDFHTMLDocument(markdown string) (htmlDocument, error) {
	stylesheet, err := readPDFStylesheet()
	if err != nil {
		return htmlDocument{}, err
	}

	return renderMarkdownDocument(markdown, htmlDocumentOptions{
		Stylesheet:    stylesheet,
		KaTeXAssetDir: "./katex",
		VisualTitle:   false,
	})
}

func renderMarkdownDocument(markdown string, options htmlDocumentOptions) (htmlDocument, error) {
	rawHTML, err := MarkdownToHTML(markdown)
	if err != nil {
		return htmlDocument{}, fmt.Errorf("goldmark conversion failed: %w", err)
	}

	documentTitle := strings.TrimSpace(options.DocumentTitle)
	if documentTitle == "" {
		documentTitle = deriveDocumentTitle(markdown)
	}

	return htmlDocument{
		DocumentTitle: documentTitle,
		HTML:          buildHTMLDocument(rawHTML, documentTitle, options),
	}, nil
}

func buildHTMLDocument(rawHTML string, documentTitle string, options htmlDocumentOptions) string {
	escapedDocumentTitle := stdhtml.EscapeString(documentTitle)
	escapedKaTeXAssetDir := stdhtml.EscapeString(strings.TrimSpace(options.KaTeXAssetDir))

	var builder strings.Builder
	builder.Grow(len(rawHTML) + len(options.Stylesheet) + 1024)
	builder.WriteString("<!DOCTYPE html>\n")
	builder.WriteString("<html lang=\"zh-CN\">\n")
	builder.WriteString("<head>\n")
	builder.WriteString("  <meta charset=\"utf-8\">\n")
	builder.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	builder.WriteString("  <title>")
	builder.WriteString(escapedDocumentTitle)
	builder.WriteString("</title>\n")
	if options.Stylesheet != "" {
		builder.WriteString("  <style>\n")
		builder.WriteString(options.Stylesheet)
		builder.WriteString("\n  </style>\n")
	}
	if escapedKaTeXAssetDir != "" {
		builder.WriteString("  <link rel=\"stylesheet\" href=\"")
		builder.WriteString(escapedKaTeXAssetDir)
		builder.WriteString("/katex.min.css\">\n")
		builder.WriteString("  <script defer src=\"")
		builder.WriteString(escapedKaTeXAssetDir)
		builder.WriteString("/katex.min.js\"></script>\n")
		builder.WriteString("  <script defer src=\"")
		builder.WriteString(escapedKaTeXAssetDir)
		builder.WriteString("/contrib/auto-render.min.js\"></script>\n")
	}
	builder.WriteString("</head>\n")
	builder.WriteString("<body>\n")
	builder.WriteString("  <div class=\"export-page\">\n")
	builder.WriteString("    <main class=\"export-paper\">\n")
	if options.VisualTitle {
		builder.WriteString("      <header class=\"export-header\">\n")
		builder.WriteString("        <h1 class=\"export-title\">")
		builder.WriteString(escapedDocumentTitle)
		builder.WriteString("</h1>\n")
		builder.WriteString("      </header>\n")
	}
	builder.WriteString("      <article class=\"export-content markdown-body\">\n")
	builder.WriteString(rawHTML)
	builder.WriteString("\n      </article>\n")
	builder.WriteString("    </main>\n")
	builder.WriteString("  </div>\n")
	builder.WriteString(buildExportReadyScript(escapedKaTeXAssetDir != ""))
	builder.WriteString("</body>\n")
	builder.WriteString("</html>")

	return builder.String()
}

func buildExportReadyScript(enableKaTeX bool) string {
	if !enableKaTeX {
		return "  <script>window.__WEKNORA_EXPORT_READY__ = true;</script>\n"
	}

	return `  <script>
    (() => {
      window.__WEKNORA_EXPORT_READY__ = false;

      const markReady = () => {
        window.requestAnimationFrame(() => {
          window.requestAnimationFrame(() => {
            window.__WEKNORA_EXPORT_READY__ = true;
          });
        });
      };

      const renderMath = () => {
        const content = document.querySelector('.export-content');
        if (!content || typeof window.renderMathInElement !== 'function') {
          markReady();
          return;
        }

        try {
          window.renderMathInElement(content, {
            delimiters: [
              { left: '$$', right: '$$', display: true },
              { left: '$', right: '$', display: false },
              { left: '\\(', right: '\\)', display: false },
              { left: '\\[', right: '\\]', display: true }
            ],
            throwOnError: false,
            strict: 'ignore'
          });
        } catch (error) {
          console.error('KaTeX render failed', error);
        }

        markReady();
      };

      if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', renderMath, { once: true });
        return;
      }

      renderMath();
    })();
  </script>
`
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

func extractLeadingDocumentTitle(markdown string) (title string, remainder string, ok bool) {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	trimmed := strings.TrimLeft(normalized, "\ufeff \t\n")
	if trimmed == "" {
		return "", markdown, false
	}

	lineEnd := strings.IndexByte(trimmed, '\n')
	headingLine := trimmed
	remaining := ""
	if lineEnd >= 0 {
		headingLine = trimmed[:lineEnd]
		remaining = trimmed[lineEnd+1:]
	}

	if !strings.HasPrefix(headingLine, "# ") {
		return "", markdown, false
	}

	title = strings.TrimSpace(strings.TrimPrefix(headingLine, "# "))
	if title == "" {
		return "", markdown, false
	}

	remaining = strings.TrimLeft(remaining, "\n")
	return title, remaining, true
}
