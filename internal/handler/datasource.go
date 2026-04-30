package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// DataSourceHandler handles HTTP requests for data source management
type DataSourceHandler struct {
	service   interfaces.DataSourceService
	kbService interfaces.KnowledgeBaseService
	kbVisible interfaces.KBVisibilityService
	schemaSvc interfaces.SchemaRegistryService
	auditRepo interfaces.DatabaseQueryAuditRepository
}

// NewDataSourceHandler creates a new data source handler
func NewDataSourceHandler(
	service interfaces.DataSourceService,
	kbService interfaces.KnowledgeBaseService,
	kbVisible interfaces.KBVisibilityService,
	schemaSvc interfaces.SchemaRegistryService,
	auditRepo interfaces.DatabaseQueryAuditRepository,
) *DataSourceHandler {
	return &DataSourceHandler{
		service:   service,
		kbService: kbService,
		kbVisible: kbVisible,
		schemaSvc: schemaSvc,
		auditRepo: auditRepo,
	}
}

func (h *DataSourceHandler) databaseSchemaErrorStatus(err error) (int, string) {
	switch err {
	case nil:
		return http.StatusOK, ""
	case service.ErrDatabaseDataSourceNotFound:
		return http.StatusNotFound, err.Error()
	case service.ErrDatabaseKnowledgeBaseRequired:
		return http.StatusBadRequest, err.Error()
	case service.ErrDatabaseSchemaSnapshotNotFound:
		return http.StatusNotFound, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}

// getTenantID safely extracts and validates tenant ID from context
// Returns 0 if tenant ID is not found (caller should return 401)
func (h *DataSourceHandler) getTenantID(c *gin.Context) uint64 {
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	return tenantID
}

func (h *DataSourceHandler) getCurrentUser(c *gin.Context) (string, bool) {
	if userVal, ok := c.Get(types.UserContextKey.String()); ok {
		if user, ok := userVal.(*types.User); ok && user != nil {
			return user.ID, user.IsSuperAdmin
		}
	}
	if userID := c.GetString(types.UserIDContextKey.String()); userID != "" {
		return userID, false
	}
	return "", false
}

func (h *DataSourceHandler) getAuthorizedKnowledgeBase(
	ctx context.Context,
	c *gin.Context,
	kbID string,
	requireManage bool,
) (*types.KnowledgeBase, int, string) {
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		return nil, http.StatusUnauthorized, "unauthorized"
	}
	if kbID == "" {
		return nil, http.StatusBadRequest, "kb_id is required"
	}

	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil || kb == nil {
		return nil, http.StatusNotFound, "knowledge base not found"
	}

	if kb.TenantID != tenantID {
		return nil, http.StatusForbidden, "access denied"
	}

	if h.kbVisible != nil {
		userID, isSuperAdmin := h.getCurrentUser(c)
		if userID != "" || isSuperAdmin {
			var allowed bool
			var permErr error
			if requireManage {
				allowed, permErr = h.kbVisible.CanManageKB(ctx, userID, tenantID, kbID, isSuperAdmin)
			} else {
				allowed, permErr = h.kbVisible.CanAccessKB(ctx, userID, tenantID, kbID, isSuperAdmin)
			}
			if permErr != nil || !allowed {
				return nil, http.StatusForbidden, "access denied"
			}
		}
	}

	return kb, http.StatusOK, ""
}

// Data source settings contain connector credentials, so management operations
// require KB management permission rather than read-only visibility.
func (h *DataSourceHandler) getManagedKnowledgeBase(ctx context.Context, c *gin.Context, kbID string) (*types.KnowledgeBase, int, string) {
	return h.getAuthorizedKnowledgeBase(ctx, c, kbID, true)
}

func (h *DataSourceHandler) getReadableKnowledgeBase(ctx context.Context, c *gin.Context, kbID string) (*types.KnowledgeBase, int, string) {
	return h.getAuthorizedKnowledgeBase(ctx, c, kbID, false)
}

func (h *DataSourceHandler) getManagedDataSource(
	ctx context.Context,
	c *gin.Context,
	id string,
) (*types.DataSource, int, string) {
	ds, err := h.service.GetDataSource(ctx, id)
	if err != nil {
		return nil, http.StatusNotFound, "data source not found"
	}

	if _, status, msg := h.getManagedKnowledgeBase(ctx, c, ds.KnowledgeBaseID); status != http.StatusOK {
		return nil, status, msg
	}

	return ds, http.StatusOK, ""
}

func (h *DataSourceHandler) sanitizeDataSource(ds *types.DataSource) *types.DataSource {
	if ds == nil {
		return nil
	}
	clone := *ds
	if cfg, err := clone.ParseDatabaseConnectionConfig(); err == nil && cfg != nil {
		masked := cfg.MaskSensitiveFields()
		if err := clone.SetDatabaseConnectionConfig(&masked); err == nil {
			return &clone
		}
	}
	return &clone
}

func (h *DataSourceHandler) sanitizeDataSources(items []*types.DataSource) []*types.DataSource {
	if items == nil {
		return nil
	}
	out := make([]*types.DataSource, 0, len(items))
	for _, item := range items {
		out = append(out, h.sanitizeDataSource(item))
	}
	return out
}

