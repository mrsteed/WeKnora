package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMarkdownToHTMLEscapesRawHTML(t *testing.T) {
	html, err := MarkdownToHTML("安全测试 <script>alert('xss')</script>")
	if err != nil {
		t.Fatalf("MarkdownToHTML returned error: %v", err)
	}

	if strings.Contains(html, "<script>") {
		t.Fatalf("expected raw script tag to be stripped, got %q", html)
	}

	if !strings.Contains(html, "安全测试") || !strings.Contains(html, "alert") {
		t.Fatalf("expected surrounding text payload to remain in output, got %q", html)
	}
}

func TestMarkdownToHTMLPreservesHTMLTables(t *testing.T) {
	markdown := "## 表1\n<table><tr><td>列1</td><td>列2</td></tr><tr><td>值1</td><td>值2</td></tr></table>"

	html, err := MarkdownToHTML(markdown)
	if err != nil {
		t.Fatalf("MarkdownToHTML returned error: %v", err)
	}

	if !strings.Contains(html, "<table>") {
		t.Fatalf("expected <table> in HTML output, got %q", html)
	}
	if !strings.Contains(html, "<tr>") {
		t.Fatalf("expected <tr> in HTML output, got %q", html)
	}
	if !strings.Contains(html, "<td>列1</td>") {
		t.Fatalf("expected table cell content to be preserved, got %q", html)
	}
}

func TestMarkdownToHTMLStripsScriptTags(t *testing.T) {
	html, err := MarkdownToHTML("正文 <script>alert(1)</script> <style>body{color:red}</style> 结束")
	if err != nil {
		t.Fatalf("MarkdownToHTML returned error: %v", err)
	}

	if strings.Contains(strings.ToLower(html), "<script") || strings.Contains(strings.ToLower(html), "<style") {
		t.Fatalf("expected script/style tags to be stripped, got %q", html)
	}
}

func TestMarkdownToHTMLStripsDangerousURLSchemes(t *testing.T) {
	cases := map[string]string{
		"javascript": "点击 <a href=\"javascript:alert(1)\">这里</a>",
		"data":       "<a href=\"data:text/html,<script>alert(1)</script>\">点击</a>",
		"minio":      "<img src=\"minio://weknora/10002/exports/example.jpg\" alt=\"x\">",
	}

	for name, markdown := range cases {
		if name == "minio" {
			html, err := MarkdownToHTML(markdown)
			if err != nil {
				t.Fatalf("%s: MarkdownToHTML returned error: %v", name, err)
			}
			if strings.Contains(html, "minio:") {
				t.Fatalf("%s: expected minio:// URL to be stripped, got %q", name, html)
			}
			continue
		}

		html, err := MarkdownToHTML(markdown)
		if err != nil {
			t.Fatalf("%s: MarkdownToHTML returned error: %v", name, err)
		}
		if strings.Contains(strings.ToLower(html), "javascript:") || strings.Contains(strings.ToLower(html), "data:text") {
			t.Fatalf("%s: expected dangerous URL to be stripped, got %q", name, html)
		}
	}
}

