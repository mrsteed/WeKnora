package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fillKnowledgeBaseCreatorNicknames populates CreatedByNickname for each knowledge base.
// The helper is shared by multiple service paths so list APIs return a consistent creator label.
// For regular knowledge bases, the label should reflect the real creator when available.
// Only built-in/system-owned records fall back to the synthetic "系统" label.
func fillKnowledgeBaseCreatorNicknames(ctx context.Context, userRepo interfaces.UserRepository, kbs []*types.KnowledgeBase) {
	if userRepo == nil || len(kbs) == 0 {
		return
	}

	creatorIDs := make(map[string]struct{})
	for _, kb := range kbs {
		if kb != nil && kb.CreatedBy != "" {
			creatorIDs[kb.CreatedBy] = struct{}{}
		}
	}

	userMap := make(map[string]string, len(creatorIDs))
	for creatorID := range creatorIDs {
		if types.IsBuiltinAgentID(creatorID) || creatorID == "system" {
			userMap[creatorID] = "系统"
			continue
		}

		user, err := userRepo.GetUserByID(ctx, creatorID)
		if err != nil {
			logger.Warnf(ctx, "Failed to get user %s: %v", creatorID, err)
			continue
		}
		if user != nil {
			userMap[creatorID] = user.Username
		}
	}

	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		if nickname, ok := userMap[kb.CreatedBy]; ok {
			kb.CreatedByNickname = nickname
			continue
		}
		if kb.CreatedBy != "" {
			kb.CreatedByNickname = kb.CreatedBy
		}
	}
}
