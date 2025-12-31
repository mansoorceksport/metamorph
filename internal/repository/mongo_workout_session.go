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

type MongoWorkoutSessionRepository struct {
	collection *mongo.Collection
}

func NewMongoWorkoutSessionRepository(db *mongo.Database) *MongoWorkoutSessionRepository {
	return &MongoWorkoutSessionRepository{
		collection: db.Collection("workout_sessions"),
	}
}

func (r *MongoWorkoutSessionRepository) Create(ctx context.Context, session *domain.WorkoutSession) error {
	session.CreatedAt = time.Now()
	session.UpdatedAt = time.Now()

	result, err := r.collection.InsertOne(ctx, session)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		session.ID = oid.Hex()
	}
	return nil
}

func (r *MongoWorkoutSessionRepository) GetByID(ctx context.Context, id string) (*domain.WorkoutSession, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var session domain.WorkoutSession
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

func (r *MongoWorkoutSessionRepository) GetByScheduleID(ctx context.Context, scheduleID string) (*domain.WorkoutSession, error) {
	var session domain.WorkoutSession
	err := r.collection.FindOne(ctx, bson.M{"schedule_id": scheduleID}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

func (r *MongoWorkoutSessionRepository) Update(ctx context.Context, session *domain.WorkoutSession) error {
	oid, err := primitive.ObjectIDFromHex(session.ID)
	if err != nil {
		return domain.ErrInvalidID
	}
	session.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			"planned_exercises": session.PlannedExercises,
			"updated_at":        session.UpdatedAt,
		},
	}

	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	return err
}
