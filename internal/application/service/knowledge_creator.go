package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// resolveKnowledgeCreatorIDFromContext resolves the document uploader/creator
// ID to stamp on a newly created knowledge entry.
//
// Preference order:
//  1. The resource-auth identity (e.g. the IM channel owner) — stable across
//     non-web call paths where the request caller may be absent or synthetic.
//  2. The request caller identity, only when it is a real user.
//
// Synthetic identities ("system-<tenantID>", empty) yield "", which keeps
// legacy rows distinguishable from "created by an unattributable caller".
func resolveKnowledgeCreatorIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if userID, ok := types.ResourceAuthUserIDFromContext(ctx); ok && !types.IsSyntheticUserID(userID) {
		return strings.TrimSpace(userID)
	}
	userID, ok := types.UserIDFromContext(ctx)
	if !ok || strings.TrimSpace(userID) == "" || types.IsSyntheticUserID(userID) {
		return ""
	}
	return strings.TrimSpace(userID)
}

// fillKnowledgeCreatorNicknames populates CreatedByNickname from CreatedBy.
//
// The nickname is always derived — never stored — so the creator_id axis
// stays the single source of truth. Unknown IDs fall back to the raw ID so a
// row with a deleted or external creator still renders something stable
// instead of a blank cell.
func fillKnowledgeCreatorNicknames(ctx context.Context, userRepo interfaces.UserRepository, knowledges []*types.Knowledge) {
	if userRepo == nil || len(knowledges) == 0 {
		return
	}

	creatorSet := make(map[string]struct{})
	creatorIDs := make([]string, 0, len(knowledges))
	nicknames := make(map[string]string)

	for _, knowledge := range knowledges {
		if knowledge == nil {
			continue
		}
		creatorID := strings.TrimSpace(knowledge.CreatedBy)
		if creatorID == "" {
			continue
		}
		if _, seen := creatorSet[creatorID]; seen {
			continue
		}
		creatorSet[creatorID] = struct{}{}
		if types.IsBuiltinAgentID(creatorID) || creatorID == "system" {
			nicknames[creatorID] = "系统"
			continue
		}
		creatorIDs = append(creatorIDs, creatorID)
	}

	if len(creatorIDs) > 0 {
		users, err := userRepo.GetUsersByIDs(ctx, creatorIDs)
		if err != nil {
			logger.Warnf(ctx, "Failed to batch load knowledge creators: %v", err)
		} else {
			for creatorID, user := range users {
				if user == nil {
					continue
				}
				if username := strings.TrimSpace(user.Username); username != "" {
					nicknames[creatorID] = username
				}
			}
		}
	}

	for _, knowledge := range knowledges {
		if knowledge == nil {
			continue
		}
		creatorID := strings.TrimSpace(knowledge.CreatedBy)
		if creatorID == "" {
			continue
		}
		if nickname := strings.TrimSpace(nicknames[creatorID]); nickname != "" {
			knowledge.CreatedByNickname = nickname
			continue
		}
		knowledge.CreatedByNickname = creatorID
	}
}
