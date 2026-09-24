// Package export provides document export utilities for converting
// Markdown content to PDF and DOCX formats using backend tools
// (Chromium print-to-PDF for PDF, pandoc for DOCX).
package export

import (
	"bytes"
	"context"
	"fmt"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const exportChromeBinEnv = "WEKNORA_EXPORT_CHROME_BIN"

var (
	pdfExportLimiterOnce  sync.Once
	pdfExportLimiter      chan struct{}
	docxExportLimiterOnce sync.Once
	docxExportLimiter     chan struct{}
)

// MarkdownToPDF converts Markdown content to PDF.
// It first converts Markdown to styled HTML using goldmark,
// then uses headless Chromium to render HTML to PDF.
func MarkdownToPDF(ctx context.Context, markdown string) ([]byte, error) {
	return withExportLimiter(ctx, pdfLimiter(), func() ([]byte, error) {
		if err := validatePDFRuntime(); err != nil {
			return nil, err
		}

		document, tempDir, err := preparePDFDocument(ctx, markdown)
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tempDir)

		return htmlToPDF(ctx, document.htmlPath, document.browserProfileDir, document.documentTitle, time.Now())
	})
}

// MarkdownToDocx converts Markdown content to DOCX.
// It uses pandoc to convert Markdown directly to DOCX format.
func MarkdownToDocx(ctx context.Context, markdown string) ([]byte, error) {
	return withExportLimiter(ctx, docxLimiter(), func() ([]byte, error) {
		return markdownToDocxViaPandoc(ctx, markdown)
	})
}

// MarkdownToXLSX converts Markdown content to a workbook that keeps
// narrative text on an overview sheet and extracts standard Markdown tables
// into dedicated sheets for server-side download.
func MarkdownToXLSX(ctx context.Context, markdown string) ([]byte, error) {
	data, err := markdownToXLSX(ctx, markdown)
	if err != nil {
		return nil, err
	}

	logger.Infof(ctx, "[Export] Successfully generated XLSX, size: %d bytes", len(data))
	return data, nil
}

// htmlToPDF converts HTML to PDF using headless Chromium.
// If Chromium is not available, returns a descriptive error.
func htmlToPDF(ctx context.Context, htmlPath string, browserProfileDir string, documentTitle string, generatedAt time.Time) ([]byte, error) {
	chromePath, ok := findChromiumExecutable()
	if !ok {
		return nil, fmt.Errorf("chromium is not installed. Set %s or install chromium/google-chrome on the server", exportChromeBinEnv)
	}

	pdfData, err := renderPDFWithChromium(ctx, chromePath, htmlPath, browserProfileDir, documentTitle, generatedAt, runningAsRoot())
	if err == nil {
		logger.Infof(ctx, "[Export] Successfully generated PDF, size: %d bytes", len(pdfData))
		return pdfData, nil
	}

	if !runningAsRoot() && shouldRetryWithoutSandbox(err) {
		logger.Warnf(ctx, "[Export] sandboxed chromium render failed, retrying without sandbox: %v", err)
		pdfData, retryErr := renderPDFWithChromium(ctx, chromePath, htmlPath, browserProfileDir, documentTitle, generatedAt, true)
		if retryErr == nil {
			logger.Infof(ctx, "[Export] Successfully generated PDF after sandbox fallback, size: %d bytes", len(pdfData))
			return pdfData, nil
		}
		err = fmt.Errorf("sandboxed render failed: %w; fallback without sandbox failed: %v", err, retryErr)
	}

	logger.Errorf(ctx, "[Export] chromium print-to-pdf failed: %v", err)
	return nil, fmt.Errorf("chromium print-to-pdf failed: %w", err)
}

