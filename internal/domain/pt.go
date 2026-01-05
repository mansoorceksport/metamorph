package domain

import (
	"context"
	"errors"
	"time"
)

// Common Errors
var (
	ErrPackageDepleted         = errors.New("pt contract has no remaining sessions")
	ErrInvalidSessionAmount    = errors.New("invalid session amount (must be 10, 20, 30, 40, or 50)")
	ErrScheduleNotFound        = errors.New("schedule not found")
	ErrContractNotFound        = errors.New("pt contract not found")
	ErrPackageTemplateNotFound = errors.New("pt package template not found")
	ErrUnauthorizedReschedule  = errors.New("unauthorized to reschedule this session")
	ErrBranchMismatch          = errors.New("branch mismatch: package, member, and coach must belong to the same branch")
)

// PT Package Constants
const (
	PackageStatusActive   = "Active"
	PackageStatusDepleted = "Depleted"
	PackageStatusExpired  = "Expired"
)

// Schedule Status Constants
const (
	ScheduleStatusScheduled           = "Scheduled"
	ScheduleStatusCompleted           = "Completed"
	ScheduleStatusNoShow              = "No-Show"
	ScheduleStatusPendingConfirmation = "Pending_Confirmation"
	ScheduleStatusCancelled           = "Cancelled"
)

// PTPackage represents a generic package Template offered by a Branch/Tenant
// e.g., "10 Sessions Promo - Downtown Branch"
type PTPackage struct {
	ID            string    `json:"id" bson:"_id,omitempty"`
	TenantID      string    `json:"tenant_id" bson:"tenant_id"`
	BranchID      string    `json:"branch_id" bson:"branch_id"` // Packages are often branch-specific for pricing/availability
	Name          string    `json:"name" bson:"name"`
	TotalSessions int       `json:"total_sessions" bson:"total_sessions"` // 10, 20, 30, 40, 50
	Price         float64   `json:"price" bson:"price"`
	Active        bool      `json:"active" bson:"active"` // If false, no new contracts can be created from this
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

// PTContract represents a specific purchase of a Package by a Member, assigned to a Coach
type PTContract struct {
	ID                string    `json:"id" bson:"_id,omitempty"`
	TenantID          string    `json:"tenant_id" bson:"tenant_id"`
	BranchID          string    `json:"branch_id" bson:"branch_id"`   // Inherited from Package/Member location
	PackageID         string    `json:"package_id" bson:"package_id"` // Reference to the Template
	MemberID          string    `json:"member_id" bson:"member_id"`
	CoachID           string    `json:"coach_id" bson:"coach_id"`
	TotalSessions     int       `json:"total_sessions" bson:"total_sessions"`         // Copied from Package at time of purchase
	RemainingSessions int       `json:"remaining_sessions" bson:"remaining_sessions"` // decrements on completion
	Price             float64   `json:"price" bson:"price"`                           // Copied from Package at time of purchase
	Status            string    `json:"status" bson:"status"`                         // Active, Depleted, Expired
	CreatedAt         time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" bson:"updated_at"`
}

// Schedule represents a single PT session, linked to a Contract
type Schedule struct {
	ID          string    `json:"id" bson:"_id,omitempty"`
	TenantID    string    `json:"tenant_id" bson:"tenant_id"`
	BranchID    string    `json:"branch_id" bson:"branch_id"`
	ContractID  string    `json:"contract_id" bson:"contract_id"` // Replaces PackageID reference
	CoachID     string    `json:"coach_id" bson:"coach_id"`
	MemberID    string    `json:"member_id" bson:"member_id"`
	StartTime   time.Time `json:"start_time" bson:"start_time"`
	EndTime     time.Time `json:"end_time" bson:"end_time"`
	Status      string    `json:"status" bson:"status"`
	SessionGoal string    `json:"session_goal,omitempty" bson:"session_goal,omitempty"` // e.g., "Leg Day - Hypertrophy Focus"
	Remarks     string    `json:"remarks,omitempty" bson:"remarks,omitempty"`           // Coach notes
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
}

// Repositories

type PTPackageRepository interface {
	Create(ctx context.Context, pkg *PTPackage) error
	GetByID(ctx context.Context, id string) (*PTPackage, error)
	GetByTenant(ctx context.Context, tenantID string) ([]*PTPackage, error)
	Update(ctx context.Context, pkg *PTPackage) error
}

// ContractWithMember represents a contract with embedded member info for client listing
type ContractWithMember struct {
	Contract *PTContract `json:"contract" bson:"contract"`
	Member   *User       `json:"member" bson:"member"`
}

type PTContractRepository interface {
	Create(ctx context.Context, contract *PTContract) error
	GetByID(ctx context.Context, id string) (*PTContract, error)
	GetActiveByMember(ctx context.Context, memberID string) ([]*PTContract, error)
	GetActiveByCoach(ctx context.Context, coachID string) ([]*PTContract, error)
	GetByTenant(ctx context.Context, tenantID string) ([]*PTContract, error)
	DecrementSession(ctx context.Context, contractID string) error
	UpdateStatus(ctx context.Context, contractID string, status string) error
	// GetLowSessionsByCoach returns contracts with remaining sessions below threshold
	GetLowSessionsByCoach(ctx context.Context, coachID string, threshold int) ([]*PTContract, error)
	// GetActiveContractsWithMembers returns contracts with embedded member info (optimized aggregation)
	GetActiveContractsWithMembers(ctx context.Context, coachID string) ([]*ContractWithMember, error)
	// GetFirstActiveContractByCoachAndMember finds the first active contract between a coach and member
	GetFirstActiveContractByCoachAndMember(ctx context.Context, coachID, memberID string) (*PTContract, error)
}

type ScheduleRepository interface {
	Create(ctx context.Context, schedule *Schedule) error
	GetByID(ctx context.Context, id string) (*Schedule, error)
	GetByCoach(ctx context.Context, coachID string, from, to time.Time) ([]*Schedule, error)
	GetByMember(ctx context.Context, memberID string, from, to time.Time) ([]*Schedule, error)
	List(ctx context.Context, tenantID string, filterOpts map[string]interface{}) ([]*Schedule, error)
	Update(ctx context.Context, schedule *Schedule) error
	UpdateStatus(ctx context.Context, id string, status string) error
	Delete(ctx context.Context, id string) error
	CountByContractAndStatus(ctx context.Context, contractID string, statuses []string) (int64, error)
	// GetAttendanceByCoach fetches all schedules for a coach within the last N days
	GetAttendanceByCoach(ctx context.Context, coachID string, days int) ([]*Schedule, error)
}
