package handler

import (
	"bytes"
	"context"
	stdErrors "errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils/export"
)

const (
	exportFailureValidation            = "validation_failed"
	exportFailureContentTooLarge       = "content_too_large"
	exportFailureCapabilityUnavailable = "capability_unavailable"
	exportFailureTimeout               = "render_timeout"
	exportFailureRender                = "render_failed"
)

// ExportHandler handles HTTP requests for document export
type ExportHandler struct{}

// NewExportHandler creates a new export handler instance
func NewExportHandler() *ExportHandler {
	return &ExportHandler{}
}

// ExportDocumentRequest represents the request body for document export
type ExportDocumentRequest struct {
	// Markdown content to export
	Content string `json:"content" binding:"required"`
	// Export format: "markdown", "pdf", "docx" or "xlsx"
	Format string `json:"format" binding:"required,oneof=markdown pdf docx xlsx"`
	// Optional filename prefix
	FilenamePrefix string `json:"filename_prefix,omitempty"`
}

// ExportDocument godoc
// @Summary      导出文档
// @Description  将 Markdown 内容导出为 Markdown、PDF、Word (DOCX) 或 XLSX 文件
// @Tags         导出
// @Accept       json
// @Produce      text/markdown,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        body  body      ExportDocumentRequest  true  "导出请求"
// @Success      200   {file}    binary                 "导出的文件"
// @Failure      400   {object}  errors.AppError        "请求参数错误"
// @Failure      500   {object}  errors.AppError        "导出失败"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /export/document [post]
func (h *ExportHandler) ExportDocument(c *gin.Context) {
	ctx := c.Request.Context()
	requestID, _ := types.RequestIDFromContext(ctx)
	startedAt := time.Now()

	var req ExportDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warnf(ctx, "[Export] request_id=%s invalid request: %v", requestID, err)
		h.failExport(c, requestID, "", exportFailureValidation, startedAt,
			errors.NewBadRequestError("Invalid request: content and format (markdown/pdf/docx/xlsx) are required"))
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		h.failExport(c, requestID, req.Format, exportFailureValidation, startedAt,
			errors.NewBadRequestError("Content cannot be empty"))
		return
	}

	policy := export.PolicyForFormat(req.Format)
	if policy.Format == "" {
		h.failExport(c, requestID, req.Format, exportFailureValidation, startedAt,
			errors.NewBadRequestError("Unsupported format: "+req.Format))
		return
	}

	contentBytes := len([]byte(req.Content))
	if contentBytes > policy.MaxContentBytes {
		h.failExport(c, requestID, req.Format, exportFailureContentTooLarge, startedAt,
			errors.NewRequestTooLargeError(fmt.Sprintf("Export content exceeds the limit for %s", req.Format)).WithDetails(gin.H{
				"request_id":        requestID,
				"category":          exportFailureContentTooLarge,
				"format":            req.Format,
				"content_bytes":     contentBytes,
				"max_content_bytes": policy.MaxContentBytes,
			}))
		return
	}

	if !policy.Available {
		h.failExport(c, requestID, req.Format, exportFailureCapabilityUnavailable, startedAt,
			errors.NewServiceUnavailableError(fmt.Sprintf("%s export is unavailable: %s", strings.ToUpper(req.Format), policy.Reason)).WithDetails(gin.H{
				"request_id":        requestID,
				"category":          exportFailureCapabilityUnavailable,
				"format":            req.Format,
				"engine":            policy.Engine,
				"reason":            policy.Reason,
				"timeout_seconds":   int(policy.Timeout / time.Second),
				"max_content_bytes": policy.MaxContentBytes,
			}))
		return
	}

	// Generate filename
	prefix := req.FilenamePrefix
	if prefix == "" {
		prefix = "对话导出"
	}
	timestamp := time.Now().Format("2006-01-02_150405")
	filename := fmt.Sprintf("%s_%s", prefix, timestamp)

	logger.Infof(ctx, "[Export] request_id=%s start format=%s engine=%s content_bytes=%d timeout_seconds=%d", requestID, req.Format, policy.Engine, contentBytes, int(policy.Timeout/time.Second))

	exportCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()

	var (
		data        []byte
		contentType string
		ext         string
		err         error
	)

	switch req.Format {
	case "markdown":
		data = []byte(req.Content)
		contentType = "text/markdown; charset=utf-8"
		ext = "md"
	case "pdf":
		data, err = export.MarkdownToPDF(exportCtx, req.Content)
		contentType = "application/pdf"
		ext = "pdf"
	case "docx":
		data, err = export.MarkdownToDocx(exportCtx, req.Content)
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		ext = "docx"
	case "xlsx":
		data, err = export.MarkdownToXLSX(exportCtx, req.Content)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		ext = "xlsx"
	}

	if err != nil {
		category := classifyExportFailureCategory(err)
		appErr := buildExportAppError(err, requestID, req.Format, policy, category, startedAt)
		h.failExport(c, requestID, req.Format, category, startedAt, appErr)
		return
	}

	durationMs := time.Since(startedAt).Milliseconds()
	logger.Infof(ctx, "[Export] request_id=%s completed format=%s engine=%s duration_ms=%d input_bytes=%d output_bytes=%d", requestID, req.Format, policy.Engine, durationMs, contentBytes, len(data))

	// Set response headers for file download
	c.Header("Content-Disposition", buildAttachmentContentDisposition(fmt.Sprintf("%s.%s", filename, ext)))
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", len(data)))
	c.Header("X-Export-Duration-Ms", fmt.Sprintf("%d", durationMs))

	c.Data(http.StatusOK, contentType, data)
}

