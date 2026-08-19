package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fillKnowledgeBaseCreatorNames populates CreatorName using CreatorID.
func fillKnowledgeBaseCreatorNames(ctx context.Context, userRepo interfaces.UserRepository, kbs []*types.KnowledgeBase) {
	if userRepo == nil || len(kbs) == 0 {
		return
	}

	creatorIDs := make(map[string]struct{})
	for _, kb := range kbs {
		if kb != nil && kb.CreatorID != "" {
			creatorIDs[kb.CreatorID] = struct{}{}
		}
	}
	if len(creatorIDs) == 0 {
		return
	}

	ids := make([]string, 0, len(creatorIDs))
	for id := range creatorIDs {
		ids = append(ids, id)
	}
	users, err := userRepo.GetUsersByIDs(ctx, ids)
	if err != nil {
		logger.Warnf(ctx, "Failed to resolve knowledge base creator names: %v", err)
		return
	}

	for _, kb := range kbs {
		if kb == nil || kb.CreatorID == "" {
			continue
		}
		u, ok := users[kb.CreatorID]
		if !ok || u == nil {
			continue
		}
		if u.Username != "" {
			kb.CreatorName = u.Username
			continue
		}
		kb.CreatorName = u.Email
	}
}