func TestBuildPDFHTMLDocumentIncludesKaTeXBootstrap(t *testing.T) {
	assetRoot := t.TempDir()
	stylePath := filepath.Join(assetRoot, "pdf.css")
	katexDir := filepath.Join(assetRoot, "katex")
	if err := os.MkdirAll(filepath.Join(katexDir, "contrib"), 0o755); err != nil {
		t.Fatalf("mkdir katex contrib: %v", err)
	}

	for filePath, content := range map[string]string{
		stylePath:                                                "body { color: #111827; }",
		filepath.Join(katexDir, "katex.min.css"):                 ".katex { font-size: 1em; }",
		filepath.Join(katexDir, "katex.min.js"):                  "window.katex = {};",
		filepath.Join(katexDir, "contrib", "auto-render.min.js"): "window.renderMathInElement = function () {};",
	} {
		if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
			t.Fatalf("write asset %s: %v", filePath, err)
		}
	}

	t.Setenv(exportPDFStylePathEnv, stylePath)

	document, err := buildPDFHTMLDocument("# 导出标题\n\n公式：$E=mc^2$")
	if err != nil {
		t.Fatalf("buildPDFHTMLDocument returned error: %v", err)
	}

	if document.DocumentTitle != "导出标题" {
		t.Fatalf("unexpected document title %q", document.DocumentTitle)
	}

	if !strings.Contains(document.HTML, "./katex/katex.min.css") {
		t.Fatalf("expected KaTeX stylesheet reference in HTML")
	}

	if !strings.Contains(document.HTML, "renderMathInElement") {
		t.Fatalf("expected KaTeX bootstrap script in HTML")
	}

	if !strings.Contains(document.HTML, "window.__WEKNORA_EXPORT_READY__ = true") {
		t.Fatalf("expected readiness marker in HTML")
	}
}

func TestBuildPDFHTMLDocumentDoesNotInjectVisualTitle(t *testing.T) {
	assetRoot := t.TempDir()
	stylePath := filepath.Join(assetRoot, "pdf.css")
	katexDir := filepath.Join(assetRoot, "katex")
	if err := os.MkdirAll(filepath.Join(katexDir, "contrib"), 0o755); err != nil {
		t.Fatalf("mkdir katex contrib: %v", err)
	}

	for filePath, content := range map[string]string{
		stylePath:                                                "body { color: #111827; }",
		filepath.Join(katexDir, "katex.min.css"):                 ".katex { font-size: 1em; }",
		filepath.Join(katexDir, "katex.min.js"):                  "window.katex = {};",
		filepath.Join(katexDir, "contrib", "auto-render.min.js"): "window.renderMathInElement = function () {};",
	} {
		if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
			t.Fatalf("write asset %s: %v", filePath, err)
		}
	}

	t.Setenv(exportPDFStylePathEnv, stylePath)

	document, err := buildPDFHTMLDocument("研究\n\n开放获取\n\n# 导出标题\n\n正文")
	if err != nil {
		t.Fatalf("buildPDFHTMLDocument returned error: %v", err)
	}

	if strings.Contains(document.HTML, "export-title") {
		t.Fatalf("expected PDF HTML to omit synthetic export title, got %q", document.HTML)
	}
	if strings.Count(document.HTML, "<h1") != 1 {
		t.Fatalf("expected PDF HTML to contain only the source heading, got %q", document.HTML)
	}
}

func TestResolveDocxAssetsHonorsExplicitEnv(t *testing.T) {
	referenceDocPath := filepath.Join(t.TempDir(), "reference.docx")
	if err := os.WriteFile(referenceDocPath, []byte("docx-template"), 0o600); err != nil {
		t.Fatalf("write reference doc: %v", err)
	}

	t.Setenv(exportReferenceDocxEnv, referenceDocPath)

	assets, err := resolveDocxAssets()
	if err != nil {
		t.Fatalf("resolveDocxAssets returned error: %v", err)
	}

	absPath, err := filepath.Abs(referenceDocPath)
	if err != nil {
		t.Fatalf("abs reference doc path: %v", err)
	}

	if assets.referenceDocxPath != absPath {
		t.Fatalf("unexpected reference doc path %q, want %q", assets.referenceDocxPath, absPath)
	}
}

func TestExtractLeadingDocumentTitle(t *testing.T) {
	title, remainder, ok := extractLeadingDocumentTitle("\n# 导出标题\n\n正文第一段\n")
	if !ok {
		t.Fatalf("expected leading title to be detected")
	}

	if title != "导出标题" {
		t.Fatalf("unexpected title %q", title)
	}

	if remainder != "正文第一段\n" {
		t.Fatalf("unexpected remainder %q", remainder)
	}
}

