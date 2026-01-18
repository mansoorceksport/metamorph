package domain

import (
	"context"
	"time"
)

// SetLogDocument represents a set log as a standalone document in the set_logs collection
// This enables atomic updates without affecting the parent PlannedExercise
type SetLogDocument struct {
	ID                string     `json:"id" bson:"_id,omitempty"`
	ClientID          string     `json:"client_id,omitempty" bson:"client_id,omitempty"` // Frontend ULID for dual-identity
	PlannedExerciseID string     `json:"planned_exercise_id" bson:"planned_exercise_id"` // Reference to PlannedExercise
	ScheduleID        string     `json:"schedule_id" bson:"schedule_id"`                 // Reference to Schedule for querying
	MemberID          string     `json:"member_id" bson:"member_id"`                     // For PB tracking
	ExerciseID        string     `json:"exercise_id" bson:"exercise_id"`                 // Reference to exercise definition (for PB)
	SetIndex          int        `json:"set_index" bson:"set_index"`                     // 1-based index for display
	Weight            float64    `json:"weight" bson:"weight"`
	Reps              int        `json:"reps" bson:"reps"`
	Remarks           string     `json:"remarks" bson:"remarks"`
	Completed         bool       `json:"completed" bson:"completed"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty" bson:"deleted_at,omitempty"` // Soft delete timestamp
	CreatedAt         time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" bson:"updated_at"`
}

// SetLogRepository handles CRUD operations for the set_logs collection
type SetLogRepository interface {
	// Create adds a new set log document
	Create(ctx context.Context, setLog *SetLogDocument) error
	// GetByID retrieves a set log by its MongoDB ObjectID
	GetByID(ctx context.Context, id string) (*SetLogDocument, error)
	// GetByClientID retrieves a set log by its frontend ULID
	GetByClientID(ctx context.Context, clientID string) (*SetLogDocument, error)
	// GetByPlannedExerciseID retrieves all set logs for a planned exercise
	GetByPlannedExerciseID(ctx context.Context, plannedExerciseID string) ([]*SetLogDocument, error)
	// GetByScheduleID retrieves all set logs for a schedule
	GetByScheduleID(ctx context.Context, scheduleID string) ([]*SetLogDocument, error)
	// Update updates an existing set log
	Update(ctx context.Context, setLog *SetLogDocument) error
	// Delete removes a set log by ID (hard delete)
	Delete(ctx context.Context, id string) error
	// SoftDelete sets deleted_at instead of removing (preserves data)
	SoftDelete(ctx context.Context, id string) error
	// DeleteByPlannedExerciseID removes all set logs for a planned exercise (cascade)
	DeleteByPlannedExerciseID(ctx context.Context, plannedExerciseID string) error
	// DeleteByScheduleID removes all set logs for a schedule (cascade)
	DeleteByScheduleID(ctx context.Context, scheduleID string) error
}
