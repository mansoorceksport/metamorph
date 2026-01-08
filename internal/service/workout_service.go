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
	scheduleRepo domain.ScheduleRepository     // To verify schedule exists
	setLogRepo   domain.SetLogRepository       // For atomic set operations
	pbRepo       domain.PersonalBestRepository // For PB tracking
	volumeRepo   domain.DailyVolumeRepository  // For volume aggregation
}

func NewWorkoutService(
	exerciseRepo domain.ExerciseRepository,
	templateRepo domain.TemplateRepository,
	sessionRepo domain.WorkoutSessionRepository,
	scheduleRepo domain.ScheduleRepository,
	setLogRepo domain.SetLogRepository,
	pbRepo domain.PersonalBestRepository,
	volumeRepo domain.DailyVolumeRepository,
) *WorkoutService {
	return &WorkoutService{
		exerciseRepo: exerciseRepo,
		templateRepo: templateRepo,
		sessionRepo:  sessionRepo,
		scheduleRepo: scheduleRepo,
		setLogRepo:   setLogRepo,
		pbRepo:       pbRepo,
		volumeRepo:   volumeRepo,
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
			continue
		}

		// ALSO create SetLogDocuments in set_logs collection
		// This enables syncScheduleSets to fetch proper IDs for the frontend
		for _, set := range defaultSets {
			setLogDoc := &domain.SetLogDocument{
				ClientID:          set.ULID, // Use the ULID as client_id for dual-identity
				PlannedExerciseID: planned.ID,
				ScheduleID:        schedule.ID,
				MemberID:          schedule.MemberID,
				ExerciseID:        ex.ID,
				SetIndex:          set.SetIndex,
				Weight:            set.Weight,
				Reps:              set.Reps,
				Completed:         false,
			}
			if err := s.setLogRepo.Create(ctx, setLogDoc); err != nil {
				fmt.Printf("failed to create set_log document: %v\n", err)
			}
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

	// Get the schedule to find member_id for PB tracking (or just verify existence)
	_, err = s.scheduleRepo.GetByID(ctx, resolvedScheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
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

	// Create PlannedExercise (without embedded sets - we use set_logs collection now)
	planned := &domain.PlannedExercise{
		ClientID:    clientID,           // Store frontend ULID for dual-identity
		ScheduleID:  resolvedScheduleID, // Use resolved MongoDB ObjectID
		ExerciseID:  ex.ID,
		Name:        ex.Name,
		Sets:        nil, // No longer using embedded sets
		Order:       order,
		TargetSets:  targetSets,
		TargetReps:  targetReps,
		RestSeconds: restSeconds,
		Notes:       notes,
	}

	if err := s.sessionRepo.AddPlannedExercise(ctx, planned); err != nil {
		return nil, err
	}

	// Create SetLogDocuments in the set_logs collection for atomic updates
	// DISABLED: Frontend now manages set creation and syncs them individually.
	/*
		for i := 0; i < targetSets; i++ {
			setLog := &domain.SetLogDocument{
				ClientID:          generateULID(), // Each set gets its own ULID
				PlannedExerciseID: planned.ID,
				ScheduleID:        resolvedScheduleID,
				MemberID:          schedule.MemberID,
				ExerciseID:        ex.ID,
				SetIndex:          i + 1,
				Weight:            0,
				Reps:              0,
				Completed:         false,
			}
			if err := s.setLogRepo.Create(ctx, setLog); err != nil {
				return nil, fmt.Errorf("failed to create set log: %w", err)
			}
		}
	*/

	return planned, nil
}

// RemovePlannedExercise removes an exercise by its ID
func (s *WorkoutService) RemovePlannedExercise(ctx context.Context, plannedExerciseID string) error {
	// 1. Delete associated set logs first
	if err := s.setLogRepo.DeleteByPlannedExerciseID(ctx, plannedExerciseID); err != nil {
		return fmt.Errorf("failed to delete associated set logs: %w", err)
	}

	// 2. Delete the planned exercise
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

func (s *WorkoutService) GetSessionBySchedule(ctx context.Context, scheduleID string) (*domain.WorkoutSession, error) {
	resolvedID, err := s.resolveScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	return s.sessionRepo.GetByScheduleID(ctx, resolvedID)
}

func (s *WorkoutService) GetExercisesBySchedule(ctx context.Context, scheduleID string) ([]*domain.PlannedExercise, error) {
	resolvedID, err := s.resolveScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	return s.sessionRepo.GetPlannedExercisesByScheduleID(ctx, resolvedID)
}

// UpdateSetLog atomically updates a set log document (new set_logs collection)
// Resolves ID (can be MongoDB ObjectID or client_id ULID)
func (s *WorkoutService) UpdateSetLog(ctx context.Context, idOrClientID string, weight float64, reps int, remarks string, completed bool) error {
	// Check if it's a valid MongoDB ObjectID (24 hex chars)
	isMongoID := len(idOrClientID) == 24
	if isMongoID {
		for _, c := range idOrClientID {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				isMongoID = false
				break
			}
		}
	}

	var setLog *domain.SetLogDocument
	var err error

	if isMongoID {
		setLog, err = s.setLogRepo.GetByID(ctx, idOrClientID)
		if err != nil {
			// Try by client_id as fallback
			setLog, err = s.setLogRepo.GetByClientID(ctx, idOrClientID)
		}
	} else {
		setLog, err = s.setLogRepo.GetByClientID(ctx, idOrClientID)
	}

	if err != nil {
		return err
	}
	if setLog == nil {
		return domain.ErrSessionNotFound
	}

	// Update fields
	setLog.Weight = weight
	setLog.Reps = reps
	setLog.Remarks = remarks
	setLog.Completed = completed

	if err := s.setLogRepo.Update(ctx, setLog); err != nil {
		return err
	}

	// Note: PB updates are now handled at session completion (PTService.CompleteSession)
	// to ensure data integrity after coach finalization.

	return nil
}

// DeleteSetLog deletes a set log by ID (Mongo ID or Client ID)
func (s *WorkoutService) DeleteSetLog(ctx context.Context, idOrClientID string) error {
	// Check if it's a valid MongoDB ObjectID
	isMongoID := len(idOrClientID) == 24
	if isMongoID {
		for _, c := range idOrClientID {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				isMongoID = false
				break
			}
		}
	}

	var setLog *domain.SetLogDocument
	var err error

	if isMongoID {
		setLog, err = s.setLogRepo.GetByID(ctx, idOrClientID)
		if err != nil {
			// Try fallback
			setLog, err = s.setLogRepo.GetByClientID(ctx, idOrClientID)
		}
	} else {
		setLog, err = s.setLogRepo.GetByClientID(ctx, idOrClientID)
	}

	if err != nil {
		if err == domain.ErrSessionNotFound {
			return nil // idempotent
		}
		return err
	}
	if setLog == nil {
		return nil
	}

	return s.setLogRepo.Delete(ctx, setLog.ID)
}

// AddSetToExercise dynamically adds a new set to an exercise
// Resolves exercise ID (can be MongoDB ObjectID or client_id ULID)
func (s *WorkoutService) AddSetToExercise(ctx context.Context, exerciseIDOrClientID string, clientID string, setIndex int) (*domain.SetLogDocument, error) {
	// Resolve exercise ID
	planned, err := s.resolvePlannedExercise(ctx, exerciseIDOrClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve exercise: %w", err)
	}

	// Get schedule to find member_id
	schedule, err := s.scheduleRepo.GetByID(ctx, planned.ScheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	// If setIndex not provided, calculate from existing sets
	if setIndex == 0 {
		existingSets, err := s.setLogRepo.GetByPlannedExerciseID(ctx, planned.ID)
		if err != nil {
			return nil, err
		}
		setIndex = len(existingSets) + 1
	}

	// Generate client_id if not provided
	if clientID == "" {
		clientID = generateULID()
	}

	setLog := &domain.SetLogDocument{
		ClientID:          clientID,
		PlannedExerciseID: planned.ID,
		ScheduleID:        planned.ScheduleID,
		MemberID:          schedule.MemberID,
		ExerciseID:        planned.ExerciseID,
		SetIndex:          setIndex,
		Weight:            0,
		Reps:              0,
		Completed:         false,
	}

	if err := s.setLogRepo.Create(ctx, setLog); err != nil {
		return nil, fmt.Errorf("failed to create set log: %w", err)
	}

	return setLog, nil
}

// GetSetsBySchedule retrieves all set logs for a given schedule ID
func (s *WorkoutService) GetSetsBySchedule(ctx context.Context, scheduleID string) ([]*domain.SetLogDocument, error) {
	// Ideally we should resolve scheduleID too (mongo vs client_id)
	resolvedScheduleID, err := s.resolveScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve schedule ID: %w", err)
	}
	return s.setLogRepo.GetByScheduleID(ctx, resolvedScheduleID)
}

// resolvePlannedExercise finds a planned exercise by MongoDB ID or client_id
func (s *WorkoutService) resolvePlannedExercise(ctx context.Context, idOrClientID string) (*domain.PlannedExercise, error) {
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
		// Try to get by MongoDB ID
		exercise, err := s.sessionRepo.GetPlannedExerciseByID(ctx, idOrClientID)
		if err == nil {
			return exercise, nil
		}
		// Fall through to try client_id
	}

	// Try to get by client_id (ULID)
	return s.sessionRepo.GetPlannedExerciseByClientID(ctx, idOrClientID)
}

