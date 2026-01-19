package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

const (
	scheduleByIDKeyPrefix       = "schedule:id:"
	scheduleByClientIDKeyPrefix = "schedule:client_id:"
	scheduleCacheTTL            = 5 * time.Minute
)

// CachedScheduleRepository wraps MongoScheduleRepository with Redis caching
type CachedScheduleRepository struct {
	mongo *MongoScheduleRepository
	cache *RedisCacheRepository
}

// NewCachedScheduleRepository creates a new cached schedule repository
func NewCachedScheduleRepository(mongo *MongoScheduleRepository, cache *RedisCacheRepository) *CachedScheduleRepository {
	return &CachedScheduleRepository{
		mongo: mongo,
		cache: cache,
	}
}

// GetByID retrieves a schedule by MongoDB ID with caching
func (r *CachedScheduleRepository) GetByID(ctx context.Context, id string) (*domain.Schedule, error) {
	key := scheduleByIDKeyPrefix + id

	// Try cache first
	var schedule domain.Schedule
	if err := r.cache.Get(ctx, key, &schedule); err == nil {
		return &schedule, nil
	}

	// Cache miss - fetch from MongoDB
	result, err := r.mongo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Store in cache (ignore cache errors)
	_ = r.cache.Set(ctx, key, result, scheduleCacheTTL)

	return result, nil
}

// GetByClientID retrieves a schedule by frontend ULID with caching
func (r *CachedScheduleRepository) GetByClientID(ctx context.Context, clientID string) (*domain.Schedule, error) {
	key := scheduleByClientIDKeyPrefix + clientID

	// Try cache first
	var schedule domain.Schedule
	if err := r.cache.Get(ctx, key, &schedule); err == nil {
		return &schedule, nil
	}

	// Cache miss - fetch from MongoDB
	result, err := r.mongo.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}

	// Store in cache (ignore cache errors)
	_ = r.cache.Set(ctx, key, result, scheduleCacheTTL)

	return result, nil
}

// Create creates a schedule and invalidates relevant caches
func (r *CachedScheduleRepository) Create(ctx context.Context, schedule *domain.Schedule) error {
	if err := r.mongo.Create(ctx, schedule); err != nil {
		return err
	}

	// Invalidate coach schedule lists
	_ = r.cache.DeleteByPattern(ctx, fmt.Sprintf("schedule:coach:%s:*", schedule.CoachID))
	return nil
}

// Update updates a schedule and invalidates caches
func (r *CachedScheduleRepository) Update(ctx context.Context, schedule *domain.Schedule) error {
	if err := r.mongo.Update(ctx, schedule); err != nil {
		return err
	}

	// Invalidate specific schedule caches
	_ = r.cache.Delete(ctx, scheduleByIDKeyPrefix+schedule.ID)
	if schedule.ClientID != "" {
		_ = r.cache.Delete(ctx, scheduleByClientIDKeyPrefix+schedule.ClientID)
	}
	_ = r.cache.DeleteByPattern(ctx, fmt.Sprintf("schedule:coach:%s:*", schedule.CoachID))
	return nil
}

// UpdateStatus updates schedule status and invalidates caches
func (r *CachedScheduleRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	// Get schedule first to know coach ID for list invalidation
	schedule, _ := r.mongo.GetByID(ctx, id)

	if err := r.mongo.UpdateStatus(ctx, id, status); err != nil {
		return err
	}

	// Invalidate caches
	_ = r.cache.Delete(ctx, scheduleByIDKeyPrefix+id)
	if schedule != nil {
		if schedule.ClientID != "" {
			_ = r.cache.Delete(ctx, scheduleByClientIDKeyPrefix+schedule.ClientID)
		}
		_ = r.cache.DeleteByPattern(ctx, fmt.Sprintf("schedule:coach:%s:*", schedule.CoachID))
	}
	return nil
}

// Delete deletes a schedule and invalidates caches
func (r *CachedScheduleRepository) Delete(ctx context.Context, id string) error {
	// Get schedule first for cache invalidation
	schedule, _ := r.mongo.GetByID(ctx, id)

	if err := r.mongo.Delete(ctx, id); err != nil {
		return err
	}

	// Invalidate caches
	_ = r.cache.Delete(ctx, scheduleByIDKeyPrefix+id)
	if schedule != nil {
		if schedule.ClientID != "" {
			_ = r.cache.Delete(ctx, scheduleByClientIDKeyPrefix+schedule.ClientID)
		}
		_ = r.cache.DeleteByPattern(ctx, fmt.Sprintf("schedule:coach:%s:*", schedule.CoachID))
	}
	return nil
}

// SoftDelete soft-deletes a schedule and invalidates caches
func (r *CachedScheduleRepository) SoftDelete(ctx context.Context, id string) error {
	schedule, _ := r.mongo.GetByID(ctx, id)

	if err := r.mongo.SoftDelete(ctx, id); err != nil {
		return err
	}

	// Invalidate caches
	_ = r.cache.Delete(ctx, scheduleByIDKeyPrefix+id)
	if schedule != nil {
		if schedule.ClientID != "" {
			_ = r.cache.Delete(ctx, scheduleByClientIDKeyPrefix+schedule.ClientID)
		}
		_ = r.cache.DeleteByPattern(ctx, fmt.Sprintf("schedule:coach:%s:*", schedule.CoachID))
	}
	return nil
}

// === Pass-through methods (no caching) ===

func (r *CachedScheduleRepository) GetByCoach(ctx context.Context, coachID string, from, to time.Time) ([]*domain.Schedule, error) {
	return r.mongo.GetByCoach(ctx, coachID, from, to)
}

func (r *CachedScheduleRepository) GetByCoachAllStatuses(ctx context.Context, coachID string, from, to time.Time) ([]*domain.Schedule, error) {
	return r.mongo.GetByCoachAllStatuses(ctx, coachID, from, to)
}

func (r *CachedScheduleRepository) GetByMember(ctx context.Context, memberID string, from, to time.Time) ([]*domain.Schedule, error) {
	return r.mongo.GetByMember(ctx, memberID, from, to)
}

func (r *CachedScheduleRepository) List(ctx context.Context, tenantID string, filterOpts map[string]interface{}) ([]*domain.Schedule, error) {
	return r.mongo.List(ctx, tenantID, filterOpts)
}

func (r *CachedScheduleRepository) CountByContractAndStatus(ctx context.Context, contractID string, statuses []string) (int64, error) {
	return r.mongo.CountByContractAndStatus(ctx, contractID, statuses)
}

func (r *CachedScheduleRepository) CountByContractsAndStatus(ctx context.Context, contractIDs []string, statuses []string) (map[string]int, error) {
	return r.mongo.CountByContractsAndStatus(ctx, contractIDs, statuses)
}

func (r *CachedScheduleRepository) GetAttendanceByCoach(ctx context.Context, coachID string, days int) ([]*domain.Schedule, error) {
	return r.mongo.GetAttendanceByCoach(ctx, coachID, days)
}

func (r *CachedScheduleRepository) GetMemberScheduleStats(ctx context.Context, memberID string) (completed int, cancelled int, noShow int, err error) {
	return r.mongo.GetMemberScheduleStats(ctx, memberID)
}
