package docparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

const mineruTimeout = 1000 * time.Second // large docs can take a while

var (
	b64DataURIPattern     = regexp.MustCompile(`^data:image/(\w+);base64,(.+)$`)
	minerUFileTypePattern = regexp.MustCompile(`^[a-z0-9]+$`)
)

// MinerU service API generations. 2.x/3.x exposes the legacy single-call
// POST /file_parse; 4.x replaced it with the V1 flow (/v1/parse/jobs +
// /v1/files/{id}/content) and removed /file_parse (probe returns 404).
type mineruAPIVersion int

const (
	mineruAPIV4 mineruAPIVersion = iota
	mineruAPILegacy
)

const (
	mineruV4PollInterval = 20 * time.Second // 4.x CPU tiers batch per poll round
	mineruV4Tier         = "basic"          // WeKnora always runs 4.x in basic tier
)

// mineruV4InlineMaxBytes — the 4.x server caps inline (base64) sources at
// max_inline_bytes (1MiB, undocumented in the openapi spec; base64 inflates
// payload 4/3x). Above it we use the 3-step /v1/uploads flow.
var mineruV4InlineMaxBytes = 700 * 1024

// mineruV4OCRMode maps WeKnora's parse method to the 4.x ocr_mode field.
func mineruV4OCRMode(parseMethod string) string {
	switch strings.ToLower(strings.TrimSpace(parseMethod)) {
	case "ocr":
		return "ocr"
	case "txt", "text", "no_ocr":
		return "txt"
	default:
		return "auto"
	}
}

// mineruV4MIME guesses the media type by file extension (images/pdf/docx/pptx).
func mineruV4MIME(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return "application/octet-stream"
	}
}

