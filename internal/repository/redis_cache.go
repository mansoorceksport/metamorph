package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	latestScanKeyPrefix = "user:latest_scan:"
	trendRecapKeyPrefix = "trend_recap:"
	scanDetailKeyPrefix = "scan:detail:" // Cache for individual scan details

	// Member endpoint caching prefixes
	memberDashboardKeyPrefix = "member:dashboard:"
	memberSchedulesKeyPrefix = "member:schedules:"
	memberWorkoutsKeyPrefix  = "member:workouts:"
	memberPBsKeyPrefix       = "member:pbs:"
	memberScansKeyPrefix     = "member:scans:"
)

// RedisCacheRepository implements domain.CacheRepository using Redis
type RedisCacheRepository struct {
	client *redis.Client
}

// NewRedisCacheRepository creates a new Redis cache repository
func NewRedisCacheRepository(client *redis.Client) *RedisCacheRepository {
	return &RedisCacheRepository{
		client: client,
	}
}

// SetLatestScan caches the latest scan for a user with TTL
func (r *RedisCacheRepository) SetLatestScan(ctx context.Context, userID string, record *domain.InBodyRecord, ttl time.Duration) error {
	key := latestScanKeyPrefix + userID

	// Serialize record to JSON
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	// Set with TTL
	err = r.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to cache latest scan: %w", err)
	}

	return nil
}

// GetLatestScan retrieves the cached latest scan for a user
func (r *RedisCacheRepository) GetLatestScan(ctx context.Context, userID string) (*domain.InBodyRecord, error) {
	key := latestScanKeyPrefix + userID

	// Get from Redis
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss, return nil
		}
		return nil, fmt.Errorf("failed to get cached scan: %w", err)
	}

	// Deserialize JSON
	var record domain.InBodyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached record: %w", err)
	}

	return &record, nil
}

// InvalidateUserCache removes cached data for a user
func (r *RedisCacheRepository) InvalidateUserCache(ctx context.Context, userID string) error {
	key := latestScanKeyPrefix + userID

	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to invalidate cache: %w", err)
	}

	return nil
}

// SetTrendRecap caches a trend recap for a user with TTL
func (r *RedisCacheRepository) SetTrendRecap(ctx context.Context, userID string, summary *domain.TrendSummary, ttl time.Duration) error {
	key := trendRecapKeyPrefix + userID

	// Serialize summary to JSON
	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal trend summary: %w", err)
	}

	// Set with TTL
	err = r.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to cache trend recap: %w", err)
	}

	return nil
}

// GetTrendRecap retrieves the cached trend recap for a user
func (r *RedisCacheRepository) GetTrendRecap(ctx context.Context, userID string) (*domain.TrendSummary, error) {
	key := trendRecapKeyPrefix + userID

	// Get from Redis
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss, return nil
		}
		return nil, fmt.Errorf("failed to get cached trend recap: %w", err)
	}

	// Deserialize JSON
	var summary domain.TrendSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached trend summary: %w", err)
	}

	return &summary, nil
}

// InvalidateTrendRecap removes cached trend recap for a user
func (r *RedisCacheRepository) InvalidateTrendRecap(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s%s", trendRecapKeyPrefix, userID)
	return r.client.Del(ctx, key).Err()
}

// SetScanByID caches a scan by its ID with TTL
func (r *RedisCacheRepository) SetScanByID(ctx context.Context, scanID string, record *domain.InBodyRecord, ttl time.Duration) error {
	key := scanDetailKeyPrefix + scanID

	// Serialize record to JSON
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal scan record: %w", err)
	}

	// Set with TTL
	err = r.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to cache scan: %w", err)
	}

	return nil
}

// GetScanByID retrieves a cached scan by its ID
// Returns nil if not found or expired
func (r *RedisCacheRepository) GetScanByID(ctx context.Context, scanID string) (*domain.InBodyRecord, error) {
	key := scanDetailKeyPrefix + scanID

	// Get from Redis
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss, return nil
		}
		return nil, fmt.Errorf("failed to get cached scan: %w", err)
	}

	// Deserialize JSON
	var record domain.InBodyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached scan: %w", err)
	}

	return &record, nil
}

// InvalidateScan removes a cached scan by its ID
func (r *RedisCacheRepository) InvalidateScan(ctx context.Context, scanID string) error {
	key := scanDetailKeyPrefix + scanID
	return r.client.Del(ctx, key).Err()
}

// =============================================================================
// Generic Cache Operations with OpenTelemetry Tracing
// =============================================================================

var ErrCacheMiss = fmt.Errorf("cache miss")

