package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type fakeVisibility struct {
	canRead        bool
	canManage      bool
	readErr        error
	manageErr      error
	lastUser       string
	lastKBID       string
	lastPrivileged bool
}

func (f *fakeVisibility) CanAccessKB(_ context.Context, userID string, _ uint64, kbID string, privileged bool) (bool, error) {
	f.lastUser = userID
	f.lastKBID = kbID
	f.lastPrivileged = privileged
	if f.readErr != nil {
		return false, f.readErr
	}
	return f.canRead, nil
}

func (f *fakeVisibility) CanManageKB(_ context.Context, userID string, _ uint64, kbID string, privileged bool) (bool, error) {
	f.lastUser = userID
	f.lastKBID = kbID
	f.lastPrivileged = privileged
	if f.manageErr != nil {
		return false, f.manageErr
	}
	return f.canManage, nil
}

func visibilityRequest() KBRequest {
	return KBRequest{Caller: types.Caller{TenantID: 42, UserID: "user-read"}}
}

func visibilityKB() *types.KnowledgeBase {
	return &types.KnowledgeBase{ID: "kb-org", TenantID: 42}
}

func TestApplySameTenantVisibilityKeepsLegacyGrantWithoutChecker(t *testing.T) {
	ctx := context.Background()

	perm, err := ApplySameTenantVisibility(ctx, nil, visibilityRequest(), visibilityKB(), types.OrgRoleViewer, false)
	require.NoError(t, err)
	require.Equal(t, types.OrgRoleAdmin, perm, "nil checker preserves the legacy full-tenant grant")

	perm, err = ApplySameTenantVisibility(ctx, &fakeVisibility{canRead: false}, visibilityRequest(), visibilityKB(), types.OrgRoleViewer, true)
	require.NoError(t, err)
	require.Equal(t, types.OrgRoleAdmin, perm, "platform-privileged operators bypass visibility")

	perm, err = ApplySameTenantVisibility(ctx, &fakeVisibility{canRead: false}, visibilityRequest(), nil, types.OrgRoleEditor, false)
	require.NoError(t, err)
	require.Equal(t, types.OrgRoleAdmin, perm, "a nil KB keeps the legacy grant")
}

func TestApplySameTenantVisibilityDowngradesReadToViewer(t *testing.T) {
	ctx := context.Background()

	vis := &fakeVisibility{canRead: true, canManage: false}
	perm, err := ApplySameTenantVisibility(ctx, vis, visibilityRequest(), visibilityKB(), types.OrgRoleViewer, false)
	require.NoError(t, err)
	require.Equal(t, types.OrgRoleViewer, perm, "read-only access must not be upgraded")
	require.Equal(t, "user-read", vis.lastUser)
	require.Equal(t, "kb-org", vis.lastKBID)
	require.False(t, vis.lastPrivileged, "the checker must be used, not a bypass")

	vis = &fakeVisibility{canRead: true, canManage: true}
	perm, err = ApplySameTenantVisibility(ctx, vis, visibilityRequest(), visibilityKB(), types.OrgRoleViewer, false)
	require.NoError(t, err)
	require.Equal(t, types.OrgRoleAdmin, perm, "manage capability keeps the Admin projection")

	vis = &fakeVisibility{canRead: false}
	_, err = ApplySameTenantVisibility(ctx, vis, visibilityRequest(), visibilityKB(), types.OrgRoleViewer, false)
	require.ErrorIs(t, err, ErrForbidden)

	boom := errors.New("storage down")
	vis = &fakeVisibility{readErr: boom}
	_, err = ApplySameTenantVisibility(ctx, vis, visibilityRequest(), visibilityKB(), types.OrgRoleViewer, false)
	require.ErrorIs(t, err, boom, "checker errors must fail closed")
}

func TestApplySameTenantVisibilityRequiresManageForWriteRoutes(t *testing.T) {
	ctx := context.Background()

	vis := &fakeVisibility{canManage: true}
	perm, err := ApplySameTenantVisibility(ctx, vis, visibilityRequest(), visibilityKB(), types.OrgRoleEditor, false)
	require.NoError(t, err)
	require.Equal(t, types.OrgRoleAdmin, perm)

	vis = &fakeVisibility{canManage: false}
	_, err = ApplySameTenantVisibility(ctx, vis, visibilityRequest(), visibilityKB(), types.OrgRoleAdmin, false)
	require.ErrorIs(t, err, ErrForbidden, "a write route must reject manage-invisible KBs")

	boom := errors.New("storage down")
	vis = &fakeVisibility{manageErr: boom}
	_, err = ApplySameTenantVisibility(ctx, vis, visibilityRequest(), visibilityKB(), types.OrgRoleEditor, false)
	require.ErrorIs(t, err, boom)
}