// CreateDataSource godoc
// @Summary Create a new data source
// @Description Create a new data source configuration for a knowledge base
// @Tags DataSource
// @Accept json
// @Produce json
// @Param request body types.DataSource true "Data source configuration"
// @Success 201 {object} types.DataSource
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource [post]
func (h *DataSourceHandler) CreateDataSource(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from context (set by auth middleware)
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: tenant context missing"})
		return
	}

	var req types.DataSource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if _, status, msg := h.getManagedKnowledgeBase(ctx, c, req.KnowledgeBaseID); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	// Enforce tenant isolation
	req.TenantID = tenantID

	ds, err := h.service.CreateDataSource(ctx, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, h.sanitizeDataSource(ds))
}

// GetDataSource godoc
// @Summary Get a data source by ID
// @Description Retrieve a data source configuration by ID
// @Tags DataSource
// @Produce json
// @Param id path string true "Data source ID"
// @Success 200 {object} types.DataSource
// @Failure 404 {object} map[string]string
// @Router /api/v1/datasource/{id} [get]
func (h *DataSourceHandler) GetDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	ds, status, msg := h.getManagedDataSource(ctx, c, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, h.sanitizeDataSource(ds))
}

// ListDataSources godoc
// @Summary List data sources for a knowledge base
// @Description List all data sources for a specific knowledge base
// @Tags DataSource
// @Produce json
// @Param kb_id query string true "Knowledge base ID"
// @Success 200 {object} []types.DataSource
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource [get]
func (h *DataSourceHandler) ListDataSources(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from context
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: tenant context missing"})
		return
	}

	kbID := c.Query("kb_id")
	if _, status, msg := h.getManagedKnowledgeBase(ctx, c, kbID); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	dataSources, err := h.service.ListDataSources(ctx, kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list data sources"})
		return
	}

	if dataSources == nil {
		dataSources = make([]*types.DataSource, 0)
	}
	c.JSON(http.StatusOK, h.sanitizeDataSources(dataSources))
}

// UpdateDataSource godoc
// @Summary Update a data source
// @Description Update an existing data source configuration
// @Tags DataSource
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param request body types.DataSource true "Updated configuration"
// @Success 200 {object} types.DataSource
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/{id} [put]
func (h *DataSourceHandler) UpdateDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	var req types.DataSource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	existing, status, msg := h.getManagedDataSource(ctx, c, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	req.ID = id
	req.TenantID = existing.TenantID
	req.KnowledgeBaseID = existing.KnowledgeBaseID
	ds, err := h.service.UpdateDataSource(ctx, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, h.sanitizeDataSource(ds))
}

