package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	latestScanKeyPrefix = "user:latest_scan:"
	trendRecapKeyPrefix = "trend_recap:"
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
