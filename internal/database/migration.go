package database

import (
	"context"
	"fmt"
	"os"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations executes all pending database migrations
// This should be called during application startup
func RunMigrations(dsn string) error {
	// Use versioned migrations directory
	migrationsPath := "file://migrations/versioned"

	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	// Check current version and dirty state before migration
	oldVersion, oldDirty, versionErr := m.Version()
	if versionErr != nil && versionErr != migrate.ErrNilVersion {
		// If we can't get version and it's not because database is empty, return error
		return fmt.Errorf("failed to get migration version: %w", versionErr)
	}

	// If database is in dirty state, return detailed error with fix instructions
	if oldDirty {
		// Calculate the version to force to (usually the previous version)
		forceVersion := int(oldVersion) - 1
		if oldVersion == 0 || forceVersion < 0 {
			forceVersion = 0
		}
		return fmt.Errorf(
			"database is in dirty state at version %d. This usually means a migration failed partway through. "+
				"To fix this:\n"+
				"1. Check if the migration partially applied changes and manually fix if needed\n"+
				"2. Use the force command to set the version to the last successful migration (usually %d):\n"+
				"   ./scripts/migrate.sh force %d\n"+
				"   Or if using make: make migrate-force version=%d\n"+
				"3. After fixing, restart the application to retry the migration",
			oldVersion,
			forceVersion,
			forceVersion,
			forceVersion,
		)
	}

	// Run all pending migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		// Check if error is due to dirty state (in case it became dirty during migration)
		currentVersion, currentDirty, versionCheckErr := m.Version()
		if versionCheckErr == nil && currentDirty {
			// Calculate the version to force to (usually the previous version)
			forceVersion := currentVersion - 1
			if currentVersion == 0 {
				forceVersion = 0
			}
			return fmt.Errorf(
				"migration failed and database is now in dirty state at version %d. "+
					"To fix this:\n"+
					"1. Check if the migration partially applied changes and manually fix if needed\n"+
					"2. Use the force command to set the version to the last successful migration (usually %d):\n"+
					"   ./scripts/migrate.sh force %d\n"+
					"   Or if using make: make migrate-force version=%d\n"+
					"3. After fixing, restart the application to retry the migration",
				currentVersion,
				forceVersion,
				forceVersion,
				forceVersion,
			)
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Get current version after migration
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if oldVersion != version {
		logger.Infof(context.Background(), "Database migrated from version %d to %d", oldVersion, version)
	} else {
		logger.Infof(context.Background(), "Database is up to date (version: %d)", version)
	}

	if dirty {
		logger.Warnf(context.Background(), "Database is in dirty state! Manual intervention may be required.")
	}

	return nil
}

// GetMigrationVersion returns the current migration version
func GetMigrationVersion() (uint, bool, error) {
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)

	migrationsPath := "file://migrations/versioned"

	m, err := migrate.New(migrationsPath, dbURL)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	version, dirty, err := m.Version()
	if err != nil {
		return 0, false, err
	}

	return version, dirty, nil
}
