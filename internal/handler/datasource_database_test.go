package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHandlerDataSourceService struct {
	interfaces.DataSourceService
	byID       map[string]*types.DataSource
	createErr  error
	deletedIDs []string
}

func (s *stubHandlerDataSourceService) GetDataSource(_ context.Context, id string) (*types.DataSource, error) {
	if ds, ok := s.byID[id]; ok {
		return ds, nil
	}
	return nil, assert.AnError
}

func (s *stubHandlerDataSourceService) CreateDataSource(_ context.Context, ds *types.DataSource) (*types.DataSource, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.byID == nil {
		s.byID = make(map[string]*types.DataSource)
	}
	clone := *ds
	if clone.ID == "" {
		clone.ID = "created-ds"
	}
	if clone.Type == "" {
		clone.Type = types.DatabaseTypeMySQL
	}
	if clone.Config == nil {
		cfg := &types.DatabaseConnectionConfig{
			Type:        clone.Type,
			Credentials: types.DatabaseCredentials{Username: "readonly", Password: "secret"},
			Settings:    types.DatabaseSourceSettings{Host: "127.0.0.1", Port: 3306, Database: "crm"},
		}
		if err := clone.SetDatabaseConnectionConfig(cfg); err != nil {
			return nil, err
		}
	}
	if clone.KnowledgeBaseID == "" {
		clone.KnowledgeBaseID = ds.KnowledgeBaseID
	}
	s.byID[clone.ID] = &clone
	return &clone, nil
}

func (s *stubHandlerDataSourceService) DeleteDataSource(_ context.Context, id string) error {
	s.deletedIDs = append(s.deletedIDs, id)
	delete(s.byID, id)
	return nil
}

type stubHandlerKnowledgeBaseService struct {
	interfaces.KnowledgeBaseService
	byID       map[string]*types.KnowledgeBase
	deletedIDs []string
}

func (s *stubHandlerKnowledgeBaseService) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	if s.byID == nil {
		s.byID = make(map[string]*types.KnowledgeBase)
	}
	clone := *kb
	if clone.ID == "" {
		clone.ID = "created-kb"
	}
	if clone.TenantID == 0 {
		clone.TenantID = 9
	}
	s.byID[clone.ID] = &clone
	return &clone, nil
}

func (s *stubHandlerKnowledgeBaseService) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if kb, ok := s.byID[id]; ok {
		return kb, nil
	}
	return nil, assert.AnError
}

func (s *stubHandlerKnowledgeBaseService) DeleteKnowledgeBase(_ context.Context, id string) error {
	s.deletedIDs = append(s.deletedIDs, id)
	delete(s.byID, id)
	return nil
}

type stubHandlerKBVisibilityService struct {
	interfaces.KBVisibilityService
	canAccess bool
	canManage bool
}

func (s *stubHandlerKBVisibilityService) CanAccessKB(_ context.Context, _ string, _ uint64, _ string, _ bool) (bool, error) {
	return s.canAccess, nil
}

func (s *stubHandlerKBVisibilityService) CanManageKB(_ context.Context, _ string, _ uint64, _ string, _ bool) (bool, error) {
	return s.canManage, nil
}

type stubHandlerSchemaRegistryService struct {
	interfaces.SchemaRegistryService
	schema        *types.DatabaseSchema
	err           error
	refreshDataID string
	getKBID       string
}

func (s *stubHandlerSchemaRegistryService) RefreshSchema(_ context.Context, dataSourceID string) error {
	s.refreshDataID = dataSourceID
	return s.err
}

func (s *stubHandlerSchemaRegistryService) GetDatabaseSchema(_ context.Context, kbID string) (*types.DatabaseSchema, error) {
	s.getKBID = kbID
	return s.schema, s.err
}

type stubHandlerDatabaseQueryAuditRepo struct {
	interfaces.DatabaseQueryAuditRepository
	items       []*types.DatabaseQueryAuditLog
	total       int64
	listTenant  uint64
	listKBID    string
	listLimit   int
	listOffset  int
	countTenant uint64
	countKBID   string
}

func (s *stubHandlerDatabaseQueryAuditRepo) ListByTenant(_ context.Context, tenantID uint64, kbID string, limit int, offset int) ([]*types.DatabaseQueryAuditLog, error) {
	s.listTenant = tenantID
	s.listKBID = kbID
	s.listLimit = limit
	s.listOffset = offset
	return s.items, nil
}

func (s *stubHandlerDatabaseQueryAuditRepo) CountByTenant(_ context.Context, tenantID uint64, kbID string) (int64, error) {
	s.countTenant = tenantID
	s.countKBID = kbID
	return s.total, nil
}

