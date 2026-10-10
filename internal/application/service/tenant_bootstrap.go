package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func isDuplicateTenantBootstrap(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique constraint")
}

type tenantBootstrapService struct {
	repo interfaces.TenantBootstrapRepository
}

func NewTenantBootstrapService(repo interfaces.TenantBootstrapRepository) interfaces.TenantBootstrapService {
	return &tenantBootstrapService{repo: repo}
}

func (s *tenantBootstrapService) GetOrCreate(ctx context.Context, tenantID uint64, idempotencyKey string) (*types.TenantBootstrapOperation, bool, error) {
	if s.repo == nil {
		return nil, false, errors.New("tenant bootstrap repository unavailable")
	}
	existing, err := s.repo.GetByTenantAndKey(ctx, tenantID, idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}

	now := time.Now().UTC()
	op := &types.TenantBootstrapOperation{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		IdempotencyKey: idempotencyKey,
		Status:         types.WorkspaceBootstrapStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.Create(ctx, op); err != nil {
		if isDuplicateTenantBootstrap(err) {
			winner, getErr := s.repo.GetByTenantAndKey(ctx, tenantID, idempotencyKey)
			if getErr != nil {
				return nil, false, getErr
			}
			if winner != nil {
				return winner, false, nil
			}
			return nil, false, errors.New("tenant bootstrap operation already exists")
		}
		return nil, false, err
	}
	return op, true, nil
}

func (s *tenantBootstrapService) Save(ctx context.Context, op *types.TenantBootstrapOperation) error {
	if s.repo == nil {
		return errors.New("tenant bootstrap repository unavailable")
	}
	if op == nil {
		return errors.New("tenant bootstrap operation is required")
	}
	op.UpdatedAt = time.Now().UTC()
	return s.repo.Update(ctx, op)
}
