package service

import (
	"context"
	"testing"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubTenantDeleteRepo struct {
	interfaces.TenantRepository
	tenant    *types.Tenant
	deletedID uint64
	onDelete  func()
}

func (s *stubTenantDeleteRepo) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	if s.tenant != nil && s.tenant.ID == id {
		return s.tenant, nil
	}
	return nil, apprepo.ErrUserNotFound
}

func (s *stubTenantDeleteRepo) DeleteTenant(_ context.Context, id uint64) error {
	s.deletedID = id
	if s.onDelete != nil {
		s.onDelete()
	}
	return nil
}

type stubTenantDeleteMemberService struct {
	interfaces.TenantMemberService
	byTenant map[uint64][]*types.TenantMember
	byUser   map[string][]*types.TenantMember
}

func (s *stubTenantDeleteMemberService) ListByTenant(_ context.Context, tenantID uint64) ([]*types.TenantMember, error) {
	return s.byTenant[tenantID], nil
}

func (s *stubTenantDeleteMemberService) ListByUser(_ context.Context, userID string) ([]*types.TenantMember, error) {
	return s.byUser[userID], nil
}

type stubTenantDeleteUserRepo struct {
	interfaces.UserRepository
	users       map[string]*types.User
	updatedIDs  []string
	deletedIDs  []string
	updatedHome []uint64
}

func (s *stubTenantDeleteUserRepo) GetUserByID(_ context.Context, id string) (*types.User, error) {
	if user, ok := s.users[id]; ok {
		clone := *user
		return &clone, nil
	}
	return nil, apprepo.ErrUserNotFound
}

func (s *stubTenantDeleteUserRepo) UpdateUser(_ context.Context, user *types.User) error {
	clone := *user
	s.users[user.ID] = &clone
	s.updatedIDs = append(s.updatedIDs, user.ID)
	s.updatedHome = append(s.updatedHome, user.TenantID)
	return nil
}

func (s *stubTenantDeleteUserRepo) DeleteUser(_ context.Context, id string) error {
	s.deletedIDs = append(s.deletedIDs, id)
	delete(s.users, id)
	return nil
}

func TestDeleteTenant_RehomesHomeTenantWhenOtherMembershipExists(t *testing.T) {
	memberSvc := &stubTenantDeleteMemberService{
		byTenant: map[uint64][]*types.TenantMember{
			1: {{UserID: "u-1", TenantID: 1, Status: types.TenantMemberStatusActive}},
		},
		byUser: map[string][]*types.TenantMember{
			"u-1": {{UserID: "u-1", TenantID: 2, Status: types.TenantMemberStatusActive}},
		},
	}
	userRepo := &stubTenantDeleteUserRepo{users: map[string]*types.User{
		"u-1": {ID: "u-1", TenantID: 1, Username: "alice", Email: "alice@example.com"},
	}}
	repo := &stubTenantDeleteRepo{tenant: &types.Tenant{ID: 1, Name: "gone"}}
	svc := &tenantService{repo: repo, memberService: memberSvc, userRepo: userRepo}

	if err := svc.DeleteTenant(context.Background(), 1); err != nil {
		t.Fatalf("DeleteTenant err = %v", err)
	}
	if len(userRepo.deletedIDs) != 0 {
		t.Fatalf("unexpected deleted users: %v", userRepo.deletedIDs)
	}
	if got := userRepo.users["u-1"].TenantID; got != 2 {
		t.Fatalf("home tenant = %d, want 2", got)
	}
}

func TestDeleteTenant_SoftDeletesUserWhenNoMembershipRemains(t *testing.T) {
	memberSvc := &stubTenantDeleteMemberService{
		byTenant: map[uint64][]*types.TenantMember{
			1: {{UserID: "u-1", TenantID: 1, Status: types.TenantMemberStatusActive}},
		},
		byUser: map[string][]*types.TenantMember{},
	}
	userRepo := &stubTenantDeleteUserRepo{users: map[string]*types.User{
		"u-1": {ID: "u-1", TenantID: 1, Username: "alice", Email: "alice@example.com"},
	}}
	repo := &stubTenantDeleteRepo{tenant: &types.Tenant{ID: 1, Name: "gone"}}
	svc := &tenantService{repo: repo, memberService: memberSvc, userRepo: userRepo}

	if err := svc.DeleteTenant(context.Background(), 1); err != nil {
		t.Fatalf("DeleteTenant err = %v", err)
	}
	if len(userRepo.deletedIDs) != 1 || userRepo.deletedIDs[0] != "u-1" {
		t.Fatalf("deleted users = %v, want [u-1]", userRepo.deletedIDs)
	}
}

func TestDeleteTenant_KeepsPlatformPrivilegedUserButClearsDanglingHomeTenant(t *testing.T) {
	memberSvc := &stubTenantDeleteMemberService{
		byTenant: map[uint64][]*types.TenantMember{
			1: {{UserID: "u-1", TenantID: 1, Status: types.TenantMemberStatusActive}},
		},
		byUser: map[string][]*types.TenantMember{},
	}
	userRepo := &stubTenantDeleteUserRepo{users: map[string]*types.User{
		"u-1": {ID: "u-1", TenantID: 1, Username: "root", Email: "root@example.com", IsSuperAdmin: true},
	}}
	repo := &stubTenantDeleteRepo{tenant: &types.Tenant{ID: 1, Name: "gone"}}
	svc := &tenantService{repo: repo, memberService: memberSvc, userRepo: userRepo}

	if err := svc.DeleteTenant(context.Background(), 1); err != nil {
		t.Fatalf("DeleteTenant err = %v", err)
	}
	if len(userRepo.deletedIDs) != 0 {
		t.Fatalf("platform user should not be deleted: %v", userRepo.deletedIDs)
	}
	if got := userRepo.users["u-1"].TenantID; got != 0 {
		t.Fatalf("home tenant = %d, want 0", got)
	}
}