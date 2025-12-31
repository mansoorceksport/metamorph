package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type MongoTemplateRepository struct {
	collection *mongo.Collection
}

func NewMongoTemplateRepository(db *mongo.Database) *MongoTemplateRepository {
	return &MongoTemplateRepository{
		collection: db.Collection("workout_templates"),
	}
}

func (r *MongoTemplateRepository) Create(ctx context.Context, tmpl *domain.WorkoutTemplate) error {
	tmpl.CreatedAt = time.Now()
	tmpl.UpdatedAt = time.Now()

	result, err := r.collection.InsertOne(ctx, tmpl)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		tmpl.ID = oid.Hex()
	}
	return nil
}

func (r *MongoTemplateRepository) GetByID(ctx context.Context, id string) (*domain.WorkoutTemplate, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var tmpl domain.WorkoutTemplate
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&tmpl)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrTemplateNotFound
		}
		return nil, err
	}
	return &tmpl, nil
}

func (r *MongoTemplateRepository) List(ctx context.Context) ([]*domain.WorkoutTemplate, error) {
	cursor, err := r.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var templates []*domain.WorkoutTemplate
	if err := cursor.All(ctx, &templates); err != nil {
		return nil, err
	}
	return templates, nil
}

func (r *MongoTemplateRepository) Update(ctx context.Context, tmpl *domain.WorkoutTemplate) error {
	oid, err := primitive.ObjectIDFromHex(tmpl.ID)
	if err != nil {
		return domain.ErrInvalidID
	}
	tmpl.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			"name":         tmpl.Name,
			"gender":       tmpl.Gender,
			"exercise_ids": tmpl.ExerciseIDs,
			"updated_at":   tmpl.UpdatedAt,
		},
	}

	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	return err
}

func (r *MongoTemplateRepository) Delete(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrInvalidID
	}
	_, err = r.collection.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}