// ExportCapabilities godoc
// @Summary      查询导出能力
// @Description  返回后端支持的导出格式（检查 Chromium/pandoc 是否安装）
// @Tags         导出
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "导出能力信息"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /export/capabilities [get]
func (h *ExportHandler) ExportCapabilities(c *gin.Context) {
	ctx := c.Request.Context()
	requestID, _ := types.RequestIDFromContext(ctx)

	logger.Infof(ctx, "[Export] request_id=%s capabilities check", requestID)

	data := gin.H{}
	for _, format := range export.SupportedFormats() {
		data[format] = export.CapabilityForFormat(format)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// ExportMarkdownToHTML godoc
// @Summary      Markdown 转 HTML 预览
// @Description  将 Markdown 转为带样式的 HTML（纯 Go 实现，无需外部工具）
// @Tags         导出
// @Accept       json
// @Produce      html
// @Param        body  body      ExportHTMLRequest  true  "Markdown 内容"
// @Success      200   {string}  string             "HTML 内容"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /export/html [post]
func (h *ExportHandler) ExportHTML(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("Invalid request: content is required"))
		return
	}

	html, err := export.MarkdownToStyledHTML(req.Content)
	if err != nil {
		logger.Errorf(ctx, "[Export] Failed to convert markdown to HTML: %v", err)
		c.Error(errors.NewInternalServerError("Failed to convert markdown to HTML"))
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func (h *ExportHandler) failExport(c *gin.Context, requestID string, format string, category string, startedAt time.Time, appErr *errors.AppError) {
	details := gin.H{
		"request_id":  requestID,
		"category":    category,
		"format":      format,
		"duration_ms": time.Since(startedAt).Milliseconds(),
	}
	if existing, ok := appErr.Details.(gin.H); ok {
		for key, value := range existing {
			details[key] = value
		}
	}
	appErr.Details = details
	logger.Warnf(c.Request.Context(), "[Export] request_id=%s failed format=%s category=%s duration_ms=%d message=%s", requestID, format, category, details["duration_ms"], appErr.Message)
	c.Error(appErr)
}

func classifyExportFailureCategory(err error) string {
	if stdErrors.Is(err, context.DeadlineExceeded) {
		return exportFailureTimeout
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not installed") || strings.Contains(message, "executable not found") {
		return exportFailureCapabilityUnavailable
	}

	return exportFailureRender
}

func buildExportAppError(err error, requestID string, format string, policy export.Policy, category string, startedAt time.Time) *errors.AppError {
	details := gin.H{
		"request_id":        requestID,
		"category":          category,
		"format":            format,
		"engine":            policy.Engine,
		"duration_ms":       time.Since(startedAt).Milliseconds(),
		"timeout_seconds":   int(policy.Timeout / time.Second),
		"max_content_bytes": policy.MaxContentBytes,
	}

	switch category {
	case exportFailureTimeout:
		return errors.NewTimeoutError(fmt.Sprintf("%s export timed out", strings.ToUpper(format))).WithDetails(details)
	case exportFailureCapabilityUnavailable:
		details["reason"] = err.Error()
		return errors.NewServiceUnavailableError(fmt.Sprintf("%s export is unavailable", strings.ToUpper(format))).WithDetails(details)
	default:
		details["reason"] = err.Error()
		return errors.NewInternalServerError(fmt.Sprintf("%s export failed", strings.ToUpper(format))).WithDetails(details)
	}
}

// ExportHTMLRequest for swagger doc
type ExportHTMLRequest struct {
	Content string `json:"content" binding:"required"`
}

// ExportMarkdownToPDFDirect provides an alternative endpoint that accepts markdown
// and returns PDF using backend rendering (goldmark → styled HTML → Chromium print-to-PDF)
func (h *ExportHandler) ExportPDFDirect(c *gin.Context) {
	ctx := c.Request.Context()

	// Read raw body as markdown
	body, err := c.GetRawData()
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		c.Error(errors.NewBadRequestError("Request body (markdown content) is required"))
		return
	}

	data, err := export.MarkdownToPDF(ctx, string(body))
	if err != nil {
		logger.Errorf(ctx, "[Export] PDF generation failed: %v", err)
		c.Error(errors.NewInternalServerError(fmt.Sprintf("PDF generation failed: %v", err)))
		return
	}

	timestamp := time.Now().Format("2006-01-02_150405")
	filename := fmt.Sprintf("export_%s.pdf", timestamp)

	c.Header("Content-Disposition", buildAttachmentContentDisposition(filename))
	c.Data(http.StatusOK, "application/pdf", data)
}

func buildAttachmentContentDisposition(filename string) string {
	encodedFilename := url.PathEscape(filename)
	if encodedFilename == "" {
		encodedFilename = "export"
	}

	fallbackName := asciiFallbackFilename(filename)
	if fallbackName == "" {
		fallbackName = "export"
	}

	mediaType := mime.FormatMediaType("attachment", map[string]string{"filename": fallbackName})
	if mediaType == "" {
		mediaType = fmt.Sprintf(`attachment; filename="%s"`, fallbackName)
	}

	return fmt.Sprintf("%s; filename*=UTF-8''%s", mediaType, encodedFilename)
}

func asciiFallbackFilename(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	var builder strings.Builder
	lastUnderscore := false

	for _, char := range base {
		switch {
		case char <= unicode.MaxASCII && (unicode.IsLetter(char) || unicode.IsDigit(char)):
			builder.WriteRune(char)
			lastUnderscore = false
		case char == '-' || char == '_' || char == '.':
			builder.WriteRune(char)
			lastUnderscore = false
		case unicode.IsSpace(char) || char > unicode.MaxASCII:
			if !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}

	trimmedBase := strings.Trim(builder.String(), "._-")
	if trimmedBase == "" {
		trimmedBase = "export"
	} else if trimmedBase[0] >= '0' && trimmedBase[0] <= '9' {
		trimmedBase = "export_" + trimmedBase
	}

	trimmedExt := ext
	if trimmedExt == "" {
		return trimmedBase
	}
	return trimmedBase + trimmedExt
}
