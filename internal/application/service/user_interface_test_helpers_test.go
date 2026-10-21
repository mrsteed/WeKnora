package service

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
)

func (s *stubUserRepoForAuth) GetUserByPhone(_ context.Context, phone string) (*types.User, error) {
	for _, user := range s.users {
		if user != nil && user.Phone == phone {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
}

func (r *cleanupUserRepo) GetUserByPhone(context.Context, string) (*types.User, error) {
	return nil, nil
}
