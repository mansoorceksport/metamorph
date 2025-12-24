package domain

import (
	"context"
	"time"
)

// InBodyRecord represents a digitized InBody scan result
type InBodyRecord struct {
	ID           string    `bson:"_id,omitempty" json:"id"`
	UserID       string    `bson:"user_id" json:"user_id"` // From Firebase JWT
	TestDateTime time.Time `bson:"test_date_time" json:"test_date_time"`

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

	Metadata struct {
		ImageURL    string    `bson:"image_url" json:"image_url"`
		ProcessedAt time.Time `bson:"processed_at" json:"processed_at"`
	} `bson:"metadata" json:"metadata"`
}

// InBodyMetrics represents the raw metrics extracted from AI
type InBodyMetrics struct {
	Weight           float64   `json:"weight"`
	SMM              float64   `json:"smm"`
	BodyFatMass      float64   `json:"body_fat_mass"`
	PBF              float64   `json:"pbf"`
	BMI              float64   `json:"bmi"`
	BMR              int       `json:"bmr"`
	VisceralFatLevel int       `json:"visceral_fat"`
	WaistHipRatio    float64   `json:"whr"`
	TestDate         time.Time `json:"test_date"`
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
}

// DigitizerService defines the interface for AI-based metric extraction
// Implementations should handle OpenRouter API communication
type DigitizerService interface {
	// ExtractMetrics uses AI to extract InBody metrics from an image
	ExtractMetrics(ctx context.Context, imageData []byte) (*InBodyMetrics, error)
}

// ScanService defines the interface for business logic around scan processing
type ScanService interface {
	// ProcessScan orchestrates the entire digitization workflow:
	// 1. Extract metrics using AI
	// 2. Save to database
	// 3. Cache the result
	ProcessScan(ctx context.Context, userID string, imageData []byte, imageURL string) (*InBodyRecord, error)
}
