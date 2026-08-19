package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type builtinAgentRepoStub struct {
	interfaces.CustomAgentRepository
	store map[string]*types.CustomAgent
}

func builtinAgentRepoKey(id string, tenantID uint64) string {
	return fmt.Sprintf("%s:%d", id, tenantID)
}

func (r *builtinAgentRepoStub) GetAgentByID(_ context.Context, id string, tenantID uint64) (*types.CustomAgent, error) {
	if agent := r.store[builtinAgentRepoKey(id, tenantID)]; agent != nil {
		clone := *agent
		return &clone, nil
	}
	return nil, repository.ErrCustomAgentNotFound
}

func (r *builtinAgentRepoStub) CreateAgent(_ context.Context, agent *types.CustomAgent) error {
	clone := *agent
	r.store[builtinAgentRepoKey(agent.ID, agent.TenantID)] = &clone
	return nil
}

func (r *builtinAgentRepoStub) UpdateAgent(_ context.Context, agent *types.CustomAgent) error {
	clone := *agent
	r.store[builtinAgentRepoKey(agent.ID, agent.TenantID)] = &clone
	return nil
}

func builtinAgentTestContext(tenantID uint64) context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
}

func installBuiltinAgentTestFactory(t *testing.T, id string) {
	t.Helper()
	previous, hadPrevious := types.BuiltinAgentRegistry[id]
	types.BuiltinAgentRegistry[id] = func(tenantID uint64) *types.CustomAgent {
		return &types.CustomAgent{
			ID:        id,
			Name:      "builtin",
			IsBuiltin: true,
			TenantID:  tenantID,
			Config: types.CustomAgentConfig{
				AgentMode: types.AgentModeSmartReasoning,
			},
		}
	}
	t.Cleanup(func() {
		if hadPrevious {
			types.BuiltinAgentRegistry[id] = previous
			return
		}
		delete(types.BuiltinAgentRegistry, id)
	})
}

func TestUpdateBuiltinAgentCreatesGlobalVisibilityOverride(t *testing.T) {
	const tenantID = uint64(10005)
	installBuiltinAgentTestFactory(t, types.BuiltinSmartReasoningID)
	repo := &builtinAgentRepoStub{store: map[string]*types.CustomAgent{}}
	svc := &customAgentService{repo: repo}

	agent, err := svc.UpdateAgent(builtinAgentTestContext(tenantID), &types.CustomAgent{
		ID: types.BuiltinSmartReasoningID,
		Config: types.CustomAgentConfig{
			AgentMode: types.AgentModeSmartReasoning,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.True(t, agent.IsBuiltin)
	assert.Equal(t, types.AgentVisibilityGlobal, agent.Visibility)
	require.Contains(t, repo.store, builtinAgentRepoKey(types.BuiltinSmartReasoningID, tenantID))
	assert.Equal(t, types.AgentVisibilityGlobal, repo.store[builtinAgentRepoKey(types.BuiltinSmartReasoningID, tenantID)].Visibility)
}

func TestUpdateBuiltinAgentNormalizesExistingOverrideToGlobal(t *testing.T) {
	const tenantID = uint64(10005)
	installBuiltinAgentTestFactory(t, types.BuiltinSmartReasoningID)
	repo := &builtinAgentRepoStub{store: map[string]*types.CustomAgent{
		builtinAgentRepoKey(types.BuiltinSmartReasoningID, tenantID): {
			ID:         types.BuiltinSmartReasoningID,
			TenantID:   tenantID,
			IsBuiltin:  true,
			Visibility: types.AgentVisibilityPrivate,
			CreatedBy:  "",
			Config:     types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning},
		},
	}}
	svc := &customAgentService{repo: repo}

	agent, err := svc.UpdateAgent(builtinAgentTestContext(tenantID), &types.CustomAgent{
		ID: types.BuiltinSmartReasoningID,
		Config: types.CustomAgentConfig{
			AgentMode: types.AgentModeSmartReasoning,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, types.AgentVisibilityGlobal, agent.Visibility)
	assert.Equal(t, types.AgentVisibilityGlobal, repo.store[builtinAgentRepoKey(types.BuiltinSmartReasoningID, tenantID)].Visibility)
}
