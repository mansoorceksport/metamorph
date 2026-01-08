package domain

import (
	"context"
	"time"
)

// DailyVolume represents aggregated workout volume for a member on a specific day
// Volume = sum(Weight * Reps) for all completed sets in a session
type DailyVolume struct {
	ID            string    `json:"id" bson:"_id,omitempty"`
	TenantID      string    `json:"tenant_id" bson:"tenant_id"`
	MemberID      string    `json:"member_id" bson:"member_id"`
	ScheduleID    string    `json:"schedule_id" bson:"schedule_id"`   // Reference to completed schedule
	Date          time.Time `json:"date" bson:"date"`                 // Day of the workout
	TotalVolume   float64   `json:"total_volume" bson:"total_volume"` // Weight * Reps summed
	TotalSets     int       `json:"total_sets" bson:"total_sets"`
	TotalReps     int       `json:"total_reps" bson:"total_reps"`
	TotalWeight   float64   `json:"total_weight" bson:"total_weight"` // Sum of all weights lifted
	ExerciseCount int       `json:"exercise_count" bson:"exercise_count"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
}

// DailyVolumeRepository handles CRUD operations for the daily_volumes collection
type DailyVolumeRepository interface {
	// Create adds a new daily volume record
	Create(ctx context.Context, volume *DailyVolume) error
	// GetByScheduleID retrieves volume for a specific schedule
	GetByScheduleID(ctx context.Context, scheduleID string) (*DailyVolume, error)
	// GetByMemberID retrieves all volume records for a member, sorted by date desc
	GetByMemberID(ctx context.Context, memberID string, limit int) ([]*DailyVolume, error)
	// GetByMemberIDAndDateRange retrieves volume records for a member within a date range
	GetByMemberIDAndDateRange(ctx context.Context, memberID string, from, to time.Time) ([]*DailyVolume, error)
	// Delete removes a volume record by ID
	Delete(ctx context.Context, id string) error
	// DeleteByScheduleID removes volume record for a schedule
	DeleteByScheduleID(ctx context.Context, scheduleID string) error
}