// detectMinerUAPIVersion probes GET /v1/health to decide which protocol the
// service speaks. Empty endpoint (e.g. cloud fallback config) falls back to the
// legacy flow to preserve previous behavior.
func (c *MinerUReader) detectMinerUAPIVersion(ctx context.Context) mineruAPIVersion {
	if c.endpoint == "" {
		return mineruAPILegacy
	}
	client := utils.NewSSRFSafeHTTPClient(utils.SSRFSafeHTTPClientConfig{
		Timeout:      5 * time.Second,
		MaxRedirects: 1,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/v1/health", nil)
	if err != nil {
		return mineruAPILegacy
	}
	resp, err := client.Do(req)
	if err != nil {
		return mineruAPILegacy
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return mineruAPIV4
	}
	return mineruAPILegacy
}

// MinerUReader calls a self-hosted MinerU API to read/convert documents.
type MinerUReader struct {
	endpoint      string
	backend       string // "pipeline", "vlm-*", "hybrid-*"
	vlmServerURL  string // vLLM server URL for vlm-http-client / hybrid-http-client
	formulaEnable bool
	tableEnable   bool
	parseMethod   string
	language      string
	apiVersion    *mineruAPIVersion // lazily probed; nil = not yet detected
}

// NewMinerUReader creates a reader from ParserEngineOverrides.
func NewMinerUReader(overrides map[string]string) *MinerUReader {
	var legacyOCREnabled *bool
	if raw, ok := overrides["mineru_enable_ocr"]; ok {
		value := parseBoolOr(raw, true)
		legacyOCREnabled = &value
	}

	c := &MinerUReader{
		endpoint:      strings.TrimRight(overrides["mineru_endpoint"], "/"),
		backend:       stringOr(overrides["mineru_model"], "pipeline"),
		vlmServerURL:  overrides["mineru_vlm_server_url"],
		formulaEnable: parseBoolOr(overrides["mineru_enable_formula"], true),
		tableEnable:   parseBoolOr(overrides["mineru_enable_table"], true),
		parseMethod:   types.ResolveMinerUParseMethod(overrides["mineru_parse_method"], legacyOCREnabled),
		language:      stringOr(overrides["mineru_language"], "ch"),
	}
	return c
}

func (c *MinerUReader) Read(ctx context.Context, req *types.ReadRequest) (*types.ReadResult, error) {
	if c.endpoint == "" {
		return &types.ReadResult{Error: "MinerU endpoint is not configured"}, nil
	}
	if err := validateMinerUOutboundURL(c.endpoint); err != nil {
		return &types.ReadResult{Error: err.Error()}, nil
	}
	if c.vlmServerURL != "" {
		if err := validateMinerUOutboundURL(c.vlmServerURL); err != nil {
			return &types.ReadResult{Error: err.Error()}, nil
		}
	}

	content := req.FileContent
	if len(content) == 0 {
		return &types.ReadResult{Error: "no file content provided"}, nil
	}

	logger.Infof(context.Background(), "[MinerU] Parsing file=%s size=%d via %s", req.FileName, len(content), c.endpoint)

	c.ensureAPIVersion(ctx)
	mdContent, imagesB64, err := c.callFileParse(ctx, content, req.FileName, req.FileType)
	if err != nil {
		return nil, fmt.Errorf("MinerU file_parse: %w", err)
	}

	// MinerU already returns markdown with embedded HTML blocks (e.g. <table>, <details>).
	// Re-running the whole document through html-to-markdown corrupts valid markdown
	// by escaping headings and image syntax. Only apply narrow compatibility fixes.
	mdContent = normalizeMinerUMarkdown(mdContent)

	// Process images: decode base64, build ImageRef list, replace refs in markdown
	imageRefs, mdContent := c.processImages(mdContent, imagesB64)

	mdContent, imageRefs = ensureOriginalImageRef(req, mdContent, imageRefs)

	logger.Infof(context.Background(), "[MinerU] Parsed successfully, markdown=%d chars, images=%d", len(mdContent), len(imageRefs))

	return &types.ReadResult{
		MarkdownContent: mdContent,
		ImageRefs:       imageRefs,
	}, nil
}

// ensureAPIVersion lazily probes the service version once per reader.
func (c *MinerUReader) ensureAPIVersion(ctx context.Context) {
	if c.apiVersion != nil {
		return
	}
	v := c.detectMinerUAPIVersion(ctx)
	c.apiVersion = &v
	var name string
	if v == mineruAPIV4 {
		name = "MinerU 4.x (V1 API)"
	} else {
		name = "MinerU 2.x/3.x (legacy /file_parse)"
	}
	logger.Infof(context.Background(), "[MinerU] %s detected at %s", name, c.endpoint)
}

type mineruFileEntry struct {
	MDContent string            `json:"md_content"`
	Images    map[string]string `json:"images"` // path -> "data:image/png;base64,..." or raw base64
}

func minerUCleanFileType(fileType string) string {
	cleanType := strings.ToLower(strings.TrimSpace(fileType))
	cleanType = strings.TrimPrefix(cleanType, ".")
	if minerUFileTypePattern.MatchString(cleanType) {
		return cleanType
	}
	return ""
}

func minerUUploadFileName(fileName, fileType string) string {
	cleanName := strings.TrimSpace(fileName)
	cleanName = strings.ReplaceAll(cleanName, `\`, "/")
	cleanName = path.Base(cleanName)
	cleanName = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		return r
	}, cleanName)
	cleanName = strings.TrimSpace(cleanName)
	if cleanName != "" && cleanName != "." && cleanName != ".." && cleanName != "/" {
		if filepath.Ext(cleanName) == "" {
			if cleanType := minerUCleanFileType(fileType); cleanType != "" {
				return cleanName + "." + cleanType
			}
		}
		return cleanName
	}

	if cleanType := minerUCleanFileType(fileType); cleanType != "" {
		return "document." + cleanType
	}
	return "document"
}

// minerUResultStem mirrors MinerU's upload.stem: the basename without extension.
func minerUResultStem(uploadFileName string) string {
	stem := strings.TrimSuffix(path.Base(uploadFileName), filepath.Ext(uploadFileName))
	if stem == "" || stem == "." {
		return ""
	}
	return stem
}

func minerUResultLookupKeys(uploadFileName string) []string {
	keys := make([]string, 0, 3)
	if stem := minerUResultStem(uploadFileName); stem != "" {
		keys = append(keys, stem)
	}
	return append(keys, "document", "files")
}

func parseMinerUFileParseResponse(respBody []byte, uploadFileName string) (string, map[string]string, string, error) {
	var envelope struct {
		Results map[string]mineruFileEntry `json:"results"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return "", nil, "", fmt.Errorf("decode response: %w", err)
	}
	if len(envelope.Results) == 0 {
		return "", nil, "", nil
	}

	for _, key := range minerUResultLookupKeys(uploadFileName) {
		if entry, ok := envelope.Results[key]; ok {
			if entry.MDContent != "" || len(entry.Images) > 0 {
				return entry.MDContent, entry.Images, key, nil
			}
		}
	}

	for key, entry := range envelope.Results {
		if entry.MDContent != "" || len(entry.Images) > 0 {
			return entry.MDContent, entry.Images, key, nil
		}
	}
	return "", nil, "", nil
}

func (c *MinerUReader) callFileParse(
	ctx context.Context,
	content []byte,
	fileName string,
	fileType string,
) (string, map[string]string, error) {
	if c.apiVersion != nil && *c.apiVersion == mineruAPIV4 {
		return c.callFileParseV4(ctx, content, fileName)
	}
	return c.callFileParseLegacy(ctx, content, fileName, fileType)
}

func (c *MinerUReader) callFileParseLegacy(
	ctx context.Context,
	content []byte,
	fileName string,
	fileType string,
) (string, map[string]string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Form fields
	fields := map[string]string{
		"return_md":           "true",
		"return_images":       "true",
		"table_enable":        fmt.Sprintf("%v", c.tableEnable),
		"formula_enable":      fmt.Sprintf("%v", c.formulaEnable),
		"parse_method":        c.parseMethod,
		"start_page_id":       "0",
		"end_page_id":         "99999",
		"backend":             c.backend,
		"response_format_zip": "false",
		"return_middle_json":  "false",
		"return_model_output": "false",
		"return_content_list": "true",
	}
	if c.language != "" {
		fields["lang_list"] = c.language
	}
	if c.vlmServerURL != "" && (strings.HasPrefix(c.backend, "vlm-http-client") || strings.HasPrefix(c.backend, "hybrid-http-client")) {
		fields["server_url"] = c.vlmServerURL
	}
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}

	uploadFileName := minerUUploadFileName(fileName, fileType)

	// File part
	part, err := writer.CreateFormFile("files", uploadFileName)
	if err != nil {
		return "", nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return "", nil, fmt.Errorf("write file content: %w", err)
	}
	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/file_parse", &body)
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	client := utils.NewSSRFSafeHTTPClient(utils.SSRFSafeHTTPClientConfig{
		Timeout:      mineruTimeout,
		MaxRedirects: 5,
	})
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("MinerU API status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read response body: %w", err)
	}

	// Dump raw response for debugging (truncate if too large)
	rawStr := string(respBody)
	if len(rawStr) > 4000 {
		logger.Infof(context.Background(), "[MinerU] Raw response (truncated to 4000 chars): %s ...", rawStr[:4000])
	} else {
		logger.Infof(context.Background(), "[MinerU] Raw response: %s", rawStr)
	}

	// Also pretty-print the top-level structure (without large base64 blobs)
	var rawMap map[string]interface{}
	if err := json.Unmarshal(respBody, &rawMap); err == nil {
		c.logMinerUResponseStructure(rawMap, "")
	}

	mdContent, imagesB64, resultKey, err := parseMinerUFileParseResponse(respBody, uploadFileName)
	if err != nil {
		return "", nil, err
	}
	if resultKey != "" {
		logger.Infof(context.Background(), "[MinerU] Using response path: results.%s", resultKey)
		return mdContent, imagesB64, nil
	}

	logger.Errorf(context.Background(), "[MinerU] Response has no markdown/images under results")
	return "", nil, nil
}

