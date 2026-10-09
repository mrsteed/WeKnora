package docparser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

func TestNewMinerUReaderResolvesParseMethod(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		want      string
	}{
		{name: "default is auto", overrides: map[string]string{}, want: "auto"},
		{name: "legacy enabled becomes auto", overrides: map[string]string{"mineru_enable_ocr": "true"}, want: "auto"},
		{name: "legacy disabled becomes text", overrides: map[string]string{"mineru_enable_ocr": "false"}, want: "txt"},
		{name: "explicit method wins", overrides: map[string]string{"mineru_parse_method": "ocr", "mineru_enable_ocr": "false"}, want: "ocr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewMinerUReader(tt.overrides)
			if reader.parseMethod != tt.want {
				t.Fatalf("parseMethod = %q, want %q", reader.parseMethod, tt.want)
			}
		})
	}
}

func TestMinerUUploadFileName(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		fileType string
		want     string
	}{
		{name: "original basename", fileName: "reports/Illustrator 研报.pdf", fileType: "pdf", want: "Illustrator 研报.pdf"},
		{
			name:     "strip multipart controls",
			fileName: "reports/\r\nIllustrator\x00 export.pdf",
			fileType: "pdf",
			want:     "Illustrator export.pdf",
		},
		{name: "append extension from type", fileName: "report", fileType: "pdf", want: "report.pdf"},
		{name: "fallback from bare type", fileType: "pdf", want: "document.pdf"},
		{name: "fallback from dotted uppercase type", fileType: ".PDF", want: "document.pdf"},
		{name: "fallback from path-only name", fileName: "/", fileType: "pdf", want: "document.pdf"},
		{name: "legacy fallback without metadata", want: "document"},
		{name: "reject unsafe fallback type", fileType: "../pdf", want: "document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := minerUUploadFileName(tt.fileName, tt.fileType); got != tt.want {
				t.Fatalf("minerUUploadFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMinerUFileParseResponse(t *testing.T) {
	tests := []struct {
		name           string
		uploadFileName string
		response       map[string]any
		wantMarkdown   string
		wantResultKey  string
	}{
		{
			name:           "match upload stem",
			uploadFileName: "Illustrator 研报.pdf",
			response: map[string]any{
				"results": map[string]any{
					"Illustrator 研报": map[string]any{"md_content": "from-stem", "images": map[string]any{}},
				},
			},
			wantMarkdown:  "from-stem",
			wantResultKey: "Illustrator 研报",
		},
		{
			name:           "legacy document key",
			uploadFileName: "document",
			response: map[string]any{
				"results": map[string]any{
					"document": map[string]any{"md_content": "legacy", "images": map[string]any{}},
				},
			},
			wantMarkdown:  "legacy",
			wantResultKey: "document",
		},
		{
			name:           "files variant",
			uploadFileName: "document.pdf",
			response: map[string]any{
				"results": map[string]any{
					"files": map[string]any{"md_content": "from-files", "images": map[string]any{}},
				},
			},
			wantMarkdown:  "from-files",
			wantResultKey: "files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			gotMarkdown, _, gotKey, err := parseMinerUFileParseResponse(respBody, tt.uploadFileName)
			if err != nil {
				t.Fatalf("parseMinerUFileParseResponse() error: %v", err)
			}
			if gotMarkdown != tt.wantMarkdown {
				t.Fatalf("markdown = %q, want %q", gotMarkdown, tt.wantMarkdown)
			}
			if gotKey != tt.wantResultKey {
				t.Fatalf("result key = %q, want %q", gotKey, tt.wantResultKey)
			}
		})
	}
}

func TestMinerUReaderPreservesMultipartFilename(t *testing.T) {
	utils.SetSSRFWhitelistFromRaw("127.0.0.1")
	t.Cleanup(func() {
		utils.SetSSRFWhitelistFromRaw("")
	})

	const fileContent = "%PDF-1.7 synthetic test content"
	tests := []struct {
		name     string
		fileName string
		fileType string
		want     string
	}{
		{name: "original basename", fileName: "reports/Illustrator 研报.pdf", fileType: "pdf", want: "Illustrator 研报.pdf"},
		{
			name:     "strip multipart controls",
			fileName: "reports/\r\nIllustrator\x00 export.pdf",
			fileType: "pdf",
			want:     "Illustrator export.pdf",
		},
		{name: "fallback from bare type", fileType: "pdf", want: "document.pdf"},
		{name: "fallback from dotted uppercase type", fileType: ".PDF", want: "document.pdf"},
		{name: "fallback from path-only name", fileName: "/", fileType: "pdf", want: "document.pdf"},
		{name: "legacy fallback without metadata", want: "document"},
		{name: "reject unsafe fallback type", fileType: "../pdf", want: "document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type capturedRequest struct {
				method   string
				path     string
				fileName string
				content  string
				err      error
			}

			captured := make(chan capturedRequest, 1)
			uploadName := minerUUploadFileName(tt.fileName, tt.fileType)
			resultStem := minerUResultStem(uploadName)
			responseBody, err := json.Marshal(map[string]any{
				"results": map[string]any{
					resultStem: map[string]any{
						"md_content": "ok",
						"images":     map[string]any{},
					},
				},
			})
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Version probe: 404 forces the reader onto the legacy /file_parse path.
				if r.URL.Path == "/v1/health" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				got := capturedRequest{method: r.Method, path: r.URL.Path}
				file, header, err := r.FormFile("files")
				if err != nil {
					got.err = err
				} else {
					got.fileName = header.Filename
					content, readErr := io.ReadAll(file)
					got.content = string(content)
					got.err = readErr
					_ = file.Close()
				}
				captured <- got
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(responseBody)
			}))
			defer server.Close()

			reader := NewMinerUReader(map[string]string{"mineru_endpoint": server.URL})
			result, err := reader.Read(context.Background(), &types.ReadRequest{
				FileContent: []byte(fileContent),
				FileName:    tt.fileName,
				FileType:    tt.fileType,
			})
			if err != nil {
				t.Fatalf("Read returned error: %v", err)
			}
			if result.MarkdownContent != "ok" {
				t.Fatalf("MarkdownContent = %q, want %q", result.MarkdownContent, "ok")
			}

			request := <-captured
			if request.err != nil {
				t.Fatalf("capture multipart request: %v", request.err)
			}
			if request.method != http.MethodPost {
				t.Errorf("method = %q, want %q", request.method, http.MethodPost)
			}
			if request.path != "/file_parse" {
				t.Errorf("path = %q, want %q", request.path, "/file_parse")
			}
			if request.fileName != tt.want {
				t.Errorf("multipart filename = %q, want %q", request.fileName, tt.want)
			}
			if request.content != fileContent {
				t.Errorf("multipart content = %q, want %q", request.content, fileContent)
			}
		})
	}
}

