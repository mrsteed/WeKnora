package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// batchDownloadKBVisibilityStub satisfies KBVisibilityService just enough for
// validateKnowledgeBaseAccessWithKBID in the same-tenant path.
// canManage/canRead model the caller's KB role: manage→Admin, read→Viewer,
// both false→no access (403 before the batch handler's Editor+ check).
type batchDownloadKBVisibilityStub struct {
	canManage bool
	canRead   bool
}

func (s batchDownloadKBVisibilityStub) ListAccessibleKBs(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s batchDownloadKBVisibilityStub) CanManageKB(context.Context, string, uint64, string, bool) (bool, error) {
	return s.canManage, nil
}
func (s batchDownloadKBVisibilityStub) CanAccessKB(context.Context, string, uint64, string, bool) (bool, error) {
	return s.canRead, nil
}

// batchDownloadFailingReader succeeds on the first Read (Phase 1 preflight is
// open-only) and fails on the second, modelling a file source that breaks
// mid-stream during Phase 2 zip writing (design doc 7.1).
type batchDownloadFailingReader struct {
	firstBytes []byte
	readErr    error
	reads      int
	closed     bool
}

func (f *batchDownloadFailingReader) Read(p []byte) (int, error) {
	f.reads++
	if f.reads == 1 {
		// Return the bytes with a nil error (NOT io.EOF): io.Copy treats
		// (n>0, io.EOF) as a clean completion, which would hide the failure.
		return copy(p, f.firstBytes), nil
	}
	return 0, f.readErr
}
func (f *batchDownloadFailingReader) Close() error {
	f.closed = true
	return nil
}

// batchDownloadKnowledgeServiceStub implements the KnowledgeService surface
// used by BatchDownloadKnowledge.
//
// fileOpeners (when set) takes precedence over the static files map; it models
// "the file source" so a test can make the first open succeed (Phase 1
// preflight) and the reopen fail (Phase 2 streaming), per design 7.1.
type batchDownloadKnowledgeServiceStub struct {
	interfaces.KnowledgeService
	byID        map[string]*types.Knowledge
	files       map[string][]byte
	filenames   map[string]string
	fileOpeners map[string]func() (io.ReadCloser, error)
}

// GetKnowledgeBatch filters by tenant exactly like the repository layer does.
func (s *batchDownloadKnowledgeServiceStub) GetKnowledgeBatch(_ context.Context, tenantID uint64, ids []string) ([]*types.Knowledge, error) {
	out := make([]*types.Knowledge, 0, len(ids))
	for _, id := range ids {
		if k, ok := s.byID[id]; ok && k.TenantID == tenantID {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *batchDownloadKnowledgeServiceStub) GetKnowledgeByIDOnly(_ context.Context, id string) (*types.Knowledge, error) {
	return s.byID[id], nil
}

func (s *batchDownloadKnowledgeServiceStub) GetKnowledgeFile(_ context.Context, id string) (io.ReadCloser, string, error) {
	name := s.filenames[id]
	if opener, ok := s.fileOpeners[id]; ok {
		rc, err := opener()
		if err != nil {
			return nil, "", err
		}
		return rc, name, nil
	}
	content, ok := s.files[id]
	if !ok {
		return nil, "", io.EOF
	}
	return io.NopCloser(bytes.NewReader(content)), name, nil
}

// batchDownloadKBServiceStub satisfies interfaces.KnowledgeBaseService.
type batchDownloadKBServiceStub struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *batchDownloadKBServiceStub) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func newBatchDownloadTestRouter(t *testing.T, h *KnowledgeHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "user-1")
		c.Next()
	})
	r.POST("/api/v1/knowledge/batch-download", h.BatchDownloadKnowledge)
	return r
}

func doBatchDownload(r *gin.Engine, kbID string, ids []string) *httptest.ResponseRecorder {
	payload := `{"kb_id":"` + kbID + `","ids":[`
	for i, id := range ids {
		if i > 0 {
			payload += ","
		}
		payload += `"` + id + `"`
	}
	payload += `]}`
	rec := httptest.NewRecorder()
	w := &closeNotifyRecorder{ResponseRecorder: rec}
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/batch-download",
		strings.NewReader(payload)))
	return rec
}

// readZipEntries parses the recorder body as a zip and returns name->size.
func readZipEntries(t *testing.T, body []byte) map[string]uint64 {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("response is not a valid zip: %v; body head=%q", err, body[:min(len(body), 200)])
	}
	out := make(map[string]uint64, len(zr.File))
	for _, f := range zr.File {
		out[f.Name] = f.UncompressedSize64
	}
	return out
}

