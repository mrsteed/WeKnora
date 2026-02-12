// Package export provides document export utilities for converting
// Markdown content to PDF and DOCX formats using backend tools
// (wkhtmltopdf for PDF, pandoc for DOCX).
package export

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/Tencent/WeKnora/internal/logger"
)

// MarkdownToPDF converts Markdown content to PDF.
// It first converts Markdown to styled HTML using goldmark,
// then uses wkhtmltopdf to render HTML to PDF.
func MarkdownToPDF(ctx context.Context, markdown string) ([]byte, error) {
	// Step 1: Convert Markdown to styled HTML
	styledHTML, err := MarkdownToStyledHTML(markdown)
	if err != nil {
		return nil, fmt.Errorf("failed to convert markdown to HTML: %w", err)
	}

	// Step 2: Convert HTML to PDF using wkhtmltopdf
	return htmlToPDF(ctx, styledHTML)
}

// MarkdownToDocx converts Markdown content to DOCX.
// It uses pandoc to convert Markdown directly to DOCX format.
func MarkdownToDocx(ctx context.Context, markdown string) ([]byte, error) {
	return markdownToDocxViaPandoc(ctx, markdown)
}

// htmlToPDF converts HTML to PDF using wkhtmltopdf.
// If wkhtmltopdf is not available, returns a descriptive error.
func htmlToPDF(ctx context.Context, html string) ([]byte, error) {
	if !IsWkhtmltopdfAvailable() {
		return nil, fmt.Errorf("wkhtmltopdf is not installed. Please install it: apt-get install -y wkhtmltopdf")
	}

	// Write HTML to temp file
	htmlFile, err := os.CreateTemp("", "weknora_export_*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp HTML file: %w", err)
	}
	defer os.Remove(htmlFile.Name())

	if _, err := htmlFile.WriteString(html); err != nil {
		htmlFile.Close()
		return nil, fmt.Errorf("failed to write HTML temp file: %w", err)
	}
	htmlFile.Close()

	// Create temp output file for PDF
	pdfFile, err := os.CreateTemp("", "weknora_export_*.pdf")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp PDF file: %w", err)
	}
	pdfFile.Close()
	defer os.Remove(pdfFile.Name())

	// Run wkhtmltopdf
	// Options:
	//   --encoding utf-8        : UTF-8 encoding
	//   --page-size A4          : A4 page size
	//   --margin-top 15mm       : margins
	//   --margin-bottom 15mm
	//   --margin-left 15mm
	//   --margin-right 15mm
	//   --enable-local-file-access : allow local file access for images
	//   --quiet                 : suppress output
	cmd := exec.CommandContext(ctx, "wkhtmltopdf",
		"--encoding", "utf-8",
		"--page-size", "A4",
		"--margin-top", "15mm",
		"--margin-bottom", "15mm",
		"--margin-left", "15mm",
		"--margin-right", "15mm",
		"--enable-local-file-access",
		"--quiet",
		htmlFile.Name(),
		pdfFile.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Errorf(ctx, "[Export] wkhtmltopdf failed: %v, stderr: %s", err, stderr.String())
		return nil, fmt.Errorf("wkhtmltopdf failed: %w (stderr: %s)", err, stderr.String())
	}

	// Read the generated PDF
	data, err := os.ReadFile(pdfFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read generated PDF: %w", err)
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

// IsWkhtmltopdfAvailable checks if wkhtmltopdf is installed and available in PATH.
func IsWkhtmltopdfAvailable() bool {
	_, err := exec.LookPath("wkhtmltopdf")
	return err == nil
}

// IsPandocAvailable checks if pandoc is installed and available in PATH.
func IsPandocAvailable() bool {
	_, err := exec.LookPath("pandoc")
	return err == nil
}
