package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type stubTenantBootstrapRepo struct {
	rows map[string]*types.TenantBootstrapOperation
}

func (r *stubTenantBootstrapRepo) Create(_ context.Context, op *types.TenantBootstrapOperation) error {
	if r.rows == nil {
		r.rows = map[string]*types.TenantBootstrapOperation{}
	}
	r.rows[op.IdempotencyKey] = op
	return nil
}

func (r *stubTenantBootstrapRepo) GetByTenantAndKey(_ context.Context, tenantID uint64, idempotencyKey string) (*types.TenantBootstrapOperation, error) {
	if r.rows == nil {
		return nil, nil
	}
	op := r.rows[idempotencyKey]
	if op == nil || op.TenantID != tenantID {
		return nil, nil
	}
	cp := *op
	return &cp, nil
}

func (r *stubTenantBootstrapRepo) Update(_ context.Context, op *types.TenantBootstrapOperation) error {
	if r.rows == nil {
		r.rows = map[string]*types.TenantBootstrapOperation{}
	}
	cp := *op
	r.rows[op.IdempotencyKey] = &cp
	return nil
}

func TestTenantBootstrapServiceGetOrCreateIsIdempotent(t *testing.T) {
	svc := NewTenantBootstrapService(&stubTenantBootstrapRepo{})
	ctx := context.Background()

	first, created, err := svc.GetOrCreate(ctx, 10004, "setup-1")
	if err != nil {
		t.Fatalf("first GetOrCreate err = %v", err)
	}
	if !created || first == nil {
		t.Fatalf("first GetOrCreate created=%v op=%v, want created op", created, first)
	}

	second, created, err := svc.GetOrCreate(ctx, 10004, "setup-1")
	if err != nil {
		t.Fatalf("second GetOrCreate err = %v", err)
	}
	if created {
		t.Fatalf("second GetOrCreate created=%v, want false", created)
	}
	if second == nil || second.ID != first.ID {
		t.Fatalf("second GetOrCreate op=%+v, want same id %q", second, first.ID)
	}
}
