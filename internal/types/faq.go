package types

import (
	"encoding/json"
	"strings"
	"time"
)

// FAQChunkMetadata 定义 FAQ 条目在 Chunk.Metadata 中的结构
type FAQChunkMetadata struct {
	StandardQuestion  string   `json:"standard_question"`
	SimilarQuestions  []string `json:"similar_questions,omitempty"`
	NegativeQuestions []string `json:"negative_questions,omitempty"`
	Answers           []string `json:"answers,omitempty"`
	Version           int      `json:"version,omitempty"`
	Source            string   `json:"source,omitempty"`
}

// Normalize 清理空白与重复项
func (m *FAQChunkMetadata) Normalize() {
	if m == nil {
		return
	}
	m.StandardQuestion = strings.TrimSpace(m.StandardQuestion)
	m.SimilarQuestions = normalizeStrings(m.SimilarQuestions)
	m.NegativeQuestions = normalizeStrings(m.NegativeQuestions)
	m.Answers = normalizeStrings(m.Answers)
	if m.Version <= 0 {
		m.Version = 1
	}
}

// FAQMetadata 解析 Chunk 中的 FAQ 元数据
func (c *Chunk) FAQMetadata() (*FAQChunkMetadata, error) {
	if c == nil || len(c.Metadata) == 0 {
		return nil, nil
	}
	var meta FAQChunkMetadata
	if err := json.Unmarshal(c.Metadata, &meta); err != nil {
		return nil, err
	}
	meta.Normalize()
	return &meta, nil
}

// SetFAQMetadata 设置 Chunk 的 FAQ 元数据
func (c *Chunk) SetFAQMetadata(meta *FAQChunkMetadata) error {
	if c == nil {
		return nil
	}
	if meta == nil {
		c.Metadata = nil
		return nil
	}
	meta.Normalize()
	bytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	c.Metadata = JSON(bytes)
	return nil
}

// FAQEntry 表示返回给前端的 FAQ 条目
type FAQEntry struct {
	ID                string       `json:"id"`
	ChunkID           string       `json:"chunk_id"`
	KnowledgeID       string       `json:"knowledge_id"`
	KnowledgeBaseID   string       `json:"knowledge_base_id"`
	StandardQuestion  string       `json:"standard_question"`
	SimilarQuestions  []string     `json:"similar_questions"`
	NegativeQuestions []string     `json:"negative_questions"`
	Answers           []string     `json:"answers"`
	IndexMode         FAQIndexMode `json:"index_mode"`
	UpdatedAt         time.Time    `json:"updated_at"`
	CreatedAt         time.Time    `json:"created_at"`
	Score             float64      `json:"score,omitempty"`
	MatchType         MatchType    `json:"match_type,omitempty"`
	ChunkType         ChunkType    `json:"chunk_type"`
}

// FAQEntryPayload 用于创建/更新 FAQ 条目的 payload
type FAQEntryPayload struct {
	StandardQuestion  string   `json:"standard_question" binding:"required"`
	SimilarQuestions  []string `json:"similar_questions"`
	NegativeQuestions []string `json:"negative_questions"`
	Answers           []string `json:"answers" binding:"required"`
}

const (
	FAQBatchModeAppend  = "append"
	FAQBatchModeReplace = "replace"
)

// FAQBatchUpsertPayload 批量导入 FAQ 条目
type FAQBatchUpsertPayload struct {
	Entries     []FAQEntryPayload `json:"entries" binding:"required"`
	Mode        string            `json:"mode" binding:"oneof=append replace"`
	KnowledgeID string            `json:"knowledge_id"`
}

// FAQSearchRequest FAQ检索请求参数
type FAQSearchRequest struct {
	QueryText       string  `json:"query_text" binding:"required"`
	VectorThreshold float64 `json:"vector_threshold"`
	MatchCount      int     `json:"match_count"`
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	dedup := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		dedup = append(dedup, trimmed)
	}
	if len(dedup) == 0 {
		return nil
	}
	return dedup
}
