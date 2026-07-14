package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type stubInvitationService struct {
	interfaces.TenantInvitationService
	create          func(ctx context.Context, tenantID uint64, inviteeUserID string, role types.TenantRole, invitedBy *string, message string) (*types.TenantInvitation, error)
	getByID         func(ctx context.Context, invID uint64) (*types.TenantInvitation, error)
	revoke          func(ctx context.Context, invID uint64) error
	createShareLink func(ctx context.Context, tenantID uint64, role types.TenantRole, invitedBy *string, message string) (*types.TenantInvitation, string, error)
}

func (s *stubInvitationService) Create(ctx context.Context, tenantID uint64, inviteeUserID string, role types.TenantRole, invitedBy *string, message string) (*types.TenantInvitation, error) {
	return s.create(ctx, tenantID, inviteeUserID, role, invitedBy, message)
}

func (s *stubInvitationService) GetByID(ctx context.Context, invID uint64) (*types.TenantInvitation, error) {
	return s.getByID(ctx, invID)
}

func (s *stubInvitationService) Revoke(ctx context.Context, invID uint64) error {
	return s.revoke(ctx, invID)
}

func (s *stubInvitationService) CreateShareLink(ctx context.Context, tenantID uint64, role types.TenantRole, invitedBy *string, message string) (*types.TenantInvitation, string, error) {
	return s.createShareLink(ctx, tenantID, role, invitedBy, message)
}

func newTestInvitationHandler(inv interfaces.TenantInvitationService, us interfaces.UserService) *TenantInvitationHandler {
	return NewTenantInvitationHandler(inv, us, nil, &config.Config{})
}

func invitationTestRouter(h *TenantInvitationHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	tenantByID := r.Group("/tenants/:id", middleware.RequirePathTenantMatch(&config.Config{
		Tenant: &config.TenantConfig{EnableCrossTenantAccess: true},
	}))
	tenantByID.POST("/invitations", h.CreateInvitation)
	tenantByID.DELETE("/invitations/:inv_id", h.RevokeInvitation)
	tenantByID.POST("/invite-links", h.CreateInviteLink)
	return r
}

func TestTenantInvitation_CreateInvitation_AdminCanGrantAdmin(t *testing.T) {
	caller := "u-admin"
	now := time.Now()
	invSvc := &stubInvitationService{
		create: func(_ context.Context, tenantID uint64, inviteeUserID string, role types.TenantRole, invitedBy *string, _ string) (*types.TenantInvitation, error) {
			if tenantID != 1 || inviteeUserID != "u-bob" || role != types.TenantRoleAdmin {
				t.Fatalf("unexpected invitation create args: tenant=%d invitee=%s role=%s", tenantID, inviteeUserID, role)
			}
			if invitedBy == nil || *invitedBy != caller {
				t.Fatalf("invited_by must be caller, got %v", invitedBy)
			}
			return &types.TenantInvitation{
				ID:            7,
				TenantID:      tenantID,
				InviteeUserID: inviteeUserID,
				InvitedBy:     invitedBy,
				Role:          role,
				Status:        types.TenantInvitationStatusPending,
				ExpiresAt:     now.Add(time.Hour),
				CreatedAt:     now,
			}, nil
		},
	}
	us := &stubMemberUserService{
		getByEmail: func(_ context.Context, email string) (*types.User, error) {
			return &types.User{ID: "u-bob", Email: email, Username: "bob"}, nil
		},
		getByID: func(_ context.Context, id string) (*types.User, error) {
			return &types.User{ID: id, Email: id + "@x.com", Username: id}, nil
		},
	}
	h := newTestInvitationHandler(invSvc, us)
	body := map[string]any{"email": "bob@x.com", "role": "admin"}
	w := doJSONWithCtx(t, invitationTestRouter(h), http.MethodPost, "/tenants/1/invitations", body,
		memberCtxOpts{callerID: caller, tenantRole: types.TenantRoleAdmin})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTenantInvitation_CreateInvitation_AdminCannotGrantOwner(t *testing.T) {
	called := false
	us := &stubMemberUserService{
		getByEmail: func(_ context.Context, _ string) (*types.User, error) {
			called = true
			return &types.User{ID: "u-bob"}, nil
		},
	}
	h := newTestInvitationHandler(&stubInvitationService{}, us)
	body := map[string]any{"email": "bob@x.com", "role": "owner"}
	w := doJSONWithCtx(t, invitationTestRouter(h), http.MethodPost, "/tenants/1/invitations", body,
		memberCtxOpts{callerID: "u-admin", tenantRole: types.TenantRoleAdmin})
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin granting owner invite must 403, got %d body=%s", w.Code, w.Body.String())
	}
	if called {
		t.Fatalf("user lookup must not run when invitation role authorization fails")
	}
}

