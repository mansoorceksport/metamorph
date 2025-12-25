package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionName = "inbody_records"
)

// MongoInBodyRepository implements domain.InBodyRepository using MongoDB
type MongoInBodyRepository struct {
	collection             *mongo.Collection
	trendSummaryCollection *mongo.Collection
}

// NewMongoInBodyRepository creates a new MongoDB repository
func NewMongoInBodyRepository(db *mongo.Database) *MongoInBodyRepository {
	collection := db.Collection(collectionName)
	trendSummaryCollection := db.Collection("trend_summaries")

	// Create indexes for better query performance
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Index on user_id and test_date_time for efficient queries
	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "test_date_time", Value: -1}, // Descending for latest first
		},
	}
	_, _ = collection.Indexes().CreateOne(ctx, indexModel)

	// Index on user_id and last_generated_at for trend summaries
	trendIndexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "last_generated_at", Value: -1}, // Descending for latest first
		},
	}
	_, _ = trendSummaryCollection.Indexes().CreateOne(ctx, trendIndexModel)

	return &MongoInBodyRepository{
		collection:             collection,
		trendSummaryCollection: trendSummaryCollection,
	}
}

// Create saves a new InBodyRecord to MongoDB
func (r *MongoInBodyRepository) Create(ctx context.Context, record *domain.InBodyRecord) error {
	// Generate new ObjectID
	objectID := primitive.NewObjectID()

	// Set processed timestamp
	processedAt := time.Now()

	// Create BSON document with proper ObjectID type for _id
	doc := bson.M{
		"_id":            objectID, // Store as ObjectID, not string
		"user_id":        record.UserID,
		"test_date_time": record.TestDateTime,
		"weight":         record.Weight,
		"smm":            record.SMM,
		"body_fat_mass":  record.BodyFatMass,
		"bmi":            record.BMI,
		"pbf":            record.PBF,
		"bmr":            record.BMR,
		"visceral_fat":   record.VisceralFatLevel,
		"whr":            record.WaistHipRatio,
		"metadata": bson.M{
			"image_url":    record.Metadata.ImageURL,
			"processed_at": processedAt,
		},
	}

	// Add V2 fields if present (backward compatibility)
	if record.SegmentalLean != nil {
		doc["segmental_lean"] = record.SegmentalLean
	}
	if record.SegmentalFat != nil {
		doc["segmental_fat"] = record.SegmentalFat
	}
	if record.Analysis != nil {
		doc["analysis"] = record.Analysis
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to insert inbody record: %w", err)
	}

	// Set the ID in the record (as hex string for domain model)
	record.ID = objectID.Hex()
	record.Metadata.ProcessedAt = processedAt

	return nil
}

// GetLatestByUserID retrieves the most recent scan for a user
func (r *MongoInBodyRepository) GetLatestByUserID(ctx context.Context, userID string) (*domain.InBodyRecord, error) {
	filter := bson.M{"user_id": userID}
	opts := options.FindOne().SetSort(bson.D{{Key: "test_date_time", Value: -1}})

	var record domain.InBodyRecord
	err := r.collection.FindOne(ctx, filter, opts).Decode(&record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No record found, return nil
		}
		return nil, fmt.Errorf("failed to find latest record: %w", err)
	}

	return &record, nil
}

// GetByUserID retrieves multiple scans for a user, limited by count
func (r *MongoInBodyRepository) GetByUserID(ctx context.Context, userID string, limit int) ([]*domain.InBodyRecord, error) {
	filter := bson.M{"user_id": userID}
	opts := options.Find().
		SetSort(bson.D{{Key: "test_date_time", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find records: %w", err)
	}
	defer cursor.Close(ctx)

	var records []*domain.InBodyRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, fmt.Errorf("failed to decode records: %w", err)
	}

	return records, nil
}

// FindAllByUserID retrieves all scans for a user, sorted by test_date_time DESC
func (r *MongoInBodyRepository) FindAllByUserID(ctx context.Context, userID string) ([]*domain.InBodyRecord, error) {
	filter := bson.M{"user_id": userID}
	opts := options.Find().SetSort(bson.D{{Key: "test_date_time", Value: -1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find records: %w", err)
	}
	defer cursor.Close(ctx)

	var records []*domain.InBodyRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, fmt.Errorf("failed to decode records: %w", err)
	}

	return records, nil
}

// FindByID retrieves a single scan by its ID
func (r *MongoInBodyRepository) FindByID(ctx context.Context, id string) (*domain.InBodyRecord, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	filter := bson.M{"_id": objectID}

	var record domain.InBodyRecord
	err = r.collection.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to find record: %w", err)
	}

	return &record, nil
}

// Update modifies an existing scan record
func (r *MongoInBodyRepository) Update(ctx context.Context, id string, record *domain.InBodyRecord) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrNotFound
	}

	filter := bson.M{"_id": objectID}
	update := bson.M{
		"$set": bson.M{
			"weight":        record.Weight,
			"smm":           record.SMM,
			"body_fat_mass": record.BodyFatMass,
			"bmi":           record.BMI,
			"pbf":           record.PBF,
			"bmr":           record.BMR,
			"visceral_fat":  record.VisceralFatLevel,
			"whr":           record.WaistHipRatio,
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}

	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// Delete removes a scan record by its ID
func (r *MongoInBodyRepository) Delete(ctx context.Context, id string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrNotFound
	}

	filter := bson.M{"_id": objectID}

	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}

	if result.DeletedCount == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// GetTrendHistory retrieves N scans for analytics, sorted ascending by test_date_time
// Uses projection to only return necessary fields for charting
func (r *MongoInBodyRepository) GetTrendHistory(ctx context.Context, userID string, limit int) ([]*domain.InBodyRecord, error) {
	filter := bson.M{"user_id": userID}

	// Sort ascending (oldest first) for left-to-right chart plotting
	opts := options.Find().
		SetSort(bson.D{{Key: "test_date_time", Value: 1}}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{
			"test_date_time": 1,
			"weight":         1,
			"smm":            1,
			"pbf":            1,
			"segmental_lean": 1,
			"segmental_fat":  1,
		})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find trend history: %w", err)
	}
	defer cursor.Close(ctx)

	var records []*domain.InBodyRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, fmt.Errorf("failed to decode trend history: %w", err)
	}

	return records, nil
}

// SaveTrendSummary saves a trend summary to the database
func (r *MongoInBodyRepository) SaveTrendSummary(ctx context.Context, summary *domain.TrendSummary) error {
	// Generate new ObjectID
	objectID := primitive.NewObjectID()

	// Create BSON document
	doc := bson.M{
		"_id":               objectID,
		"user_id":           summary.UserID,
		"summary_text":      summary.SummaryText,
		"last_generated_at": summary.LastGeneratedAt,
		"included_scan_ids": summary.IncludedScanIDs,
	}

	_, err := r.trendSummaryCollection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to insert trend summary: %w", err)
	}

	// Set the ID in the summary (as hex string for domain model)
	summary.ID = objectID.Hex()

	return nil
}

// GetLatestTrendSummary retrieves the most recent trend summary for a user
func (r *MongoInBodyRepository) GetLatestTrendSummary(ctx context.Context, userID string) (*domain.TrendSummary, error) {
	filter := bson.M{"user_id": userID}
	opts := options.FindOne().SetSort(bson.D{{Key: "last_generated_at", Value: -1}})

	var summary domain.TrendSummary
	err := r.trendSummaryCollection.FindOne(ctx, filter, opts).Decode(&summary)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No summary found, return nil
		}
		return nil, fmt.Errorf("failed to find latest trend summary: %w", err)
	}

	return &summary, nil
}