// DeleteDataSource godoc
// @Summary Delete a data source
// @Description Delete a data source (soft delete)
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 204
// @Failure 404 {object} map[string]string
// @Router /api/v1/datasource/{id} [delete]
func (h *DataSourceHandler) DeleteDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getManagedDataSource(ctx, c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.DeleteDataSource(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete data source"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ValidateConnection godoc
// @Summary Test data source connection
// @Description Validate the connection to an external data source
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/{id}/validate [post]
func (h *DataSourceHandler) ValidateConnection(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getManagedDataSource(ctx, c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.ValidateConnection(ctx, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "connected"})
}

// RefreshSchema godoc
// @Summary Refresh database schema snapshot
// @Description Discover and persist the latest schema for a database data source
// @Tags DataSource
// @Produce json
// @Param id path string true "Data source ID"
// @Success 200 {object} types.DatabaseSchema
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/v1/datasource/{id}/refresh-schema [post]
func (h *DataSourceHandler) RefreshSchema(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	ds, status, msg := h.getManagedDataSource(ctx, c, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.schemaSvc.RefreshSchema(ctx, id); err != nil {
		status, msg := h.databaseSchemaErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}

	schema, err := h.schemaSvc.GetDatabaseSchema(ctx, ds.KnowledgeBaseID)
	if err != nil {
		status, msg := h.databaseSchemaErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, schema)
}

// GetDatabaseSchema godoc
// @Summary Get database schema snapshot
// @Description Retrieve the latest persisted schema for a database knowledge base
// @Tags DataSource
// @Produce json
// @Param id path string true "Knowledge base ID"
// @Success 200 {object} types.DatabaseSchema
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/v1/knowledge-bases/{id}/database-schema [get]
func (h *DataSourceHandler) GetDatabaseSchema(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	kbID := c.Param("id")
	if _, status, msg := h.getReadableKnowledgeBase(ctx, c, kbID); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	schema, err := h.schemaSvc.GetDatabaseSchema(ctx, kbID)
	if err != nil {
		status, msg := h.databaseSchemaErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, schema)
}

// ListDatabaseQueryAudits godoc
// @Summary List external database query audits
// @Description List tenant-scoped audit logs for external database queries with pagination
// @Tags DataSource
// @Produce json
// @Param knowledge_base_id query string false "Knowledge base ID"
// @Param limit query int false "Limit (default: 20)"
// @Param offset query int false "Offset (default: 0)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Router /api/v1/database-query-audits [get]
func (h *DataSourceHandler) ListDatabaseQueryAudits(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	kbID := c.Query("knowledge_base_id")
	if kbID == "" {
		kbID = c.Query("kb_id")
	}
	if kbID != "" {
		if _, status, msg := h.getReadableKnowledgeBase(ctx, c, kbID); status != http.StatusOK {
			c.JSON(status, gin.H{"error": msg})
			return
		}
	}

	limit := 20
	offset := 0
	if v := c.Query("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		limit = parsed
	}
	if v := c.Query("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
			return
		}
		offset = parsed
	}

	items, err := h.auditRepo.ListByTenant(ctx, tenantID, kbID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	total, err := h.auditRepo.CountByTenant(ctx, tenantID, kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = make([]*types.DatabaseQueryAuditLog, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ValidateCredentials godoc
// @Summary Test connection with raw credentials (no persistence)
// @Description Validate connectivity to an external data source using type + credentials
//
//	without creating or updating any database records.
//	Used by the frontend "Test Connection" button during data source creation.
//
// @Tags DataSource
// @Accept json
// @Produce json
// @Param request body object true "type and credentials"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/validate-credentials [post]
func (h *DataSourceHandler) ValidateCredentials(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		Type        string                 `json:"type" binding:"required"`
		Credentials map[string]interface{} `json:"credentials" binding:"required"`
		Settings    map[string]interface{} `json:"settings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: type and credentials are required"})
		return
	}

	if err := h.service.ValidateCredentials(ctx, req.Type, req.Credentials, req.Settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "connected"})
}

// @Summary List available resources in data source
// @Description List resources available for sync in the external system
// @Tags DataSource
// @Produce json
// @Param id path string true "Data source ID"
// @Success 200 {object} []types.Resource
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/{id}/resources [get]
func (h *DataSourceHandler) ListAvailableResources(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getManagedDataSource(ctx, c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	resources, err := h.service.ListAvailableResources(ctx, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if resources == nil {
		resources = make([]types.Resource, 0)
	}
	c.JSON(http.StatusOK, resources)
}

// ManualSync godoc
// @Summary Trigger immediate sync
// @Description Trigger an immediate sync for a data source
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} types.SyncLog
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/{id}/sync [post]
func (h *DataSourceHandler) ManualSync(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getManagedDataSource(ctx, c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	syncLog, err := h.service.ManualSync(ctx, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, syncLog)
}

// PauseDataSource godoc
// @Summary Pause data source
// @Description Pause a data source's scheduled syncs
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/{id}/pause [post]
func (h *DataSourceHandler) PauseDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getManagedDataSource(ctx, c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.PauseDataSource(ctx, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "paused"})
}

// ResumeDataSource godoc
// @Summary Resume data source
// @Description Resume a paused data source
// @Tags DataSource
// @Param id path string true "Data source ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/{id}/resume [post]
func (h *DataSourceHandler) ResumeDataSource(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getManagedDataSource(ctx, c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if err := h.service.ResumeDataSource(ctx, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "active"})
}

// GetSyncLogs godoc
// @Summary Get sync logs
// @Description Retrieve sync history for a data source
// @Tags DataSource
// @Produce json
// @Param id path string true "Data source ID"
// @Param limit query int false "Limit (default: 10)"
// @Param offset query int false "Offset (default: 0)"
// @Success 200 {object} []types.SyncLog
// @Failure 400 {object} map[string]string
// @Router /api/v1/datasource/{id}/logs [get]
func (h *DataSourceHandler) GetSyncLogs(c *gin.Context) {
	ctx := c.Request.Context()
	if h.getTenantID(c) == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")

	if _, status, msg := h.getManagedDataSource(ctx, c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	limit := 10
	offset := 0

	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	logs, err := h.service.GetSyncLogs(ctx, id, limit, offset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if logs == nil {
		logs = make([]*types.SyncLog, 0)
	}
	c.JSON(http.StatusOK, logs)
}

// GetSyncLog godoc
// @Summary Get specific sync log
// @Description Retrieve a specific sync log entry
// @Tags DataSource
// @Produce json
// @Param log_id path string true "Sync log ID"
// @Success 200 {object} types.SyncLog
// @Failure 404 {object} map[string]string
// @Router /api/v1/datasource/logs/{log_id} [get]
func (h *DataSourceHandler) GetSyncLog(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	logID := c.Param("log_id")

	log, err := h.service.GetSyncLog(ctx, logID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync log not found"})
		return
	}

	if _, status, msg := h.getManagedDataSource(ctx, c, log.DataSourceID); status != http.StatusOK {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, log)
}

// GetAvailableConnectors godoc
// @Summary Get available connectors
// @Description Get list of available data source connectors
// @Tags DataSource
// @Produce json
// @Success 200 {object} []datasource.ConnectorMetadata
// @Router /api/v1/datasource/types [get]
func (h *DataSourceHandler) GetAvailableConnectors(c *gin.Context) {
	connectors := datasource.ListAvailableConnectors()
	c.JSON(http.StatusOK, connectors)
}
