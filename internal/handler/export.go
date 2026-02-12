package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/utils/export"
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
	// Export format: "pdf" or "docx"
	Format string `json:"format" binding:"required,oneof=pdf docx"`
	// Optional filename prefix
	FilenamePrefix string `json:"filename_prefix,omitempty"`
}

// ExportDocument godoc
// @Summary      导出文档
// @Description  将 Markdown 内容导出为 PDF 或 Word (DOCX) 文件
// @Tags         导出
// @Accept       json
// @Produce      application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document
// @Param        body  body      ExportDocumentRequest  true  "导出请求"
// @Success      200   {file}    binary                 "导出的文件"
// @Failure      400   {object}  errors.AppError        "请求参数错误"
// @Failure      500   {object}  errors.AppError        "导出失败"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /export/document [post]
func (h *ExportHandler) ExportDocument(c *gin.Context) {
	ctx := c.Request.Context()

	var req ExportDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warnf(ctx, "[Export] Invalid request: %v", err)
		c.Error(errors.NewBadRequestError("Invalid request: content and format (pdf/docx) are required"))
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		c.Error(errors.NewBadRequestError("Content cannot be empty"))
		return
	}

	// Generate filename
	prefix := req.FilenamePrefix
	if prefix == "" {
		prefix = "对话导出"
	}
	timestamp := time.Now().Format("2006-01-02_150405")
	filename := fmt.Sprintf("%s_%s", prefix, timestamp)

	logger.Infof(ctx, "[Export] Exporting document as %s, content length: %d", req.Format, len(req.Content))

	var (
		data        []byte
		contentType string
		ext         string
		err         error
	)

	switch req.Format {
	case "pdf":
		data, err = export.MarkdownToPDF(ctx, req.Content)
		contentType = "application/pdf"
		ext = "pdf"
	case "docx":
		data, err = export.MarkdownToDocx(ctx, req.Content)
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		ext = "docx"
	default:
		c.Error(errors.NewBadRequestError("Unsupported format: " + req.Format))
		return
	}

	if err != nil {
		logger.Errorf(ctx, "[Export] Failed to export as %s: %v", req.Format, err)
		c.Error(errors.NewInternalServerError(fmt.Sprintf("Export failed: %v", err)))
		return
	}

	logger.Infof(ctx, "[Export] Successfully exported %s, size: %d bytes", req.Format, len(data))

	// Set response headers for file download
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, filename, ext))
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", len(data)))

	c.Data(http.StatusOK, contentType, data)
}

// ExportCapabilities godoc
// @Summary      查询导出能力
// @Description  返回后端支持的导出格式（检查 wkhtmltopdf/pandoc 是否安装）
// @Tags         导出
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "导出能力信息"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /export/capabilities [get]
func (h *ExportHandler) ExportCapabilities(c *gin.Context) {
	ctx := c.Request.Context()

	pdfAvailable := export.IsWkhtmltopdfAvailable()
	docxAvailable := export.IsPandocAvailable()

	logger.Infof(ctx, "[Export] Capabilities check - PDF: %v, DOCX: %v", pdfAvailable, docxAvailable)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"pdf": gin.H{
				"available": pdfAvailable,
				"tool":      "wkhtmltopdf",
			},
			"docx": gin.H{
				"available": docxAvailable,
				"tool":      "pandoc",
			},
		},
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

// ExportHTMLRequest for swagger doc
type ExportHTMLRequest struct {
	Content string `json:"content" binding:"required"`
}

// ExportMarkdownToPDFDirect provides an alternative endpoint that accepts markdown
// and returns PDF using pure Go rendering (goldmark → HTML → wkhtmltopdf)
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

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/pdf", data)
}
