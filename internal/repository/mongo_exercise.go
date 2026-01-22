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

type MongoExerciseRepository struct {
	collection *mongo.Collection
}

func NewMongoExerciseRepository(db *mongo.Database) *MongoExerciseRepository {
	coll := db.Collection("exercises")

	// Create Index
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mod := mongo.IndexModel{
		Keys:    bson.M{"name": 1},
		Options: options.Index().SetUnique(true),
	}
	coll.Indexes().CreateOne(ctx, mod)

	return &MongoExerciseRepository{
		collection: coll,
	}
}

func (r *MongoExerciseRepository) Create(ctx context.Context, ex *domain.Exercise) error {
	ex.CreatedAt = time.Now()
	ex.UpdatedAt = time.Now()

	result, err := r.collection.InsertOne(ctx, ex)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrDuplicateExercise
		}
		return fmt.Errorf("failed to create exercise: %w", err)
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		ex.ID = oid.Hex()
	}
	return nil
}

func (r *MongoExerciseRepository) GetByID(ctx context.Context, id string) (*domain.Exercise, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var ex domain.Exercise
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&ex)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrExerciseNotFound
		}
		return nil, err
	}
	return &ex, nil
}

// GetByIDs retrieves multiple exercises by their IDs in a single query (batch lookup)
func (r *MongoExerciseRepository) GetByIDs(ctx context.Context, ids []string) ([]*domain.Exercise, error) {
	if len(ids) == 0 {
		return []*domain.Exercise{}, nil
	}

	// Convert string IDs to ObjectIDs
	oids := make([]primitive.ObjectID, 0, len(ids))
	for _, id := range ids {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			continue // Skip invalid IDs
		}
		oids = append(oids, oid)
	}

	if len(oids) == 0 {
		return []*domain.Exercise{}, nil
	}

	// Use $in operator for batch lookup
	cursor, err := r.collection.Find(ctx, bson.M{"_id": bson.M{"$in": oids}})
	if err != nil {
		return nil, fmt.Errorf("failed to find exercises: %w", err)
	}
	defer cursor.Close(ctx)

	var exercises []*domain.Exercise
	if err := cursor.All(ctx, &exercises); err != nil {
		return nil, fmt.Errorf("failed to decode exercises: %w", err)
	}

	return exercises, nil
}

func (r *MongoExerciseRepository) List(ctx context.Context, filter map[string]interface{}) ([]*domain.Exercise, error) {
	// Simple find. Could expand filter logic.
	query := bson.M{}
	if name, ok := filter["name"].(string); ok && name != "" {
		query["name"] = bson.M{"$regex": name, "$options": "i"}
	}

	cursor, err := r.collection.Find(ctx, query)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var exercises []*domain.Exercise
	if err := cursor.All(ctx, &exercises); err != nil {
		return nil, err
	}
	return exercises, nil
}

func (r *MongoExerciseRepository) Update(ctx context.Context, ex *domain.Exercise) error {
	oid, err := primitive.ObjectIDFromHex(ex.ID)
	if err != nil {
		return domain.ErrInvalidID
	}
	ex.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			"name":          ex.Name,
			"muscle_group":  ex.MuscleGroup,
			"equipment":     ex.Equipment,
			"video_url":     ex.VideoURL,
			"reference_url": ex.ReferenceURL,
			"updated_at":    ex.UpdatedAt,
		},
	}

	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	return err
}

func (r *MongoExerciseRepository) Delete(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrInvalidID
	}
	_, err = r.collection.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}