// markdownToDocxViaPandoc converts Markdown to DOCX using pandoc.
func markdownToDocxViaPandoc(ctx context.Context, markdown string) ([]byte, error) {
	if err := validateDocxRuntime(); err != nil {
		return nil, err
	}

	assets, err := resolveDocxAssets()
	if err != nil {
		return nil, err
	}

	workDir, err := os.MkdirTemp("", "weknora_export_docx_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create DOCX export temp directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	markdownBody, err := materializeStorageImages(ctx, markdown, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare DOCX images: %w", err)
	}
	markdownBody = docparser.NormalizeHTMLTables(markdownBody)
	markdownBody = repairDocxTableMathMarkdown(markdownBody)

	// Write Markdown to temp file
	mdPath := filepath.Join(workDir, "document.md")
	mdFile, err := os.Create(mdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp markdown file: %w", err)
	}

	if _, err := mdFile.WriteString(markdownBody); err != nil {
		mdFile.Close()
		return nil, fmt.Errorf("failed to write markdown temp file: %w", err)
	}
	mdFile.Close()

	// Create temp output file for DOCX
	docxPath := filepath.Join(workDir, "document.docx")
	docxFile, err := os.Create(docxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp DOCX file: %w", err)
	}
	docxFile.Close()

	// Run pandoc
	// Options:
	//   -f markdown              : from Markdown
	//   -t docx                  : to DOCX
	//   --wrap=none              : don't wrap lines
	//   -o output.docx           : output file
	cmd := exec.CommandContext(ctx, "pandoc",
		"-f", "markdown",
		"-t", "docx",
		"--wrap=none",
		"--reference-doc", assets.referenceDocxPath,
		"-o", docxPath,
		mdPath,
	)
	cmd.Dir = workDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Errorf(ctx, "[Export] pandoc failed: %v, stderr: %s", err, stderr.String())
		return nil, fmt.Errorf("pandoc failed: %w (stderr: %s)", err, stderr.String())
	}

	// Read the generated DOCX
	data, err := os.ReadFile(docxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read generated DOCX: %w", err)
	}

	logger.Infof(ctx, "[Export] Successfully generated DOCX, size: %d bytes", len(data))
	return data, nil
}

func prepareDocxTitleAndBody(markdown string) (title string, body string) {
	if extractedTitle, remainder, ok := extractLeadingDocumentTitle(markdown); ok {
		if strings.TrimSpace(remainder) == "" {
			return extractedTitle, ""
		}
		return extractedTitle, remainder
	}

	return deriveDocumentTitle(markdown), markdown
}

// IsChromiumAvailable checks if a Chromium-compatible executable is installed and available.
func IsChromiumAvailable() bool {
	_, ok := findChromiumExecutable()
	return ok
}

// IsPandocAvailable checks if pandoc is installed and available in PATH.
func IsPandocAvailable() bool {
	_, err := exec.LookPath("pandoc")
	return err == nil
}

func findChromiumExecutable() (string, bool) {
	if configured := strings.TrimSpace(os.Getenv(exportChromeBinEnv)); configured != "" {
		if path, err := exec.LookPath(configured); err == nil {
			return path, true
		}
	}

	for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, true
		}
	}

	return "", false
}

type pdfDocument struct {
	documentTitle     string
	htmlPath          string
	browserProfileDir string
}

func preparePDFDocument(ctx context.Context, markdown string) (pdfDocument, string, error) {
	assets, err := resolvePDFAssets()
	if err != nil {
		return pdfDocument{}, "", err
	}

	tempDir, err := os.MkdirTemp(chromiumWorkingDirectoryBase(), "weknora_export_pdf_*")
	if err != nil {
		return pdfDocument{}, "", fmt.Errorf("failed to create export temp directory: %w", err)
	}

	preparedMarkdown, err := materializeStorageImages(ctx, markdown, tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		return pdfDocument{}, "", fmt.Errorf("failed to prepare PDF images: %w", err)
	}

	document, err := buildPDFHTMLDocument(preparedMarkdown)
	if err != nil {
		os.RemoveAll(tempDir)
		return pdfDocument{}, "", fmt.Errorf("failed to convert markdown to HTML: %w", err)
	}

	katexTargetDir := filepath.Join(tempDir, "katex")
	if err := copyDirectory(assets.katexDir, katexTargetDir); err != nil {
		os.RemoveAll(tempDir)
		return pdfDocument{}, "", fmt.Errorf("failed to copy KaTeX assets: %w", err)
	}

	htmlPath := filepath.Join(tempDir, "document.html")
	if err := os.WriteFile(htmlPath, []byte(document.HTML), 0o600); err != nil {
		os.RemoveAll(tempDir)
		return pdfDocument{}, "", fmt.Errorf("failed to write export HTML: %w", err)
	}

	browserProfileDir := filepath.Join(tempDir, "chromium-profile")
	if err := os.MkdirAll(browserProfileDir, 0o700); err != nil {
		os.RemoveAll(tempDir)
		return pdfDocument{}, "", fmt.Errorf("failed to create chromium profile directory: %w", err)
	}

	return pdfDocument{
		documentTitle:     document.DocumentTitle,
		htmlPath:          htmlPath,
		browserProfileDir: browserProfileDir,
	}, tempDir, nil
}

