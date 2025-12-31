package domain

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// InBodyRecord represents a digitized InBody scan result
type InBodyRecord struct {
	ID           string             `bson:"_id,omitempty" json:"id"`
	UserID       primitive.ObjectID `bson:"user_id" json:"user_id"` // Stored as ObjectID
	TestDateTime time.Time          `bson:"test_date_time" json:"test_date_time"`

	// Core Metrics
	Weight      float64 `bson:"weight" json:"weight"`
	SMM         float64 `bson:"smm" json:"smm"` // Skeletal Muscle Mass
	BodyFatMass float64 `bson:"body_fat_mass" json:"body_fat_mass"`
	BMI         float64 `bson:"bmi" json:"bmi"`
	PBF         float64 `bson:"pbf" json:"pbf"` // Percent Body Fat

	// Health Indicators
	BMR              int     `bson:"bmr" json:"bmr"` // Basal Metabolic Rate
	VisceralFatLevel int     `bson:"visceral_fat" json:"visceral_fat"`
	WaistHipRatio    float64 `bson:"whr" json:"whr"`
	InBodyScore      float64 `bson:"inbody_score" json:"inbody_score"`
	ObesityDegree    float64 `bson:"obesity_degree" json:"obesity_degree"`

	// Body Composition Analysis
	FatFreeMass float64 `bson:"fat_free_mass" json:"fat_free_mass"`

	// Control Guide
	RecommendedCalorieIntake int     `bson:"recommended_calorie_intake" json:"recommended_calorie_intake"`
	TargetWeight             float64 `bson:"target_weight" json:"target_weight"`
	WeightControl            float64 `bson:"weight_control" json:"weight_control"`
	FatControl               float64 `bson:"fat_control" json:"fat_control"`
	MuscleControl            float64 `bson:"muscle_control" json:"muscle_control"`

	// Segmental Body Composition (V2) - Optional for backward compatibility
	SegmentalLean *SegmentalData `bson:"segmental_lean,omitempty" json:"segmental_lean,omitempty"`
	SegmentalFat  *SegmentalData `bson:"segmental_fat,omitempty" json:"segmental_fat,omitempty"`

	// AI-Generated Analysis (V2) - Optional for backward compatibility
	Analysis *BodyAnalysis `bson:"analysis,omitempty" json:"analysis,omitempty"`

	Metadata struct {
		ImageURL    string    `bson:"image_url" json:"image_url"`
		ProcessedAt time.Time `bson:"processed_at" json:"processed_at"`
	} `bson:"metadata" json:"metadata"`
}

// SegmentalData represents body composition for different body segments
type SegmentalData struct {
	RightArm SegmentMetrics `bson:"right_arm" json:"right_arm"`
	LeftArm  SegmentMetrics `bson:"left_arm" json:"left_arm"`
	Trunk    SegmentMetrics `bson:"trunk" json:"trunk"`
	RightLeg SegmentMetrics `bson:"right_leg" json:"right_leg"`
	LeftLeg  SegmentMetrics `bson:"left_leg" json:"left_leg"`
}

// SegmentMetrics represents mass and percentage for a body segment
type SegmentMetrics struct {
	Mass       float64 `bson:"mass" json:"mass"`             // in kg
	Percentage float64 `bson:"percentage" json:"percentage"` // relative to total
}

// BodyAnalysis represents AI-generated analysis and feedback
type BodyAnalysis struct {
	Summary          string   `bson:"summary" json:"summary"`
	PositiveFeedback []string `bson:"positive_feedback" json:"positive_feedback"`
	Improvements     []string `bson:"improvements" json:"improvements"`
	Advice           []string `bson:"advice" json:"advice"`
}

// InBodyMetrics represents the raw metrics extracted from AI
type InBodyMetrics struct {
	// V1 Fields (Always present)
	Weight           float64   `json:"weight"`
	SMM              float64   `json:"smm"`
	BodyFatMass      float64   `json:"body_fat_mass"`
	PBF              float64   `json:"pbf"`
	BMI              float64   `json:"bmi"`
	BMR              int       `json:"bmr"`
	VisceralFatLevel int       `json:"visceral_fat"`
	WaistHipRatio    float64   `json:"whr"`
	TestDate         time.Time `json:"test_date"`

	// New Metrics
	InBodyScore              float64 `json:"inbody_score"`
	ObesityDegree            float64 `json:"obesity_degree"`
	FatFreeMass              float64 `json:"fat_free_mass"`
	RecommendedCalorieIntake int     `json:"recommended_calorie_intake"`
	TargetWeight             float64 `json:"target_weight"`
	WeightControl            float64 `json:"weight_control"`
	FatControl               float64 `json:"fat_control"`
	MuscleControl            float64 `json:"muscle_control"`

	// V2 Fields (Optional for backward compatibility)
	SegmentalLean *SegmentalData `json:"segmental_lean,omitempty"`
	SegmentalFat  *SegmentalData `json:"segmental_fat,omitempty"`
	Analysis      *BodyAnalysis  `json:"analysis,omitempty"`
}

