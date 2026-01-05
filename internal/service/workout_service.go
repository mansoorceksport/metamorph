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

	// 4. Create Session (Empty)
	session := &domain.WorkoutSession{
		ScheduleID: schedule.ID,
		TenantID:   schedule.TenantID,
		BranchID:   schedule.BranchID,
		CoachID:    schedule.CoachID,
		MemberID:   schedule.MemberID,
		// PlannedExercises is strictly read-only after fetch, storage is separate
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	// 5. Add Exercises from Template
	for i, exID := range template.ExerciseIDs {
		ex, err := s.exerciseRepo.GetByID(ctx, exID)
		if err != nil {
			continue // graceful skip
		}

		// Create 3 default empty sets with ULIDs
		defaultSets := make([]*domain.SetLog, 3)
		for j := 0; j < 3; j++ {
			defaultSets[j] = &domain.SetLog{
				ULID:     generateULID(),
				SetIndex: j + 1,
				Reps:     0,
				Weight:   0,
			}
		}

		planned := &domain.PlannedExercise{
			ScheduleID:  schedule.ID,
			ExerciseID:  ex.ID,
			Name:        ex.Name,
			Sets:        defaultSets,
			Order:       i + 1,
			TargetSets:  3, // Default from template logic or just static default?
			TargetReps:  10,
			RestSeconds: 60,
		}

		// Save each individually
		if err := s.sessionRepo.AddPlannedExercise(ctx, planned); err != nil {
			// logging error?
			fmt.Printf("failed to add initial exercise: %v\n", err)
		}
	}

	// Return full session with exercises inflated
	return s.GetSession(ctx, session.ID)
}

// resolveScheduleID accepts either MongoDB ObjectID or ULID and resolves to the schedule's MongoDB ID
// This handles the case where frontend sends ULID before schedule sync completes
func (s *WorkoutService) resolveScheduleID(ctx context.Context, idOrClientID string) (string, error) {
	// Check if it's a valid MongoDB ObjectID (24 hex chars, all lowercase hex)
	isMongoID := len(idOrClientID) == 24
	if isMongoID {
		for _, c := range idOrClientID {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				isMongoID = false
				break
			}
		}
	}

	if isMongoID {
		// Try to get by MongoDB ObjectID first
		schedule, err := s.scheduleRepo.GetByID(ctx, idOrClientID)
		if err == nil {
			return schedule.ID, nil
		}
		// If not found by ID, fall through to try client_id
	}

	// Try to get by client_id (ULID)
	schedule, err := s.scheduleRepo.GetByClientID(ctx, idOrClientID)
	if err != nil {
		return "", fmt.Errorf("schedule not found for ID/ULID: %s", idOrClientID)
	}
	return schedule.ID, nil
}

// AddExerciseToSession adds an exercise dynamically (at end) - RETURNS the added exercise
// clientID is the frontend ULID for dual-identity handshake
// targetSets, targetReps, restSeconds, notes, order are passed from the frontend
func (s *WorkoutService) AddExerciseToSession(ctx context.Context, scheduleID string, exerciseID string, clientID string, targetSets int, targetReps int, restSeconds int, notes string, order int) (*domain.PlannedExercise, error) {
	// Resolve scheduleID (handles both MongoDB ObjectID and frontend ULID)
	resolvedScheduleID, err := s.resolveScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, err
	}

	// If order not provided, calculate from count
	if order == 0 {
		count, err := s.sessionRepo.CountPlannedExercises(ctx, resolvedScheduleID)
		if err != nil {
			return nil, err
		}
		order = int(count) + 1
	}

	ex, err := s.exerciseRepo.GetByID(ctx, exerciseID)
	if err != nil {
		return nil, err
	}

	// Use defaults if not provided
	if targetSets == 0 {
		targetSets = 3
	}
	if targetReps == 0 {
		targetReps = 10
	}
	if restSeconds == 0 {
		restSeconds = 60
	}

	// Create sets with ULIDs based on target count
	sets := make([]*domain.SetLog, targetSets)
	for i := 0; i < targetSets; i++ {
		sets[i] = &domain.SetLog{
			ULID:     generateULID(),
			SetIndex: i + 1,
		}
	}

	planned := &domain.PlannedExercise{
		ClientID:    clientID,           // Store frontend ULID for dual-identity
		ScheduleID:  resolvedScheduleID, // Use resolved MongoDB ObjectID
		ExerciseID:  ex.ID,
		Name:        ex.Name,
		Sets:        sets,
		Order:       order,
		TargetSets:  targetSets,
		TargetReps:  targetReps,
		RestSeconds: restSeconds,
		Notes:       notes,
	}

	if err := s.sessionRepo.AddPlannedExercise(ctx, planned); err != nil {
		return nil, err
	}

	return planned, nil
}

// RemovePlannedExercise removes an exercise by its ID
func (s *WorkoutService) RemovePlannedExercise(ctx context.Context, plannedExerciseID string) error {
	// Verify it exists? Repo will error if not found or just delete?
	// Just call repo
	return s.sessionRepo.RemovePlannedExercise(ctx, plannedExerciseID)
}

// UpdatePlannedExercise updates details of a planned exercise
func (s *WorkoutService) UpdatePlannedExercise(ctx context.Context, ex *domain.PlannedExercise) error {
	return s.sessionRepo.UpdatePlannedExercise(ctx, ex)
}

// LogSet updates a specific set (legacy index-based)
// FIXME: This legacy method relies on array index in slice. Since we separated collection, "Plan" is a list.
// We should probably deprecate or adapt. But for now, we leave it broken or fix it to fetch-find-update?
// Since User uses LogSetByULID, we prioritize that. Let's fix LogSetByULID below.

// LogSetByULID atomically updates or inserts a set using ULID-based targeting
func (s *WorkoutService) LogSetByULID(ctx context.Context, sessionID, exerciseID string, setLog *domain.SetLog) error {
	// ExerciseID is now the _id of the PlannedExercise document
	// We pass sessionID just for context or if we need it, but UpsertSetLog in repo uses exerciseID (as _id)
	return s.sessionRepo.UpsertSetLog(ctx, sessionID, exerciseID, setLog)
}

func (s *WorkoutService) GetSession(ctx context.Context, id string) (*domain.WorkoutSession, error) {
	return s.sessionRepo.GetByID(ctx, id)
}
