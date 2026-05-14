package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("chat-document-artifact-backfill", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "target session id to backfill chat document artifacts for")
	limit := fs.Int("limit", 80, "number of recent messages to scan in the session")
	dryRun := fs.Bool("dry-run", false, "scan and report without committing new artifacts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sessionID == "" {
		return errors.New("--session-id is required")
	}
	if *limit <= 0 {
		return errors.New("--limit must be greater than 0")
	}

	db, err := openDatabase()
	if err != nil {
		return err
	}
	if *dryRun {
		tx := db.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		defer tx.Rollback()
		db = tx
	}

	ctx := context.Background()
	var sessionModel types.Session
	if err := db.WithContext(ctx).First(&sessionModel, "id = ?", *sessionID).Error; err != nil {
		return fmt.Errorf("load session %s: %w", *sessionID, err)
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, sessionModel.TenantID)
	if sessionModel.UserID != "" {
		ctx = context.WithValue(ctx, types.UserIDContextKey, sessionModel.UserID)
	}

	messageRepo := repository.NewMessageRepository(db)
	artifactRepo := repository.NewChatDocumentArtifactRepository(db)
	evidenceRefRepo := repository.NewChatDocumentEvidenceRefRepository(db)
	artifactService := service.NewChatDocumentArtifactService(artifactRepo, evidenceRefRepo)

	messages, err := messageRepo.GetRecentMessagesBySession(ctx, sessionModel.ID, *limit)
	if err != nil {
		return fmt.Errorf("load recent messages: %w", err)
	}
	if len(messages) == 0 {
		fmt.Println("no messages found")
		return nil
	}

	requestUserQuery := make(map[string]string, len(messages))
	missingRequestIDs := make([]string, 0)
	seenRequestIDs := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if message == nil || message.RequestID == "" {
			continue
		}
		if message.Role == "user" {
			requestUserQuery[message.RequestID] = message.Content
			continue
		}
		if message.Role == "assistant" {
			if _, ok := seenRequestIDs[message.RequestID]; ok {
				continue
			}
			if _, ok := requestUserQuery[message.RequestID]; ok {
				continue
			}
			seenRequestIDs[message.RequestID] = struct{}{}
			missingRequestIDs = append(missingRequestIDs, message.RequestID)
		}
	}
	if len(missingRequestIDs) > 0 {
		partners, err := messageRepo.GetMessagesByRequestIDs(ctx, missingRequestIDs)
		if err != nil {
			return fmt.Errorf("load request partners: %w", err)
		}
		for _, partner := range partners {
			if partner == nil || partner.Role != "user" || partner.SessionID != sessionModel.ID || partner.RequestID == "" {
				continue
			}
			if _, ok := requestUserQuery[partner.RequestID]; !ok {
				requestUserQuery[partner.RequestID] = partner.Content
			}
		}
	}
	createdCount := 0
	existingCount := 0
	skippedCount := 0
	failureCount := 0
	var previousArtifact *types.ChatDocumentArtifact

	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == "user" {
			continue
		}
		if message.Role != "assistant" {
			continue
		}

		existing, err := artifactRepo.GetArtifactBySourceMessageID(ctx, sessionModel.TenantID, message.ID)
		if err != nil {
			failureCount++
			fmt.Fprintf(os.Stderr, "check existing artifact for message %s: %v\n", message.ID, err)
			continue
		}
		if existing != nil {
			existingCount++
			previousArtifact = existing
			fmt.Printf("existing artifact kept: message=%s artifact=%s revision=%d\n", message.ID, existing.ID, existing.RevisionNo)
			continue
		}

		userQuery := requestUserQuery[message.RequestID]
		intentResult, err := artifactService.DetectIntent(ctx, sessionModel.ID, userQuery, "")
		if err != nil {
			failureCount++
			fmt.Fprintf(os.Stderr, "detect intent for message %s: %v\n", message.ID, err)
			continue
		}

		var baseArtifact *types.ChatDocumentArtifact
		if intentResult != nil && (intentResult.Intent == types.ChatDocumentIntentContinue || intentResult.Intent == types.ChatDocumentIntentRevise) {
			baseArtifact = previousArtifact
		}

		artifact, err := artifactService.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
			UserQuery:    userQuery,
			Intent:       valueOrDefault(intentResult, types.ChatDocumentIntentNormal),
			Operation:    operationOrDefault(intentResult),
			BaseArtifact: baseArtifact,
		})
		if err != nil {
			failureCount++
			fmt.Fprintf(os.Stderr, "register artifact for message %s: %v\n", message.ID, err)
			continue
		}
		if artifact == nil {
			skippedCount++
			fmt.Printf("skipped message=%s completion=%s\n", message.ID, message.CompletionStatusOrLegacy())
			continue
		}

		createdCount++
		previousArtifact = artifact
		fmt.Printf("created artifact: message=%s artifact=%s revision=%d status=%s issues=%v\n",
			message.ID,
			artifact.ID,
			artifact.RevisionNo,
			artifact.Status,
			artifact.QualityIssues,
		)
	}

	mode := "committed"
	if *dryRun {
		mode = "dry-run"
	}
	fmt.Printf("summary: session=%s mode=%s created=%d existing=%d skipped=%d failed=%d\n", sessionModel.ID, mode, createdCount, existingCount, skippedCount, failureCount)
	return nil
}

func valueOrDefault(result *types.DocumentIntentResult, fallback string) string {
	if result == nil || result.Intent == "" {
		return fallback
	}
	return result.Intent
}

func operationOrDefault(result *types.DocumentIntentResult) string {
	if result == nil || result.Operation == "" {
		return types.ChatDocumentOperationCreate
	}
	return result.Operation
}

func openDatabase() (*gorm.DB, error) {
	switch os.Getenv("DB_DRIVER") {
	case "postgres":
		gormDSN := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
		)
		return gorm.Open(postgres.Open(gormDSN), &gorm.Config{})
	case "sqlite":
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "./data/weknora.db"
		}
		if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create sqlite data dir %s: %w", dir, err)
			}
		}
		return gorm.Open(sqlite.Open(dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", os.Getenv("DB_DRIVER"))
	}
}
