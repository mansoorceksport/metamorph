package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

const (
	trendRecapTTL    = 7 * 24 * time.Hour // 7 days
	trendScanLimit   = 30                 // Last 30 scans for trend analysis
	trendRecapMaxAge = 7 * 24 * time.Hour // Regenerate if older than 7 days
)

// TrendService handles trend analysis and AI-generated recaps
type TrendService struct {
	repository domain.InBodyRepository
	cache      domain.CacheRepository
}

// NewTrendService creates a new trend service
func NewTrendService(
	repository domain.InBodyRepository,
	cache domain.CacheRepository,
) *TrendService {
	return &TrendService{
		repository: repository,
		cache:      cache,
	}
}

// GenerateTrendRecap generates or retrieves a trend recap for a user
// Implements 3-tier caching strategy: Redis -> MongoDB -> AI Generation
func (s *TrendService) GenerateTrendRecap(ctx context.Context, userID string) (*domain.TrendSummary, error) {
	// Tier 1: Check Redis cache (read-through)
	cachedSummary, err := s.cache.GetTrendRecap(ctx, userID)
	if err != nil {
		// Log but don't fail - cache errors shouldn't block the request
		fmt.Printf("Warning: failed to check Redis cache: %v\n", err)
	}
	if cachedSummary != nil {
		fmt.Printf("Cache hit: Redis trend recap for user %s\n", userID)
		return cachedSummary, nil
	}

	// Tier 2: Check MongoDB for recent summary
	dbSummary, err := s.repository.GetLatestTrendSummary(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trend summary from database: %w", err)
	}

	// If MongoDB summary exists and is fresh (< 7 days old), use it
	if dbSummary != nil {
		age := time.Since(dbSummary.LastGeneratedAt)
		if age < trendRecapMaxAge {
			fmt.Printf("MongoDB cache hit: trend summary for user %s (age: %v)\n", userID, age)

			// Write-through to Redis for faster next access
			if err := s.cache.SetTrendRecap(ctx, userID, dbSummary, trendRecapTTL); err != nil {
				fmt.Printf("Warning: failed to cache trend recap in Redis: %v\n", err)
			}

			return dbSummary, nil
		}
		fmt.Printf("MongoDB summary is stale (age: %v), regenerating...\n", age)
	}

	// Tier 3: Generate new trend recap using AI
	fmt.Printf("Generating new trend recap for user %s\n", userID)

	// Fetch scan history for trend analysis
	scans, err := s.repository.GetTrendHistory(ctx, userID, trendScanLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scan history: %w", err)
	}

	// If no scans available, return empty summary
	if len(scans) == 0 {
		return &domain.TrendSummary{
			UserID:          userID,
			SummaryText:     "No scans available yet! Upload your first InBody scan to start tracking your progress.",
			LastGeneratedAt: time.Now(),
			IncludedScanIDs: []string{},
		}, nil
	}

	// If only one scan, provide a welcome message
	if len(scans) == 1 {
		firstScan := scans[0]
		summaryText := fmt.Sprintf(
			"Welcome to House of Metamorfit! 🎉 We've recorded your first scan on %s. "+
				"Your baseline: %.1f kg weight, %.1f kg SMM, %.1f%% body fat. "+
				"Keep scanning regularly to track your transformation journey!",
			firstScan.TestDateTime.Format("Jan 2, 2006"),
			firstScan.Weight,
			firstScan.SMM,
			firstScan.PBF,
		)
		summary := &domain.TrendSummary{
			UserID:          userID,
			SummaryText:     summaryText,
			LastGeneratedAt: time.Now(),
			IncludedScanIDs: []string{firstScan.ID},
		}

		// Save to MongoDB and Redis
		if err := s.repository.SaveTrendSummary(ctx, summary); err != nil {
			fmt.Printf("Warning: failed to save trend summary to MongoDB: %v\n", err)
		}
		if err := s.cache.SetTrendRecap(ctx, userID, summary, trendRecapTTL); err != nil {
			fmt.Printf("Warning: failed to cache trend recap in Redis: %v\n", err)
		}

		return summary, nil
	}

	// Calculate numerical deltas (first to latest)
	firstScan := scans[0]
	latestScan := scans[len(scans)-1]

	weightDelta := latestScan.Weight - firstScan.Weight
	smmDelta := latestScan.SMM - firstScan.SMM
	pbfDelta := latestScan.PBF - firstScan.PBF

	// Generate AI-powered trend recap
	summaryText := fmt.Sprintf(
		"Looking at your %d scans from %s to %s: ",
		len(scans),
		firstScan.TestDateTime.Format("Jan 2"),
		latestScan.TestDateTime.Format("Jan 2, 2006"),
	)

	// Add delta insights
	if weightDelta > 0 {
		summaryText += fmt.Sprintf("Your weight increased by %.1f kg. ", abs(weightDelta))
	} else if weightDelta < 0 {
		summaryText += fmt.Sprintf("You lost %.1f kg! ", abs(weightDelta))
	} else {
		summaryText += "Your weight remained stable. "
	}

	if smmDelta > 0 {
		summaryText += fmt.Sprintf("You gained %.1f kg of muscle mass! ", smmDelta)
	} else if smmDelta < 0 {
		summaryText += fmt.Sprintf("Muscle mass decreased by %.1f kg—consider more strength training. ", abs(smmDelta))
	}

	if pbfDelta < 0 {
		summaryText += fmt.Sprintf("Body fat dropped by %.1f%%! ", abs(pbfDelta))
	} else if pbfDelta > 0 {
		summaryText += fmt.Sprintf("Body fat increased by %.1f%%. ", pbfDelta)
	}

	summaryText += "Keep up the great work at House of Metamorfit! 💪"

	// Collect scan IDs for reference
	scanIDs := make([]string, len(scans))
	for i, scan := range scans {
		scanIDs[i] = scan.ID
	}

	// Create and save trend summary
	summary := &domain.TrendSummary{
		UserID:          userID,
		SummaryText:     summaryText,
		LastGeneratedAt: time.Now(),
		IncludedScanIDs: scanIDs,
	}

	// Save to MongoDB (write-through)
	if err := s.repository.SaveTrendSummary(ctx, summary); err != nil {
		return nil, fmt.Errorf("failed to save trend summary: %w", err)
	}

	// Cache in Redis (write-through)
	if err := s.cache.SetTrendRecap(ctx, userID, summary, trendRecapTTL); err != nil {
		fmt.Printf("Warning: failed to cache trend recap in Redis: %v\n", err)
	}

	return summary, nil
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
