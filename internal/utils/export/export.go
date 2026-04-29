// Package export provides document export utilities for converting
// Markdown content to PDF and DOCX formats using backend tools
// (Chromium print-to-PDF for PDF, pandoc for DOCX).
package export

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const exportChromeBinEnv = "WEKNORA_EXPORT_CHROME_BIN"

// MarkdownToPDF converts Markdown content to PDF.
// It first converts Markdown to styled HTML using goldmark,
// then uses headless Chromium to render HTML to PDF.
func MarkdownToPDF(ctx context.Context, markdown string) ([]byte, error) {
	// Step 1: Convert Markdown to styled HTML
	styledHTML, err := MarkdownToStyledHTML(markdown)
	if err != nil {
		return nil, fmt.Errorf("failed to convert markdown to HTML: %w", err)
	}

	// Step 2: Convert HTML to PDF using headless Chromium
	return htmlToPDF(ctx, styledHTML)
}

// MarkdownToDocx converts Markdown content to DOCX.
// It uses pandoc to convert Markdown directly to DOCX format.
func MarkdownToDocx(ctx context.Context, markdown string) ([]byte, error) {
	return markdownToDocxViaPandoc(ctx, markdown)
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
func htmlToPDF(ctx context.Context, html string) ([]byte, error) {
	chromePath, ok := findChromiumExecutable()
	if !ok {
		return nil, fmt.Errorf("chromium is not installed. Set %s or install chromium/google-chrome on the server", exportChromeBinEnv)
	}

	htmlURL := buildHTMLDataURL(html)
	allocatorOptions := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("hide-scrollbars", true),
	)

	allocatorCtx, cancel := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer cancel()

	browserCtx, cancel := chromedp.NewContext(allocatorCtx)
	defer cancel()

	var (
		data []byte
		err  error
	)
	err = chromedp.Run(browserCtx,
		chromedp.Navigate(htmlURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(runCtx context.Context) error {
			pdfData, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPreferCSSPageSize(true).
				WithMarginTop(0.4).
				WithMarginBottom(0.55).
				WithMarginLeft(0.4).
				WithMarginRight(0.4).
				Do(runCtx)
			if err != nil {
				return err
			}
			data = pdfData
			return nil
		}),
	)
	if err != nil {
		logger.Errorf(ctx, "[Export] chromium print-to-pdf failed: %v", err)
		return nil, fmt.Errorf("chromium print-to-pdf failed: %w", err)
	}

	logger.Infof(ctx, "[Export] Successfully generated PDF, size: %d bytes", len(data))
	return data, nil
}

// markdownToDocxViaPandoc converts Markdown to DOCX using pandoc.
func markdownToDocxViaPandoc(ctx context.Context, markdown string) ([]byte, error) {
	if !IsPandocAvailable() {
		return nil, fmt.Errorf("pandoc is not installed. Please install it: apt-get install -y pandoc")
	}

	// Write Markdown to temp file
	mdFile, err := os.CreateTemp("", "weknora_export_*.md")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp markdown file: %w", err)
	}
	defer os.Remove(mdFile.Name())

	if _, err := mdFile.WriteString(markdown); err != nil {
		mdFile.Close()
		return nil, fmt.Errorf("failed to write markdown temp file: %w", err)
	}
	mdFile.Close()

	// Create temp output file for DOCX
	docxFile, err := os.CreateTemp("", "weknora_export_*.docx")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp DOCX file: %w", err)
	}
	docxFile.Close()
	defer os.Remove(docxFile.Name())

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
		"-o", docxFile.Name(),
		mdFile.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Errorf(ctx, "[Export] pandoc failed: %v, stderr: %s", err, stderr.String())
		return nil, fmt.Errorf("pandoc failed: %w (stderr: %s)", err, stderr.String())
	}

	// Read the generated DOCX
	data, err := os.ReadFile(docxFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read generated DOCX: %w", err)
	}

	logger.Infof(ctx, "[Export] Successfully generated DOCX, size: %d bytes", len(data))
	return data, nil
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

func buildHTMLDataURL(html string) string {
	encodedHTML := base64.StdEncoding.EncodeToString([]byte(html))
	return "data:text/html;charset=utf-8;base64," + encodedHTML
}