// TrendSummary represents an AI-generated trend recap for a user
type TrendSummary struct {
	ID              string             `bson:"_id,omitempty" json:"id"`
	UserID          primitive.ObjectID `bson:"user_id" json:"user_id"`
	SummaryText     string             `bson:"summary_text" json:"summary_text"`
	LastGeneratedAt time.Time          `bson:"last_generated_at" json:"last_generated_at"`
	IncludedScanIDs []string           `bson:"included_scan_ids" json:"included_scan_ids"`
}

// InBodyRepository defines the interface for InBodyRecord persistence
// Implementations should handle MongoDB operations
type InBodyRepository interface {
	// Create saves a new InBodyRecord to the database
	Create(ctx context.Context, record *InBodyRecord) error

	// GetLatestByUserID retrieves the most recent scan for a user
	GetLatestByUserID(ctx context.Context, userID string) (*InBodyRecord, error)

	// GetByUserID retrieves multiple scans for a user, limited by count
	GetByUserID(ctx context.Context, userID string, limit int) ([]*InBodyRecord, error)

	// FindAllByUserID retrieves all scans for a user, sorted by test_date_time DESC
	FindAllByUserID(ctx context.Context, userID string) ([]*InBodyRecord, error)

	// FindByID retrieves a single scan by its ID
	FindByID(ctx context.Context, id string) (*InBodyRecord, error)

	// Update modifies an existing scan record
	Update(ctx context.Context, id string, record *InBodyRecord) error

	// Delete removes a scan record by its ID
	Delete(ctx context.Context, id string) error

	// GetTrendHistory retrieves N scans for analytics, sorted ascending by date
	GetTrendHistory(ctx context.Context, userID string, limit int) ([]*InBodyRecord, error)

	// SaveTrendSummary saves a trend summary to the database
	SaveTrendSummary(ctx context.Context, summary *TrendSummary) error

	// GetLatestTrendSummary retrieves the most recent trend summary for a user
	GetLatestTrendSummary(ctx context.Context, userID string) (*TrendSummary, error)
}

// CacheRepository defines the interface for caching operations
// Implementations should handle Redis operations
type CacheRepository interface {
	// SetLatestScan caches the latest scan for a user with TTL
	SetLatestScan(ctx context.Context, userID string, record *InBodyRecord, ttl time.Duration) error

	// GetLatestScan retrieves the cached latest scan for a user
	// Returns nil if not found or expired
	GetLatestScan(ctx context.Context, userID string) (*InBodyRecord, error)

	// InvalidateUserCache removes cached data for a user
	InvalidateUserCache(ctx context.Context, userID string) error

	// SetTrendRecap caches a trend recap for a user with TTL
	SetTrendRecap(ctx context.Context, userID string, summary *TrendSummary, ttl time.Duration) error

	// GetTrendRecap retrieves the cached trend recap for a user
	// Returns nil if not found or expired
	GetTrendRecap(ctx context.Context, userID string) (*TrendSummary, error)

	// InvalidateTrendRecap removes cached trend recap for a user
	InvalidateTrendRecap(ctx context.Context, userID string) error
}

// DigitizerService defines the interface for AI-based metric extraction
// Implementations should handle OpenRouter API communication
type DigitizerService interface {
	// ExtractMetrics uses AI to extract InBody metrics from an image
	// userID is required for SaaS context (fetching tenant persona)
	ExtractMetrics(ctx context.Context, userID string, imageData []byte) (*InBodyMetrics, error)
}

// ScanService defines the interface for business logic around scan processing
type ScanService interface {
	// ProcessScan orchestrates the entire digitization workflow:
	// 1. Extract metrics using AI
	// 2. Save to database
	// 3. Cache the result
	ProcessScan(ctx context.Context, userID string, imageData []byte, imageURL string) (*InBodyRecord, error)

	// GetAllScans retrieves all scans for a user
	GetAllScans(ctx context.Context, userID string) ([]*InBodyRecord, error)

	// GetScanByID retrieves a single scan with ownership verification
	GetScanByID(ctx context.Context, userID string, scanID string) (*InBodyRecord, error)

	// UpdateScan updates specific metrics with ownership verification
	UpdateScan(ctx context.Context, userID string, scanID string, updates map[string]interface{}) (*InBodyRecord, error)

	// DeleteScan removes a scan and its associated image with ownership verification
	DeleteScan(ctx context.Context, userID string, scanID string) error
}