// AggregateSessionVolume calculates and saves the total volume for a completed schedule
// This should be called when a Schedule status changes to 'completed'
// Volume = sum(Weight * Reps) for all completed sets
func (s *WorkoutService) AggregateSessionVolume(ctx context.Context, scheduleID string, memberID string, tenantID string) (*domain.DailyVolume, error) {
	// Check if we already have a volume record for this schedule
	existing, err := s.volumeRepo.GetByScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing volume: %w", err)
	}

	// Get all set logs for this schedule
	setLogs, err := s.setLogRepo.GetByScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch set logs: %w", err)
	}

	// Calculate aggregates
	var totalVolume float64
	var totalWeight float64
	var totalReps int
	var totalSets int
	exerciseIDs := make(map[string]bool)

	for _, log := range setLogs {
		if log.Completed && log.Weight > 0 && log.Reps > 0 {
			volume := log.Weight * float64(log.Reps)
			totalVolume += volume
			totalWeight += log.Weight
			totalReps += log.Reps
			totalSets++
			exerciseIDs[log.ExerciseID] = true
		}
	}

	// Get the schedule to determine the date
	schedule, err := s.scheduleRepo.GetByID(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schedule: %w", err)
	}

	// Create or update volume record
	dailyVolume := &domain.DailyVolume{
		TenantID:      tenantID,
		MemberID:      memberID,
		ScheduleID:    scheduleID,
		Date:          schedule.StartTime,
		TotalVolume:   totalVolume,
		TotalSets:     totalSets,
		TotalReps:     totalReps,
		TotalWeight:   totalWeight,
		ExerciseCount: len(exerciseIDs),
	}

	if existing != nil {
		// Delete old record and create new one (simpler than upsert)
		if err := s.volumeRepo.Delete(ctx, existing.ID); err != nil {
			return nil, fmt.Errorf("failed to delete old volume: %w", err)
		}
	}

	if err := s.volumeRepo.Create(ctx, dailyVolume); err != nil {
		return nil, fmt.Errorf("failed to save daily volume: %w", err)
	}

	return dailyVolume, nil
}

// GetMemberVolumeHistory retrieves volume history for charting
func (s *WorkoutService) GetMemberVolumeHistory(ctx context.Context, memberID string, limit int) ([]*domain.DailyVolume, error) {
	return s.volumeRepo.GetByMemberID(ctx, memberID, limit)
}
