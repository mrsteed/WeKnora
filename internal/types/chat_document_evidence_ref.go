package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ChatDocumentEvidenceTypeChunk = "chunk"
)

type ChatDocumentEvidenceRef struct {
	ID              string    `json:"id,omitempty" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64    `json:"tenant_id,omitempty" gorm:"index;not null"`
	RunID           string    `json:"run_id,omitempty" gorm:"type:varchar(36);index"`
	ArtifactID      string    `json:"artifact_id,omitempty" gorm:"type:varchar(36);index;not null"`
	MessageID       string    `json:"message_id,omitempty" gorm:"type:varchar(36);index"`
	SectionID       string    `json:"section_id,omitempty" gorm:"type:varchar(128);index"`
	SectionHeading  string    `json:"section_heading,omitempty" gorm:"type:varchar(255)"`
	Query           string    `json:"query,omitempty" gorm:"type:text"`
	KnowledgeBaseID string    `json:"knowledge_base_id,omitempty" gorm:"type:varchar(36);index"`
	KnowledgeID     string    `json:"knowledge_id,omitempty" gorm:"type:varchar(36);index"`
	ChunkID         string    `json:"chunk_id,omitempty" gorm:"type:varchar(128);index"`
	SourceTitle     string    `json:"source_title,omitempty" gorm:"type:varchar(255)"`
	Excerpt         string    `json:"excerpt,omitempty" gorm:"type:text"`
	SourceStartAt   int       `json:"source_start_at,omitempty" gorm:"not null;default:0"`
	SourceEndAt     int       `json:"source_end_at,omitempty" gorm:"not null;default:0"`
	Score           float64   `json:"score,omitempty" gorm:"type:double precision"`
	EvidenceType    string    `json:"evidence_type,omitempty" gorm:"type:varchar(32);index"`
	ContentChecksum string    `json:"content_checksum,omitempty" gorm:"type:varchar(64);index"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
}

func (r *ChatDocumentEvidenceRef) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if strings.TrimSpace(r.EvidenceType) == "" {
		r.EvidenceType = ChatDocumentEvidenceTypeChunk
	}
	return nil
}

func NormalizeChatDocumentEvidenceRefs(value interface{}) []ChatDocumentEvidenceRef {
	switch refs := value.(type) {
	case nil:
		return nil
	case []ChatDocumentEvidenceRef:
		return uniqueChatDocumentEvidenceRefs(refs)
	case []*ChatDocumentEvidenceRef:
		result := make([]ChatDocumentEvidenceRef, 0, len(refs))
		for _, ref := range refs {
			if normalized, ok := normalizeChatDocumentEvidenceRefValue(ref); ok {
				result = append(result, normalized)
			}
		}
		return uniqueChatDocumentEvidenceRefs(result)
	case []interface{}:
		result := make([]ChatDocumentEvidenceRef, 0, len(refs))
		for _, ref := range refs {
			if normalized, ok := normalizeChatDocumentEvidenceRefValue(ref); ok {
				result = append(result, normalized)
			}
		}
		return uniqueChatDocumentEvidenceRefs(result)
	default:
		if normalized, ok := normalizeChatDocumentEvidenceRefValue(refs); ok {
			return []ChatDocumentEvidenceRef{normalized}
		}
		return nil
	}
}

func normalizeChatDocumentEvidenceRefValue(value interface{}) (ChatDocumentEvidenceRef, bool) {
	switch ref := value.(type) {
	case nil:
		return ChatDocumentEvidenceRef{}, false
	case ChatDocumentEvidenceRef:
		return normalizeChatDocumentEvidenceRef(ref)
	case *ChatDocumentEvidenceRef:
		if ref == nil {
			return ChatDocumentEvidenceRef{}, false
		}
		return normalizeChatDocumentEvidenceRef(*ref)
	case map[string]interface{}:
		candidate := ChatDocumentEvidenceRef{
			ID:              normalizeChatDocumentEvidenceString(ref["id"]),
			RunID:           normalizeChatDocumentEvidenceString(ref["run_id"]),
			ArtifactID:      normalizeChatDocumentEvidenceString(ref["artifact_id"]),
			MessageID:       normalizeChatDocumentEvidenceString(ref["message_id"]),
			SectionID:       normalizeChatDocumentEvidenceString(ref["section_id"]),
			SectionHeading:  normalizeChatDocumentEvidenceString(ref["section_heading"]),
			Query:           normalizeChatDocumentEvidenceString(ref["query"]),
			KnowledgeBaseID: normalizeChatDocumentEvidenceString(ref["knowledge_base_id"]),
			KnowledgeID:     normalizeChatDocumentEvidenceString(ref["knowledge_id"]),
			ChunkID:         normalizeChatDocumentEvidenceString(ref["chunk_id"]),
			SourceTitle:     normalizeChatDocumentEvidenceString(ref["source_title"]),
			Excerpt:         normalizeChatDocumentEvidenceString(ref["excerpt"]),
			SourceStartAt:   normalizeChatDocumentEvidenceInt(ref["source_start_at"]),
			SourceEndAt:     normalizeChatDocumentEvidenceInt(ref["source_end_at"]),
			EvidenceType:    normalizeChatDocumentEvidenceString(ref["evidence_type"]),
			ContentChecksum: normalizeChatDocumentEvidenceString(ref["content_checksum"]),
			Score:           normalizeChatDocumentEvidenceFloat64(ref["score"]),
		}
		if candidate.SourceTitle == "" {
			candidate.SourceTitle = normalizeChatDocumentEvidenceString(ref["knowledge_title"])
		}
		return normalizeChatDocumentEvidenceRef(candidate)
	default:
		raw, err := json.Marshal(value)
		if err != nil || len(raw) == 0 || string(raw) == "null" {
			return ChatDocumentEvidenceRef{}, false
		}
		var candidate ChatDocumentEvidenceRef
		if err := json.Unmarshal(raw, &candidate); err != nil {
			return ChatDocumentEvidenceRef{}, false
		}
		return normalizeChatDocumentEvidenceRef(candidate)
	}
}

