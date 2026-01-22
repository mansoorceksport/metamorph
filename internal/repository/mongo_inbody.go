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
		"_id":                        objectID, // Store as ObjectID, not string
		"user_id":                    record.UserID,
		"test_date_time":             record.TestDateTime,
		"weight":                     record.Weight,
		"smm":                        record.SMM,
		"body_fat_mass":              record.BodyFatMass,
		"bmi":                        record.BMI,
		"pbf":                        record.PBF,
		"bmr":                        record.BMR,
		"visceral_fat":               record.VisceralFatLevel,
		"whr":                        record.WaistHipRatio,
		"inbody_score":               record.InBodyScore,
		"obesity_degree":             record.ObesityDegree,
		"fat_free_mass":              record.FatFreeMass,
		"recommended_calorie_intake": record.RecommendedCalorieIntake,
		"target_weight":              record.TargetWeight,
		"weight_control":             record.WeightControl,
		"fat_control":                record.FatControl,
		"muscle_control":             record.MuscleControl,
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
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	filter := bson.M{"user_id": oid}
	opts := options.FindOne().SetSort(bson.D{{Key: "test_date_time", Value: -1}})

	var record domain.InBodyRecord
	err = r.collection.FindOne(ctx, filter, opts).Decode(&record)
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
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	filter := bson.M{"user_id": oid}
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
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	filter := bson.M{"user_id": oid}
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
			"weight":                     record.Weight,
			"smm":                        record.SMM,
			"body_fat_mass":              record.BodyFatMass,
			"bmi":                        record.BMI,
			"pbf":                        record.PBF,
			"bmr":                        record.BMR,
			"visceral_fat":               record.VisceralFatLevel,
			"whr":                        record.WaistHipRatio,
			"inbody_score":               record.InBodyScore,
			"obesity_degree":             record.ObesityDegree,
			"fat_free_mass":              record.FatFreeMass,
			"recommended_calorie_intake": record.RecommendedCalorieIntake,
			"target_weight":              record.TargetWeight,
			"weight_control":             record.WeightControl,
			"fat_control":                record.FatControl,
			"muscle_control":             record.MuscleControl,
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
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	filter := bson.M{"user_id": oid}

	// Sort ascending (oldest first) for left-to-right chart plotting
	opts := options.Find().
		SetSort(bson.D{{Key: "test_date_time", Value: 1}}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{
			"test_date_time":             1,
			"weight":                     1,
			"smm":                        1,
			"pbf":                        1,
			"inbody_score":               1,
			"obesity_degree":             1,
			"fat_free_mass":              1,
			"recommended_calorie_intake": 1,
			"target_weight":              1,
			"weight_control":             1,
			"fat_control":                1,
			"muscle_control":             1,
			"segmental_lean":             1,
			"segmental_fat":              1,
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
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	filter := bson.M{"user_id": oid}
	opts := options.FindOne().SetSort(bson.D{{Key: "last_generated_at", Value: -1}})

	var summary domain.TrendSummary
	err = r.trendSummaryCollection.FindOne(ctx, filter, opts).Decode(&summary)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No summary found, return nil
		}
		return nil, fmt.Errorf("failed to find latest trend summary: %w", err)
	}

	return &summary, nil
}

