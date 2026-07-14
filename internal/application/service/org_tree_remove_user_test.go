package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubRemoveUserOrgTreeRepo struct {
	interfaces.OrgTreeRepository
	org *types.Organization
}

func (s *stubRemoveUserOrgTreeRepo) GetByIDAndTenant(_ context.Context, id string, tenantID uint64) (*types.Organization, error) {
	if s.org != nil && s.org.ID == id && s.org.OrgTenantID != nil && *s.org.OrgTenantID == tenantID {
		return s.org, nil
	}
	return nil, errors.New("not found")
}

type stubRemoveUserOrganizationRepo struct {
	interfaces.OrganizationRepository
	userOrgs         []*types.Organization
	removedOrgID     string
	removedUserID    string
	removeMemberErr  error
}

func (s *stubRemoveUserOrganizationRepo) ListOrgTreeOrganizationsByUserID(context.Context, string) ([]*types.Organization, error) {
	return s.userOrgs, nil
}

func (s *stubRemoveUserOrganizationRepo) RemoveOrgTreeMember(_ context.Context, orgID string, userID string) error {
	s.removedOrgID = orgID
	s.removedUserID = userID
	return s.removeMemberErr
	}

type stubRemoveUserTenantMemberService struct {
	interfaces.TenantMemberService
	membership        *types.TenantMember
	tenantMembers     []*types.TenantMember
	userMemberships   []*types.TenantMember
	removedUserID     string
	removedTenantID   uint64
	removeMemberErr   error
}

func (s *stubRemoveUserTenantMemberService) GetMembership(context.Context, string, uint64) (*types.TenantMember, error) {
	return s.membership, nil
}

func (s *stubRemoveUserTenantMemberService) ListByTenant(context.Context, uint64) ([]*types.TenantMember, error) {
	return s.tenantMembers, nil
}

func (s *stubRemoveUserTenantMemberService) RemoveMember(_ context.Context, userID string, tenantID uint64) error {
	s.removedUserID = userID
	s.removedTenantID = tenantID
	return s.removeMemberErr
}

func (s *stubRemoveUserTenantMemberService) ListByUser(context.Context, string) ([]*types.TenantMember, error) {
	return s.userMemberships, nil
}

type stubRemoveUserUserService struct {
	interfaces.UserService
	deletedUserID string
}

func (s *stubRemoveUserUserService) DeleteUser(_ context.Context, id string) error {
	s.deletedUserID = id
	return nil
}

func TestRemoveUserFromOrg_CascadesTenantMemberAndUserDelete(t *testing.T) {
	tenantID := uint64(42)
	org := &types.Organization{ID: "org-1", OwnerID: "owner", OrgTenantID: &tenantID}
	orgTreeRepo := &stubRemoveUserOrgTreeRepo{org: org}
	orgRepo := &stubRemoveUserOrganizationRepo{
		userOrgs: []*types.Organization{{ID: "org-1", OrgTenantID: &tenantID}},
	}
	tenantMembers := &stubRemoveUserTenantMemberService{
		membership: &types.TenantMember{UserID: "user-1", TenantID: tenantID, Role: types.TenantRoleContributor},
		userMemberships: nil,
	}
	userSvc := &stubRemoveUserUserService{}
	svc := &orgTreeService{
		orgTreeRepo:         orgTreeRepo,
		orgRepo:             orgRepo,
		tenantMemberService: tenantMembers,
		userService:         userSvc,
	}

	err := svc.RemoveUserFromOrg(context.Background(), "org-1", tenantID, &types.RemoveUserFromOrgRequest{UserID: "user-1"})
	if err != nil {
		t.Fatalf("RemoveUserFromOrg err = %v", err)
	}
	if orgRepo.removedOrgID != "org-1" || orgRepo.removedUserID != "user-1" {
		t.Fatalf("org member removal = (%q, %q), want (org-1, user-1)", orgRepo.removedOrgID, orgRepo.removedUserID)
	}
	if tenantMembers.removedUserID != "user-1" || tenantMembers.removedTenantID != tenantID {
		t.Fatalf("tenant member removal = (%q, %d), want (user-1, %d)", tenantMembers.removedUserID, tenantMembers.removedTenantID, tenantID)
	}
	if userSvc.deletedUserID != "user-1" {
		t.Fatalf("deleted user = %q, want user-1", userSvc.deletedUserID)
	}
}

func TestRemoveUserFromOrg_SkipsTenantCascadeWhenOtherOrgRemains(t *testing.T) {
	tenantID := uint64(42)
	otherTenantID := uint64(7)
	org := &types.Organization{ID: "org-1", OwnerID: "owner", OrgTenantID: &tenantID}
	orgTreeRepo := &stubRemoveUserOrgTreeRepo{org: org}
	orgRepo := &stubRemoveUserOrganizationRepo{
		userOrgs: []*types.Organization{
			{ID: "org-1", OrgTenantID: &tenantID},
			{ID: "org-2", OrgTenantID: &tenantID},
			{ID: "org-x", OrgTenantID: &otherTenantID},
		},
	}
	tenantMembers := &stubRemoveUserTenantMemberService{}
	userSvc := &stubRemoveUserUserService{}
	svc := &orgTreeService{
		orgTreeRepo:         orgTreeRepo,
		orgRepo:             orgRepo,
		tenantMemberService: tenantMembers,
		userService:         userSvc,
	}

	err := svc.RemoveUserFromOrg(context.Background(), "org-1", tenantID, &types.RemoveUserFromOrgRequest{UserID: "user-1"})
	if err != nil {
		t.Fatalf("RemoveUserFromOrg err = %v", err)
	}
	if tenantMembers.removedUserID != "" {
		t.Fatalf("tenant membership should not be removed, got %q", tenantMembers.removedUserID)
	}
	if userSvc.deletedUserID != "" {
		t.Fatalf("user should not be soft deleted, got %q", userSvc.deletedUserID)
	}
}

func TestRemoveUserFromOrg_BlocksLastTenantOwnerCascade(t *testing.T) {
	tenantID := uint64(42)
	org := &types.Organization{ID: "org-1", OwnerID: "other-owner", OrgTenantID: &tenantID}
	orgTreeRepo := &stubRemoveUserOrgTreeRepo{org: org}
	orgRepo := &stubRemoveUserOrganizationRepo{
		userOrgs: []*types.Organization{{ID: "org-1", OrgTenantID: &tenantID}},
	}
	tenantMembers := &stubRemoveUserTenantMemberService{
		membership:    &types.TenantMember{UserID: "owner-1", TenantID: tenantID, Role: types.TenantRoleOwner},
		tenantMembers: []*types.TenantMember{{UserID: "owner-1", TenantID: tenantID, Role: types.TenantRoleOwner}},
	}
	svc := &orgTreeService{
		orgTreeRepo:         orgTreeRepo,
		orgRepo:             orgRepo,
		tenantMemberService: tenantMembers,
	}

	err := svc.RemoveUserFromOrg(context.Background(), "org-1", tenantID, &types.RemoveUserFromOrgRequest{UserID: "owner-1"})
	if !errors.Is(err, ErrLastOwner) {
		t.Fatalf("RemoveUserFromOrg err = %v, want ErrLastOwner", err)
	}
	if orgRepo.removedOrgID != "" {
		t.Fatalf("org membership should stay intact on last-owner rejection, got removal of %q", orgRepo.removedOrgID)
	}
}