func TestDataSourceHandlerRefreshSchemaReturnsLatestSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	schemaSvc := &stubHandlerSchemaRegistryService{schema: &types.DatabaseSchema{KnowledgeBaseID: "kb-1", DatabaseName: "crm"}}
	h := NewDataSourceHandler(
		&stubHandlerDataSourceService{byID: map[string]*types.DataSource{"ds-1": {ID: "ds-1", KnowledgeBaseID: "kb-1"}}},
		&stubHandlerKnowledgeBaseService{byID: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 9, Type: types.KnowledgeBaseTypeDatabase}}},
		&stubHandlerKBVisibilityService{canManage: true},
		schemaSvc,
		&stubHandlerDatabaseQueryAuditRepo{},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/datasource/ds-1/refresh-schema", nil).WithContext(ctx)
	c.Params = gin.Params{{Key: "id", Value: "ds-1"}}
	c.Set(types.TenantIDContextKey.String(), uint64(9))
	c.Set(types.UserContextKey.String(), &types.User{ID: "user-1", IsSuperAdmin: false})

	h.RefreshSchema(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "ds-1", schemaSvc.refreshDataID)
	assert.Equal(t, "kb-1", schemaSvc.getKBID)
	var body types.DatabaseSchema
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "crm", body.DatabaseName)
}

func TestDataSourceHandlerGetDatabaseSchemaRejectsForeignTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDataSourceHandler(
		&stubHandlerDataSourceService{},
		&stubHandlerKnowledgeBaseService{byID: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 11, Type: types.KnowledgeBaseTypeDatabase}}},
		&stubHandlerKBVisibilityService{canAccess: true},
		&stubHandlerSchemaRegistryService{},
		&stubHandlerDatabaseQueryAuditRepo{},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-1/database-schema", nil).WithContext(ctx)
	c.Params = gin.Params{{Key: "id", Value: "kb-1"}}
	c.Set(types.TenantIDContextKey.String(), uint64(9))
	c.Set(types.UserContextKey.String(), &types.User{ID: "user-1", IsSuperAdmin: false})

	h.GetDatabaseSchema(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestDataSourceHandlerGetDatabaseSchemaAllowsReadableKnowledgeBase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	schemaSvc := &stubHandlerSchemaRegistryService{schema: &types.DatabaseSchema{KnowledgeBaseID: "kb-1", DatabaseName: "crm"}}
	h := NewDataSourceHandler(
		&stubHandlerDataSourceService{},
		&stubHandlerKnowledgeBaseService{byID: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 9, Type: types.KnowledgeBaseTypeDatabase, CreatedBy: "owner-1"}}},
		&stubHandlerKBVisibilityService{canAccess: true},
		schemaSvc,
		&stubHandlerDatabaseQueryAuditRepo{},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-1/database-schema", nil).WithContext(ctx)
	c.Params = gin.Params{{Key: "id", Value: "kb-1"}}
	c.Set(types.TenantIDContextKey.String(), uint64(9))
	c.Set(types.UserContextKey.String(), &types.User{ID: "reader-1", IsSuperAdmin: false})

	h.GetDatabaseSchema(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "kb-1", schemaSvc.getKBID)
	var body types.DatabaseSchema
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "crm", body.DatabaseName)
}

func TestDataSourceHandlerListDatabaseQueryAuditsSupportsPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auditRepo := &stubHandlerDatabaseQueryAuditRepo{
		items: []*types.DatabaseQueryAuditLog{{ID: "audit-1", TenantID: 9, KnowledgeBaseID: "kb-1", Status: types.DatabaseQueryAuditStatusSuccess}},
		total: 7,
	}
	h := NewDataSourceHandler(
		&stubHandlerDataSourceService{},
		&stubHandlerKnowledgeBaseService{byID: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 9, Type: types.KnowledgeBaseTypeDatabase}}},
		&stubHandlerKBVisibilityService{canAccess: true},
		&stubHandlerSchemaRegistryService{},
		auditRepo,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/database-query-audits?knowledge_base_id=kb-1&limit=5&offset=10", nil).WithContext(ctx)
	c.Set(types.TenantIDContextKey.String(), uint64(9))
	c.Set(types.UserContextKey.String(), &types.User{ID: "user-1", IsSuperAdmin: false})

	h.ListDatabaseQueryAudits(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, uint64(9), auditRepo.listTenant)
	assert.Equal(t, "kb-1", auditRepo.listKBID)
	assert.Equal(t, 5, auditRepo.listLimit)
	assert.Equal(t, 10, auditRepo.listOffset)
	assert.Equal(t, uint64(9), auditRepo.countTenant)

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.EqualValues(t, 7, body["total"])
	assert.EqualValues(t, 5, body["limit"])
	assert.EqualValues(t, 10, body["offset"])
	items, ok := body["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
}

