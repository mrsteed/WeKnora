package export

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	exportReferenceDocxEnv   = "WEKNORA_EXPORT_REFERENCE_DOCX"
	exportPDFStylePathEnv    = "WEKNORA_EXPORT_PDF_STYLE_PATH"
	exportPDFConcurrencyEnv  = "WEKNORA_EXPORT_PDF_CONCURRENCY"
	exportDOCXConcurrencyEnv = "WEKNORA_EXPORT_DOCX_CONCURRENCY"
)

type pdfAssets struct {
	stylePath string
	katexDir  string
}

type docxAssets struct {
	referenceDocxPath string
}

func resolvePDFAssets() (pdfAssets, error) {
	stylePath, err := resolveExportAssetPath(exportPDFStylePathEnv, "pdf.css")
	if err != nil {
		return pdfAssets{}, err
	}

	katexDir := filepath.Join(filepath.Dir(stylePath), "katex")
	if !pathExists(katexDir) {
		return pdfAssets{}, fmt.Errorf("missing KaTeX asset directory %q next to %s", katexDir, exportPDFStylePathEnv)
	}

	for _, required := range []string{
		"katex.min.css",
		"katex.min.js",
		filepath.Join("contrib", "auto-render.min.js"),
	} {
		candidate := filepath.Join(katexDir, required)
		if !pathExists(candidate) {
			return pdfAssets{}, fmt.Errorf("missing KaTeX asset: %s", candidate)
		}
	}

	return pdfAssets{
		stylePath: stylePath,
		katexDir:  katexDir,
	}, nil
	}

func resolveDocxAssets() (docxAssets, error) {
	referenceDocxPath, err := resolveExportAssetPath(exportReferenceDocxEnv, "reference.docx")
	if err != nil {
		return docxAssets{}, err
	}

	return docxAssets{referenceDocxPath: referenceDocxPath}, nil
}

func readPDFStylesheet() (string, error) {
	assets, err := resolvePDFAssets()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(assets.stylePath)
	if err != nil {
		return "", fmt.Errorf("read PDF stylesheet %q: %w", assets.stylePath, err)
	}

	return string(data), nil
}

func validatePDFRuntime() error {
	if !IsChromiumAvailable() {
		return fmt.Errorf("chromium is not installed. Set %s or install chromium/google-chrome on the server", exportChromeBinEnv)
	}

	_, err := resolvePDFAssets()
	return err
}

func validateDocxRuntime() error {
	if !IsPandocAvailable() {
		return fmt.Errorf("pandoc is not installed. Please install it: apt-get install -y pandoc")
	}

	_, err := resolveDocxAssets()
	return err
}

func resolveExportAssetPath(envKey string, defaultRelativePath string) (string, error) {
	if configured := strings.TrimSpace(os.Getenv(envKey)); configured != "" {
		absPath, err := filepath.Abs(configured)
		if err != nil {
			return "", fmt.Errorf("resolve %s path %q: %w", envKey, configured, err)
		}
		if !pathExists(absPath) {
			return "", fmt.Errorf("%s points to a missing path: %s", envKey, absPath)
		}
		return absPath, nil
	}

	for _, root := range candidateExportRoots() {
		candidate := filepath.Join(root, "config", "export", defaultRelativePath)
		if pathExists(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("export asset %q not found under config/export; set %s explicitly", defaultRelativePath, envKey)
}

func candidateExportRoots() []string {
	var roots []string
	seen := map[string]struct{}{}
	addRoot := func(path string) {
		if path == "" {
			return
		}
		cleaned := filepath.Clean(path)
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		roots = append(roots, cleaned)
	}

	addAncestors := func(start string, depth int) {
		dir := filepath.Clean(start)
		for i := 0; i <= depth; i++ {
			addRoot(dir)
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}

	if workingDir, err := os.Getwd(); err == nil {
		addAncestors(workingDir, 6)
	}

	if executablePath, err := os.Executable(); err == nil {
		addAncestors(filepath.Dir(executablePath), 4)
	}

	if _, currentFile, _, ok := runtime.Caller(0); ok {
		addAncestors(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."), 2)
	}

	return roots
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}

	_, err := os.Stat(path)
	return err == nil
}