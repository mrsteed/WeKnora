package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBuiltinAgentSvc implements interfaces.CustomAgentService, returning a
// single fixed agent for GetAgentByID and no-ops for the rest.
type stubBuiltinAgentSvc struct {
	interfaces.CustomAgentService // embed: unimplemented members panic only if called
	stored                        *types.CustomAgent
}

func (s *stubBuiltinAgentSvc) GetAgentByID(_ context.Context, _ string) (*types.CustomAgent, error) {
	if s.stored == nil {
		return nil, repository.ErrCustomAgentNotFound
	}
	clone := *s.stored
	return &clone, nil
}

// withBuiltinAgent registers the given agent ID in the builtin registry for
// the duration of the test so that types.IsBuiltinAgentID reports true, and
// restores the original registry afterwards.
func withBuiltinAgent(t *testing.T, agentID string) {
	t.Helper()
	orig := types.BuiltinAgentRegistry
	t.Cleanup(func() { types.BuiltinAgentRegistry = orig })
	reg := make(map[string]func(uint64) *types.CustomAgent, len(orig)+1)
	for id, f := range orig {
		reg[id] = f
	}
	reg[agentID] = func(tid uint64) *types.CustomAgent {
		a := &types.CustomAgent{ID: agentID, IsBuiltin: true, TenantID: tid}
		a.EnsureDefaults()
		return a
	}
	types.BuiltinAgentRegistry = reg
}

// TestCanAccessAgent_BuiltinPrivateNoOwnerReadableByAnyTenantMember
// 回归测试（内置智能体推荐问题不显示修复）：
// updateBuiltinAgent 生成的内置智能体持久化行历史上是
// visibility='private' + created_by=”（无主私有）。私有规则要求
// CreatedBy 非空且等于请求者，因此该状态会把所有非特权用户挡在
// 详情/复制/推荐问题接口之外（1002）。内置智能体是租户级平台资源，
// 必须对同租户所有成员可读。
func TestCanAccessAgent_BuiltinPrivateNoOwnerReadableByAnyTenantMember(t *testing.T) {
	agentID := "builtin-test-agent"
	withBuiltinAgent(t, agentID)

	svc := NewAgentVisibilityService(
		&stubVisibilityAgentRepo{stored: &types.CustomAgent{
			ID:         agentID,
			IsBuiltin:  true,
			TenantID:   7,
			Visibility: types.AgentVisibilityPrivate,
			CreatedBy:  "", // 修复前的持久化行状态
		}},
		&stubBuiltinAgentSvc{stored: &types.CustomAgent{
			ID:         agentID,
			IsBuiltin:  true,
			TenantID:   7,
			Visibility: types.AgentVisibilityPrivate,
			CreatedBy:  "",
		}},
		&stubSameTenantOrgTreeService{},
	)

	allowed, err := svc.CanAccessAgent(context.Background(), "any-tenant-member", 7, agentID, false)
	require.NoError(t, err)
	assert.True(t, allowed, "builtin agents must be readable by any member of the owning tenant even when persisted as private with empty created_by")

	// 跨租户仍不可读：agent 仅属于 tenant 7
	allowed, err = svc.CanAccessAgent(context.Background(), "other-tenant-member", 8, agentID, false)
	require.NoError(t, err)
	assert.False(t, allowed, "builtin agents of another tenant must stay hidden")
}

// TestCanAccessAgent_BuiltinGlobalReadableByAnyTenantMember 验证修复后
// updateBuiltinAgent 新写入的 visibility='global' 行同样可读。
func TestCanAccessAgent_BuiltinGlobalReadableByAnyTenantMember(t *testing.T) {
	agentID := "builtin-test-agent-global"
	withBuiltinAgent(t, agentID)

	svc := NewAgentVisibilityService(
		&stubVisibilityAgentRepo{},
		&stubBuiltinAgentSvc{stored: &types.CustomAgent{
			ID:         agentID,
			IsBuiltin:  true,
			TenantID:   7,
			Visibility: types.AgentVisibilityGlobal,
		}},
		&stubSameTenantOrgTreeService{},
	)

	allowed, err := svc.CanAccessAgent(context.Background(), "any-tenant-member", 7, agentID, false)
	require.NoError(t, err)
	assert.True(t, allowed, "global-visibility builtin agent rows must be readable")
}

// TestCanAccessAgent_CustomPrivateNoOwnerStillDenied 确认内置智能体的
// 豁免仅限内置 ID：自定义智能体的“无主私有”行仍应拒绝（fail-closed），
// 避免该豁免被误用放大出越权。
func TestCanAccessAgent_CustomPrivateNoOwnerStillDenied(t *testing.T) {
	svc := NewAgentVisibilityService(
		&stubVisibilityAgentRepo{stored: &types.CustomAgent{
			ID:         "custom-agent-1",
			TenantID:   7,
			Visibility: types.AgentVisibilityPrivate,
			CreatedBy:  "",
		}},
		&stubBuiltinAgentSvc{stored: &types.CustomAgent{
			ID:         "custom-agent-1",
			TenantID:   7,
			Visibility: types.AgentVisibilityPrivate,
			CreatedBy:  "",
		}},
		&stubSameTenantOrgTreeService{},
	)

	allowed, err := svc.CanAccessAgent(context.Background(), "some-user", 7, "custom-agent-1", false)
	require.NoError(t, err)
	assert.False(t, allowed, "custom private agents with empty created_by must remain unreadable")
}
