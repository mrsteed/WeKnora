package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type stubVisibilityKBRepo struct {
	created *types.KnowledgeBase
	updated *types.KnowledgeBase
	stored  *types.KnowledgeBase
	tenantScoped *types.KnowledgeBase
}

func (s *stubVisibilityKBRepo) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) error {
	s.created = kb
	return nil
}
func (s *stubVisibilityKBRepo) GetKnowledgeBaseByID(_ context.Context, _ string) (*types.KnowledgeBase, error) {
	if s.stored == nil {
		return nil, repository.ErrKnowledgeBaseNotFound
	}
	clone := *s.stored
	return &clone, nil
}
func (s *stubVisibilityKBRepo) GetKnowledgeBaseByIDAndTenant(_ context.Context, _ string, _ uint64) (*types.KnowledgeBase, error) {
	if s.tenantScoped == nil {
		return nil, repository.ErrKnowledgeBaseNotFound
	}
	clone := *s.tenantScoped
	return &clone, nil
}
func (s *stubVisibilityKBRepo) GetKnowledgeBaseByIDs(_ context.Context, _ []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubVisibilityKBRepo) ListKnowledgeBases(_ context.Context) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubVisibilityKBRepo) ListKnowledgeBasesByTenantID(_ context.Context, _ uint64) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubVisibilityKBRepo) ListAccessibleKBs(_ context.Context, _ string, _ uint64, _ []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubVisibilityKBRepo) ListKBsByOrganization(_ context.Context, _ string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubVisibilityKBRepo) UpdateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) error {
	s.updated = kb
	return nil
}
func (s *stubVisibilityKBRepo) DeleteKnowledgeBase(_ context.Context, _ string) error { return nil }
func (s *stubVisibilityKBRepo) TogglePinKnowledgeBase(_ context.Context, _ string, _ uint64) (*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubVisibilityKBRepo) CountByVectorStoreID(_ context.Context, _ *gorm.DB, _ uint64, _ string) (int64, error) {
	return 0, nil
}
func (s *stubVisibilityKBRepo) CountByModelID(_ context.Context, _ uint64, _ string) (int64, error) {
	return 0, nil
}
func (s *stubVisibilityKBRepo) SetUserKBPin(_ context.Context, _ uint64, _ string, _ string, _ bool) (*time.Time, error) {
	return nil, nil
}
func (s *stubVisibilityKBRepo) ListUserKBPinIDs(_ context.Context, _ uint64, _ string) (map[string]time.Time, error) {
	return nil, nil
}

type stubVisibilityAgentRepo struct {
	created *types.CustomAgent
	updated *types.CustomAgent
	stored  *types.CustomAgent
}

func (s *stubVisibilityAgentRepo) CreateAgent(_ context.Context, agent *types.CustomAgent) error {
	s.created = agent
	return nil
}
func (s *stubVisibilityAgentRepo) GetAgentByID(_ context.Context, _ string, _ uint64) (*types.CustomAgent, error) {
	if s.stored == nil {
		return nil, repository.ErrCustomAgentNotFound
	}
	clone := *s.stored
	return &clone, nil
}
func (s *stubVisibilityAgentRepo) ListAgentsByTenantID(_ context.Context, _ uint64) ([]*types.CustomAgent, error) {
	return nil, nil
}
func (s *stubVisibilityAgentRepo) ListAccessibleAgents(_ context.Context, _ string, _ uint64, _ []string) ([]*types.CustomAgent, error) {
	return nil, nil
}
func (s *stubVisibilityAgentRepo) UpdateAgent(_ context.Context, agent *types.CustomAgent) error {
	s.updated = agent
	return nil
}
func (s *stubVisibilityAgentRepo) DeleteAgent(_ context.Context, _ string, _ uint64) error { return nil }
func (s *stubVisibilityAgentRepo) CountByModelID(_ context.Context, _ uint64, _ string) (int64, error) {
	return 0, nil
}

func TestCreateKnowledgeBase_ClearsOrganizationOutsideOrgVisibility(t *testing.T) {
	repo := &stubVisibilityKBRepo{}
	svc := &knowledgeBaseService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	kb, err := svc.CreateKnowledgeBase(ctx, &types.KnowledgeBase{
		Name:           "kb",
		Visibility:     types.KBVisibilityGlobal,
		OrganizationID: "org-stale",
	})
	require.NoError(t, err)
	require.NotNil(t, kb)
	assert.Equal(t, types.KBVisibilityGlobal, kb.Visibility)
	assert.Empty(t, kb.OrganizationID)
	require.NotNil(t, repo.created)
	assert.Empty(t, repo.created.OrganizationID)
}

