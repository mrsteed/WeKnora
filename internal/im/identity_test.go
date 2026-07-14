package im

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubIMUserService struct {
	interfaces.UserService
	user *types.User
	err  error
}

func (s *stubIMUserService) GetUserByID(context.Context, string) (*types.User, error) {
	return s.user, s.err
}

func TestWithIMIdentity(t *testing.T) {
	const tenantID uint64 = 42
	msg := &IncomingMessage{Platform: PlatformFeishu, UserID: "open-id-1"}
	ctx := withIMIdentity(context.Background(), tenantID, "channel-1", msg)

	gotTenant, ok := types.TenantIDFromContext(ctx)
	if !ok || gotTenant != tenantID {
		t.Fatalf("TenantID = %d (ok=%v), want %d", gotTenant, ok, tenantID)
	}

	userID, ok := types.UserIDFromContext(ctx)
	if !ok || userID == "" {
		t.Fatalf("UserID = %q (ok=%v), want non-empty synthetic user", userID, ok)
	}
	if want := "system-42"; userID != want {
		t.Fatalf("UserID = %q, want %q", userID, want)
	}

	// The synthetic shape must be recognised so RBAC code does not record it
	// as a resource creator.
	if !types.IsSyntheticUserID(userID) {
		t.Fatalf("IsSyntheticUserID(%q) = false, want true", userID)
	}

	// Non-empty UserID is the gate the shared-KB resolution relies on; without
	// it Organization-shared KBs are silently skipped on the IM path.
	if role := types.TenantRoleFromContext(ctx); role != types.TenantRoleViewer {
		t.Fatalf("TenantRole = %v, want %v", role, types.TenantRoleViewer)
	}

	principal, ok := types.PrincipalFromContext(ctx)
	if !ok {
		t.Fatalf("Principal missing")
	}
	if principal.Type != types.PrincipalIMUser || principal.ID != "42:channel-1:feishu:open-id-1" {
		t.Fatalf("Principal = %#v, want im_user for the external IM user", principal)
	}
}

func TestResourceAuthUserDoesNotChangeIMSessionOwner(t *testing.T) {
	ctx := withIMIdentity(context.Background(), 42, "channel-1", &IncomingMessage{Platform: PlatformFeishu, UserID: "open-id-1"})
	ctx = types.WithResourceAuthUserID(ctx, "creator-1")
	ctx = types.WithResourceAuthUser(ctx, &types.User{ID: "creator-1"})

	if got := types.SessionOwnerIDFromContext(ctx); got != "system-42" {
		t.Fatalf("SessionOwnerIDFromContext() = %q, want %q", got, "system-42")
	}
	if got, ok := types.ResourceAuthUserIDFromContext(ctx); !ok || got != "creator-1" {
		t.Fatalf("ResourceAuthUserIDFromContext() = %q (ok=%v), want creator-1", got, ok)
	}
	if user, ok := types.ResourceAuthUserFromContext(ctx); !ok || user == nil || user.ID != "creator-1" {
		t.Fatalf("ResourceAuthUserFromContext() = %#v (ok=%v), want creator-1", user, ok)
	}
}

func TestWithIMResourceAuthContextLoadsChannelCreator(t *testing.T) {
	svc := &Service{userService: &stubIMUserService{user: &types.User{ID: "creator-1"}}}
	ctx := withIMIdentity(context.Background(), 42, "channel-1", &IncomingMessage{Platform: PlatformFeishu, UserID: "open-id-1"})
	ctx = svc.withIMResourceAuthContext(ctx, &IMChannel{ID: "ch-1", CreatedBy: "creator-1"})

	if got := types.SessionOwnerIDFromContext(ctx); got != "system-42" {
		t.Fatalf("SessionOwnerIDFromContext() = %q, want %q", got, "system-42")
	}
	if got, ok := types.ResourceAuthUserIDFromContext(ctx); !ok || got != "creator-1" {
		t.Fatalf("ResourceAuthUserIDFromContext() = %q (ok=%v), want creator-1", got, ok)
	}
	if user, ok := types.ResourceAuthUserFromContext(ctx); !ok || user == nil || user.ID != "creator-1" {
		t.Fatalf("ResourceAuthUserFromContext() = %#v (ok=%v), want creator-1", user, ok)
	}
}
