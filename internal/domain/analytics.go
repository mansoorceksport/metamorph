package domain

import (
	"context"
	"time"
)

// AnalyticsService defines methods for retrieving analytics data
type AnalyticsService interface {
	GetHistory(ctx context.Context, userID string, limit int) (*AnalyticsHistory, error)
}

// TrendData represents a single data point in the analytics history
type TrendData struct {
	Date            time.Time        `json:"date"`
	CoreMetrics     CoreTrendMetric  `json:"core_metrics"`
	ExtendedMetrics *ExtendedMetrics `json:"extended_metrics,omitempty"`
	SegmentalTrends *SegmentalTrend  `json:"segmental_trends,omitempty"`
}

// CoreTrendMetric represents the key metrics for charting
type CoreTrendMetric struct {
	Weight float64 `json:"weight"` // kg
	SMM    float64 `json:"smm"`    // kg - Skeletal Muscle Mass
	PBF    float64 `json:"pbf"`    // % - Percent Body Fat
}

// ExtendedMetrics represents the newly added detailed metrics
type ExtendedMetrics struct {
	InBodyScore              float64 `json:"inbody_score"`
	ObesityDegree            float64 `json:"obesity_degree"`
	FatFreeMass              float64 `json:"fat_free_mass"`
	RecommendedCalorieIntake int     `json:"recommended_calorie_intake"`
	TargetWeight             float64 `json:"target_weight"`
	WeightControl            float64 `json:"weight_control"`
	FatControl               float64 `json:"fat_control"`
	MuscleControl            float64 `json:"muscle_control"`
}

// SegmentalTrend represents segmental data for trend analysis
type SegmentalTrend struct {
	Lean SegmentalTrendData `json:"lean"`
	Fat  SegmentalTrendData `json:"fat"`
}

// SegmentalTrendData represents trend data for body segments
type SegmentalTrendData struct {
	RightArm TrendSegmentMetrics `json:"right_arm"`
	LeftArm  TrendSegmentMetrics `json:"left_arm"`
	Trunk    TrendSegmentMetrics `json:"trunk"`
	RightLeg TrendSegmentMetrics `json:"right_leg"`
	LeftLeg  TrendSegmentMetrics `json:"left_leg"`
}

// TrendSegmentMetrics represents mass and percentage for a segment
type TrendSegmentMetrics struct {
	Mass       float64 `json:"mass"`       // kg
	Percentage float64 `json:"percentage"` // %
}

// ProgressSummary represents total progress from first to latest scan
type ProgressSummary struct {
	TotalScans     int     `json:"total_scans"`
	FirstScanDate  string  `json:"first_scan_date"`
	LatestScanDate string  `json:"latest_scan_date"`
	WeightChange   float64 `json:"weight_change"`   // kg (positive = gained)
	MuscleGained   float64 `json:"muscle_gained"`   // kg SMM gained
	BodyFatChange  float64 `json:"body_fat_change"` // % PBF change (negative = lost)
}

// AnalyticsHistory represents the complete analytics response
type AnalyticsHistory struct {
	Progress ProgressSummary `json:"progress"`
	History  []TrendData     `json:"history"`
}
