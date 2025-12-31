package domain

import (
	"context"
	"time"
)

// User represents a unified identity with multiple roles
type User struct {
	ID           string    `bson:"_id,omitempty" json:"id"`
	FirebaseUID  string    `bson:"firebase_uid,omitempty" json:"firebase_uid"`
	Email        string    `bson:"email" json:"email"`
	Name         string    `bson:"name" json:"name"`
	Roles        []string  `bson:"roles" json:"roles"` // ["coach", "member", "admin"]
	TenantID     string    `bson:"tenant_id" json:"tenant_id"`
	HomeBranchID string    `bson:"home_branch_id" json:"home_branch_id"` // For coaches
	BranchAccess []string  `bson:"branch_access" json:"branch_access"`   // For members: list of accessible branch IDs
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

// HasRole checks if user has a specific role
func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// UserRepository defines operations for managing users
type UserRepository interface {
	// Core CRUD operations
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByFirebaseUID(ctx context.Context, uid string) (*User, error)
	Update(ctx context.Context, user *User) error
	UpdateFirebaseUID(ctx context.Context, userID string, firebaseUID string) error
	Delete(ctx context.Context, id string) error

	// Upsert operations
	UpsertByFirebaseUID(ctx context.Context, user *User) error

	// Role management
	AddRole(ctx context.Context, userID string, role string) error
	RemoveRole(ctx context.Context, userID string, role string) error

	// Query operations
	GetAll(ctx context.Context) ([]*User, error)
	GetByRole(ctx context.Context, role string) ([]*User, error)
	GetByTenant(ctx context.Context, tenantID string) ([]*User, error)
	GetByTenantAndRole(ctx context.Context, tenantID string, role string) ([]*User, error)
}

// Role constants
const (
	RoleCoach       = "coach"
	RoleMember      = "member"
	RoleSuperAdmin  = "super_admin"  // Platform Owner (Metamorph) - no tenant restriction
	RoleTenantAdmin = "tenant_admin" // Gym Owner - restricted to specific tenant
)