func setupBatchDownloadHandler(t *testing.T, entries map[string]*types.Knowledge) (*KnowledgeHandler, *batchDownloadKnowledgeServiceStub) {
	t.Helper()
	byID := make(map[string]*types.Knowledge, len(entries))
	files := make(map[string][]byte)
	filenames := make(map[string]string)
	for id, k := range entries {
		byID[id] = k
		content := []byte("content-of-" + id)
		files[id] = content
		fn := k.FileName
		if fn == "" {
			fn = k.Title
		}
		filenames[id] = fn
	}
	svc := &batchDownloadKnowledgeServiceStub{byID: byID, files: files, filenames: filenames}
	h := &KnowledgeHandler{
		kgService:    svc,
		kbService:    &batchDownloadKBServiceStub{kb: &types.KnowledgeBase{ID: "kb1", Name: "Test KB", TenantID: 1}},
		kbVisibility: batchDownloadKBVisibilityStub{canManage: true, canRead: true},
	}
	return h, svc
}

func TestBatchDownloadKnowledgeHappyPath(t *testing.T) {
	entries := map[string]*types.Knowledge{
		"k1": {ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "report.pdf"},
		"k2": {ID: "k2", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "report.pdf", Type: "manual", Title: "notes", CreatedBy: "user-1"},
	}
	entries["k2"].FileName = "notes.md"
	h, _ := setupBatchDownloadHandler(t, entries)
	r := newBatchDownloadTestRouter(t, h)

	w := doBatchDownload(r, "kb1", []string{"k1", "k2", "k1"}) // dup id k1
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/zip") {
		t.Fatalf("Content-Type = %q, want application/zip", got)
	}
	if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") || !strings.Contains(got, ".zip") {
		t.Fatalf("Content-Disposition = %q, want attachment *.zip", got)
	}
	entriesMap := readZipEntries(t, w.Body.Bytes())
	if len(entriesMap) != 2 {
		t.Fatalf("zip entry count = %d (%v), want 2 (dup ids collapsed)", len(entriesMap), entriesMap)
	}
	if _, ok := entriesMap["report.pdf"]; !ok {
		t.Fatalf("zip entries = %v, want report.pdf", entriesMap)
	}
	if _, ok := entriesMap["notes.md"]; !ok {
		t.Fatalf("zip entries = %v, want notes.md", entriesMap)
	}
}

func TestBatchDownloadKnowledgeValidates(t *testing.T) {
	entries := map[string]*types.Knowledge{
		"k1":      {ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "a.txt"},
		"other":   {ID: "other", TenantID: 1, KnowledgeBaseID: "kb2", FileName: "b.txt"},
		"foreign": {ID: "foreign", TenantID: 99, KnowledgeBaseID: "kb1", FileName: "c.txt"},
	}
	h, _ := setupBatchDownloadHandler(t, entries)
	r := newBatchDownloadTestRouter(t, h)

	cases := []struct {
		name       string
		kbID       string
		ids        []string
		wantStatus int
		wantErr    string
	}{
		{"empty ids", "kb1", []string{}, http.StatusBadRequest, "ids cannot be empty"},
		{"cross-KB id", "kb1", []string{"k1", "other"}, http.StatusNotFound, "does not belong"},
		{"cross-tenant id", "kb1", []string{"k1", "foreign"}, http.StatusNotFound, "Knowledge not found"},
		{"missing id", "kb1", []string{"k1", "nope"}, http.StatusNotFound, "Knowledge not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doBatchDownload(r, tc.kbID, tc.ids)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if got := w.Body.String(); !strings.Contains(got, tc.wantErr) {
				t.Fatalf("body = %s, want to contain %q", got, tc.wantErr)
			}
		})
	}
}

// readZipEntryContent returns the decompressed content of one zip entry. The
// mid-stream failure tests must assert on the decoded _DOWNLOAD_ERROR.txt text
// because Deflate entries are not readable in the raw body.
func readZipEntryContent(t *testing.T, body []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("parse zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", name, err)
		}
		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read entry %s: %v", name, err)
		}
		return string(data)
	}
	t.Fatalf("zip has no entry %q (entries: %v)", name, readZipEntries(t, body))
	return ""
}

