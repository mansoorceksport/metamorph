package repository

import (
	"context"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoPersonalBestRepository struct {
	collection *mongo.Collection
}

func NewMongoPersonalBestRepository(db *mongo.Database) *MongoPersonalBestRepository {
	return &MongoPersonalBestRepository{
		collection: db.Collection("personal_bests"),
	}
}

func (r *MongoPersonalBestRepository) GetByMemberAndExercise(ctx context.Context, memberID, exerciseID string) (*domain.PersonalBest, error) {
	var pb domain.PersonalBest
	err := r.collection.FindOne(ctx, bson.M{
		"member_id":   memberID,
		"exercise_id": exerciseID,
	}).Decode(&pb)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No PB exists yet
		}
		return nil, err
	}
	return &pb, nil
}

// Upsert creates or updates a PB if the new weight exceeds the existing one
// Returns true if a new PB was set
func (r *MongoPersonalBestRepository) Upsert(ctx context.Context, pb *domain.PersonalBest) (bool, error) {
	now := time.Now()

	// Try to find existing PB
	existing, err := r.GetByMemberAndExercise(ctx, pb.MemberID, pb.ExerciseID)
	if err != nil {
		return false, err
	}

	// Check if this beats the existing PB
	if existing != nil && pb.Weight <= existing.Weight {
		return false, nil // Not a new PB
	}

	if existing == nil {
		// Create new PB
		pb.CreatedAt = now
		pb.UpdatedAt = now
		pb.AchievedAt = now

		result, err := r.collection.InsertOne(ctx, pb)
		if err != nil {
			return false, err
		}
		if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
			pb.ID = oid.Hex()
		}
		return true, nil
	}

	// Update existing PB
	update := bson.M{
		"$set": bson.M{
			"weight":      pb.Weight,
			"reps":        pb.Reps,
			"achieved_at": now,
			"schedule_id": pb.ScheduleID,
			"updated_at":  now,
		},
	}

	_, err = r.collection.UpdateOne(ctx,
		bson.M{"member_id": pb.MemberID, "exercise_id": pb.ExerciseID},
		update,
	)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *MongoPersonalBestRepository) GetByMember(ctx context.Context, memberID string) ([]*domain.PersonalBest, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"member_id": memberID}, options.Find().SetSort(bson.M{"exercise_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pbs []*domain.PersonalBest
	if err := cursor.All(ctx, &pbs); err != nil {
		return nil, err
	}
	return pbs, nil
}