// Get retrieves a value from cache by key with OTel tracing
func (r *RedisCacheRepository) Get(ctx context.Context, key string, dest interface{}) error {
	tracer := otel.Tracer("redis")
	ctx, span := tracer.Start(ctx, "redis.Get",
		trace.WithAttributes(attribute.String("cache.key", key)),
	)
	defer span.End()

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			span.SetAttributes(attribute.String("cache.result", "miss"))
			return ErrCacheMiss
		}
		span.RecordError(err)
		return fmt.Errorf("redis get error: %w", err)
	}

	span.SetAttributes(attribute.String("cache.result", "hit"))
	if err := json.Unmarshal(data, dest); err != nil {
		span.RecordError(err)
		return fmt.Errorf("unmarshal error: %w", err)
	}

	return nil
}

// Set stores a value in cache with TTL and OTel tracing
func (r *RedisCacheRepository) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	tracer := otel.Tracer("redis")
	ctx, span := tracer.Start(ctx, "redis.Set",
		trace.WithAttributes(
			attribute.String("cache.key", key),
			attribute.Int64("cache.ttl_seconds", int64(ttl.Seconds())),
		),
	)
	defer span.End()

	data, err := json.Marshal(value)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("marshal error: %w", err)
	}

	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("redis set error: %w", err)
	}

	return nil
}

// Delete removes keys from cache with OTel tracing
func (r *RedisCacheRepository) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	tracer := otel.Tracer("redis")
	ctx, span := tracer.Start(ctx, "redis.Delete",
		trace.WithAttributes(attribute.Int("cache.key_count", len(keys))),
	)
	defer span.End()

	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("redis delete error: %w", err)
	}

	return nil
}

// DeleteByPattern removes keys matching a pattern (use sparingly - O(N))
func (r *RedisCacheRepository) DeleteByPattern(ctx context.Context, pattern string) error {
	tracer := otel.Tracer("redis")
	ctx, span := tracer.Start(ctx, "redis.DeleteByPattern",
		trace.WithAttributes(attribute.String("cache.pattern", pattern)),
	)
	defer span.End()

	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("redis keys error: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	span.SetAttributes(attribute.Int("cache.matched_keys", len(keys)))
	return r.client.Del(ctx, keys...).Err()
}

// =============================================================================
// Member Endpoint Caching Methods
// =============================================================================

// SetMemberDashboard caches member dashboard data
func (r *RedisCacheRepository) SetMemberDashboard(ctx context.Context, userID string, data interface{}, ttl time.Duration) error {
	return r.Set(ctx, memberDashboardKeyPrefix+userID, data, ttl)
}

// GetMemberDashboard retrieves cached member dashboard data
func (r *RedisCacheRepository) GetMemberDashboard(ctx context.Context, userID string, dest interface{}) error {
	return r.Get(ctx, memberDashboardKeyPrefix+userID, dest)
}

// SetMemberSchedules caches member schedules data
func (r *RedisCacheRepository) SetMemberSchedules(ctx context.Context, userID string, data interface{}, ttl time.Duration) error {
	return r.Set(ctx, memberSchedulesKeyPrefix+userID, data, ttl)
}

// GetMemberSchedules retrieves cached member schedules data
func (r *RedisCacheRepository) GetMemberSchedules(ctx context.Context, userID string, dest interface{}) error {
	return r.Get(ctx, memberSchedulesKeyPrefix+userID, dest)
}

// SetMemberPBs caches member personal bests data
func (r *RedisCacheRepository) SetMemberPBs(ctx context.Context, userID string, data interface{}, ttl time.Duration) error {
	return r.Set(ctx, memberPBsKeyPrefix+userID, data, ttl)
}

// GetMemberPBs retrieves cached member personal bests data
func (r *RedisCacheRepository) GetMemberPBs(ctx context.Context, userID string, dest interface{}) error {
	return r.Get(ctx, memberPBsKeyPrefix+userID, dest)
}

// InvalidateMemberCache removes all cached data for a member
func (r *RedisCacheRepository) InvalidateMemberCache(ctx context.Context, userID string) error {
	keys := []string{
		memberDashboardKeyPrefix + userID,
		memberSchedulesKeyPrefix + userID,
		memberPBsKeyPrefix + userID,
	}
	return r.Delete(ctx, keys...)
}

// InvalidateMemberDashboard removes cached dashboard for a member
func (r *RedisCacheRepository) InvalidateMemberDashboard(ctx context.Context, userID string) error {
	return r.Delete(ctx, memberDashboardKeyPrefix+userID)
}

// InvalidateMemberSchedules removes cached schedules for a member
func (r *RedisCacheRepository) InvalidateMemberSchedules(ctx context.Context, userID string) error {
	return r.Delete(ctx, memberSchedulesKeyPrefix+userID)
}

// InvalidateMemberPBs removes cached personal bests for a member
func (r *RedisCacheRepository) InvalidateMemberPBs(ctx context.Context, userID string) error {
	return r.Delete(ctx, memberPBsKeyPrefix+userID)
}
