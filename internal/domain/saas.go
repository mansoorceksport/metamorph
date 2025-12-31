package domain

import (
	"context"
	"time"
)

// Tenant represents a gym brand using the platform
type Tenant struct {
	ID         string     `bson:"_id,omitempty" json:"id"`
	Name       string     `bson:"name" json:"name"`
	JoinCode   string     `bson:"join_code" json:"join_code"` // Unique code for members to join
	LogoURL    string     `bson:"logo_url" json:"logo_url"`
	AISettings AISettings `bson:"ai_settings" json:"ai_settings"`
	CreatedAt  time.Time  `bson:"created_at" json:"created_at"`
}

// AISettings defines the persona and style for the AI digitizer
type AISettings struct {
	Tone    string `bson:"tone" json:"tone"`       // e.g., "Encouraging", "Aggressive", "Tactical"
	Style   string `bson:"style" json:"style"`     // e.g., "Concise", "Detailed"
	Persona string `bson:"persona" json:"persona"` // e.g., "Drill Sergeant", "Supportive Coach"
}

// CoachAssignment represents a link between a coach and a member
type CoachAssignment struct {
	ID         string    `bson:"_id,omitempty" json:"id"`
	CoachID    string    `bson:"coach_id" json:"coach_id"`   // User ID with coach role
	MemberID   string    `bson:"member_id" json:"member_id"` // User ID from users collection
	TenantID   string    `bson:"tenant_id" json:"tenant_id"` // Ensure assignment is within same tenant
	AssignedAt time.Time `bson:"assigned_at" json:"assigned_at"`
}

// TenantRepository defines operations for managing tenants
type TenantRepository interface {
	Create(ctx context.Context, tenant *Tenant) error
	GetByID(ctx context.Context, id string) (*Tenant, error)
	GetByJoinCode(ctx context.Context, code string) (*Tenant, error)
	GetAll(ctx context.Context) ([]*Tenant, error)
	Update(ctx context.Context, tenant *Tenant) error
}

// Branch represents a specific location within a tenant
type Branch struct {
	ID        string    `bson:"_id,omitempty" json:"id"`
	TenantID  string    `bson:"tenant_id" json:"tenant_id"`
	Name      string    `bson:"name" json:"name"`
	JoinCode  string    `bson:"join_code" json:"join_code"` // Unique code for members to join branch
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// BranchRepository defines operations for managing branches
type BranchRepository interface {
	Create(ctx context.Context, branch *Branch) error
	GetByID(ctx context.Context, id string) (*Branch, error)
	GetByJoinCode(ctx context.Context, code string) (*Branch, error)
	GetByTenantID(ctx context.Context, tenantID string) ([]*Branch, error)
	Update(ctx context.Context, branch *Branch) error
	Delete(ctx context.Context, id string) error
	GetAll(ctx context.Context) ([]*Branch, error)
}

// AssignmentRepository defines operations for managing coach-member links
type AssignmentRepository interface {
	Create(ctx context.Context, assignment *CoachAssignment) error
	GetByID(ctx context.Context, id string) (*CoachAssignment, error)
	Delete(ctx context.Context, id string) error
	GetByMemberID(ctx context.Context, memberID string) (*CoachAssignment, error)
	GetByCoachID(ctx context.Context, coachID string) ([]*CoachAssignment, error)
	// FindMembersByCoach retrieves actual User objects for a coach's roster
	FindMembersByCoach(ctx context.Context, coachID string) ([]*User, error)
}
