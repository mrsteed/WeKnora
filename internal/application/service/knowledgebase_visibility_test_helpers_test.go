package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

func (r *fakeKBRepo) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, orgIDs []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}

func (r *kbDeleteKBRepo) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, orgIDs []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}

func (r *stubKBRepoForModelDelete) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, orgIDs []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}

func (r *realKBRepo) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, orgIDs []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