func TestTenantInvitation_RevokeInvitation_AdminCannotRevokeOwnerInvite(t *testing.T) {
	invSvc := &stubInvitationService{
		getByID: func(_ context.Context, invID uint64) (*types.TenantInvitation, error) {
			return &types.TenantInvitation{ID: invID, TenantID: 1, Role: types.TenantRoleOwner, Status: types.TenantInvitationStatusPending}, nil
		},
		revoke: func(context.Context, uint64) error {
			t.Fatalf("Revoke must not be reached")
			return nil
		},
	}
	h := newTestInvitationHandler(invSvc, &stubMemberUserService{})
	w := doJSONWithCtx(t, invitationTestRouter(h), http.MethodDelete, "/tenants/1/invitations/7", nil,
		memberCtxOpts{callerID: "u-admin", tenantRole: types.TenantRoleAdmin})
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin revoking owner invite must 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTenantInvitation_CreateInviteLink_AdminCanGrantAdmin(t *testing.T) {
	caller := "u-admin"
	now := time.Now()
	invSvc := &stubInvitationService{
		createShareLink: func(_ context.Context, tenantID uint64, role types.TenantRole, invitedBy *string, _ string) (*types.TenantInvitation, string, error) {
			if tenantID != 1 || role != types.TenantRoleAdmin {
				t.Fatalf("unexpected share-link args: tenant=%d role=%s", tenantID, role)
			}
			if invitedBy == nil || *invitedBy != caller {
				t.Fatalf("invited_by must be caller, got %v", invitedBy)
			}
			return &types.TenantInvitation{
				ID:        9,
				TenantID:  tenantID,
				InvitedBy: invitedBy,
				Role:      role,
				Status:    types.TenantInvitationStatusPending,
				ExpiresAt: now.Add(time.Hour),
				CreatedAt: now,
				Token:     "plain-token",
			}, "plain-token", nil
		},
	}
	us := &stubMemberUserService{
		getByID: func(_ context.Context, id string) (*types.User, error) {
			return &types.User{ID: id, Email: id + "@x.com", Username: id}, nil
		},
	}
	h := newTestInvitationHandler(invSvc, us)
	body := map[string]any{"role": "admin"}
	w := doJSONWithCtx(t, invitationTestRouter(h), http.MethodPost, "/tenants/1/invite-links", body,
		memberCtxOpts{callerID: caller, tenantRole: types.TenantRoleAdmin})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTenantInvitation_CreateInviteLink_AdminCannotGrantOwner(t *testing.T) {
	invSvc := &stubInvitationService{
		createShareLink: func(context.Context, uint64, types.TenantRole, *string, string) (*types.TenantInvitation, string, error) {
			t.Fatalf("CreateShareLink must not be reached")
			return nil, "", nil
		},
	}
	h := newTestInvitationHandler(invSvc, &stubMemberUserService{})
	body := map[string]any{"role": "owner"}
	w := doJSONWithCtx(t, invitationTestRouter(h), http.MethodPost, "/tenants/1/invite-links", body,
		memberCtxOpts{callerID: "u-admin", tenantRole: types.TenantRoleAdmin})
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin granting owner invite link must 403, got %d body=%s", w.Code, w.Body.String())
	}
}