func normalizeChatDocumentEvidenceRef(ref ChatDocumentEvidenceRef) (ChatDocumentEvidenceRef, bool) {
	ref.ID = strings.TrimSpace(ref.ID)
	ref.RunID = strings.TrimSpace(ref.RunID)
	ref.ArtifactID = strings.TrimSpace(ref.ArtifactID)
	ref.MessageID = strings.TrimSpace(ref.MessageID)
	ref.SectionID = strings.TrimSpace(ref.SectionID)
	ref.SectionHeading = strings.TrimSpace(ref.SectionHeading)
	ref.Query = strings.TrimSpace(ref.Query)
	ref.KnowledgeBaseID = strings.TrimSpace(ref.KnowledgeBaseID)
	ref.KnowledgeID = strings.TrimSpace(ref.KnowledgeID)
	ref.ChunkID = strings.TrimSpace(ref.ChunkID)
	ref.SourceTitle = strings.TrimSpace(ref.SourceTitle)
	ref.Excerpt = strings.TrimSpace(ref.Excerpt)
	ref.EvidenceType = strings.TrimSpace(ref.EvidenceType)
	ref.ContentChecksum = strings.TrimSpace(ref.ContentChecksum)
	if ref.SourceStartAt < 0 {
		ref.SourceStartAt = 0
	}
	if ref.SourceEndAt < 0 {
		ref.SourceEndAt = 0
	}
	if ref.EvidenceType == "" {
		ref.EvidenceType = ChatDocumentEvidenceTypeChunk
	}
	if ref.ChunkID == "" && ref.KnowledgeID == "" && ref.KnowledgeBaseID == "" && ref.Query == "" {
		return ChatDocumentEvidenceRef{}, false
	}
	return ref, true
}

func uniqueChatDocumentEvidenceRefs(refs []ChatDocumentEvidenceRef) []ChatDocumentEvidenceRef {
	if len(refs) == 0 {
		return nil
	}
	result := make([]ChatDocumentEvidenceRef, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		normalized, ok := normalizeChatDocumentEvidenceRef(ref)
		if !ok {
			continue
		}
		key := strings.Join([]string{
			normalized.SectionHeading,
			normalized.Query,
			normalized.KnowledgeBaseID,
			normalized.KnowledgeID,
			normalized.ChunkID,
			normalized.Excerpt,
			fmt.Sprintf("%d", normalized.SourceStartAt),
			fmt.Sprintf("%d", normalized.SourceEndAt),
			normalized.ContentChecksum,
		}, "|")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeChatDocumentEvidenceString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	case json.Number:
		return strings.TrimSpace(current.String())
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}

func normalizeChatDocumentEvidenceFloat64(value interface{}) float64 {
	switch current := value.(type) {
	case float64:
		return current
	case float32:
		return float64(current)
	case int:
		return float64(current)
	case int32:
		return float64(current)
	case int64:
		return float64(current)
	case json.Number:
		parsed, err := current.Float64()
		if err == nil {
			return parsed
		}
	}
	return 0
}

func normalizeChatDocumentEvidenceInt(value interface{}) int {
	switch current := value.(type) {
	case int:
		return current
	case int32:
		return int(current)
	case int64:
		return int(current)
	case float32:
		return int(current)
	case float64:
		return int(current)
	case json.Number:
		parsed, err := current.Int64()
		if err == nil {
			return int(parsed)
		}
	}
	return 0
}