func TestPrepareDocxTitleAndBodyPreservesNonLeadingHeading(t *testing.T) {
	markdown := "开场段落\n\n# 后续标题\n\n正文"
	title, body := prepareDocxTitleAndBody(markdown)

	if title != "后续标题" {
		t.Fatalf("unexpected title %q", title)
	}

	if body != markdown {
		t.Fatalf("expected body to remain unchanged when heading is not leading")
	}
}

func TestReferenceDocxTemplateContainsHeaderFooterAndStyles(t *testing.T) {
	assetPath := referenceDocxFixturePath(t)
	reader, err := zip.OpenReader(assetPath)
	if err != nil {
		t.Fatalf("open reference docx: %v", err)
	}
	defer reader.Close()

	entries := map[string][]byte{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		entries[file.Name] = data
	}

	if _, ok := entries["word/header1.xml"]; ok {
		t.Fatalf("expected reference template to omit header1.xml")
	}
	if _, ok := entries["word/footer1.xml"]; !ok {
		t.Fatalf("expected footer1.xml in reference template")
	}

	documentXML := string(entries["word/document.xml"])
	if strings.Contains(documentXML, "headerReference") || !strings.Contains(documentXML, "footerReference") {
		t.Fatalf("expected reference template document.xml to declare footer without header references")
	}

	relsXML := string(entries["word/_rels/document.xml.rels"])
	if strings.Contains(relsXML, "ns0:Relationship") {
		t.Fatalf("expected document relationships to avoid undeclared namespace prefixes, got %q", relsXML)
	}
	if err := ensureXMLWellFormed(entries["word/_rels/document.xml.rels"]); err != nil {
		t.Fatalf("expected document relationships XML to be well formed: %v", err)
	}

	stylesXML := string(entries["word/styles.xml"])
	for _, needle := range []string{"styleId=\"SourceCode\"", "Microsoft YaHei", "SimSun", "F8FAFC", "EAF2FF"} {
		if !strings.Contains(stylesXML, needle) {
			t.Fatalf("expected styles.xml to contain %q", needle)
		}
	}
}

func TestReferenceDocxTableStyleIsVisible(t *testing.T) {
	assetPath := referenceDocxFixturePath(t)
	reader, err := zip.OpenReader(assetPath)
	if err != nil {
		t.Fatalf("open reference docx: %v", err)
	}
	defer reader.Close()

	entries := map[string][]byte{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		entries[file.Name] = data
	}

	stylesXML := string(entries["word/styles.xml"])
	if strings.Contains(stylesXML, `styleId="Table"`+strings.Repeat(` `, 0)+`>.*<w:semiHidden />`) {
		t.Fatalf("expected Table style to not be marked semiHidden")
	}

	tableBlock := strings.Index(stylesXML, `styleId="Table"`)
	if tableBlock < 0 {
		t.Fatalf("expected styles.xml to define Table style")
	}
	tableBlockSection := stylesXML[tableBlock : tableBlock+2000]
	if strings.Contains(tableBlockSection, "<w:semiHidden") {
		t.Fatalf("expected Table style to not include semiHidden, got block:\n%s", tableBlockSection)
	}
	if strings.Contains(tableBlockSection, "<w:unhideWhenUsed") {
		t.Fatalf("expected Table style to not include unhideWhenUsed, got block:\n%s", tableBlockSection)
	}

	settingsXML := string(entries["word/settings.xml"])
	for _, needle := range []string{`<w:stylePaneFormatFilter w:val="0000"`, `<w:tblBorders>`} {
		if !strings.Contains(settingsXML, needle) && !strings.Contains(stylesXML, needle) {
			t.Fatalf("expected template to contain %q", needle)
		}
	}
	if !strings.Contains(settingsXML, `<w:updateFields w:val="false"`) {
		t.Fatalf("expected template to disable automatic field updates, got %q", settingsXML)
	}
}

