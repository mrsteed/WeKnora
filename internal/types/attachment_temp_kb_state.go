package types

import "time"

// AttachmentTempKBState stores session-scoped hidden KB state for chat attachments.
// The state is kept outside the messages table so large sessions can reuse the
// same temporary KB across multiple turns without bloating message payloads.
type AttachmentTempKBState struct {
	KBID                string            `json:"kbID"`
	KnowledgeIDs        []string          `json:"knowledgeIDs"`
	AttachmentKnowledge map[string]string `json:"attachmentKnowledge"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}

// EnsureDefaults initializes nil maps/slices so callers can safely append and write.
func (s *AttachmentTempKBState) EnsureDefaults() {
	if s == nil {
		return
	}
	if s.KnowledgeIDs == nil {
		s.KnowledgeIDs = make([]string, 0)
	}
	if s.AttachmentKnowledge == nil {
		s.AttachmentKnowledge = make(map[string]string)
	}
}