// TestBatchDownloadKnowledgeForbidden covers design 7.1 "无 KB Editor+ 权限 →
// 403，未开流（content-type application/json）": a KB viewer passes KB access
// but fails the handler's Editor/Admin gate before any zip bytes are written.
func TestBatchDownloadKnowledgeForbidden(t *testing.T) {
	entries := map[string]*types.Knowledge{
		"k1": {ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "a.txt"},
	}
	h, _ := setupBatchDownloadHandler(t, entries)
	h.kbVisibility = batchDownloadKBVisibilityStub{canManage: false, canRead: true} // viewer
	r := newBatchDownloadTestRouter(t, h)

	w := doBatchDownload(r, "kb1", []string{"k1"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json (stream must not start)", got)
	}
	if !strings.Contains(w.Body.String(), "No permission to download") {
		t.Fatalf("body = %s, want permission-denied message", w.Body.String())
	}
}

// TestBatchDownloadKnowledgeContextCancel covers design 7.1 "ctx 取消（流式
// 中断）→ 循环退出、无 panic、无句柄泄漏": the request context is cancelled
// before ServeHTTP, so the Phase-2 select hits ctx.Done mid-stream; the zip
// must end with _DOWNLOAD_ERROR.txt and every opened handle must be closed.
func TestBatchDownloadKnowledgeContextCancel(t *testing.T) {
	entries := map[string]*types.Knowledge{
		"k1": {ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "a.txt"},
		"k2": {ID: "k2", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "b.txt"},
	}
	h, _ := setupBatchDownloadHandler(t, entries)
	r := newBatchDownloadTestRouter(t, h)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/batch-download",
		strings.NewReader(`{"kb_id":"kb1","ids":["k1","k2"]}`))
	req = req.WithContext(ctx)
	cancel() // cancel before the handler starts: Phase-2 select must trip
	rec := httptest.NewRecorder()
	w := &closeNotifyRecorder{ResponseRecorder: rec}
	r.ServeHTTP(w, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (stream already started with headers)", rec.Code)
	}
	if content := readZipEntryContent(t, rec.Body.Bytes(), "_DOWNLOAD_ERROR.txt"); !strings.Contains(content, "批量下载在打包过程中被中止") {
		t.Fatalf("_DOWNLOAD_ERROR.txt content = %q, want abort message", content)
	}
}

// TestBatchDownloadKnowledgeMidStreamFailure covers design 7.1 "文件源 Phase 2
// 失败桩 → zip 含 _DOWNLOAD_ERROR.txt，zw.Close 正常": the first file opens
// fine in preflight but its source breaks on the second read during Phase 2.
func TestBatchDownloadKnowledgeMidStreamFailure(t *testing.T) {
	entries := map[string]*types.Knowledge{
		"k1": {ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "a.txt"},
		"k2": {ID: "k2", TenantID: 1, KnowledgeBaseID: "kb1", FileName: "b.txt"},
	}
	h, svc := setupBatchDownloadHandler(t, entries)
	reader := &batchDownloadFailingReader{
		firstBytes: []byte("partial"),
		readErr:    io.ErrUnexpectedEOF,
	}
	svc.fileOpeners = map[string]func() (io.ReadCloser, error){
		"k1": func() (io.ReadCloser, error) { return reader, nil },
	}
	r := newBatchDownloadTestRouter(t, h)

	w := doBatchDownload(r, "kb1", []string{"k1", "k2"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (mid-stream failure keeps the stream open)", w.Code)
	}
	body := w.Body.Bytes()
	content := readZipEntryContent(t, body, "_DOWNLOAD_ERROR.txt")
	if !strings.Contains(content, "批量下载在打包过程中被中止") {
		t.Fatalf("_DOWNLOAD_ERROR.txt content = %q, want abort message", content)
	}
	if !strings.Contains(content, "a.txt") {
		t.Fatalf("_DOWNLOAD_ERROR.txt should name the failing entry, content: %s", content)
	}
	if !reader.closed {
		t.Fatalf("opened file handle must be closed after mid-stream failure")
	}
}

func TestBatchDownloadKnowledgeTooManyIDs(t *testing.T) {
	h, _ := setupBatchDownloadHandler(t, map[string]*types.Knowledge{})
	r := newBatchDownloadTestRouter(t, h)

	ids := make([]string, 0, maxBatchDownloadLimit+1)
	for i := 0; i <= maxBatchDownloadLimit; i++ {
		ids = append(ids, "k"+string(rune('a'+i%26))+string(rune('0'+i/26%10)))
	}
	w := doBatchDownload(r, "kb1", ids)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestSanitizeZipEntryName(t *testing.T) {
	mc := map[string]int{}
	if got := sanitizeZipEntryName("k1", "/tmp/a/b.pdf", mc); got != "b.pdf" {
		t.Errorf("got %q, want b.pdf", got)
	}
	if got := sanitizeZipEntryName("k1", "/tmp/a/b.pdf", mc); got != "b (2).pdf" {
		t.Errorf("got %q, want b (2).pdf", got)
	}
	if got := sanitizeZipEntryName("k1", "", mc); got != "k1" {
		t.Errorf("got %q, want k1 fallback", got)
	}
	// Path separators are stripped to the final segment, so "../evil" -> "evil".
	if got := sanitizeZipEntryName("k2", "../evil", mc); got != "evil" {
		t.Errorf("got %q, want evil (final path segment)", got)
	}
	// A name that reduces to nothing falls back to the knowledge id.
	if got := sanitizeZipEntryName("k5", "/", mc); got != "k5" {
		t.Errorf("got %q, want k5 fallback for separator-only name", got)
	}
	if got := sanitizeZipEntryName("k3", "noext", mc); got != "noext" {
		t.Errorf("got %q, want noext", got)
	}
	if got := sanitizeZipEntryName("k4", "noext", mc); got != "noext (2)" {
		t.Errorf("got %q, want noext (2)", got)
	}
}