func TestMarkdownToDocxConvertsHTMLTablesToWordTables(t *testing.T) {
	if !IsPandocAvailable() {
		t.Skip("pandoc is not installed")
	}

	t.Setenv(exportReferenceDocxEnv, referenceDocxFixturePath(t))

	markdown := "# 文档标题\n\n表 1\n<table><tr><td rowspan=\"2\"></td><td colspan=\"2\">FLACC 的 AUC</td></tr><tr><td>C 组</td><td>D 组</td></tr><tr><td>静息状态</td><td>37.25</td><td>19.25</td></tr></table>\n"

	data, err := MarkdownToDocx(context.Background(), markdown)
	if err != nil {
		t.Fatalf("MarkdownToDocx returned error: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open generated docx: %v", err)
	}

	documentXML := ""
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open generated entry %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read generated entry %s: %v", file.Name, err)
		}
		documentXML = string(content)
		break
	}

	if !strings.Contains(documentXML, "<w:tbl>") {
		t.Fatalf("expected generated document.xml to contain a Word table, got %q", documentXML)
	}
	for _, want := range []string{"FLACC", "AUC", "C ", "D ", "静息状态", "37.25", "19.25"} {
		if !strings.Contains(documentXML, want) {
			t.Fatalf("expected generated Word table to retain %q, got %q", want, documentXML)
		}
	}
	if strings.Contains(documentXML, "<table") {
		t.Fatalf("expected raw HTML table markup to be absent from generated document.xml, got %q", documentXML)
	}
}

func TestMarkdownToDocxUsesReferenceTemplate(t *testing.T) {
	if !IsPandocAvailable() {
		t.Skip("pandoc is not installed")
	}

	t.Setenv(exportReferenceDocxEnv, referenceDocxFixturePath(t))

	data, err := MarkdownToDocx(context.Background(), "研究\n\n开放获取\n\n# 文档标题\n\n正文段落\n\n```go\nfmt.Println(\"hello\")\n```\n")
	if err != nil {
		t.Fatalf("MarkdownToDocx returned error: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open generated docx: %v", err)
	}

	entries := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open generated entry %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read generated entry %s: %v", file.Name, err)
		}
		entries[file.Name] = string(content)
	}

	documentXML := entries["word/document.xml"]
	if strings.Count(documentXML, ">文档标题<") != 1 {
		t.Fatalf("expected visible title text to appear once in document.xml, got %d occurrences", strings.Count(documentXML, ">文档标题<"))
	}
	if strings.Contains(documentXML, "w:pStyle w:val=\"Title\"") {
		t.Fatalf("expected generated document to avoid synthetic Title style block")
	}
	if !strings.Contains(documentXML, "w:pStyle w:val=\"SourceCode\"") {
		t.Fatalf("expected generated document to use SourceCode style")
	}
	if strings.Contains(documentXML, "headerReference") || !strings.Contains(documentXML, "footerReference") {
		t.Fatalf("expected generated document to preserve footer without synthetic header")
	}

	relsXML := entries["word/_rels/document.xml.rels"]
	if strings.Contains(relsXML, "ns0:Relationship") {
		t.Fatalf("expected generated document relationships to avoid undeclared namespace prefixes")
	}
	if err := ensureXMLWellFormed([]byte(relsXML)); err != nil {
		t.Fatalf("expected generated document relationships XML to be well formed: %v", err)
	}
}

func TestMaterializeStorageImagesRewritesProviderMarkdownAndHTMLImages(t *testing.T) {
	resolver := StorageImageResolver
	StorageImageResolver = func(context.Context, string) ([]byte, bool) {
		return tinyPNGBytes(t), true
	}
	t.Cleanup(func() {
		StorageImageResolver = resolver
	})

	baseDir := t.TempDir()
	updated, err := materializeStorageImages(context.Background(), "![图](minio://bucket/10002/exports/a.png \"标题\")\n<img src=\"local://10002/exports/b.png\" alt=\"b\">", baseDir)
	if err != nil {
		t.Fatalf("materializeStorageImages returned error: %v", err)
	}

	for _, want := range []string{"./export-images/image-001.png", "./export-images/image-002.png"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected rewritten markdown to contain %q, got %q", want, updated)
		}
	}
	for _, path := range []string{
		filepath.Join(baseDir, "export-images", "image-001.png"),
		filepath.Join(baseDir, "export-images", "image-002.png"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected exported image asset %s to exist: %v", path, err)
		}
	}
}

