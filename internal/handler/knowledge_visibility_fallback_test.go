package handler

import (
	"context"
	"testing"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestValidateKnowledgeBaseAccessWithKBID_SameTenantVisibilityApplies(t *testing.T) {
	h := &KnowledgeHandler{
		kbService: &stubKBService{
			get: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
				return &types.KnowledgeBase{ID: "kb-1", TenantID: 1, Visibility: types.KBVisibilityPrivate, CreatedBy: "u-owner"}, nil
			},
		},
		kbVisibility: &stubKBVisibilityService{
			canManage: func(_ context.Context, _ string, _ uint64, _ string, _ bool) (bool, error) {
				return false, nil
			},
		},
	}

	_, _, _, _, err := h.validateKnowledgeBaseAccessWithKBID(newKBLookupCtx(t, 1, "kb-1"), "kb-1")
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok || appErr.Code != apperrors.ErrForbidden {
		t.Fatalf("expected forbidden AppError, got %T %v", err, err)
	}
}

func TestValidateKnowledgeBaseAccessWithKBID_TenantAdminBypassesVisibility(t *testing.T) {
	c := newKBLookupCtx(t, 1, "kb-1")
	ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleAdmin)
	c.Request = c.Request.WithContext(ctx)

	h := &KnowledgeHandler{
		kbService: &stubKBService{
			get: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
				return &types.KnowledgeBase{ID: "kb-1", TenantID: 1, Visibility: types.KBVisibilityPrivate, CreatedBy: "u-owner"}, nil
			},
		},
		kbVisibility: &stubKBVisibilityService{
			canManage: func(_ context.Context, _ string, _ uint64, _ string, _ bool) (bool, error) {
				return false, nil
			},
		},
	}

	_, _, _, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, "kb-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if permission != types.OrgRoleAdmin {
		t.Fatalf("expected admin permission, got %s", permission)
	}
}