// ---------------------------------------------------------------------------
// MinerU 4.x (V1 API) flow: /v1/parse/jobs + poll + /v1/files/{id}/content
// ---------------------------------------------------------------------------

// callFileParseV4 runs the 4.x V1 flow: submit file (inline < 700KB, else
// 3-step upload) -> create parse job -> poll to terminal -> fetch markdown
// content. Mirrors the legacy contract: returns markdown (data-URI images
// already rewritten to images/<hash>.<ext>) plus the images map keyed by the
// same paths, which processImages() consumes the same way as 2.x results.
func (c *MinerUReader) callFileParseV4(
	ctx context.Context,
	content []byte,
	fileName string,
) (string, map[string]string, error) {
	uploadName := minerUUploadFileName(fileName, "")
	if uploadName == "" {
		uploadName = "document.pdf"
	}
	uploadName = strings.TrimSpace(uploadName)
	mimeStr := mineruV4MIME(uploadName)

	var fileID string
	var err error
	if len(content) <= mineruV4InlineMaxBytes {
		inlineSource := map[string]any{
			"type": "inline",
			"name": uploadName,
			"data": base64.StdEncoding.EncodeToString(content),
		}
		fileID, err = c.createV4Job(ctx, []map[string]any{inlineSource})
		if err != nil {
			// Server may reject inline above its undocumented cap — retry via upload.
			if isMinerUV4SourceError(err) {
				logger.Warnf(context.Background(), "[MinerU] inline source rejected (%v), falling back to upload", err)
				fileID, err = c.uploadV4File(ctx, uploadName, content, mimeStr)
				if err != nil {
					return "", nil, err
				}
				fileID, err = c.createV4Job(ctx, []map[string]any{{"type": "file_id", "file_id": fileID}})
			}
			if err != nil {
				return "", nil, err
			}
		}
	} else {
		fileID, err = c.uploadV4File(ctx, uploadName, content, mimeStr)
		if err != nil {
			return "", nil, err
		}
		fileID, err = c.createV4Job(ctx, []map[string]any{{"type": "file_id", "file_id": fileID}})
		if err != nil {
			return "", nil, err
		}
	}

	job, err := c.pollV4Job(ctx, fileID)
	if err != nil {
		return "", nil, err
	}

	mdBytes := c.v4OutputFile(job, "markdown")
	if len(mdBytes) == 0 {
		return "", nil, fmt.Errorf("MinerU 4.x job finished but has no markdown output: %v", job)
	}

	mdContent := string(mdBytes)
	mdContent, imagesB64 := splitMinerU4xImages(mdContent)
	return mdContent, imagesB64, nil
}

