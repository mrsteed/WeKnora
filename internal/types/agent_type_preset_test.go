package types

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAgentTypePresetsConfigIncludesDatabaseAnalysisPreset(t *testing.T) {
	agentTypePresetsMu.Lock()
	savedPresets := agentTypePresets
	savedIDs := agentTypePresetIDs
	agentTypePresets = nil
	agentTypePresetIDs = nil
	agentTypePresetsOnce = sync.Once{}
	agentTypePresetsMu.Unlock()

	t.Cleanup(func() {
		agentTypePresetsMu.Lock()
		defer agentTypePresetsMu.Unlock()
		agentTypePresets = savedPresets
		agentTypePresetIDs = savedIDs
		agentTypePresetsOnce = sync.Once{}
	})

	configDir := filepath.Join("..", "..", "config")
	require.NoError(t, LoadAgentTypePresetsConfig(configDir))

	preset := GetAgentTypePreset(AgentTypeDatabaseAnalysis)
	require.NotNil(t, preset)
	require.NotNil(t, preset.Config)
	require.NotNil(t, preset.KBFilter)

	assert.Equal(t, "database_analyst", preset.Config.SystemPromptID)
	assert.Equal(t, "selected", preset.Config.KBSelectionMode)
	assert.Equal(t, []string{"database"}, preset.KBFilter.AllOf)
	assert.Contains(t, preset.Config.AllowedTools, "external_database_schema")
	assert.Contains(t, preset.Config.AllowedTools, "external_database_query")
	assert.Contains(t, preset.Config.AllowedTools, "final_answer")
	assert.Contains(t, agentTypePresetIDs, AgentTypeDatabaseAnalysis)

	localized := preset.I18n["zh-CN"]
	assert.Equal(t, "数据库分析", localized.Label)
	assert.Contains(t, localized.Description, "Database")
}
