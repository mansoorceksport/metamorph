package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSessionNotFound = errors.New("workout session not found")
)

type SetLog struct {
	SetIndex  int     `json:"set_index" bson:"set_index"` // 1-based index (1, 2, 3)
	Weight    float64 `json:"weight" bson:"weight"`
	Reps      int     `json:"reps" bson:"reps"`
	Remarks   string  `json:"remarks" bson:"remarks"`
	Completed bool    `json:"completed" bson:"completed"`
}

type PlannedExercise struct {
	ExerciseID string    `json:"exercise_id" bson:"exercise_id"`
	Name       string    `json:"name" bson:"name"` // Denormalized for easy display
	Sets       []*SetLog `json:"sets" bson:"sets"`
}

type WorkoutSession struct {
	ID               string             `json:"id" bson:"_id,omitempty"`
	TenantID         string             `json:"tenant_id" bson:"tenant_id"`
	BranchID         string             `json:"branch_id" bson:"branch_id"`
	ScheduleID       string             `json:"schedule_id" bson:"schedule_id"` // Links to the Schedule event
	CoachID          string             `json:"coach_id" bson:"coach_id"`
	MemberID         string             `json:"member_id" bson:"member_id"`
	PlannedExercises []*PlannedExercise `json:"planned_exercises" bson:"planned_exercises"`
	CreatedAt        time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" bson:"updated_at"`
}

type WorkoutSessionRepository interface {
	Create(ctx context.Context, session *WorkoutSession) error
	GetByID(ctx context.Context, id string) (*WorkoutSession, error)
	GetByScheduleID(ctx context.Context, scheduleID string) (*WorkoutSession, error)
	Update(ctx context.Context, session *WorkoutSession) error
}