// GetRecentScansByMembers retrieves the N most recent scans for multiple members
// Uses aggregation pipeline for efficient bulk processing
func (r *MongoInBodyRepository) GetRecentScansByMembers(ctx context.Context, memberIDs []string, limit int) (map[string][]*domain.InBodyRecord, error) {
	if len(memberIDs) == 0 {
		return make(map[string][]*domain.InBodyRecord), nil
	}

	// Convert string IDs to ObjectIDs
	oids := make([]primitive.ObjectID, 0, len(memberIDs))
	for _, id := range memberIDs {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			continue
		}
		oids = append(oids, oid)
	}

	if len(oids) == 0 {
		return make(map[string][]*domain.InBodyRecord), nil
	}

	// Aggregation pipeline:
	// 1. Match documents for specified members
	// 2. Sort by test_date_time descending
	// 3. Group by user_id and take first N documents
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"user_id": bson.M{"$in": oids}}}},
		{{Key: "$sort", Value: bson.D{{Key: "test_date_time", Value: -1}}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$user_id",
			"scans": bson.M{"$push": "$$ROOT"},
		}}},
		{{Key: "$project", Value: bson.M{
			"_id":   1,
			"scans": bson.M{"$slice": bson.A{"$scans", limit}},
		}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate scans: %w", err)
	}
	defer cursor.Close(ctx)

	// Parse aggregation results
	result := make(map[string][]*domain.InBodyRecord)
	for cursor.Next(ctx) {
		var doc struct {
			ID    primitive.ObjectID     `bson:"_id"`
			Scans []*domain.InBodyRecord `bson:"scans"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		result[doc.ID.Hex()] = doc.Scans
	}

	return result, nil
}

// FindPaginatedByUserID retrieves scans with cursor-based pagination and date filtering
// Returns lightweight ScanListItem records for efficient list rendering
func (r *MongoInBodyRepository) FindPaginatedByUserID(ctx context.Context, userID string, query *domain.ScanListQuery) (*domain.ScanListResult, error) {
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	// Build filter
	filter := bson.M{"user_id": oid}

	// Add date range filter if provided
	if !query.From.IsZero() || !query.To.IsZero() {
		dateFilter := bson.M{}
		if !query.From.IsZero() {
			dateFilter["$gte"] = query.From
		}
		if !query.To.IsZero() {
			dateFilter["$lte"] = query.To
		}
		filter["test_date_time"] = dateFilter
	}

	// Handle cursor-based pagination
	// Cursor format: "timestamp_id" (e.g., "2025-12-20T09:45:00Z_6950fee4b51bc914bed65c47")
	if query.Cursor != "" {
		// Parse cursor to get the timestamp and id for keyset pagination
		var cursorTime time.Time
		var cursorID primitive.ObjectID

		// Split cursor by underscore to get timestamp and id
		parts := splitCursor(query.Cursor)
		if len(parts) == 2 {
			cursorTime, _ = time.Parse(time.RFC3339, parts[0])
			cursorID, _ = primitive.ObjectIDFromHex(parts[1])

			// For descending sort, get records with test_date_time < cursor OR (test_date_time == cursor AND _id < cursor_id)
			filter["$or"] = bson.A{
				bson.M{"test_date_time": bson.M{"$lt": cursorTime}},
				bson.M{
					"test_date_time": cursorTime,
					"_id":            bson.M{"$lt": cursorID},
				},
			}
		}
	}

	// Set default limit
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50 // Cap at 50 to prevent abuse
	}

	// Get total count for the user (without cursor/pagination)
	countFilter := bson.M{"user_id": oid}
	if !query.From.IsZero() || !query.To.IsZero() {
		dateFilter := bson.M{}
		if !query.From.IsZero() {
			dateFilter["$gte"] = query.From
		}
		if !query.To.IsZero() {
			dateFilter["$lte"] = query.To
		}
		countFilter["test_date_time"] = dateFilter
	}
	total, err := r.collection.CountDocuments(ctx, countFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	// Find with projection for lightweight response
	opts := options.Find().
		SetSort(bson.D{
			{Key: "test_date_time", Value: -1},
			{Key: "_id", Value: -1},
		}).
		SetLimit(int64(limit + 1)). // Fetch one extra to determine hasMore
		SetProjection(bson.M{
			"_id":            1,
			"test_date_time": 1,
			"weight":         1,
			"pbf":            1,
			"smm":            1,
		})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find records: %w", err)
	}
	defer cursor.Close(ctx)

	var items []domain.ScanListItem
	for cursor.Next(ctx) {
		var item domain.ScanListItem
		if err := cursor.Decode(&item); err != nil {
			continue
		}
		items = append(items, item)
	}

	// Determine if there are more results
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit] // Remove the extra item
	}

	// Build next cursor from the last item
	var nextCursor string
	if hasMore && len(items) > 0 {
		lastItem := items[len(items)-1]
		nextCursor = fmt.Sprintf("%s_%s", lastItem.TestDateTime.Format(time.RFC3339), lastItem.ID)
	}

	return &domain.ScanListResult{
		Items:      items,
		Total:      total,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// splitCursor splits a cursor string by underscore, handling timestamps that contain underscores
func splitCursor(cursor string) []string {
	// Find the last underscore to split timestamp_id
	lastIdx := -1
	for i := len(cursor) - 1; i >= 0; i-- {
		if cursor[i] == '_' {
			lastIdx = i
			break
		}
	}
	if lastIdx == -1 {
		return nil
	}
	return []string{cursor[:lastIdx], cursor[lastIdx+1:]}
}
