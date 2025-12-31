package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTemplateNotFound = errors.New("workout template not found")
)

// WorkoutTemplate represents a predefined workout structure
type WorkoutTemplate struct {
	ID          string    `json:"id" bson:"_id,omitempty"`
	Name        string    `json:"name" bson:"name"`
	Gender      string    `json:"gender" bson:"gender"` // "Male", "Female", "All"
	ExerciseIDs []string  `json:"exercise_ids" bson:"exercise_ids"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
}

type TemplateRepository interface {
	Create(ctx context.Context, template *WorkoutTemplate) error
	GetByID(ctx context.Context, id string) (*WorkoutTemplate, error)
	List(ctx context.Context) ([]*WorkoutTemplate, error)
	Update(ctx context.Context, template *WorkoutTemplate) error
	Delete(ctx context.Context, id string) error
}
