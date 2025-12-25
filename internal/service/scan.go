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
	digitizer      domain.DigitizerService
	repository     domain.InBodyRepository
	cache          domain.CacheRepository
	fileRepository domain.FileRepository
}

// NewScanService creates a new scan service
func NewScanService(
	digitizer domain.DigitizerService,
	repository domain.InBodyRepository,
	cache domain.CacheRepository,
	fileRepository domain.FileRepository,
) *ScanServiceImpl {
	return &ScanServiceImpl{
		digitizer:      digitizer,
		repository:     repository,
		cache:          cache,
		fileRepository: fileRepository,
	}
}

// ProcessScan orchestrates the entire digitization workflow
func (s *ScanServiceImpl) ProcessScan(ctx context.Context, userID string, imageData []byte, imageURL string) (*domain.InBodyRecord, error) {
	// Step 0: Upload image to S3 (SeaweedFS) if fileRepository is available
	// We generate a filename based on userID and timestamp
	if s.fileRepository != nil {
		filename := fmt.Sprintf("%s/%d.jpg", userID, time.Now().UnixNano()) // Simple path strategy
		contentType := "image/jpeg"                                         // Default, ideally detect dynamically

		// Improve content type detection if possible (reusing logic from digitizer would be good, but keep simple for now)
		if len(imageData) > 0 && imageData[0] == 0x89 && imageData[1] == 0x50 {
			contentType = "image/png"
			filename = fmt.Sprintf("%s/%d.png", userID, time.Now().UnixNano())
		}

		uploadedURL, err := s.fileRepository.Upload(ctx, imageData, filename, contentType)
		if err != nil {
			return nil, fmt.Errorf("failed to upload image: %w", err)
		}
		imageURL = uploadedURL // Use the permanent URL
	}

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
