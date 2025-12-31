package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

	// Step 1: Extract metrics using AI (analyzing current scan only)
	metrics, err := s.digitizer.ExtractMetrics(ctx, userID, imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metrics: %w", err)
	}

	userObjectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	// Step 2: Build InBodyRecord with V1 fields
	record := &domain.InBodyRecord{
		UserID:                   userObjectID,
		TestDateTime:             metrics.TestDate,
		Weight:                   metrics.Weight,
		SMM:                      metrics.SMM,
		BodyFatMass:              metrics.BodyFatMass,
		BMI:                      metrics.BMI,
		PBF:                      metrics.PBF,
		BMR:                      metrics.BMR,
		VisceralFatLevel:         metrics.VisceralFatLevel,
		WaistHipRatio:            metrics.WaistHipRatio,
		InBodyScore:              metrics.InBodyScore,
		ObesityDegree:            metrics.ObesityDegree,
		FatFreeMass:              metrics.FatFreeMass,
		RecommendedCalorieIntake: metrics.RecommendedCalorieIntake,
		TargetWeight:             metrics.TargetWeight,
		WeightControl:            metrics.WeightControl,
		FatControl:               metrics.FatControl,
		MuscleControl:            metrics.MuscleControl,
	}

	// Step 2.5: Map V2 fields if present (backward compatible)
	if metrics.SegmentalLean != nil {
		record.SegmentalLean = metrics.SegmentalLean
	}
	if metrics.SegmentalFat != nil {
		record.SegmentalFat = metrics.SegmentalFat
	}
	if metrics.Analysis != nil {
		record.Analysis = metrics.Analysis
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

	// Invalidate trend recap cache
	if err := s.cache.InvalidateTrendRecap(ctx, userID); err != nil {
		fmt.Printf("Warning: failed to invalidate trend recap cache: %v\n", err)
	}

	return record, nil
}

// GetAllScans retrieves all scans for a user
func (s *ScanServiceImpl) GetAllScans(ctx context.Context, userID string) ([]*domain.InBodyRecord, error) {
	return s.repository.FindAllByUserID(ctx, userID)
}

// GetScanByID retrieves a single scan with ownership verification
func (s *ScanServiceImpl) GetScanByID(ctx context.Context, userID string, scanID string) (*domain.InBodyRecord, error) {
	record, err := s.repository.FindByID(ctx, scanID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if record.UserID.Hex() != userID {
		return nil, domain.ErrForbidden
	}

	return record, nil
}

// UpdateScan updates specific metrics with ownership verification
func (s *ScanServiceImpl) UpdateScan(ctx context.Context, userID string, scanID string, updates map[string]interface{}) (*domain.InBodyRecord, error) {
	// Fetch existing record
	record, err := s.repository.FindByID(ctx, scanID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if record.UserID.Hex() != userID {
		return nil, domain.ErrForbidden
	}

	// Apply updates to allowed fields
	if weight, ok := updates["weight"].(float64); ok {
		record.Weight = weight
	}
	if smm, ok := updates["smm"].(float64); ok {
		record.SMM = smm
	}
	if bodyFatMass, ok := updates["body_fat_mass"].(float64); ok {
		record.BodyFatMass = bodyFatMass
	}
	if pbf, ok := updates["pbf"].(float64); ok {
		record.PBF = pbf
	}
	if bmi, ok := updates["bmi"].(float64); ok {
		record.BMI = bmi
	}
	if bmr, ok := updates["bmr"].(float64); ok {
		record.BMR = int(bmr)
	}
	if visceralFat, ok := updates["visceral_fat"].(float64); ok {
		record.VisceralFatLevel = int(visceralFat)
	}
	if whr, ok := updates["whr"].(float64); ok {
		record.WaistHipRatio = whr
	}
	if score, ok := updates["inbody_score"].(float64); ok {
		record.InBodyScore = score
	}
	if obesityDegree, ok := updates["obesity_degree"].(float64); ok {
		record.ObesityDegree = obesityDegree
	}
	if ffm, ok := updates["fat_free_mass"].(float64); ok {
		record.FatFreeMass = ffm
	}
	if calories, ok := updates["recommended_calorie_intake"].(float64); ok {
		record.RecommendedCalorieIntake = int(calories)
	}
	if target, ok := updates["target_weight"].(float64); ok {
		record.TargetWeight = target
	}
	if weightCtrl, ok := updates["weight_control"].(float64); ok {
		record.WeightControl = weightCtrl
	}
	if fatCtrl, ok := updates["fat_control"].(float64); ok {
		record.FatControl = fatCtrl
	}
	if muscleCtrl, ok := updates["muscle_control"].(float64); ok {
		record.MuscleControl = muscleCtrl
	}

	// Update in database
	if err := s.repository.Update(ctx, scanID, record); err != nil {
		return nil, err
	}

	// Invalidate cache for this user
	_ = s.cache.InvalidateUserCache(ctx, userID)
	_ = s.cache.InvalidateTrendRecap(ctx, userID)

	// Return updated record
	return s.repository.FindByID(ctx, scanID)
}

// DeleteScan removes a scan and its associated image with ownership verification
func (s *ScanServiceImpl) DeleteScan(ctx context.Context, userID string, scanID string) error {
	// Fetch existing record
	record, err := s.repository.FindByID(ctx, scanID)
	if err != nil {
		return err
	}

	// Verify ownership
	if record.UserID.Hex() != userID {
		return domain.ErrForbidden
	}

	// Delete image from S3 if fileRepository is available and imageURL exists
	if s.fileRepository != nil && record.Metadata.ImageURL != "" {
		if err := s.fileRepository.Delete(ctx, record.Metadata.ImageURL); err != nil {
			// Log error but continue with database deletion
			fmt.Printf("Warning: failed to delete image from storage: %v\n", err)
		}
	}

	// Delete from database
	if err := s.repository.Delete(ctx, scanID); err != nil {
		return err
	}

	// Invalidate cache for this user
	_ = s.cache.InvalidateUserCache(ctx, userID)
	_ = s.cache.InvalidateTrendRecap(ctx, userID)

	return nil
}
