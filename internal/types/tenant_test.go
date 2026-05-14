package types

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrieverEngineMappingIncludesTencentVectorDBHybridCapabilities(t *testing.T) {
	mapping := GetRetrieverEngineMapping()

	assert.Contains(t, mapping["tencent_vectordb"], RetrieverEngineParams{
		RetrieverType:       KeywordsRetrieverType,
		RetrieverEngineType: TencentVectorDBRetrieverEngineType,
	})
	assert.Contains(t, mapping["tencent_vectordb"], RetrieverEngineParams{
		RetrieverType:       VectorRetrieverType,
		RetrieverEngineType: TencentVectorDBRetrieverEngineType,
	})
}

func TestTenantGetEffectiveEngines_AllowsNilReceiver(t *testing.T) {
	oldDriver := os.Getenv("RETRIEVE_DRIVER")
	t.Cleanup(func() {
		require.NoError(t, os.Setenv("RETRIEVE_DRIVER", oldDriver))
	})
	require.NoError(t, os.Setenv("RETRIEVE_DRIVER", "sqlite"))

	var tenant *Tenant
	engines := tenant.GetEffectiveEngines()

	require.NotEmpty(t, engines)
	assert.Equal(t, KeywordsRetrieverType, engines[0].RetrieverType)
	assert.Equal(t, SQLiteRetrieverEngineType, engines[0].RetrieverEngineType)
}