func renderPDFWithChromium(ctx context.Context, chromePath string, htmlPath string, browserProfileDir string, documentTitle string, generatedAt time.Time, disableSandbox bool) ([]byte, error) {
	documentURL, shutdownServer, err := serveExportHTMLDirectory(filepath.Dir(htmlPath), filepath.Base(htmlPath))
	if err != nil {
		return nil, err
	}
	defer shutdownServer()

	allocatorOptions := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(browserProfileDir),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("allow-file-access-from-files", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)
	if disableSandbox {
		allocatorOptions = append(allocatorOptions,
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-setuid-sandbox", true),
		)
	}

	allocatorCtx, cancel := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer cancel()

	browserCtx, cancel := chromedp.NewContext(allocatorCtx)
	defer cancel()

	var data []byte
	err = chromedp.Run(browserCtx,
		chromedp.Navigate(documentURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.ActionFunc(func(runCtx context.Context) error {
			pdfData, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPreferCSSPageSize(true).
				WithDisplayHeaderFooter(true).
				WithHeaderTemplate(buildPDFHeaderTemplate(documentTitle)).
				WithFooterTemplate(buildPDFFooterTemplate(generatedAt)).
				WithMarginTop(0.55).
				WithMarginBottom(0.75).
				WithMarginLeft(0.45).
				WithMarginRight(0.45).
				Do(runCtx)
			if err != nil {
				return err
			}
			data = pdfData
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func buildPDFHeaderTemplate(documentTitle string) string {
	return `<div style="width:100%%;font-size:8px;padding:0 12mm;color:#64748b;display:flex;align-items:center;">` + stdhtml.EscapeString(documentTitle) + `</div>`
}

func buildPDFFooterTemplate(generatedAt time.Time) string {
	generatedAtLabel := stdhtml.EscapeString(generatedAt.Format("2006-01-02 15:04:05"))
	return `<div style="width:100%%;font-size:8px;padding:0 12mm;color:#64748b;display:flex;justify-content:space-between;align-items:center;"><span>生成时间：` + generatedAtLabel + `</span><span><span class="pageNumber"></span>/<span class="totalPages"></span></span></div>`
}

func runningAsRoot() bool {
	currentUser, err := user.Current()
	return err == nil && currentUser.Uid == "0"
}

func shouldRetryWithoutSandbox(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no usable sandbox") ||
		strings.Contains(message, "setuid sandbox") ||
		strings.Contains(message, "namespace sandbox") ||
		strings.Contains(message, "zygote")
}

func pdfLimiter() chan struct{} {
	pdfExportLimiterOnce.Do(func() {
		pdfExportLimiter = make(chan struct{}, parseConcurrencyLimit(exportPDFConcurrencyEnv))
	})
	return pdfExportLimiter
}

func docxLimiter() chan struct{} {
	docxExportLimiterOnce.Do(func() {
		docxExportLimiter = make(chan struct{}, parseConcurrencyLimit(exportDOCXConcurrencyEnv))
	})
	return docxExportLimiter
}

func parseConcurrencyLimit(envKey string) int {
	defaultLimit := runtime.NumCPU() / 2
	if defaultLimit < 1 {
		defaultLimit = 1
	}

	configured := strings.TrimSpace(os.Getenv(envKey))
	if configured == "" {
		return defaultLimit
	}

	parsed, err := strconv.Atoi(configured)
	if err != nil || parsed < 1 {
		return defaultLimit
	}

	return parsed
}

func withExportLimiter(ctx context.Context, limiter chan struct{}, fn func() ([]byte, error)) ([]byte, error) {
	select {
	case limiter <- struct{}{}:
		defer func() { <-limiter }()
		return fn()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func copyDirectory(sourceDir string, targetDir string) error {
	return filepath.Walk(sourceDir, func(sourcePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(sourceDir, sourcePath)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(targetDir, relativePath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		return copyFile(sourcePath, targetPath, info.Mode())
	})
}

func copyFile(sourcePath string, targetPath string, fileMode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}

func serveExportHTMLDirectory(directory string, documentName string) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("failed to start local export file server: %w", err)
	}

	server := &http.Server{
		Handler: http.FileServer(http.Dir(directory)),
	}

	go func() {
		_ = server.Serve(listener)
	}()

	shutdown := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}

	documentURL := url.URL{
		Scheme: "http",
		Host:   listener.Addr().String(),
		Path:   "/" + documentName,
	}

	return documentURL.String(), shutdown, nil
}

func chromiumWorkingDirectoryBase() string {
	if runtime.GOOS != "windows" && pathExists("/tmp") {
		return "/tmp"
	}

	return ""
}
