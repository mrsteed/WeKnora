package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// dupCheckRepo 实现全局重名检查所需的最小方法集，
// 其余接口方法保持零值桩（本测试路径不会触达）。
type dupCheckRepo struct {
	interfaces.OrgTreeRepository
	allNodes []*types.Organization
}

func (r *dupCheckRepo) ListByTenantID(ctx context.Context, tenantID uint64) ([]*types.Organization, error) {
	return r.allNodes, nil
}

func (r *dupCheckRepo) GetByIDAndTenant(ctx context.Context, id string, tenantID uint64) (*types.Organization, error) {
	return nil, errors.New("org parent not found (test stub)")
}

func (r *dupCheckRepo) Create(ctx context.Context, org *types.Organization) error {
	return errors.New("Create not expected in this test (test stub)")
}

type dupCreateProbe struct {
	*dupCheckRepo
	parent        *types.Organization
	createdCalled *bool
}

func (r *dupCreateProbe) GetByIDAndTenant(ctx context.Context, id string, tenantID uint64) (*types.Organization, error) {
	return r.parent, nil
}

func (r *dupCreateProbe) Create(ctx context.Context, org *types.Organization) error {
	*r.createdCalled = true
	return nil
}

func strPtr(s string) *string { return &s }

func TestHasDuplicateName(t *testing.T) {
	allNodes := []*types.Organization{
		{ID: "root-1", Name: "指挥中心"},
		{ID: "child-1", Name: "灭火科", ParentID: strPtr("root-1")},
		{ID: "root-2", Name: "Command Center"},
		{ID: "root-3", Name: "HQ Bureau"},
	}
	svc := &orgTreeService{orgTreeRepo: &dupCheckRepo{allNodes: allNodes}}

	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"根级精确命中", "指挥中心", true},
		{"子级跨层级命中（核心用例）", "灭火科", true},
		{"英文大小写不敏感（小写）", "command center", true},
		{"英文大小写不敏感（大写）", "COMMAND CENTER", true},
		{"首尾空白命中", "  HQ Bureau  ", true},
		{"不存在的名称", "后勤科", false},
		{"空白名称不匹配", "   ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.hasDuplicateName(context.Background(), 1, tc.query)
			if got != tc.want {
				t.Fatalf("hasDuplicateName(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestCreateNodeRejectsGlobalDuplicate(t *testing.T) {
	allNodes := []*types.Organization{
		{ID: "root-1", Name: "指挥中心"},
		{ID: "child-1", Name: "灭火科", ParentID: strPtr("root-1")},
	}
	svc := &orgTreeService{orgTreeRepo: &dupCheckRepo{allNodes: allNodes}}

	_, err := svc.CreateNode(context.Background(), 1, "user-1", &types.CreateOrgTreeNodeRequest{
		Name: "灭火科",
	})
	if !errors.Is(err, ErrOrgTreeNodeNameDuplicate) {
		t.Fatalf("global duplicate (cross-level): err = %v, want ErrOrgTreeNodeNameDuplicate", err)
	}

	_, err = svc.CreateNode(context.Background(), 1, "user-1", &types.CreateOrgTreeNodeRequest{
		Name: "后勤科",
	})
	if errors.Is(err, ErrOrgTreeNodeNameDuplicate) {
		t.Fatalf("unique name should not be rejected as duplicate, got: %v", err)
	}
}

func TestCreateNodeAllowsUniqueName(t *testing.T) {
	parentA := "parent-a"
	parent := &types.Organization{ID: parentA, Name: "指挥中心", Path: "/root-1", Level: 1}
	allNodes := []*types.Organization{
		{ID: "root-1", Name: "某支队"},
		{ID: parentA, Name: "指挥中心", ParentID: strPtr("root-1")},
		{ID: "c-others", Name: "灭火科", ParentID: strPtr(parentA)},
	}
	repo := &dupCheckRepo{allNodes: allNodes}
	created := false
	svc := &orgTreeService{orgTreeRepo: &dupCreateProbe{
		dupCheckRepo:  repo,
		parent:        parent,
		createdCalled: &created,
	}}

	org, err := svc.CreateNode(context.Background(), 1, "user-1", &types.CreateOrgTreeNodeRequest{
		Name:     "防火监督科",
		ParentID: strPtr(parentA),
	})
	if err != nil {
		t.Fatalf("CreateNode with unique name: unexpected error %v", err)
	}
	if org == nil || org.Name != "防火监督科" {
		t.Fatalf("CreateNode returned %+v, want name 防火监督科", org)
	}
	if !created {
		t.Fatal("repo.Create was not called for a non-duplicate name")
	}
}
