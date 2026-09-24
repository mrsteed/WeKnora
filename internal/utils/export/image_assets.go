package export

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
)

var StorageImageResolver func(ctx context.Context, storageURL string) ([]byte, bool)

var providerHTMLImageSrcPattern = regexp.MustCompile(`(?i)<img\b([^>]*?)\bsrc\s*=\s*['"]([^'"]+)['"]([^>]*)>`)
var inlineMathSpanPattern = regexp.MustCompile(`\$[^$\n]+\$`)
var texCommandEscapePattern = regexp.MustCompile(`\\\\([A-Za-z]+)`)

const exportImageAssetDir = "export-images"

func materializeStorageImages(ctx context.Context, markdown string, baseDir string) (string, error) {
	if !strings.Contains(markdown, "://") {
		return markdown, nil
	}

	rewriter := exportImageAssetRewriter{
		ctx:     ctx,
		baseDir: baseDir,
		cache:   make(map[string]string),
	}

	rewritten, err := rewriter.rewriteMarkdownImages(markdown)
	if err != nil {
		return "", err
	}

	rewritten, err = rewriter.rewriteHTMLImages(rewritten)
	if err != nil {
		return "", err
	}

	return rewritten, nil
}

func repairDocxTableMathMarkdown(markdown string) string {
	lines := strings.Split(markdown, "\n")
	inTable := false

	for index := 0; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if isDocxMarkdownTableRow(trimmed) && index+1 < len(lines) && isDocxMarkdownTableSeparator(strings.TrimSpace(lines[index+1])) {
			lines[index] = repairDocxMathLine(lines[index])
			inTable = true
			continue
		}

		if !inTable {
			continue
		}

		if !isDocxMarkdownTableRow(trimmed) {
			inTable = false
			continue
		}

		lines[index] = repairDocxMathLine(lines[index])
	}

	return strings.Join(lines, "\n")
}

func repairDocxMathLine(line string) string {
	return inlineMathSpanPattern.ReplaceAllStringFunc(line, func(span string) string {
		if len(span) < 2 {
			return span
		}
		inner := span[1 : len(span)-1]
		inner = texCommandEscapePattern.ReplaceAllString(inner, `\$1`)
		return "$" + inner + "$"
	})
}

func isDocxMarkdownTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}

	return strings.Count(trimmed, "|") >= 2
}

func isDocxMarkdownTableSeparator(line string) bool {
	if !isDocxMarkdownTableRow(line) {
		return false
	}

	inner := strings.TrimSpace(line)
	inner = strings.TrimPrefix(inner, "|")
	inner = strings.TrimSuffix(inner, "|")
	parts := strings.Split(inner, "|")
	if len(parts) == 0 {
		return false
	}

	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell == "" {
			return false
		}
		for _, char := range cell {
			if char != '-' && char != ':' {
				return false
			}
		}
	}

	return true
}

type exportImageAssetRewriter struct {
	ctx       context.Context
	baseDir   string
	cache     map[string]string
	nextIndex int
	created   bool
}

func (r *exportImageAssetRewriter) rewriteMarkdownImages(markdown string) (string, error) {
	spans := docparser.ScanMarkdownImageTargets(markdown)
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		rawTarget := markdown[span.TargetStart:span.TargetEnd]
		storageURL, pathStart, pathEnd, ok := splitStorageMarkdownImageTarget(rawTarget)
		if !ok {
			continue
		}

		relPath, replaced, err := r.materialize(storageURL)
		if err != nil {
			return "", err
		}
		if !replaced {
			continue
		}

		markdown = markdown[:span.TargetStart+pathStart] + relPath + markdown[span.TargetStart+pathEnd:]
	}

	return markdown, nil
}

func (r *exportImageAssetRewriter) rewriteHTMLImages(markdown string) (string, error) {
	matches := providerHTMLImageSrcPattern.FindAllStringSubmatchIndex(markdown, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		if len(match) < 6 {
			continue
		}

		src := strings.TrimSpace(markdown[match[4]:match[5]])
		if !isProviderStorageURL(src) {
			continue
		}

		relPath, replaced, err := r.materialize(src)
		if err != nil {
			return "", err
		}
		if !replaced {
			continue
		}

		markdown = markdown[:match[4]] + relPath + markdown[match[5]:]
	}

	return markdown, nil
}

func (r *exportImageAssetRewriter) materialize(storageURL string) (string, bool, error) {
	if relPath, ok := r.cache[storageURL]; ok {
		return relPath, true, nil
	}

	if StorageImageResolver == nil {
		return "", false, nil
	}

	data, ok := StorageImageResolver(r.ctx, storageURL)
	if !ok || len(data) == 0 {
		logger.Warnf(r.ctx, "[Export] failed to resolve storage image: %s", storageURL)
		return "", false, nil
	}

	if !r.created {
		if err := os.MkdirAll(filepath.Join(r.baseDir, exportImageAssetDir), 0o700); err != nil {
			return "", false, fmt.Errorf("create export image dir: %w", err)
		}
		r.created = true
	}

	r.nextIndex++
	ext := exportImageExt(storageURL, data)
	fileName := fmt.Sprintf("image-%03d%s", r.nextIndex, ext)
	relPath := "./" + filepath.ToSlash(filepath.Join(exportImageAssetDir, fileName))
	absPath := filepath.Join(r.baseDir, exportImageAssetDir, fileName)
	if err := os.WriteFile(absPath, data, 0o600); err != nil {
		return "", false, fmt.Errorf("write export image asset: %w", err)
	}

	r.cache[storageURL] = relPath
	return relPath, true, nil
}

func splitStorageMarkdownImageTarget(raw string) (path string, pathStart int, pathEnd int, ok bool) {
	start, end := trimMarkdownSpaceBounds(raw, 0, len(raw))
	if start >= end {
		return "", 0, 0, false
	}

	if raw[start] == '<' {
		for i := start + 1; i < end; i++ {
			if raw[i] == '>' && !isEscaped(raw, i) {
				candidate := raw[start+1 : i]
				if !isProviderStorageURL(candidate) {
					return "", 0, 0, false
				}
				return candidate, start + 1, i, true
			}
		}
		return "", 0, 0, false
	}

	pathEnd = end
	for i := start; i < end; i++ {
		if isMarkdownSpace(raw[i]) {
			pathEnd = i
			break
		}
	}
	if pathEnd <= start {
		return "", 0, 0, false
	}

	candidate := raw[start:pathEnd]
	if !isProviderStorageURL(candidate) {
		return "", 0, 0, false
	}

	return candidate, start, pathEnd, true
}

func exportImageExt(storageURL string, data []byte) string {
	if ext := strings.ToLower(path.Ext(storageURL)); ext != "" {
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
			return ext
		}
	}

	switch http.DetectContentType(data) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".png"
	}
}

func isProviderStorageURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	for _, prefix := range []string{"local://", "minio://", "cos://", "tos://", "s3://", "obs://", "oss://", "ks3://"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func trimMarkdownSpaceBounds(raw string, start int, end int) (int, int) {
	for start < end && isMarkdownSpace(raw[start]) {
		start++
	}
	for end > start && isMarkdownSpace(raw[end-1]) {
		end--
	}
	return start, end
}

func isMarkdownSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isEscaped(s string, pos int) bool {
	backslashes := 0
	for i := pos - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}