// isMinerUV4SourceError reports whether a 4.x job-creation error is an
// inline-source rejection (server-side max_inline_bytes cap).
func isMinerUV4SourceError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unsupported_source") ||
		strings.Contains(msg, "max_inline_bytes")
}

// v4JSON posts a JSON body to the given path and decodes the JSON response.
func (c *MinerUReader) v4JSON(ctx context.Context, method, path string, payload any, out any) error {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := utils.NewSSRFSafeHTTPClient(utils.SSRFSafeHTTPClientConfig{
		Timeout:      120 * time.Second,
		MaxRedirects: 3,
	})
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("MinerU 4.x API %s %s status %d: %s", method, path, resp.StatusCode, truncateString(string(respBody), 400))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
	}
	return nil
}

// createV4Job submits a parse job and returns its job id.
// sources are the per-file source objects ({"type":"inline",...} /
// {"type":"file_id",...}); the server requires each one wrapped as
// {"source": {...}} inside the files array, so wrap them here.
func (c *MinerUReader) createV4Job(ctx context.Context, sources []map[string]any) (string, error) {
	files := make([]map[string]any, 0, len(sources))
	for _, s := range sources {
		files = append(files, map[string]any{"source": s})
	}
	payload := map[string]any{
		"files":          files,
		"tier":           mineruV4Tier,
		"ocr_mode":       mineruV4OCRMode(c.parseMethod),
		"output_formats": []string{"markdown"},
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	if err := c.v4JSON(ctx, http.MethodPost, "/v1/parse/jobs", payload, &resp); err != nil {
		return "", err
	}
	if resp.JobID == "" {
		return "", fmt.Errorf("MinerU 4.x create job returned empty job_id")
	}
	return resp.JobID, nil
}

// uploadV4File runs the 3-step 4.x upload: create -> PUT raw bytes -> complete.
func (c *MinerUReader) uploadV4File(ctx context.Context, name string, data []byte, mimeStr string) (string, error) {
	var created struct {
		UploadURL     string            `json:"upload_url"`
		UploadHeaders map[string]string `json:"upload_headers"`
		FileID        string            `json:"file_id"`
		File          struct {
			ID string `json:"id"`
		} `json:"file"`
	}
	payload := map[string]any{
		"filename":  name,
		"bytes":     len(data),
		"mime_type": mimeStr,
		"purpose":   "parse",
	}
	if err := c.v4JSON(ctx, http.MethodPost, "/v1/uploads", payload, &created); err != nil {
		return "", fmt.Errorf("init upload: %w", err)
	}
	uploadURL := created.UploadURL
	if uploadURL == "" {
		return "", fmt.Errorf("MinerU 4.x upload init returned no upload_url")
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if created.FileID == "" {
		created.FileID = created.File.ID
	}
	putReq.Header.Set("Content-Type", mimeStr)
	for k, v := range created.UploadHeaders {
		putReq.Header.Set(k, v)
	}
	client := utils.NewSSRFSafeHTTPClient(utils.SSRFSafeHTTPClientConfig{
		Timeout:      mineruTimeout,
		MaxRedirects: 3,
	})
	resp, err := client.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("upload PUT: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upload PUT status %d: %s", resp.StatusCode, truncateString(string(body), 400))
	}

	var completed struct {
		File struct {
			ID string `json:"id"`
		} `json:"file"`
	}
	if err := c.v4JSON(ctx, http.MethodPost, "/v1/uploads/"+created.FileID+"/complete", map[string]any{}, &completed); err != nil {
		return "", fmt.Errorf("complete upload: %w", err)
	}
	if completed.File.ID == "" {
		return "", fmt.Errorf("MinerU 4.x upload complete returned no file id")
	}
	return completed.File.ID, nil
}

// pollV4Job polls GET /v1/parse/jobs/{id} until a terminal status or context deadline.
func (c *MinerUReader) pollV4Job(ctx context.Context, jobID string) (map[string]any, error) {
	client := utils.NewSSRFSafeHTTPClient(utils.SSRFSafeHTTPClientConfig{
		Timeout:      60 * time.Second,
		MaxRedirects: 3,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/v1/parse/jobs/"+jobID, nil)
	if err != nil {
		return nil, err
	}
	var job map[string]any
	t := time.NewTicker(mineruV4PollInterval)
	defer t.Stop()
	for {
		resp, err := client.Do(req.Clone(ctx))
		if err != nil {
			return nil, fmt.Errorf("poll job: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("poll job status %d: %s", resp.StatusCode, truncateString(string(body), 400))
		}
		if err := json.Unmarshal(body, &job); err != nil {
			return nil, fmt.Errorf("decode job response: %w", err)
		}
		status, _ := job["status"].(string)
		switch status {
		case "completed", "partial":
			return job, nil
		case "failed", "canceled":
			// Surface per-file errors when present.
			if files, ok := job["files"].([]any); ok {
				for _, item := range files {
					if m, ok := item.(map[string]any); ok {
						if errMsg, _ := m["error"].(string); errMsg != "" {
							return job, fmt.Errorf("MinerU 4.x job %s: %s", status, errMsg)
						}
					}
				}
			}
			return job, fmt.Errorf("MinerU 4.x job %s", status)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for MinerU 4.x job %s (last status %q)", jobID, status)
		case <-t.C:
		}
	}
}

// v4OutputFile fetches GET /v1/files/{file_id}/content for the first file's
// output entry in the given format; returns nil when absent.
func (c *MinerUReader) v4OutputFile(job map[string]any, format string) []byte {
	files, _ := job["files"].([]any)
	for _, item := range files {
		f, ok := item.(map[string]any)
		if !ok {
			continue
		}
		outputs, _ := f["output_files"].(map[string]any)
		ref, _ := outputs[format].(map[string]any)
		fid, _ := ref["file_id"].(string)
		if fid == "" {
			continue
		}
		client := utils.NewSSRFSafeHTTPClient(utils.SSRFSafeHTTPClientConfig{
		Timeout:      mineruTimeout,
		MaxRedirects: 3,
		})
		resp, err := client.Get(c.endpoint + "/v1/files/" + fid + "/content")
		if err != nil {
			logger.Errorf(context.Background(), "[MinerU] v4 content fetch %s: %v", fid, err)
			return nil
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 || readErr != nil {
			logger.Errorf(context.Background(), "[MinerU] v4 content fetch %s status=%d err=%v", fid, resp.StatusCode, readErr)
			return nil
		}
		return body
	}
	return nil
}

// mineruV4InlineDataURIPattern matches inline data-URI images in 4.x markdown
// (images embedded directly in the body, not in a separate images map).
// Group 2 stops at the first character outside the base64 alphabet, which in
// markdown is the closing paren/quote — sufficient for the generated layout.
// (Go's RE2 has no lookahead, so the data group cannot be made lazy-stopping.)
var mineruV4InlineDataURIPattern = regexp.MustCompile(`data:image/([a-zA-Z0-9.+-]+);base64,([A-Za-z0-9+/=\s]*)`)

var mineruV4MIMEToExt = map[string]string{
	"jpeg": "jpg", "jpg": "jpg", "png": "png", "gif": "gif",
	"webp": "webp", "bmp": "bmp", "svg+xml": "svg", "tif": "tif", "tiff": "tif",
}

// splitMinerU4xImages performs a single pass over 4.x markdown: every inline
// data-URI image (a) gets an images/<sha256>_<n>.<ext> path that replaces the
// data URI inside the markdown, and (b) is recorded in the returned images map
// (path -> data URI), mirroring the 2.x result contract processImages() expects.
// The counter in the key disambiguates repeated identical images (map keys
// must be unique per entry so each occurrence can resolve to its own ref).
func splitMinerU4xImages(mdContent string) (string, map[string]string) {
	images := make(map[string]string)
	matches := mineruV4InlineDataURIPattern.FindAllStringSubmatchIndex(mdContent, -1)
	if len(matches) == 0 {
		return mdContent, images
	}
	type span struct {
		start, end int
		key        string
	}
	var spans []span
	counter := 0
	for _, loc := range matches {
		// FindAllStringSubmatchIndex returns [full, group1, group2] offsets.
		prefix := mdContent[loc[0]:loc[4]] // "data:image/<mime>;base64,"
		raw := mdContent[loc[4]:loc[5]]
		// The greedy group can swallow whitespace after the closing paren/quote;
		// the replaceable span ends at the last real base64 character.
		payload := strings.TrimRight(raw, " \t\r\n")
		if payload == "" {
			continue
		}
		dataEnd := loc[4] + len(payload)
		ext := "png"
		if mi := strings.Index(prefix, "/"); mi >= 0 {
			mimePart := strings.ToLower(prefix[mi+1:])
			if semi := strings.Index(mimePart, ";"); semi >= 0 {
				mimePart = mimePart[:semi]
			}
			if e, ok := mineruV4MIMEToExt[mimePart]; ok {
				ext = e
			} else if mimePart != "" {
				ext = mimePart
			}
		}
		key := fmt.Sprintf("images/%x_%d.%s", sha256.Sum256([]byte(payload)), counter, ext)
		spans = append(spans, span{start: loc[0], end: dataEnd, key: key})
		images[key] = prefix + payload
		counter++
	}
	if len(spans) == 0 {
		return mdContent, images
	}
	var sb strings.Builder
	last := 0
	for _, s := range spans {
		sb.WriteString(mdContent[last:s.start])
		sb.WriteString(s.key)
		last = s.end
	}
	sb.WriteString(mdContent[last:])
	return sb.String(), images
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// processImages decodes base64 images from MinerU response and returns ImageRef list.
// It also replaces image references in the markdown content.
func (c *MinerUReader) processImages(mdContent string, imagesB64 map[string]string) ([]types.ImageRef, string) {
	var refs []types.ImageRef

	for ipath, b64Str := range imagesB64 {
		matchedRefs := mineruImageOriginalRefs(mdContent, ipath)
		if len(matchedRefs) == 0 {
			continue
		}

		var imgBytes []byte
		var ext string

		if m := b64DataURIPattern.FindStringSubmatch(b64Str); len(m) == 3 {
			ext = m[1]
			decoded, err := base64.StdEncoding.DecodeString(m[2])
			if err != nil {
				logger.Errorf(context.Background(), "[MinerU] Failed to decode base64 image %s: %v", ipath, err)
				continue
			}
			imgBytes = decoded
		} else {
			// raw base64 without data URI prefix
			decoded, err := base64.StdEncoding.DecodeString(b64Str)
			if err != nil {
				logger.Errorf(context.Background(), "[MinerU] Failed to decode raw base64 image %s: %v", ipath, err)
				continue
			}
			imgBytes = decoded
			ext = strings.TrimPrefix(filepath.Ext(ipath), ".")
			if ext == "" {
				ext = "png"
			}
		}

		mimeType := mime.TypeByExtension("." + ext)
		if mimeType == "" {
			mimeType = "image/png"
		}

		for _, originalRef := range matchedRefs {
			refs = append(refs, types.ImageRef{
				Filename:    ipath,
				OriginalRef: originalRef,
				MimeType:    mimeType,
				ImageData:   imgBytes,
			})
		}
	}

	return refs, mdContent
}

// logMinerUResponseStructure logs the structure of the MinerU API response.
func (c *MinerUReader) logMinerUResponseStructure(obj interface{}, prefix string) {
	logResponseStructure("MinerU", obj, prefix)
}

// validateMinerUOutboundURL rejects MinerU endpoints that would reach private
// or otherwise restricted hosts when parsed or probed from the app server.
func validateMinerUOutboundURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	if err := utils.ValidateURLForSSRF(rawURL); err != nil {
		return fmt.Errorf("MinerU URL blocked by SSRF check: %v", err)
	}
	return nil
}

// PingMinerU checks if the self-hosted MinerU service is reachable.
func PingMinerU(endpoint string) (bool, string) {
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == "" {
		return false, "未配置 MinerU 端点"
	}
	if err := validateMinerUOutboundURL(endpoint); err != nil {
		return false, err.Error()
	}
	client := utils.NewSSRFSafeHTTPClient(utils.SSRFSafeHTTPClientConfig{
		Timeout:      5 * time.Second,
		MaxRedirects: 5,
	})
	resp, err := client.Get(endpoint + "/docs")
	if err != nil {
		return false, fmt.Sprintf("MinerU 服务不可达: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false, fmt.Sprintf("MinerU 服务返回状态 %d", resp.StatusCode)
	}
	return true, ""
}

// escapedImageSyntaxPattern matches markdown image references whose `[` was
// over-escaped to `\[` by html-to-markdown. The URL group mirrors the
// downstream image-extraction regex so escapes are only stripped for actual
// image references.
var escapedImageSyntaxPattern = regexp.MustCompile(`!\\\[(.*?)\\?\]\(([^()\n]*(?:\([^)]*\)[^()\n]*)*)\)`)

// escapedHeadingPattern restores markdown headings that were over-escaped to
// \# Heading. We only touch line-leading heading markers to avoid rewriting
// body text that intentionally contains escaped # characters.
var escapedHeadingPattern = regexp.MustCompile(`(?m)^\\(#{1,6})(\s+)`)

// unescapeMarkdownImageSyntax restores `![alt](url)` from html-to-markdown's
// over-escaped `!\[alt\](url)` form. Without this, the downstream image regex
// in ImageResolver fails to match and images are never persisted.
func unescapeMarkdownImageSyntax(content string) string {
	return escapedImageSyntaxPattern.ReplaceAllString(content, "![$1]($2)")
}

func normalizeEscapedMarkdownHeadings(content string) string {
	return escapedHeadingPattern.ReplaceAllString(content, `$1$2`)
}

func normalizeMinerUMarkdown(content string) string {
	content = unescapeMarkdownImageSyntax(content)
	content = normalizeEscapedMarkdownHeadings(content)
	return content
}

func mineruImageOriginalRefs(mdContent, imagePath string) []string {
	normalizedTarget := normalizeMinerUImagePath(imagePath)
	if normalizedTarget == "" {
		return nil
	}

	referenced := extractImageRefsFromContent(mdContent)
	seen := make(map[string]struct{}, len(referenced))
	var matched []string
	for _, refPath := range referenced {
		if normalizeMinerUImagePath(refPath) != normalizedTarget {
			continue
		}
		if _, ok := seen[refPath]; ok {
			continue
		}
		matched = append(matched, refPath)
		seen[refPath] = struct{}{}
	}

	return matched
}

// imgMarkdownPatternAllowSpaces matches markdown image syntax while allowing
// spaces in the URL group, so that paths like "images/第 1 页.jpg" produced by
// MinerU on Chinese documents are still detected as image references.
var imgMarkdownPatternAllowSpaces = regexp.MustCompile(
	`!\[(.*?)\]\(([^()\n]*(?:\([^)]*\)[^()\n]*)*)\)`,
)

func extractImageRefsFromContent(content string) []string {
	var refs []string

	for _, match := range imgMarkdownPatternAllowSpaces.FindAllStringSubmatch(content, -1) {
		if len(match) >= 3 {
			refs = append(refs, strings.TrimSpace(match[2]))
		}
	}
	for _, match := range imgHTMLSrc.FindAllStringSubmatch(content, -1) {
		if len(match) >= 3 {
			refs = append(refs, match[2])
		}
	}

	return refs
}

func normalizeMinerUImagePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(p); err == nil && decoded != "" {
		p = decoded
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "images/")
	return p
}
