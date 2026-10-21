package errors_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func walkGoFiles(t *testing.T, roots ...string) []string {
	t.Helper()
	var files []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return files
}

func TestNoRawErrorJSONInProductionHTTPPackages(t *testing.T) {
	patterns := []struct {
		name string
		re   *regexp.Regexp
	}{
		{name: "raw error json", re: regexp.MustCompile(`gin\.H\{"error":`)},
		{name: "legacy code msg", re: regexp.MustCompile(`c\.JSON\((200|400|401|403|404|409|500),\s*gin\.H\{"code": 1, "msg":`)},
	}

	files := walkGoFiles(t, filepath.Join("..", "handler"), filepath.Join("..", "router"), filepath.Join("..", "middleware"))
	var offenders []string
	for _, filePath := range files {
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s: %v", filePath, err)
		}
		content := string(contentBytes)
		for _, pattern := range patterns {
			if pattern.re.MatchString(content) {
				offenders = append(offenders, filePath+" -> "+pattern.name)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("found banned source patterns:\n%s", strings.Join(offenders, "\n"))
	}
}
