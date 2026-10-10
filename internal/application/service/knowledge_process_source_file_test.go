package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"testing"

	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
)

type sourceFileRepoStub struct {
	interfaces.KnowledgeRepository
	knowledge    *types.Knowledge
	updateCalls  int
	columnWrites int
}

func (r *sourceFileRepoStub) GetKnowledgeByID(_ context.Context, _ uint64, _ string) (*types.Knowledge, error) {
	return r.knowledge, nil
}

func (r *sourceFileRepoStub) UpdateKnowledge(_ context.Context, knowledge *types.Knowledge) error {
	r.updateCalls++
	r.knowledge = knowledge
	return nil
}

func (r *sourceFileRepoStub) UpdateKnowledgeColumn(_ context.Context, _ string, _ string, _ interface{}) error {
	r.columnWrites++
	return nil
}

type sourceFileKBServiceStub struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *sourceFileKBServiceStub) GetKnowledgeBaseByID(_ context.Context, _ string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

type sourceFileTenantRepoStub struct {
	interfaces.TenantRepository
	tenant *types.Tenant
}

func (r *sourceFileTenantRepoStub) GetTenantByID(_ context.Context, _ uint64) (*types.Tenant, error) {
	return r.tenant, nil
}

type sourceFileTenantServiceStub struct {
	interfaces.TenantService
}

func (sourceFileTenantServiceStub) GetWeKnoraCloudCredentials(context.Context) *types.WeKnoraCloudCredentials {
	return nil
}

type missingSourceFileService struct {
	interfaces.FileService
}

func (missingSourceFileService) CheckConnectivity(context.Context) error { return nil }
func (missingSourceFileService) SaveFile(context.Context, *multipart.FileHeader, uint64, string) (string, error) {
	return "", nil
}
func (missingSourceFileService) SaveBytes(context.Context, []byte, uint64, string, bool) (string, error) {
	return "", nil
}
func (missingSourceFileService) GetFile(context.Context, string) (io.ReadCloser, error) {
	return nil, os.ErrNotExist
}
func (missingSourceFileService) GetFileURL(context.Context, string) (string, error) { return "", nil }
func (missingSourceFileService) DeleteFile(context.Context, string) error           { return nil }
func (missingSourceFileService) CopyFile(context.Context, string, uint64, string) (string, error) {
	return "", nil
}

func TestReparseKnowledgeFailsFastWhenSourceFileIsMissing(t *testing.T) {
	knowledge := &types.Knowledge{
		ID:              "knowledge-1",
		TenantID:        7,
		KnowledgeBaseID: "kb-1",
		Type:            "file",
		ParseStatus:     types.ParseStatusFailed,
		FilePath:        "local://missing.txt",
		FileName:        "missing.txt",
	}
	repo := &sourceFileRepoStub{knowledge: knowledge}
	svc := &knowledgeService{
		repo:          repo,
		kbService:     &sourceFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:       missingSourceFileService{},
		tenantRepo:    &sourceFileTenantRepoStub{tenant: &types.Tenant{ID: 7}},
		tenantService: sourceFileTenantServiceStub{},
	}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	got, err := svc.ReparseKnowledge(ctx, knowledge.ID, nil)

	if got != knowledge {
		t.Fatalf("ReparseKnowledge returned %p, want original knowledge %p", got, knowledge)
	}
	var appErr *werrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %v", err)
	}
	if appErr.Code != werrors.ErrBadRequest {
		t.Fatalf("error code = %d, want %d", appErr.Code, werrors.ErrBadRequest)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("UpdateKnowledge calls = %d, want 0", repo.updateCalls)
	}
	if repo.columnWrites != 0 {
		t.Fatalf("UpdateKnowledgeColumn calls = %d, want 0", repo.columnWrites)
	}
}

func TestProcessDocumentStopsRetryingWhenSourceFileIsMissing(t *testing.T) {
	knowledge := &types.Knowledge{
		ID:              "knowledge-1",
		TenantID:        7,
		KnowledgeBaseID: "kb-1",
		Type:            "file",
		ParseStatus:     types.ParseStatusPending,
		FilePath:        "local://missing.txt",
		FileName:        "missing.txt",
	}
	repo := &sourceFileRepoStub{knowledge: knowledge}
	svc := &knowledgeService{
		repo:          repo,
		kbService:     &sourceFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:       missingSourceFileService{},
		tenantRepo:    &sourceFileTenantRepoStub{tenant: &types.Tenant{ID: 7}},
		tenantService: sourceFileTenantServiceStub{},
	}

	payload := types.DocumentProcessPayload{
		TenantID:        7,
		KnowledgeID:     knowledge.ID,
		KnowledgeBaseID: knowledge.KnowledgeBaseID,
		FilePath:        knowledge.FilePath,
		FileName:        knowledge.FileName,
		FileType:        "txt",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}

	err = svc.ProcessDocument(context.Background(), asynq.NewTask(types.TypeDocumentProcess, body))

	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("ProcessDocument error = %v, want SkipRetry", err)
	}
	if knowledge.ParseStatus != types.ParseStatusFailed {
		t.Fatalf("ParseStatus = %q, want %q", knowledge.ParseStatus, types.ParseStatusFailed)
	}
	if knowledge.ErrorMessage == "" {
		t.Fatal("ErrorMessage should be populated for missing source file")
	}
	if repo.updateCalls < 2 {
		t.Fatalf("UpdateKnowledge calls = %d, want at least 2", repo.updateCalls)
	}
}
