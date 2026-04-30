package service

import (
	"context"
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAgentKnowledgeBaseService struct {
	interfaces.KnowledgeBaseService
	byID map[string]*types.KnowledgeBase
}

func (s *stubAgentKnowledgeBaseService) GetKnowledgeBaseByIDOnly(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if kb, ok := s.byID[id]; ok {
		return kb, nil
	}
	return nil, nil
}

type stubAgentSchemaRegistryService struct {
	interfaces.SchemaRegistryService
}

type stubAgentStructuredQueryService struct {
	interfaces.StructuredQueryService
}

func TestAgentServiceRegisterToolsIncludesExternalDatabaseToolsWhenDatabaseKBInScope(t *testing.T) {
	svc := &agentService{
		knowledgeBaseService:   &stubAgentKnowledgeBaseService{byID: map[string]*types.KnowledgeBase{"kb-db": {ID: "kb-db", Type: types.KnowledgeBaseTypeDatabase}}},
		schemaRegistryService:  &stubAgentSchemaRegistryService{},
		structuredQueryService: &stubAgentStructuredQueryService{},
	}
	registry := agenttools.NewToolRegistry()
	config := &types.AgentConfig{
		AllowedTools:   []string{agenttools.ToolExternalDatabaseSchema, agenttools.ToolExternalDatabaseQuery},
		KnowledgeBases: []string{"kb-db"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-db"}},
	}

	err := svc.registerTools(context.Background(), registry, config, nil, nil, "")
	require.NoError(t, err)

	_, err = registry.GetTool(agenttools.ToolExternalDatabaseSchema)
	require.NoError(t, err)
	_, err = registry.GetTool(agenttools.ToolExternalDatabaseQuery)
	require.NoError(t, err)
	_, err = registry.GetTool(agenttools.ToolFinalAnswer)
	require.NoError(t, err)
}

func TestAgentServiceRegisterToolsFiltersExternalDatabaseToolsWithoutDatabaseKB(t *testing.T) {
	svc := &agentService{
		knowledgeBaseService:   &stubAgentKnowledgeBaseService{byID: map[string]*types.KnowledgeBase{"kb-doc": {ID: "kb-doc", Type: types.KnowledgeBaseTypeDocument}}},
		schemaRegistryService:  &stubAgentSchemaRegistryService{},
		structuredQueryService: &stubAgentStructuredQueryService{},
	}
	registry := agenttools.NewToolRegistry()
	config := &types.AgentConfig{
		AllowedTools:   []string{agenttools.ToolExternalDatabaseSchema, agenttools.ToolExternalDatabaseQuery},
		KnowledgeBases: []string{"kb-doc"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-doc"}},
	}

	err := svc.registerTools(context.Background(), registry, config, nil, nil, "")
	require.NoError(t, err)

	_, err = registry.GetTool(agenttools.ToolExternalDatabaseSchema)
	assert.Error(t, err)
	_, err = registry.GetTool(agenttools.ToolExternalDatabaseQuery)
	assert.Error(t, err)
	_, err = registry.GetTool(agenttools.ToolFinalAnswer)
	require.NoError(t, err)
}
