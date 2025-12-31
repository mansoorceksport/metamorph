package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrExerciseNotFound  = errors.New("exercise not found")
	ErrDuplicateExercise = errors.New("exercise name already exists")
)

// Exercise represents a move in the global library
type Exercise struct {
	ID          string    `json:"id" bson:"_id,omitempty"`
	Name        string    `json:"name" bson:"name"`                 // Unique Index
	MuscleGroup string    `json:"muscle_group" bson:"muscle_group"` // e.g., "Legs", "Chest"
	Equipment   string    `json:"equipment" bson:"equipment"`       // e.g., "Barbell", "Dumbbell"
	VideoURL    string    `json:"video_url" bson:"video_url"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
}

type ExerciseRepository interface {
	Create(ctx context.Context, exercise *Exercise) error
	GetByID(ctx context.Context, id string) (*Exercise, error)
	List(ctx context.Context, filter map[string]interface{}) ([]*Exercise, error)
	Update(ctx context.Context, exercise *Exercise) error
	Delete(ctx context.Context, id string) error
}
