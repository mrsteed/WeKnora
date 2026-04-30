package database
package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionedMigrationsHaveUniqueVersions(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "migrations", "versioned"))
	if err != nil {
		t.Fatalf("read versioned migrations: %v", err)
	}

	seen := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Fatalf("migration filename %q does not contain a version prefix", name)
		}

		if previous, exists := seen[prefix]; exists {
			t.Fatalf("duplicate migration version %s: %s and %s", prefix, previous, name)
		}
		seen[prefix] = name
	}

	if len(seen) == 0 {
		t.Fatal("no versioned migrations found")
	}
}