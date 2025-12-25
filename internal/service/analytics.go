package service

import (
	"context"
	"fmt"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

// AnalyticsService handles analytics and trend data
type AnalyticsService struct {
	repository domain.InBodyRepository
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(repository domain.InBodyRepository) *AnalyticsService {
	return &AnalyticsService{
		repository: repository,
	}
}

// GetHistory retrieves analytics history with progress calculation
func (s *AnalyticsService) GetHistory(ctx context.Context, userID string, limit int) (*domain.AnalyticsHistory, error) {
	// Fetch trend history from repository
	records, err := s.repository.GetTrendHistory(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get trend history: %w", err)
	}

	if len(records) == 0 {
		// No data available
		return &domain.AnalyticsHistory{
			Progress: domain.ProgressSummary{
				TotalScans: 0,
			},
			History: []domain.TrendData{},
		}, nil
	}

	// Transform records into TrendData
	history := make([]domain.TrendData, len(records))
	for i, record := range records {
		trend := domain.TrendData{
			Date: record.TestDateTime,
			CoreMetrics: domain.CoreTrendMetric{
				Weight: record.Weight,
				SMM:    record.SMM,
				PBF:    record.PBF,
			},
		}

		// Add segmental data if available
		if record.SegmentalLean != nil && record.SegmentalFat != nil {
			trend.SegmentalTrends = &domain.SegmentalTrend{
				Lean: domain.SegmentalTrendData{
					RightArm: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalLean.RightArm.Mass,
						Percentage: record.SegmentalLean.RightArm.Percentage,
					},
					LeftArm: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalLean.LeftArm.Mass,
						Percentage: record.SegmentalLean.LeftArm.Percentage,
					},
					Trunk: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalLean.Trunk.Mass,
						Percentage: record.SegmentalLean.Trunk.Percentage,
					},
					RightLeg: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalLean.RightLeg.Mass,
						Percentage: record.SegmentalLean.RightLeg.Percentage,
					},
					LeftLeg: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalLean.LeftLeg.Mass,
						Percentage: record.SegmentalLean.LeftLeg.Percentage,
					},
				},
				Fat: domain.SegmentalTrendData{
					RightArm: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalFat.RightArm.Mass,
						Percentage: record.SegmentalFat.RightArm.Percentage,
					},
					LeftArm: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalFat.LeftArm.Mass,
						Percentage: record.SegmentalFat.LeftArm.Percentage,
					},
					Trunk: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalFat.Trunk.Mass,
						Percentage: record.SegmentalFat.Trunk.Percentage,
					},
					RightLeg: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalFat.RightLeg.Mass,
						Percentage: record.SegmentalFat.RightLeg.Percentage,
					},
					LeftLeg: domain.TrendSegmentMetrics{
						Mass:       record.SegmentalFat.LeftLeg.Mass,
						Percentage: record.SegmentalFat.LeftLeg.Percentage,
					},
				},
			}
		}

		history[i] = trend
	}

	// Calculate progress summary (first to latest)
	firstScan := records[0]
	latestScan := records[len(records)-1]

	progress := domain.ProgressSummary{
		TotalScans:     len(records),
		FirstScanDate:  firstScan.TestDateTime.Format("2006-01-02"),
		LatestScanDate: latestScan.TestDateTime.Format("2006-01-02"),
		WeightChange:   latestScan.Weight - firstScan.Weight,
		MuscleGained:   latestScan.SMM - firstScan.SMM,
		BodyFatChange:  latestScan.PBF - firstScan.PBF,
	}

	return &domain.AnalyticsHistory{
		Progress: progress,
		History:  history,
	}, nil
}
