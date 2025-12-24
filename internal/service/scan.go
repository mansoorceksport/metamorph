package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

const (
	cacheLatestScanTTL = 24 * time.Hour
)

// ScanServiceImpl implements domain.ScanService
type ScanServiceImpl struct {
	digitizer  domain.DigitizerService
	repository domain.InBodyRepository
	cache      domain.CacheRepository
}

// NewScanService creates a new scan service
func NewScanService(
	digitizer domain.DigitizerService,
	repository domain.InBodyRepository,
	cache domain.CacheRepository,
) *ScanServiceImpl {
	return &ScanServiceImpl{
		digitizer:  digitizer,
		repository: repository,
		cache:      cache,
	}
}

// ProcessScan orchestrates the entire digitization workflow
func (s *ScanServiceImpl) ProcessScan(ctx context.Context, userID string, imageData []byte, imageURL string) (*domain.InBodyRecord, error) {
	// Step 1: Extract metrics using AI
	metrics, err := s.digitizer.ExtractMetrics(ctx, imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metrics: %w", err)
	}

	// Step 2: Build InBodyRecord
	record := &domain.InBodyRecord{
		UserID:           userID,
		TestDateTime:     metrics.TestDate,
		Weight:           metrics.Weight,
		SMM:              metrics.SMM,
		BodyFatMass:      metrics.BodyFatMass,
		BMI:              metrics.BMI,
		PBF:              metrics.PBF,
		BMR:              metrics.BMR,
		VisceralFatLevel: metrics.VisceralFatLevel,
		WaistHipRatio:    metrics.WaistHipRatio,
	}
	record.Metadata.ImageURL = imageURL
	record.Metadata.ProcessedAt = time.Now()

	// Step 3: Save to MongoDB
	if err := s.repository.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to save record: %w", err)
	}

	// Step 4: Cache in Redis (24h TTL) - non-blocking, log error but don't fail
	if err := s.cache.SetLatestScan(ctx, userID, record, cacheLatestScanTTL); err != nil {
		// Log error but don't fail the request
		// In production, you'd use a proper logger here
		fmt.Printf("Warning: failed to cache latest scan: %v\n", err)
	}

	return record, nil
}
