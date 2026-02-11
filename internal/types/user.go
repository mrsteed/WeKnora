package types

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	// Unique identifier of the user
	ID string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	// Username of the user
	Username string `json:"username"   gorm:"type:varchar(100);uniqueIndex;not null"`
	// Email address of the user
	Email string `json:"email"      gorm:"type:varchar(255);uniqueIndex;not null"`
	// Phone number of the user (optional)
	Phone string `json:"phone"      gorm:"type:varchar(20)"`
	// Hashed password of the user
	PasswordHash string `json:"-"          gorm:"type:varchar(255);not null"`
	// Avatar URL of the user
	Avatar string `json:"avatar"     gorm:"type:varchar(500)"`
	// Tenant ID that the user belongs to
	TenantID uint64 `json:"tenant_id"  gorm:"index"`
	// Whether the user is active
	IsActive bool `json:"is_active"  gorm:"default:true"`
	// Whether the user can access all tenants (cross-tenant access)
	CanAccessAllTenants bool `json:"can_access_all_tenants" gorm:"default:false"`
	// Whether the user is a super admin (platform-level privileges)
	IsSuperAdmin bool `json:"is_super_admin" gorm:"default:false"`
	// Creation time of the user
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the user
	UpdatedAt time.Time `json:"updated_at"`
	// Deletion time of the user
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Association relationship, not stored in the database
	Tenant *Tenant `json:"tenant,omitempty" gorm:"foreignKey:TenantID"`
}

// AuthToken represents an authentication token
type AuthToken struct {
	// Unique identifier of the token
	ID string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	// User ID that owns this token
	UserID string `json:"user_id"    gorm:"type:varchar(36);index;not null"`
	// Token value (JWT or other format)
	Token string `json:"token"      gorm:"type:text;not null"`
	// Token type (access_token, refresh_token)
	TokenType string `json:"token_type" gorm:"type:varchar(50);not null"`
	// Token expiration time
	ExpiresAt time.Time `json:"expires_at"`
	// Whether the token is revoked
	IsRevoked bool `json:"is_revoked" gorm:"default:false"`
	// Creation time of the token
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the token
	UpdatedAt time.Time `json:"updated_at"`

	// Association relationship
	User *User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message,omitempty"`
	User         *User   `json:"user,omitempty"`
	Tenant       *Tenant `json:"tenant,omitempty"`
	Token        string  `json:"token,omitempty"`
	RefreshToken string  `json:"refresh_token,omitempty"`
}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message,omitempty"`
	User    *User   `json:"user,omitempty"`
	Tenant  *Tenant `json:"tenant,omitempty"`
}

// UserInfo represents user information for API responses
type UserInfo struct {
	ID                  string    `json:"id"`
	Username            string    `json:"username"`
	Email               string    `json:"email"`
	Phone               string    `json:"phone"`
	Avatar              string    `json:"avatar"`
	TenantID            uint64    `json:"tenant_id"`
	IsActive            bool      `json:"is_active"`
	CanAccessAllTenants bool      `json:"can_access_all_tenants"`
	IsSuperAdmin        bool      `json:"is_super_admin"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// ToUserInfo converts User to UserInfo (without sensitive data)
func (u *User) ToUserInfo() *UserInfo {
	return &UserInfo{
		ID:                  u.ID,
		Username:            u.Username,
		Email:               u.Email,
		Phone:               u.Phone,
		Avatar:              u.Avatar,
		TenantID:            u.TenantID,
		IsActive:            u.IsActive,
		CanAccessAllTenants: u.CanAccessAllTenants,
		IsSuperAdmin:        u.IsSuperAdmin,
		CreatedAt:           u.CreatedAt,
		UpdatedAt:           u.UpdatedAt,
	}
}

// CreateUserInOrgRequest represents a request to create a new user and assign to an organization
type CreateUserInOrgRequest struct {
	Username string `json:"username" binding:"required,min=2,max=50"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password" binding:"required,min=8,max=32"`
	Role     string `json:"role" binding:"required"`
}

// CreateUserInOrgResponse represents the response for creating a user in an organization
type CreateUserInOrgResponse struct {
	Success bool      `json:"success"`
	Message string    `json:"message,omitempty"`
	User    *UserInfo `json:"user,omitempty"`
}

// UpdateUserInOrgRequest represents a request to update user information
type UpdateUserInOrgRequest struct {
	Username string `json:"username" binding:"required,min=2,max=50"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
}
