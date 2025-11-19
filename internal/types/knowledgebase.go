package types

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

const (
	InitDefaultKnowledgeBaseID = "kb-00000001"
)

const (
	KnowledgeBaseTypeDocument = "document"
	KnowledgeBaseTypeFAQ      = "faq"
)

// FAQIndexMode 表示 FAQ 索引策略：仅索引标准问还是同时索引问答内容
type FAQIndexMode string

const (
	// FAQIndexModeQuestionOnly 仅使用标准问及相似问向量化
	FAQIndexModeQuestionOnly FAQIndexMode = "question_only"
	// FAQIndexModeQuestionAnswer 将标准问与答案拼接后向量化
	FAQIndexModeQuestionAnswer FAQIndexMode = "question_answer"
)

// FAQQuestionIndexMode 表示 FAQ 问题索引方式：一起索引还是分别索引
type FAQQuestionIndexMode string

const (
	// FAQQuestionIndexModeCombined 将标准问和相似问一起索引（当前方式）
	FAQQuestionIndexModeCombined FAQQuestionIndexMode = "combined"
	// FAQQuestionIndexModeSeparate 将标准问和相似问分别索引
	FAQQuestionIndexModeSeparate FAQQuestionIndexMode = "separate"
)

// KnowledgeBase represents a knowledge base
type KnowledgeBase struct {
	// Unique identifier of the knowledge base
	ID string `yaml:"id" json:"id" gorm:"type:varchar(36);primaryKey"`
	// Name of the knowledge base
	Name string `yaml:"name" json:"name"`
	// Type of the knowledge base (document, faq, etc.)
	Type string `yaml:"type" json:"type" gorm:"type:varchar(32);default:'document'"`
	// Whether this knowledge base is temporary (ephemeral) and should be hidden from UI
	IsTemporary bool `yaml:"is_temporary" json:"is_temporary" gorm:"default:false"`
	// Description of the knowledge base
	Description string `yaml:"description" json:"description"`
	// Tenant ID
	TenantID uint `yaml:"tenant_id" json:"tenant_id"`
	// Chunking configuration
	ChunkingConfig ChunkingConfig `yaml:"chunking_config" json:"chunking_config" gorm:"type:json"`
	// Image processing configuration
	ImageProcessingConfig ImageProcessingConfig `yaml:"image_processing_config" json:"image_processing_config" gorm:"type:json"`
	// ID of the embedding model
	EmbeddingModelID string `yaml:"embedding_model_id" json:"embedding_model_id"`
	// Summary model ID
	SummaryModelID string `yaml:"summary_model_id" json:"summary_model_id"`
	// VLM model ID
	VLMModelID string `yaml:"vlm_model_id" json:"vlm_model_id"`
	// VLM config
	VLMConfig VLMConfig `yaml:"vlm_config" json:"vlm_config" gorm:"type:json"`
	// Storage config
	StorageConfig StorageConfig `yaml:"cos_config" json:"cos_config" gorm:"column:cos_config;type:json"`
	// Extract config
	ExtractConfig *ExtractConfig `yaml:"extract_config" json:"extract_config" gorm:"column:extract_config;type:json"`
	// FAQConfig stores FAQ specific configuration such as indexing strategy
	FAQConfig *FAQConfig `yaml:"faq_config" json:"faq_config" gorm:"column:faq_config;type:json"`
	// Creation time of the knowledge base
	CreatedAt time.Time `yaml:"created_at" json:"created_at"`
	// Last updated time of the knowledge base
	UpdatedAt time.Time `yaml:"updated_at" json:"updated_at"`
	// Deletion time of the knowledge base
	DeletedAt gorm.DeletedAt `yaml:"deleted_at" json:"deleted_at" gorm:"index"`
}

// KnowledgeBaseConfig represents the knowledge base configuration
type KnowledgeBaseConfig struct {
	// Chunking configuration
	ChunkingConfig ChunkingConfig `yaml:"chunking_config" json:"chunking_config"`
	// Image processing configuration
	ImageProcessingConfig ImageProcessingConfig `yaml:"image_processing_config" json:"image_processing_config"`
	// FAQ configuration (only for FAQ type knowledge bases)
	FAQConfig *FAQConfig `yaml:"faq_config" json:"faq_config"`
}

