package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/oklog/ulid/v2"
)

type WorkoutService struct {
	exerciseRepo domain.ExerciseRepository
	templateRepo domain.TemplateRepository
	sessionRepo  domain.WorkoutSessionRepository
	scheduleRepo domain.ScheduleRepository // To verify schedule exists
}

func NewWorkoutService(
	exerciseRepo domain.ExerciseRepository,
	templateRepo domain.TemplateRepository,
	sessionRepo domain.WorkoutSessionRepository,
	scheduleRepo domain.ScheduleRepository,
) *WorkoutService {
	return &WorkoutService{
		exerciseRepo: exerciseRepo,
		templateRepo: templateRepo,
		sessionRepo:  sessionRepo,
		scheduleRepo: scheduleRepo,
	}
}

// generateULID creates a new ULID string
func generateULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

// InitializeSession creates a WorkoutSession from a Template linked to a Schedule
func (s *WorkoutService) InitializeSession(ctx context.Context, scheduleID string, templateID string) (*domain.WorkoutSession, error) {
	// 1. Verify Schedule
	schedule, err := s.scheduleRepo.GetByID(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("invalid schedule: %w", err)
	}

	// 2. Check if session already exists for this schedule?
	existing, err := s.sessionRepo.GetByScheduleID(ctx, scheduleID)
	if err == nil && existing != nil {
		return nil, errors.New("session already initialized for this schedule")
	}

	// 3. Fetch Template
	template, err := s.templateRepo.GetByID(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("invalid template: %w", err)
	}

	// 4. Hydrate Exercises with ULIDs
	var planned []*domain.PlannedExercise
	for _, exID := range template.ExerciseIDs {
		ex, err := s.exerciseRepo.GetByID(ctx, exID)
		if err != nil {
			// If one fails, skip? Or fail? Better to fail or log.
			// Failsafe: handle gracefully
			continue
		}

		// Create 3 default empty sets with ULIDs
		defaultSets := make([]*domain.SetLog, 3)
		for i := 0; i < 3; i++ {
			defaultSets[i] = &domain.SetLog{
				ULID:     generateULID(),
				SetIndex: i + 1,
				Reps:     0,
				Weight:   0,
			}
		}

		planned = append(planned, &domain.PlannedExercise{
			ULID:       generateULID(),
			ExerciseID: ex.ID,
			Name:       ex.Name,
			Sets:       defaultSets,
		})
	}

	// 5. Create Session
	session := &domain.WorkoutSession{
		ScheduleID:       schedule.ID,
		TenantID:         schedule.TenantID,
		BranchID:         schedule.BranchID,
		CoachID:          schedule.CoachID,
		MemberID:         schedule.MemberID,
		PlannedExercises: planned,
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

// AddExerciseToSession adds an exercise dynamically (at end)
func (s *WorkoutService) AddExerciseToSession(ctx context.Context, sessionID string, exerciseID string) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	ex, err := s.exerciseRepo.GetByID(ctx, exerciseID)
	if err != nil {
		return err
	}

	// Default 3 sets with ULIDs
	sets := make([]*domain.SetLog, 3)
	for i := 0; i < 3; i++ {
		sets[i] = &domain.SetLog{
			ULID:     generateULID(),
			SetIndex: i + 1,
		}
	}

	session.PlannedExercises = append(session.PlannedExercises, &domain.PlannedExercise{
		ULID:       generateULID(),
		ExerciseID: ex.ID,
		Name:       ex.Name,
		Sets:       sets,
	})

	return s.sessionRepo.Update(ctx, session)
}

// RemoveExerciseFromSession removes an exercise by index
func (s *WorkoutService) RemoveExerciseFromSession(ctx context.Context, sessionID string, exerciseIndex int) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	if exerciseIndex < 0 || exerciseIndex >= len(session.PlannedExercises) {
		return errors.New("invalid exercise index")
	}

	// Remove slice element
	session.PlannedExercises = append(session.PlannedExercises[:exerciseIndex], session.PlannedExercises[exerciseIndex+1:]...)

	return s.sessionRepo.Update(ctx, session)
}

// LogSet updates a specific set (legacy index-based - kept for backward compatibility)
func (s *WorkoutService) LogSet(ctx context.Context, sessionID string, exerciseIndex int, setIndex int, weight float64, reps int, remarks string) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	if exerciseIndex < 0 || exerciseIndex >= len(session.PlannedExercises) {
		return errors.New("invalid exercise index")
	}

	ex := session.PlannedExercises[exerciseIndex]
	if setIndex < 0 || setIndex >= len(ex.Sets) {
		return errors.New("invalid set index")
	}

	set := ex.Sets[setIndex]
	set.Weight = weight
	set.Reps = reps
	set.Remarks = remarks
	set.Completed = true

	return s.sessionRepo.Update(ctx, session)
}

// LogSetByULID atomically updates or inserts a set using ULID-based targeting
func (s *WorkoutService) LogSetByULID(ctx context.Context, sessionID, exerciseULID string, setLog *domain.SetLog) error {
	return s.sessionRepo.UpsertSetLog(ctx, sessionID, exerciseULID, setLog)
}

func (s *WorkoutService) GetSession(ctx context.Context, id string) (*domain.WorkoutSession, error) {
	return s.sessionRepo.GetByID(ctx, id)
}