func TestNormalizeMinerUMarkdownPreservesMarkdownAndHTML(t *testing.T) {
	input := strings.Join([]string{
		"# Heading",
		"",
		"![](images/cover.jpg)",
		"",
		`<details><summary>text_image</summary>caption</details>`,
		"",
		`<table><tr><td><img src="images/profile.jpg"/></td></tr></table>`,
	}, "\n")

	got := normalizeMinerUMarkdown(input)

	if !strings.Contains(got, "# Heading") {
		t.Fatalf("expected heading to stay intact, got: %q", got)
	}
	if strings.Contains(got, `\# Heading`) {
		t.Fatalf("expected heading to avoid escaped form, got: %q", got)
	}
	if !strings.Contains(got, "![](images/cover.jpg)") {
		t.Fatalf("expected markdown image syntax to stay intact, got: %q", got)
	}
	if strings.Contains(got, `!\[](images/cover.jpg)`) {
		t.Fatalf("expected markdown image syntax to avoid escaped form, got: %q", got)
	}
	if !strings.Contains(got, `<details><summary>text_image</summary>caption</details>`) {
		t.Fatalf("expected details/summary block to be preserved, got: %q", got)
	}
	if !strings.Contains(got, `<img src="images/profile.jpg"/>`) {
		t.Fatalf("expected html img tag to be preserved, got: %q", got)
	}
}

func TestProcessImagesKeepsReferencedVariants(t *testing.T) {
	reader := &MinerUReader{}
	mdContent := strings.Join([]string{
		"![](images/cover.jpg)",
		`<img src="./images/profile.jpg"/>`,
		`![](plain.jpg)`,
	}, "\n")

	png := createTestPNG(200, 150)
	b64 := base64.StdEncoding.EncodeToString(png)
	images := map[string]string{
		"cover.jpg":   "data:image/png;base64," + b64,
		"profile.jpg": "data:image/png;base64," + b64,
		"plain.jpg":   "data:image/png;base64," + b64,
	}

	refs, gotMarkdown := reader.processImages(mdContent, images)

	if gotMarkdown != mdContent {
		t.Fatalf("processImages should not rewrite markdown content")
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 image refs, got %d", len(refs))
	}
}

// TestProcessImagesMatchesPathsWithSpaces guards against a regression where
// MinerU image filenames containing spaces (common on Chinese documents,
// e.g. "images/第 1 页.jpg") would be silently dropped because the markdown
// regex used to extract refs disallowed whitespace inside the URL group.
func TestProcessImagesMatchesPathsWithSpaces(t *testing.T) {
	reader := &MinerUReader{}
	mdContent := "![](images/第 1 页.jpg)"

	png := createTestPNG(200, 150)
	b64 := base64.StdEncoding.EncodeToString(png)
	images := map[string]string{
		"第 1 页.jpg": "data:image/png;base64," + b64,
	}

	refs, _ := reader.processImages(mdContent, images)
	if len(refs) != 1 {
		t.Fatalf("expected 1 image ref for path with spaces, got %d", len(refs))
	}
	if refs[0].OriginalRef != "images/第 1 页.jpg" {
		t.Fatalf("unexpected OriginalRef: %q", refs[0].OriginalRef)
	}
}

