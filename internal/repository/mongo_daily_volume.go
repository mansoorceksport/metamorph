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

type MongoDailyVolumeRepository struct {
	collection *mongo.Collection
}

func NewMongoDailyVolumeRepository(db *mongo.Database) *MongoDailyVolumeRepository {
	return &MongoDailyVolumeRepository{
		collection: db.Collection("daily_volumes"),
	}
}

func (r *MongoDailyVolumeRepository) Create(ctx context.Context, volume *domain.DailyVolume) error {
	volume.CreatedAt = time.Now()
	result, err := r.collection.InsertOne(ctx, volume)
	if err != nil {
		return err
	}
	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		volume.ID = oid.Hex()
	}
	return nil
}

func (r *MongoDailyVolumeRepository) GetByScheduleID(ctx context.Context, scheduleID string) (*domain.DailyVolume, error) {
	var volume domain.DailyVolume
	err := r.collection.FindOne(ctx, bson.M{"schedule_id": scheduleID}).Decode(&volume)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &volume, nil
}

func (r *MongoDailyVolumeRepository) GetByMemberID(ctx context.Context, memberID string, limit int) ([]*domain.DailyVolume, error) {
	opts := options.Find().SetSort(bson.D{{Key: "date", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := r.collection.Find(ctx, bson.M{"member_id": memberID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var volumes []*domain.DailyVolume
	if err = cursor.All(ctx, &volumes); err != nil {
		return nil, err
	}
	return volumes, nil
}

func (r *MongoDailyVolumeRepository) GetByMemberIDAndDateRange(ctx context.Context, memberID string, from, to time.Time) ([]*domain.DailyVolume, error) {
	filter := bson.M{
		"member_id": memberID,
		"date": bson.M{
			"$gte": from,
			"$lte": to,
		},
	}
	opts := options.Find().SetSort(bson.D{{Key: "date", Value: 1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var volumes []*domain.DailyVolume
	if err = cursor.All(ctx, &volumes); err != nil {
		return nil, err
	}
	return volumes, nil
}

// GetByMemberIDAndFocusArea retrieves volume records, optionally filtered by focus area
func (r *MongoDailyVolumeRepository) GetByMemberIDAndFocusArea(ctx context.Context, memberID string, limit int, focusArea string) ([]*domain.DailyVolume, error) {
	filter := bson.M{"member_id": memberID}
	if focusArea != "" {
		filter["focus_area"] = focusArea
	}

	opts := options.Find().SetSort(bson.D{{Key: "date", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var volumes []*domain.DailyVolume
	if err = cursor.All(ctx, &volumes); err != nil {
		return nil, err
	}
	return volumes, nil
}

func (r *MongoDailyVolumeRepository) Delete(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = r.collection.DeleteOne(ctx, bson.M{"_id": objID})
	return err
}

func (r *MongoDailyVolumeRepository) DeleteByScheduleID(ctx context.Context, scheduleID string) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"schedule_id": scheduleID})
	return err
}