func TestUpdateKnowledgeBase_ClearsOrganizationOutsideOrgVisibility(t *testing.T) {
	repo := &stubVisibilityKBRepo{stored: &types.KnowledgeBase{
		ID:             "kb-1",
		Name:           "before",
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "org-old",
	}}
	svc := &knowledgeBaseService{repo: repo}

	kb, err := svc.UpdateKnowledgeBase(context.Background(), "kb-1", "after", "", nil, types.KBVisibilityPrivate, "org-old")
	require.NoError(t, err)
	require.NotNil(t, kb)
	assert.Equal(t, types.KBVisibilityPrivate, kb.Visibility)
	assert.Empty(t, kb.OrganizationID)
	require.NotNil(t, repo.updated)
	assert.Empty(t, repo.updated.OrganizationID)
}

func TestCreateAgent_ClearsOrganizationOutsideOrgVisibility(t *testing.T) {
	repo := &stubVisibilityAgentRepo{}
	svc := &customAgentService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")

	agent, err := svc.CreateAgent(ctx, &types.CustomAgent{
		Name:           "agent",
		Visibility:     types.AgentVisibilityGlobal,
		OrganizationID: "org-stale",
	})
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, types.AgentVisibilityGlobal, agent.Visibility)
	assert.Empty(t, agent.OrganizationID)
	require.NotNil(t, repo.created)
	assert.Empty(t, repo.created.OrganizationID)
}

func TestUpdateAgent_ClearsOrganizationOutsideOrgVisibility(t *testing.T) {
	repo := &stubVisibilityAgentRepo{stored: &types.CustomAgent{
		ID:             "agent-1",
		Name:           "before",
		TenantID:       9,
		Visibility:     types.AgentVisibilityOrg,
		OrganizationID: "org-old",
	}}
	svc := &customAgentService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))

	agent, err := svc.UpdateAgent(ctx, &types.CustomAgent{
		ID:             "agent-1",
		Name:           "after",
		Visibility:     types.AgentVisibilityPrivate,
		OrganizationID: "org-old",
	})
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, types.AgentVisibilityPrivate, agent.Visibility)
	assert.Empty(t, agent.OrganizationID)
	require.NotNil(t, repo.updated)
	assert.Empty(t, repo.updated.OrganizationID)
}

func TestNormalizeCustomAgentVisibility_RequiresOrganizationForOrgScope(t *testing.T) {
	_, _, err := normalizeCustomAgentVisibility(types.AgentVisibilityOrg, "")
	require.Error(t, err)
	assert.EqualError(t, err, "organization_id is required when visibility is org")
}

func TestCanAccessKB_RequiresMatchingOrganizationScopeForOrgVisibleKB(t *testing.T) {
	repo := &stubVisibilityKBRepo{tenantScoped: &types.KnowledgeBase{
		ID:             "kb-org",
		TenantID:       7,
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "org-a",
		CreatedBy:      "owner-1",
	}}
	svc := NewKBVisibilityService(repo, &stubSameTenantOrgTreeService{
		userOrgs: []*types.OrgTreeNode{{ID: "org-a", Path: "/org-a", Level: 1, MyIsAdmin: false}},
	}, nil, nil, nil, nil)

	allowed, err := svc.CanAccessKB(context.Background(), "viewer-1", 7, "kb-org", false)
	require.NoError(t, err)
	assert.True(t, allowed, "org-visible KBs should be readable when organization_id falls within the user's readable org scope")
}

func TestCanAccessKB_DeniesOrgVisibleKBOutsideReadableOrganizations(t *testing.T) {
	repo := &stubVisibilityKBRepo{tenantScoped: &types.KnowledgeBase{
		ID:             "kb-org",
		TenantID:       7,
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "org-a",
		CreatedBy:      "owner-1",
	}}
	svc := NewKBVisibilityService(repo, &stubSameTenantOrgTreeService{
		userOrgs: []*types.OrgTreeNode{{ID: "org-b", Path: "/org-b", Level: 1, MyIsAdmin: false}},
	}, nil, nil, nil, nil)

	allowed, err := svc.CanAccessKB(context.Background(), "viewer-1", 7, "kb-org", false)
	require.NoError(t, err)
	assert.False(t, allowed, "org-visible KBs should be denied when organization_id is outside the user's readable org scope")
}