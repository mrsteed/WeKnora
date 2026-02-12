package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

// HuggingFaceProvider implements HuggingFace / TEI provider
type HuggingFaceProvider struct{}

func init() {
	Register(&HuggingFaceProvider{})
}

// Info returns metadata for HuggingFace provider
func (p *HuggingFaceProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderHuggingFace,
		DisplayName: "HuggingFace / TEI",
		Description: "Text Embeddings Inference (bge-m3, etc.)",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeEmbedding: "http://localhost:8080",
			types.ModelTypeRerank:    "http://localhost:8080",
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
		},
		RequiresAuth: false, // Usually local deployment doesn't require auth
	}
}

// ValidateConfig validates HuggingFace provider config
func (p *HuggingFaceProvider) ValidateConfig(config *Config) error {
	// TEI usually just needs a BaseURL
	return nil
}