// ChunkingConfig represents the document splitting configuration
type ChunkingConfig struct {
	// Chunk size
	ChunkSize int `yaml:"chunk_size" json:"chunk_size"`
	// Chunk overlap
	ChunkOverlap int `yaml:"chunk_overlap" json:"chunk_overlap"`
	// Separators
	Separators []string `yaml:"separators" json:"separators"`
	// Enable multimodal
	EnableMultimodal bool `yaml:"enable_multimodal" json:"enable_multimodal"`
}

// COSConfig represents the COS configuration
type StorageConfig struct {
	// Secret ID
	SecretID string `yaml:"secret_id" json:"secret_id"`
	// Secret Key
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	// Region
	Region string `yaml:"region" json:"region"`
	// Bucket Name
	BucketName string `yaml:"bucket_name" json:"bucket_name"`
	// App ID
	AppID string `yaml:"app_id" json:"app_id"`
	// Path Prefix
	PathPrefix string `yaml:"path_prefix" json:"path_prefix"`
	// Provider
	Provider string `yaml:"provider" json:"provider"`
}

func (c StorageConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *StorageConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// ImageProcessingConfig represents the image processing configuration
type ImageProcessingConfig struct {
	// Model ID
	ModelID string `yaml:"model_id" json:"model_id"`
}

// Value implements the driver.Valuer interface, used to convert ChunkingConfig to database value
func (c ChunkingConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to ChunkingConfig
func (c *ChunkingConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// Value implements the driver.Valuer interface, used to convert ImageProcessingConfig to database value
func (c ImageProcessingConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to ImageProcessingConfig
func (c *ImageProcessingConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// VLMConfig represents the VLM configuration
type VLMConfig struct {
	// Model Name
	ModelName string `yaml:"model_name" json:"model_name"`
	// Base URL
	BaseURL string `yaml:"base_url" json:"base_url"`
	// API Key
	APIKey string `yaml:"api_key" json:"api_key"`
	// Interface Type: "ollama" or "openai"
	InterfaceType string `yaml:"interface_type" json:"interface_type"`
}

// Value implements the driver.Valuer interface, used to convert VLMConfig to database value
func (c VLMConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to VLMConfig
func (c *VLMConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

type ExtractConfig struct {
	Text      string           `yaml:"text" json:"text"`
	Tags      []string         `yaml:"tags" json:"tags"`
	Nodes     []*GraphNode     `yaml:"nodes" json:"nodes"`
	Relations []*GraphRelation `yaml:"relations" json:"relations"`
}

// Value implements the driver.Valuer interface, used to convert ExtractConfig to database value
func (e ExtractConfig) Value() (driver.Value, error) {
	return json.Marshal(e)
}

// Scan implements the sql.Scanner interface, used to convert database value to ExtractConfig
func (e *ExtractConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, e)
}

// FAQConfig 存储 FAQ 知识库的特有配置
type FAQConfig struct {
	IndexMode         FAQIndexMode         `yaml:"index_mode" json:"index_mode"`
	QuestionIndexMode FAQQuestionIndexMode `yaml:"question_index_mode" json:"question_index_mode"`
}

// Value implements driver.Valuer
func (f FAQConfig) Value() (driver.Value, error) {
	return json.Marshal(f)
}

// Scan implements sql.Scanner
func (f *FAQConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, f)
}

// EnsureDefaults 确保类型与配置具备默认值
func (kb *KnowledgeBase) EnsureDefaults() {
	if kb == nil {
		return
	}
	if kb.Type == "" {
		kb.Type = KnowledgeBaseTypeDocument
	}
	if kb.Type != KnowledgeBaseTypeFAQ {
		kb.FAQConfig = nil
		return
	}
	if kb.FAQConfig == nil {
		kb.FAQConfig = &FAQConfig{
			IndexMode:         FAQIndexModeQuestionAnswer,
			QuestionIndexMode: FAQQuestionIndexModeCombined,
		}
		return
	}
	if kb.FAQConfig.IndexMode == "" {
		kb.FAQConfig.IndexMode = FAQIndexModeQuestionAnswer
	}
	if kb.FAQConfig.QuestionIndexMode == "" {
		kb.FAQConfig.QuestionIndexMode = FAQQuestionIndexModeCombined
	}
}
