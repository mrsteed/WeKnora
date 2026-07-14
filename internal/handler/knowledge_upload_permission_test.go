package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type stubUploadKnowledgeService struct {
	interfaces.KnowledgeService
	called   bool
	kbID     string
	fileName string
}

func (s *stubUploadKnowledgeService) CreateKnowledgeFromFile(
	_ context.Context,
	kbID string,
	file *multipart.FileHeader,
	_ map[string]string,
	_ *bool,
	_ string,
	_ []string,
	_ string,
	_ *types.KnowledgeProcessOverrides,
) (*types.Knowledge, error) {
	s.called = true
	s.kbID = kbID
	if file != nil {
		s.fileName = file.Filename
	}
	return &types.Knowledge{ID: "k-1", KnowledgeBaseID: kbID, FileName: s.fileName}, nil
}

func TestCreateKnowledgeFromFile_AllowsViewerPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "viewer.txt")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("viewer upload")); err != nil {
		t.Fatalf("part.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/knowledge-bases/kb-1/knowledge/file", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx := context.WithValue(req.Context(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "u-viewer")
	c.Request = req.WithContext(ctx)
	c.Params = gin.Params{{Key: "id", Value: "kb-1"}}
	c.Set(types.UserContextKey.String(), &types.User{ID: "u-viewer"})
	c.Set(middleware.KBAccessContextKey, &middleware.KBAccess{
		KnowledgeBase:     &types.KnowledgeBase{ID: "kb-1", TenantID: 1},
		EffectiveTenantID: 1,
		Permission:        types.OrgRoleViewer,
	})

	svc := &stubUploadKnowledgeService{}
	h := &KnowledgeHandler{kgService: svc}

	h.CreateKnowledgeFromFile(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !svc.called {
		t.Fatal("expected CreateKnowledgeFromFile service to be called for viewer permission")
	}
	if svc.kbID != "kb-1" {
		t.Fatalf("kbID = %q, want kb-1", svc.kbID)
	}
	if svc.fileName != "viewer.txt" {
		t.Fatalf("fileName = %q, want viewer.txt", svc.fileName)
	}
}