func TestPreparePDFDocumentMaterializesProviderImages(t *testing.T) {
	assetRoot := t.TempDir()
	stylePath := filepath.Join(assetRoot, "pdf.css")
	katexDir := filepath.Join(assetRoot, "katex")
	if err := os.MkdirAll(filepath.Join(katexDir, "contrib"), 0o755); err != nil {
		t.Fatalf("mkdir katex contrib: %v", err)
	}

	for filePath, content := range map[string]string{
		stylePath:                                                "body { color: #111827; }",
		filepath.Join(katexDir, "katex.min.css"):                 ".katex { font-size: 1em; }",
		filepath.Join(katexDir, "katex.min.js"):                  "window.katex = {};",
		filepath.Join(katexDir, "contrib", "auto-render.min.js"): "window.renderMathInElement = function () {};",
	} {
		if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
			t.Fatalf("write asset %s: %v", filePath, err)
		}
	}
	t.Setenv(exportPDFStylePathEnv, stylePath)

	resolver := StorageImageResolver
	StorageImageResolver = func(context.Context, string) ([]byte, bool) {
		return tinyPNGBytes(t), true
	}
	t.Cleanup(func() {
		StorageImageResolver = resolver
	})

	document, tempDir, err := preparePDFDocument(context.Background(), "# 标题\n\n![](minio://bucket/10002/exports/a.png)")
	if err != nil {
		t.Fatalf("preparePDFDocument returned error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	htmlBytes, err := os.ReadFile(document.htmlPath)
	if err != nil {
		t.Fatalf("read generated html: %v", err)
	}
	html := string(htmlBytes)
	if !strings.Contains(html, "./export-images/image-001.png") {
		t.Fatalf("expected generated HTML to reference localized image asset, got %q", html)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "export-images", "image-001.png")); err != nil {
		t.Fatalf("expected localized image asset to exist: %v", err)
	}
}

func TestMarkdownToDocxEmbedsProviderImages(t *testing.T) {
	if !IsPandocAvailable() {
		t.Skip("pandoc is not installed")
	}

	t.Setenv(exportReferenceDocxEnv, referenceDocxFixturePath(t))
	resolver := StorageImageResolver
	StorageImageResolver = func(context.Context, string) ([]byte, bool) {
		return tinyPNGBytes(t), true
	}
	t.Cleanup(func() {
		StorageImageResolver = resolver
	})

	data, err := MarkdownToDocx(context.Background(), "# 文档标题\n\n![](minio://bucket/10002/exports/a.png)\n")
	if err != nil {
		t.Fatalf("MarkdownToDocx returned error: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open generated docx: %v", err)
	}

	foundImage := false
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "word/media/") {
			foundImage = true
			break
		}
	}
	if !foundImage {
		t.Fatalf("expected generated docx to embed provider image in word/media")
	}
}

