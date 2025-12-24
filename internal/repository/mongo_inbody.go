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
	collection *mongo.Collection
}

// NewMongoInBodyRepository creates a new MongoDB repository
func NewMongoInBodyRepository(db *mongo.Database) *MongoInBodyRepository {
	collection := db.Collection(collectionName)

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

	return &MongoInBodyRepository{
		collection: collection,
	}
}

// Create saves a new InBodyRecord to MongoDB
func (r *MongoInBodyRepository) Create(ctx context.Context, record *domain.InBodyRecord) error {
	// Generate new ObjectID if not provided
	if record.ID == "" {
		record.ID = primitive.NewObjectID().Hex()
	}

	// Set processed timestamp
	record.Metadata.ProcessedAt = time.Now()

	_, err := r.collection.InsertOne(ctx, record)
	if err != nil {
		return fmt.Errorf("failed to insert inbody record: %w", err)
	}

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
