package domain

import (
	"context"
	"time"
)

// PersonalBest tracks a member's personal best for an exercise
type PersonalBest struct {
	ID         string    `json:"id" bson:"_id,omitempty"`
	MemberID   string    `json:"member_id" bson:"member_id"`
	ExerciseID string    `json:"exercise_id" bson:"exercise_id"` // Reference to exercise definition
	Weight     float64   `json:"weight" bson:"weight"`
	Reps       int       `json:"reps" bson:"reps"`
	AchievedAt time.Time `json:"achieved_at" bson:"achieved_at"`
	ScheduleID string    `json:"schedule_id" bson:"schedule_id"` // Session where PB was achieved
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
}

// PersonalBestRepository handles CRUD operations for personal bests
type PersonalBestRepository interface {
	// GetByMemberAndExercise retrieves a member's PB for a specific exercise
	GetByMemberAndExercise(ctx context.Context, memberID, exerciseID string) (*PersonalBest, error)
	// Upsert creates or updates a PB if the new weight exceeds the existing one
	Upsert(ctx context.Context, pb *PersonalBest) (bool, error) // Returns true if PB was updated
	// GetByMember retrieves all PBs for a member
	GetByMember(ctx context.Context, memberID string) ([]*PersonalBest, error)
}