func TestMarkdownToDocxRendersHTMLTableMath(t *testing.T) {
	if !IsPandocAvailable() {
		t.Skip("pandoc is not installed")
	}

	t.Setenv(exportReferenceDocxEnv, referenceDocxFixturePath(t))

	markdown := "表4\n\n<table><tr><td>变量</td><td>时间点</td><td>D组 $( n = 3 4 )$ </td><td>E组 $( n = 3 4 )$ </td><td>C组 $( n = 3 4 )$ </td><td> $P$ </td></tr><tr><td>HR (bpm)</td><td> $\\mathrm { T } _ { 0 }$ </td><td> $9 4 . 5 3 \\pm 1 4 . 7 9$ </td><td> $9 0 . 5 6 \\pm 1 2 . 0 4$ </td><td> $9 6 . 7 6 \\pm 1 5 . 1 0$ </td><td>0.187</td></tr></table>\n"

	data, err := MarkdownToDocx(context.Background(), markdown)
	if err != nil {
		t.Fatalf("MarkdownToDocx returned error: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open generated docx: %v", err)
	}

	documentXML := ""
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open generated entry %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read generated entry %s: %v", file.Name, err)
		}
		documentXML = string(content)
		break
	}

	if !strings.Contains(documentXML, "<m:oMath") {
		t.Fatalf("expected generated table cells to contain Word math objects, got %q", documentXML)
	}
	for _, forbidden := range []string{`$\\mathrm`, `$9 4 . 5 3`, `\\pm`, `\\mathrm`} {
		if strings.Contains(documentXML, forbidden) {
			t.Fatalf("expected generated document.xml to strip raw TeX fragment %q, got %q", forbidden, documentXML)
		}
	}
}

func TestMarkdownToPDFRendersDocument(t *testing.T) {
	if !IsChromiumAvailable() {
		t.Skip("chromium is not installed")
	}

	data, err := MarkdownToPDF(context.Background(), "# PDF 标题\n\n这是一个 PDF 导出集成测试。")
	if err != nil {
		t.Fatalf("MarkdownToPDF returned error: %v", err)
	}

	if len(data) == 0 {
		t.Fatalf("expected generated PDF to be non-empty")
	}

	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("expected generated bytes to look like a PDF, got prefix %q", data[:minInt(len(data), 8)])
	}
}

func TestMarkdownToPDFRendersLargeDocumentWithHTMLTables(t *testing.T) {
	if !IsChromiumAvailable() {
		t.Skip("chromium is not installed")
	}

	var builder strings.Builder
	builder.WriteString("研究\n\n开放获取\n\n# 鼻内右美托咪定与艾司氯胺酮用于儿童扁桃体切除术和腺样体切除术术前用药对术后疼痛的影响：一项随机临床试验\n\n")
	builder.WriteString("![](minio://weknora/10002/exports/example.jpg)\n\n")

	paragraph := "扁桃体切除术是儿童最常见的外科手术之一，术后疼痛可能持续较长时间。右美托咪定通过刺激脊髓背角的α-2受体来减轻疼痛，艾司氯胺酮通过抑制NMDA受体并阻断疼痛信号传入，从而共同减轻疼痛。\n\n"
	for i := 0; i < 140; i++ {
		builder.WriteString("## 研究段落\n\n")
		builder.WriteString(paragraph)
	}

	builder.WriteString("表 3 患者的其他次要结局\n\n")
	builder.WriteString("<table><tr><td></td><td>C组 (N=58)</td><td>D组 (N=58)</td><td>DS组 (N=57)</td><td>P值</td></tr>")
	builder.WriteString("<tr><td>鼻内镇静成功患者数 (n, %)</td><td>/</td><td>52(89.7%)</td><td>52(91.2%)</td><td></td></tr>")
	builder.WriteString("<tr><td>达到满意镇静所需时间 (min)</td><td>/</td><td>22.41(2.47)</td><td>18.65(2.15)</td><td>&lt; 0.001</td></tr>")
	builder.WriteString("<tr><td>ICC评分</td><td>3(2-5)</td><td>2(1-2) a,b</td><td>1(0-2) a</td><td>0.001</td></tr></table>\n")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	data, err := MarkdownToPDF(ctx, builder.String())
	if err != nil {
		t.Fatalf("MarkdownToPDF returned error for large document fixture: %v", err)
	}

	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("expected generated bytes to look like a PDF, got prefix %q", data[:minInt(len(data), 8)])
	}
}

func referenceDocxFixturePath(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}

	return filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "config", "export", "reference.docx")
}

func ensureXMLWellFormed(data []byte) error {
	var value any
	return xml.Unmarshal(data, &value)
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y8y8AAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode tiny png: %v", err)
	}
	return data
}
