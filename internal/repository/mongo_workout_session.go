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

// GetSessionsByCoachAndDateRange retrieves all workout sessions for a coach within a date range
func (r *MongoWorkoutSessionRepository) GetSessionsByCoachAndDateRange(ctx context.Context, coachID string, from, to time.Time) ([]*domain.WorkoutSession, error) {
	filter := bson.M{
		"coach_id":   coachID,
		"created_at": bson.M{"$gte": from, "$lte": to},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []*domain.WorkoutSession
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// UpsertSetLog atomically updates or inserts a set log using ULID-based targeting
// Uses MongoDB arrayFilters for precise nested array updates
func (r *MongoWorkoutSessionRepository) UpsertSetLog(ctx context.Context, sessionID, exerciseULID string, setLog *domain.SetLog) error {
	oid, err := primitive.ObjectIDFromHex(sessionID)
	if err != nil {
		return domain.ErrInvalidID
	}

	// First, try to update an existing set with the same ULID
	filter := bson.M{
		"_id":                    oid,
		"planned_exercises.ulid": exerciseULID,
	}

	// Update existing set using arrayFilters
	update := bson.M{
		"$set": bson.M{
			"planned_exercises.$[ex].sets.$[set].weight":    setLog.Weight,
			"planned_exercises.$[ex].sets.$[set].reps":      setLog.Reps,
			"planned_exercises.$[ex].sets.$[set].remarks":   setLog.Remarks,
			"planned_exercises.$[ex].sets.$[set].completed": setLog.Completed,
			"planned_exercises.$[ex].sets.$[set].set_index": setLog.SetIndex,
			"updated_at": time.Now(),
		},
	}

	arrayFilters := options.Update().SetArrayFilters(options.ArrayFilters{
		Filters: []interface{}{
			bson.M{"ex.ulid": exerciseULID},
			bson.M{"set.ulid": setLog.ULID},
		},
	})

	result, err := r.collection.UpdateOne(ctx, filter, update, arrayFilters)
	if err != nil {
		return fmt.Errorf("failed to update set: %w", err)
	}

	// If no document matched, exercise ULID doesn't exist
	if result.MatchedCount == 0 {
		return domain.ErrExerciseULIDNotFound
	}

	// If set was not modified (ULID not found), push new set
	if result.ModifiedCount == 0 {
		pushUpdate := bson.M{
			"$push": bson.M{
				"planned_exercises.$[ex].sets": setLog,
			},
			"$set": bson.M{
				"updated_at": time.Now(),
			},
		}

		pushArrayFilters := options.Update().SetArrayFilters(options.ArrayFilters{
			Filters: []interface{}{
				bson.M{"ex.ulid": exerciseULID},
			},
		})

		_, err = r.collection.UpdateOne(ctx, filter, pushUpdate, pushArrayFilters)
		if err != nil {
			return fmt.Errorf("failed to push new set: %w", err)
		}
	}

	return nil
}
