package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type stubCreateUserAdminRepo struct {
	usernameUser *types.User
	emailUser    *types.User
	phoneUser    *types.User
	createErr    error
}

func (r *stubCreateUserAdminRepo) CreateUser(context.Context, *types.User) error { return r.createErr }
func (r *stubCreateUserAdminRepo) GetUserByID(context.Context, string) (*types.User, error) {
	return nil, nil
}
func (r *stubCreateUserAdminRepo) GetUsersByIDs(context.Context, []string) (map[string]*types.User, error) {
	return nil, nil
}
func (r *stubCreateUserAdminRepo) GetUserByPhone(context.Context, string) (*types.User, error) {
	return r.phoneUser, nil
}
func (r *stubCreateUserAdminRepo) GetUserByEmail(context.Context, string) (*types.User, error) {
	return r.emailUser, nil
}
func (r *stubCreateUserAdminRepo) GetUserByUsername(context.Context, string) (*types.User, error) {
	return r.usernameUser, nil
}
func (r *stubCreateUserAdminRepo) GetUserByTenantID(context.Context, uint64) (*types.User, error) {
	return nil, nil
}
func (r *stubCreateUserAdminRepo) UpdateUser(context.Context, *types.User) error { return nil }
func (r *stubCreateUserAdminRepo) DeleteUser(context.Context, string) error      { return nil }
func (r *stubCreateUserAdminRepo) ListUsers(context.Context, int, int) ([]*types.User, error) {
	return nil, nil
}
func (r *stubCreateUserAdminRepo) ListSystemAdmins(context.Context, int, int) ([]*types.User, int64, error) {
	return nil, 0, nil
}
func (r *stubCreateUserAdminRepo) RevokeSystemAdmin(context.Context, string, string) (*types.User, error) {
	return nil, nil
}
func (r *stubCreateUserAdminRepo) SearchUsers(context.Context, string, int) ([]*types.User, error) {
	return nil, nil
}

func TestCreateUserByAdminRejectsDuplicatePhoneBeforeInsert(t *testing.T) {
	svc := &userService{
		userRepo: &stubCreateUserAdminRepo{
			phoneUser: &types.User{ID: "existing-phone"},
		},
	}

	_, err := svc.CreateUserByAdmin(context.Background(), &types.CreateUserInOrgRequest{
		Username: "new-user",
		Phone:    "13800000001",
		Password: "pass1234",
	}, 10004)
	if !errors.Is(err, ErrPhoneAlreadyExists) {
		t.Fatalf("CreateUserByAdmin duplicate phone err = %v, want ErrPhoneAlreadyExists", err)
	}
}

func TestCreateUserByAdminMapsUniqueConstraintToSpecificField(t *testing.T) {
	tests := []struct {
		name      string
		createErr error
		wantErr   error
	}{
		{
			name:      "phone unique index",
			createErr: errors.New(`ERROR: duplicate key value violates unique constraint "idx_users_phone_active_unique"`),
			wantErr:   ErrPhoneAlreadyExists,
		},
		{
			name:      "email unique index",
			createErr: errors.New(`UNIQUE constraint failed: users.email`),
			wantErr:   ErrEmailAlreadyExists,
		},
		{
			name:      "username unique index",
			createErr: errors.New(`Duplicate entry 'ops-admin' for key 'idx_users_username_active_unique'`),
			wantErr:   ErrUsernameAlreadyExists,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &userService{
				userRepo: &stubCreateUserAdminRepo{createErr: tc.createErr},
			}

			_, err := svc.CreateUserByAdmin(context.Background(), &types.CreateUserInOrgRequest{
				Username: "ops-admin",
				Email:    "ops@example.com",
				Phone:    "13800000002",
				Password: "pass1234",
			}, 10004)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CreateUserByAdmin(%s) err = %v, want %v", tc.name, err, tc.wantErr)
			}
		})
	}
}
