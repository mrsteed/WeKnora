package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

func resolveKnowledgeCreatorIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	uid, ok := types.UserIDFromContext(ctx)
	if !ok || strings.TrimSpace(uid) == "" || types.IsSyntheticUserID(uid) {
		return ""
	}
	return uid
}
