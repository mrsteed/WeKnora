package types

import "time"

// WorkspaceBootstrapMode identifies which responsible-person onboarding path
// the tenant bootstrap flow is executing.
type WorkspaceBootstrapMode string

const (
	WorkspaceBootstrapModeExisting WorkspaceBootstrapMode = "existing"
	WorkspaceBootstrapModeNew      WorkspaceBootstrapMode = "new"
	WorkspaceBootstrapModeInvite   WorkspaceBootstrapMode = "invite"
)

func (m WorkspaceBootstrapMode) IsValid() bool {
	switch m {
	case WorkspaceBootstrapModeExisting, WorkspaceBootstrapModeNew, WorkspaceBootstrapModeInvite:
		return true
	default:
		return false
	}
}

// WorkspaceBootstrapStatus is the persisted operation state.
type WorkspaceBootstrapStatus string

const (
	WorkspaceBootstrapStatusPending        WorkspaceBootstrapStatus = "pending"
	WorkspaceBootstrapStatusInProgress     WorkspaceBootstrapStatus = "in_progress"
	WorkspaceBootstrapStatusCompleted      WorkspaceBootstrapStatus = "completed"
	WorkspaceBootstrapStatusPartialSuccess WorkspaceBootstrapStatus = "partial_success"
)

// WorkspaceBootstrapStepStatus is the per-step response status.
type WorkspaceBootstrapStepStatus string

const (
	WorkspaceBootstrapStepPending   WorkspaceBootstrapStepStatus = "pending"
	WorkspaceBootstrapStepSkipped   WorkspaceBootstrapStepStatus = "skipped"
	WorkspaceBootstrapStepCompleted WorkspaceBootstrapStepStatus = "completed"
	WorkspaceBootstrapStepReused    WorkspaceBootstrapStepStatus = "reused"
	WorkspaceBootstrapStepFailed    WorkspaceBootstrapStepStatus = "failed"
)

type WorkspaceBootstrapRequest struct {
	IdempotencyKey     string                 `json:"idempotency_key" binding:"required,min=8,max=128"`
	Mode               WorkspaceBootstrapMode `json:"mode" binding:"required"`
	TenantRole         string                 `json:"tenant_role" binding:"required"`
	ShouldSetupOrg     bool                   `json:"should_setup_org"`
	RootOrgID          string                 `json:"root_org_id,omitempty"`
	RootOrgName        string                 `json:"root_org_name,omitempty"`
	RootOrgDescription string                 `json:"root_org_description,omitempty"`
	OrgRole            string                 `json:"org_role,omitempty"`
	ExistingEmail      string                 `json:"existing_email,omitempty"`
	InviteMessage      string                 `json:"invite_message,omitempty"`
	NewUsername        string                 `json:"new_username,omitempty"`
	NewEmail           string                 `json:"new_email,omitempty"`
	NewPhone           string                 `json:"new_phone,omitempty"`
	NewPassword        string                 `json:"new_password,omitempty"`
}

type WorkspaceBootstrapStepResult struct {
	Status  WorkspaceBootstrapStepStatus `json:"status"`
	Message string                       `json:"message,omitempty"`
}

type WorkspaceBootstrapSteps struct {
	RootOrg          WorkspaceBootstrapStepResult `json:"root_org"`
	UserProvision    WorkspaceBootstrapStepResult `json:"user_provision"`
	TenantMembership WorkspaceBootstrapStepResult `json:"tenant_membership"`
	OrgAssignment    WorkspaceBootstrapStepResult `json:"org_assignment"`
	InviteLink       WorkspaceBootstrapStepResult `json:"invite_link"`
}

type WorkspaceBootstrapRootOrg struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reused bool   `json:"reused"`
}

type WorkspaceBootstrapResult struct {
	IdempotencyKey string                     `json:"idempotency_key"`
	Status         WorkspaceBootstrapStatus   `json:"status"`
	Mode           WorkspaceBootstrapMode     `json:"mode"`
	RootOrg        *WorkspaceBootstrapRootOrg `json:"root_org,omitempty"`
	User           *UserInfo                  `json:"user,omitempty"`
	InviteURL      string                     `json:"invite_url,omitempty"`
	Steps          WorkspaceBootstrapSteps    `json:"steps"`
}

type WorkspaceBootstrapResponse struct {
	Success bool                      `json:"success"`
	Data    *WorkspaceBootstrapResult `json:"data,omitempty"`
	Message string                    `json:"message,omitempty"`
}

// TenantBootstrapOperation persists a resumable tenant bootstrap workflow
// keyed by tenant + idempotency key so refresh/retry can continue from the
// last committed step.
type TenantBootstrapOperation struct {
	ID                   string                   `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID             uint64                   `json:"tenant_id" gorm:"not null;uniqueIndex:idx_tenant_bootstrap_tenant_key,priority:1;index"`
	IdempotencyKey       string                   `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex:idx_tenant_bootstrap_tenant_key,priority:2"`
	Mode                 WorkspaceBootstrapMode   `json:"mode" gorm:"type:varchar(16);not null;default:'existing'"`
	Status               WorkspaceBootstrapStatus `json:"status" gorm:"type:varchar(24);not null;default:'pending';index"`
	RequestHash          string                   `json:"request_hash" gorm:"type:varchar(64);default:''"`
	RequestPayload       JSON                     `json:"request_payload" gorm:"type:jsonb"`
	ResultPayload        JSON                     `json:"result_payload" gorm:"type:jsonb"`
	RootOrgID            string                   `json:"root_org_id" gorm:"type:varchar(36);default:''"`
	RootOrgName          string                   `json:"root_org_name" gorm:"type:varchar(255);default:''"`
	UserID               string                   `json:"user_id" gorm:"type:varchar(36);default:''"`
	InviteURL            string                   `json:"invite_url" gorm:"type:text;default:''"`
	TenantMembershipDone bool                     `json:"tenant_membership_done" gorm:"not null;default:false"`
	OrgAssignmentDone    bool                     `json:"org_assignment_done" gorm:"not null;default:false"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
}

func (TenantBootstrapOperation) TableName() string {
	return "tenant_bootstrap_operations"
}
