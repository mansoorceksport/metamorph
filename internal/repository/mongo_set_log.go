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

type MongoSetLogRepository struct {
	collection *mongo.Collection
}

func NewMongoSetLogRepository(db *mongo.Database) *MongoSetLogRepository {
	return &MongoSetLogRepository{
		collection: db.Collection("set_logs"),
	}
}

func (r *MongoSetLogRepository) Create(ctx context.Context, setLog *domain.SetLogDocument) error {
	setLog.CreatedAt = time.Now()
	setLog.UpdatedAt = time.Now()

	result, err := r.collection.InsertOne(ctx, setLog)
	if err != nil {
		return fmt.Errorf("failed to create set log: %w", err)
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		setLog.ID = oid.Hex()
	}
	return nil
}

func (r *MongoSetLogRepository) GetByID(ctx context.Context, id string) (*domain.SetLogDocument, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var setLog domain.SetLogDocument
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&setLog)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrSessionNotFound // Reuse existing error
		}
		return nil, err
	}
	return &setLog, nil
}

func (r *MongoSetLogRepository) GetByClientID(ctx context.Context, clientID string) (*domain.SetLogDocument, error) {
	var setLog domain.SetLogDocument
	err := r.collection.FindOne(ctx, bson.M{"client_id": clientID}).Decode(&setLog)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	return &setLog, nil
}

func (r *MongoSetLogRepository) GetByPlannedExerciseID(ctx context.Context, plannedExerciseID string) ([]*domain.SetLogDocument, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"planned_exercise_id": plannedExerciseID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var setLogs []*domain.SetLogDocument
	if err := cursor.All(ctx, &setLogs); err != nil {
		return nil, err
	}
	return setLogs, nil
}

func (r *MongoSetLogRepository) GetByScheduleID(ctx context.Context, scheduleID string) ([]*domain.SetLogDocument, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"schedule_id": scheduleID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var setLogs []*domain.SetLogDocument
	if err := cursor.All(ctx, &setLogs); err != nil {
		return nil, err
	}
	return setLogs, nil
}

func (r *MongoSetLogRepository) Update(ctx context.Context, setLog *domain.SetLogDocument) error {
	oid, err := primitive.ObjectIDFromHex(setLog.ID)
	if err != nil {
		return domain.ErrInvalidID
	}

	setLog.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			"weight":     setLog.Weight,
			"reps":       setLog.Reps,
			"remarks":    setLog.Remarks,
			"completed":  setLog.Completed,
			"set_index":  setLog.SetIndex,
			"updated_at": setLog.UpdatedAt,
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return domain.ErrSessionNotFound
	}
	return nil
}

func (r *MongoSetLogRepository) Delete(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrInvalidID
	}

	_, err = r.collection.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}

func (r *MongoSetLogRepository) DeleteByPlannedExerciseID(ctx context.Context, plannedExerciseID string) error {
	_, err := r.collection.DeleteMany(ctx, bson.M{"planned_exercise_id": plannedExerciseID})
	return err
}

func (r *MongoSetLogRepository) DeleteByScheduleID(ctx context.Context, scheduleID string) error {
	_, err := r.collection.DeleteMany(ctx, bson.M{"schedule_id": scheduleID})
	return err
}