// TestSplitMinerU4xImages verifies that inline data-URI images in 4.x
// markdown are split into an images/<hash>_<n>.<ext> map and that the markdown
// references are rewritten to the same paths (2.x-style layout).
func TestSplitMinerU4xImages(t *testing.T) {
	png := createTestPNG(4, 2)
	b64 := base64.StdEncoding.EncodeToString(png)
	dataURI := "data:image/png;base64," + b64
	md := "# doc\n\nbefore ![](data:image/png;base64," + b64 + ") after\n\n!" +
		"[](data:image/png;base64," + b64 +")\n" +
		`<img src="data:image/png;base64,` + b64 + `">\n`

	gotMD, imgs := splitMinerU4xImages(md)
	if len(imgs) != 3 {
		t.Fatalf("expected 3 images, got %d", len(imgs))
	}
	if strings.Contains(gotMD, "data:image/") {
		t.Fatalf("markdown still contains data URIs: %q", gotMD[:min(200, len(gotMD))])
	}
	for key, val := range imgs {
		if !strings.HasPrefix(key, "images/") || !strings.HasSuffix(key, ".png") {
			t.Fatalf("unexpected image key: %q", key)
		}
		if val != dataURI {
			t.Fatalf("image %s: value = %q, want the data URI", key, val[:40])
		}
		if !strings.Contains(gotMD, "![]("+key+")") && !strings.Contains(gotMD, `src="`+key+`"`) {
			t.Fatalf("markdown does not reference %s", key)
		}
	}
}

// TestSplitMinerU4xImagesNoImages ensures plain markdown passes through untouched.
func TestSplitMinerU4xImagesNoImages(t *testing.T) {
	md := "# heading\n\nplain text\n"
	got, imgs := splitMinerU4xImages(md)
	if got != md || len(imgs) != 0 {
		t.Fatalf("unexpected change: %q / %d images", got, len(imgs))
	}
}

// TestMinerUReaderV4FlowEndToEnd exercises the 4.x V1 API path: version probe,
// inline job submission, polling, markdown content fetch and data-URI image
// splitting, ending with a ReadResult carrying decoded ImageRefs.
func TestMinerUReaderV4FlowEndToEnd(t *testing.T) {
	utils.SetSSRFWhitelistFromRaw("127.0.0.1")
	t.Cleanup(func() {
		utils.SetSSRFWhitelistFromRaw("")
	})

	png := createTestPNG(8, 6)
	b64 := base64.StdEncoding.EncodeToString(png)
	mdBody := "# report\n\ntext line\n\n![](data:image/png;base64," + b64 + ")\n"

	var jobPayload map[string]any
	job := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/health":
			_, _ = w.Write([]byte(`{"status":"ok","version":"4.0.10"}`))
		case r.URL.Path == "/v1/parse/jobs" && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&jobPayload)
			_, _ = w.Write([]byte(`{"job_id":"job_test42"}`))
		case r.URL.Path == "/v1/parse/jobs/job_test42":
			_, _ = w.Write([]byte(`{"status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"markdown":{"file_id":"file_md1"}}}]}`))
		case r.URL.Path == "/v1/files/file_md1/content":
			_, _ = w.Write([]byte(mdBody))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
		}
	}))
	defer job.Close()

	reader := NewMinerUReader(map[string]string{"mineru_endpoint": job.URL})
	res, err := reader.Read(context.Background(), &types.ReadRequest{
		FileContent: []byte("%PDF-1.7 tiny"),
		FileName:    "tiny.doc",
		FileType:    "pdf",
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(res.MarkdownContent, "# report") {
		t.Fatalf("markdown missing heading: %q", res.MarkdownContent)
	}
	if strings.Contains(res.MarkdownContent, "data:image/") {
		t.Fatalf("markdown still inlines a data URI: %q", res.MarkdownContent)
	}
	if len(res.ImageRefs) != 1 {
		t.Fatalf("expected 1 ImageRef, got %d", len(res.ImageRefs))
	}
	if string(res.ImageRefs[0].ImageData) != string(png) {
		t.Fatalf("ImageRef data does not match the original image bytes")
	}
	if res.ImageRefs[0].MimeType != "image/png" {
		t.Fatalf("unexpected mime type: %q", res.ImageRefs[0].MimeType)
	}
	// inline source must carry base64 of the exact file bytes and be
	// wrapped as {"source": {...}} per the 4.x V1 API contract
	files, _ := jobPayload["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("job payload files: %v", jobPayload["files"])
	}
	wrapped, _ := files[0].(map[string]any)
	src, _ := wrapped["source"].(map[string]any)
	if src == nil {
		t.Fatalf("expected files[0] to wrap the source object, got %v", files[0])
	}
	if src["type"] != "inline" {
		t.Fatalf("expected inline source, got %v", src)
	}
	if got, _ := src["data"].(string); got != base64.StdEncoding.EncodeToString([]byte("%PDF-1.7 tiny")) {
		t.Fatalf("inline data mismatch")
	}
}