func TestDataSourceHandlerGetDataSourceMasksDatabasePassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ds := &types.DataSource{ID: "ds-1", KnowledgeBaseID: "kb-1", Type: types.DatabaseTypeMySQL}
	require.NoError(t, ds.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type:        types.DatabaseTypeMySQL,
		Credentials: types.DatabaseCredentials{Username: "readonly", Password: "super-secret"},
		Settings:    types.DatabaseSourceSettings{Host: "127.0.0.1", Port: 3306, Database: "crm"},
	}))
	h := NewDataSourceHandler(
		&stubHandlerDataSourceService{byID: map[string]*types.DataSource{"ds-1": ds}},
		&stubHandlerKnowledgeBaseService{byID: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 9, Type: types.KnowledgeBaseTypeDatabase}}},
		&stubHandlerKBVisibilityService{canManage: true},
		&stubHandlerSchemaRegistryService{},
		&stubHandlerDatabaseQueryAuditRepo{},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/datasource/ds-1", nil).WithContext(ctx)
	c.Params = gin.Params{{Key: "id", Value: "ds-1"}}
	c.Set(types.TenantIDContextKey.String(), uint64(9))
	c.Set(types.UserContextKey.String(), &types.User{ID: "user-1", IsSuperAdmin: false})

	h.GetDataSource(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body types.DataSource
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	cfg, err := body.ParseDatabaseConnectionConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "***", cfg.Credentials.Password)
	assert.Equal(t, "readonly", cfg.Credentials.Username)
}

func TestKnowledgeBaseHandlerCreateDatabaseKnowledgeBaseCreatesDataSourceAndSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsSvc := &stubHandlerDataSourceService{}
	kbSvc := &stubHandlerKnowledgeBaseService{}
	schemaSvc := &stubHandlerSchemaRegistryService{}
	h := NewKnowledgeBaseHandler(
		kbSvc,
		dsSvc,
		nil,
		nil,
		nil,
		nil,
		schemaSvc,
		nil,
	)

	body := `{
		"name":"订单数据库",
		"type":"database",
		"summary_model_id":"llm-1",
		"database_config":{
			"data_source_name":"订单 MySQL",
			"connection":{
				"type":"mysql",
				"credentials":{"username":"readonly","password":"secret"},
				"settings":{"host":"127.0.0.1","port":3306,"database":"crm"}
			}
		}
	}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases", strings.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(types.TenantIDContextKey.String(), uint64(9))
	c.Set(types.UserIDContextKey.String(), "user-1")

	h.CreateKnowledgeBase(c)

	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Contains(t, kbSvc.byID, "created-kb")
	require.Contains(t, dsSvc.byID, "created-ds")
	assert.Equal(t, "created-ds", schemaSvc.refreshDataID)
	assert.Empty(t, kbSvc.deletedIDs)
	assert.Empty(t, dsSvc.deletedIDs)

	createdDS := dsSvc.byID["created-ds"]
	require.NotNil(t, createdDS)
	cfg, err := createdDS.ParseDatabaseConnectionConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, types.DatabaseTypeMySQL, cfg.Type)
	assert.Equal(t, "readonly", cfg.Credentials.Username)
	assert.Equal(t, "crm", cfg.Settings.Database)
	assert.Equal(t, "订单 MySQL", createdDS.Name)
	assert.Equal(t, "created-kb", createdDS.KnowledgeBaseID)
	assert.Equal(t, uint64(9), createdDS.TenantID)
	assert.Equal(t, "user-1", kbSvc.byID["created-kb"].CreatedBy)
}

func TestKnowledgeBaseHandlerCreateDatabaseKnowledgeBaseRequiresConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKnowledgeBaseHandler(
		&stubHandlerKnowledgeBaseService{},
		&stubHandlerDataSourceService{},
		nil,
		nil,
		nil,
		nil,
		&stubHandlerSchemaRegistryService{},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases", strings.NewReader(`{"name":"DB KB","type":"database"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(types.TenantIDContextKey.String(), uint64(9))

	h.CreateKnowledgeBase(c)

	require.Len(t, c.Errors, 1)
	assert.Contains(t, c.Errors.Last().Error(), "database_config is required")
}

func TestKnowledgeBaseHandlerCreateDatabaseKnowledgeBaseRollsBackOnDatasourceFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsSvc := &stubHandlerDataSourceService{createErr: assert.AnError}
	kbSvc := &stubHandlerKnowledgeBaseService{}
	h := NewKnowledgeBaseHandler(
		kbSvc,
		dsSvc,
		nil,
		nil,
		nil,
		nil,
		&stubHandlerSchemaRegistryService{},
		nil,
	)

	body := `{
		"name":"订单数据库",
		"type":"database",
		"database_config":{
			"connection":{
				"type":"mysql",
				"credentials":{"username":"readonly","password":"secret"},
				"settings":{"host":"127.0.0.1","port":3306,"database":"crm"}
			}
		}
	}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases", strings.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(types.TenantIDContextKey.String(), uint64(9))

	h.CreateKnowledgeBase(c)

	require.Len(t, c.Errors, 1)
	assert.Contains(t, c.Errors.Last().Error(), assert.AnError.Error())
	assert.Equal(t, []string{"created-kb"}, kbSvc.deletedIDs)
	assert.Empty(t, dsSvc.deletedIDs)
}
