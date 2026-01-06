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
	collection     *mongo.Collection
	planCollection *mongo.Collection
}

func NewMongoWorkoutSessionRepository(db *mongo.Database) *MongoWorkoutSessionRepository {
	return &MongoWorkoutSessionRepository{
		collection:     db.Collection("workout_sessions"),
		planCollection: db.Collection("planned_exercises"),
	}
}

func (r *MongoWorkoutSessionRepository) Create(ctx context.Context, session *domain.WorkoutSession) error {
	session.CreatedAt = time.Now()
	session.UpdatedAt = time.Now()

	// We don't store PlannedExercises in the session document anymore
	// But we might want to ensure they are saved if provided?
	// The Service InitializeSession separates them now.
	// So we just save the session strictly.

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

	// Inflate PlannedExercises
	if err := r.loadPlannedExercises(ctx, &session); err != nil {
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

	// Inflate PlannedExercises
	if err := r.loadPlannedExercises(ctx, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (r *MongoWorkoutSessionRepository) loadPlannedExercises(ctx context.Context, session *domain.WorkoutSession) error {
	cursor, err := r.planCollection.Find(ctx, bson.M{"schedule_id": session.ScheduleID}, options.Find().SetSort(bson.M{"order": 1}))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var exercises []*domain.PlannedExercise
	if err := cursor.All(ctx, &exercises); err != nil {
		return err
	}
	session.PlannedExercises = exercises
	return nil
}

func (r *MongoWorkoutSessionRepository) Update(ctx context.Context, session *domain.WorkoutSession) error {
	oid, err := primitive.ObjectIDFromHex(session.ID)
	if err != nil {
		return domain.ErrInvalidID
	}
	session.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			// "planned_exercises" is no longer here
			"updated_at": session.UpdatedAt,
		},
	}

	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	return err
}

// GetPlannedExercisesByScheduleID retrieves planned exercises directly (ignoring session doc existence)
func (r *MongoWorkoutSessionRepository) GetPlannedExercisesByScheduleID(ctx context.Context, scheduleID string) ([]*domain.PlannedExercise, error) {
	cursor, err := r.planCollection.Find(ctx, bson.M{"schedule_id": scheduleID}, options.Find().SetSort(bson.M{"order": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var exercises []*domain.PlannedExercise
	if err := cursor.All(ctx, &exercises); err != nil {
		return nil, err
	}
	return exercises, nil
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

	// TODO: Use $lookup aggregation for performance instead of N+1?
	// For now, simple loop is fine as batch size is small (filtered by date)
	for _, s := range sessions {
		_ = r.loadPlannedExercises(ctx, s) // ignore error?
	}

	return sessions, nil
}

// UpdatePlannedExercise updates a planned exercise
func (r *MongoWorkoutSessionRepository) UpdatePlannedExercise(ctx context.Context, exercise *domain.PlannedExercise) error {
	oid, err := primitive.ObjectIDFromHex(exercise.ID)
	if err != nil {
		return domain.ErrInvalidID
	}

	update := bson.M{
		"$set": bson.M{
			"target_sets":  exercise.TargetSets,
			"target_reps":  exercise.TargetReps,
			"rest_seconds": exercise.RestSeconds,
			"notes":        exercise.Notes,
			//"order":        exercise.Order, // Order changes might need reordering logic, explicit separate method? For now allow update.
		},
	}

	result, err := r.planCollection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return domain.ErrSessionNotFound // Reuse not found error or specific one
	}
	return nil
}

// GetPlannedExerciseByID retrieves a planned exercise by MongoDB ObjectID
func (r *MongoWorkoutSessionRepository) GetPlannedExerciseByID(ctx context.Context, id string) (*domain.PlannedExercise, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var exercise domain.PlannedExercise
	err = r.planCollection.FindOne(ctx, bson.M{"_id": oid}).Decode(&exercise)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	return &exercise, nil
}

// GetPlannedExerciseByClientID retrieves a planned exercise by frontend ULID (client_id)
func (r *MongoWorkoutSessionRepository) GetPlannedExerciseByClientID(ctx context.Context, clientID string) (*domain.PlannedExercise, error) {
	var exercise domain.PlannedExercise
	err := r.planCollection.FindOne(ctx, bson.M{"client_id": clientID}).Decode(&exercise)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	return &exercise, nil
}

// CountPlannedExercises counts the number of planned exercises for a schedule
func (r *MongoWorkoutSessionRepository) CountPlannedExercises(ctx context.Context, scheduleID string) (int64, error) {
	return r.planCollection.CountDocuments(ctx, bson.M{"schedule_id": scheduleID})
}

// UpsertSetLog atomically updates or inserts a set log using ULID-based targeting
// Targets 'planned_exercises' collection directly using updated schema
func (r *MongoWorkoutSessionRepository) UpsertSetLog(ctx context.Context, sessionID, exerciseID string, setLog *domain.SetLog) error {
	// exerciseID is now the _id of the PlannedExercise document
	oid, err := primitive.ObjectIDFromHex(exerciseID)
	if err != nil {
		return domain.ErrInvalidID
	}

	filter := bson.M{
		"_id": oid,
	}

	// Update existing set using arrayFilters
	update := bson.M{
		"$set": bson.M{
			"sets.$[set].weight":    setLog.Weight,
			"sets.$[set].reps":      setLog.Reps,
			"sets.$[set].remarks":   setLog.Remarks,
			"sets.$[set].completed": setLog.Completed,
			"sets.$[set].set_index": setLog.SetIndex,
		},
	}

	arrayFilters := options.Update().SetArrayFilters(options.ArrayFilters{
		Filters: []interface{}{
			bson.M{"set.ulid": setLog.ULID}, // SetLog still uses ULID for identity within array
		},
	})

	result, err := r.planCollection.UpdateOne(ctx, filter, update, arrayFilters)
	if err != nil {
		return fmt.Errorf("failed to update set: %w", err)
	}

	if result.MatchedCount == 0 {
		return domain.ErrExerciseULIDNotFound
	}

	if result.ModifiedCount == 0 {
		// Push new set if not found
		pushUpdate := bson.M{
			"$push": bson.M{
				"sets": setLog,
			},
		}
		_, err = r.planCollection.UpdateOne(ctx, filter, pushUpdate)
		if err != nil {
			return fmt.Errorf("failed to push new set: %w", err)
		}
	}

	return nil
}

func (r *MongoWorkoutSessionRepository) AddPlannedExercise(ctx context.Context, exercise *domain.PlannedExercise) error {
	// Ensure schedule_id is present
	if exercise.ScheduleID == "" {
		return fmt.Errorf("schedule_id required")
	}

	res, err := r.planCollection.InsertOne(ctx, exercise)
	if err != nil {
		return err
	}
	if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
		exercise.ID = oid.Hex()
	}
	return nil
}

func (r *MongoWorkoutSessionRepository) RemovePlannedExercise(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrInvalidID
	}
	_, err = r.planCollection.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}

// DeletePlannedExercisesBySchedule deletes all planned exercises for a schedule (cascade delete)
func (r *MongoWorkoutSessionRepository) DeletePlannedExercisesBySchedule(ctx context.Context, scheduleID string) error {
	_, err := r.planCollection.DeleteMany(ctx, bson.M{"schedule_id": scheduleID})
	return err
}
