package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSessionNotFound      = errors.New("workout session not found")
	ErrExerciseULIDNotFound = errors.New("exercise ULID not found in session")
)

type SetLog struct {
	ULID      string  `json:"ulid" bson:"ulid"`           // Unique identifier for atomic operations
	SetIndex  int     `json:"set_index" bson:"set_index"` // 1-based index for display (1, 2, 3)
	Weight    float64 `json:"weight" bson:"weight"`
	Reps      int     `json:"reps" bson:"reps"`
	Remarks   string  `json:"remarks" bson:"remarks"`
	Completed bool    `json:"completed" bson:"completed"`
}

type PlannedExercise struct {
	ID          string    `json:"id" bson:"_id,omitempty"`
	ScheduleID  string    `json:"schedule_id" bson:"schedule_id"`
	ExerciseID  string    `json:"exercise_id" bson:"exercise_id"`
	Name        string    `json:"name" bson:"name"`
	TargetSets  int       `json:"target_sets" bson:"target_sets"`
	TargetReps  int       `json:"target_reps" bson:"target_reps"`
	RestSeconds int       `json:"rest_seconds" bson:"rest_seconds"`
	Notes       string    `json:"notes" bson:"notes"`
	Order       int       `json:"order" bson:"order"`
	Sets        []*SetLog `json:"sets" bson:"sets"` // Logs for execution
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
	// GetSessionsByCoachAndDateRange retrieves all workout sessions for a coach within a date range
	GetSessionsByCoachAndDateRange(ctx context.Context, coachID string, from, to time.Time) ([]*WorkoutSession, error)
	// AddPlannedExercise adds a planned exercise to the session's plan
	AddPlannedExercise(ctx context.Context, exercise *PlannedExercise) error
	// RemovePlannedExercise removes a planned exercise by its ID
	RemovePlannedExercise(ctx context.Context, id string) error
	// UpdatePlannedExercise updates a planned exercise
	UpdatePlannedExercise(ctx context.Context, exercise *PlannedExercise) error
	CountPlannedExercises(ctx context.Context, scheduleID string) (int64, error)
	// UpsertSetLog atomically updates or inserts a set log using ULID-based targeting
	UpsertSetLog(ctx context.Context, sessionID, exerciseID string, setLog *SetLog) error
}
