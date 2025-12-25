package domain

import "time"

// TrendData represents a single data point in the analytics history
type TrendData struct {
	Date            time.Time       `json:"date"`
	CoreMetrics     CoreTrendMetric `json:"core_metrics"`
	SegmentalTrends *SegmentalTrend `json:"segmental_trends,omitempty"`
}

// CoreTrendMetric represents the key metrics for charting
type CoreTrendMetric struct {
	Weight float64 `json:"weight"` // kg
	SMM    float64 `json:"smm"`    // kg - Skeletal Muscle Mass
	PBF    float64 `json:"pbf"`    // % - Percent Body Fat
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
