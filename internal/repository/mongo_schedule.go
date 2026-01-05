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

type MongoScheduleRepository struct {
	collection *mongo.Collection
}

func NewMongoScheduleRepository(db *mongo.Database) *MongoScheduleRepository {
	return &MongoScheduleRepository{
		collection: db.Collection("schedules"),
	}
}

func (r *MongoScheduleRepository) Create(ctx context.Context, schedule *domain.Schedule) error {
	schedule.CreatedAt = time.Now()
	schedule.UpdatedAt = time.Now()

	result, err := r.collection.InsertOne(ctx, schedule)
	if err != nil {
		return fmt.Errorf("failed to create schedule: %w", err)
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		schedule.ID = oid.Hex()
	}
	return nil
}

func (r *MongoScheduleRepository) GetByID(ctx context.Context, id string) (*domain.Schedule, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var schedule domain.Schedule
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&schedule)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrScheduleNotFound
		}
		return nil, err
	}
	return &schedule, nil
}

func (r *MongoScheduleRepository) GetByCoach(ctx context.Context, coachID string, from, to time.Time) ([]*domain.Schedule, error) {
	filter := bson.M{
		"coach_id": coachID,
		"start_time": bson.M{
			"$gte": from,
			"$lte": to,
		},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schedules []*domain.Schedule
	if err := cursor.All(ctx, &schedules); err != nil {
		return nil, err
	}
	return schedules, nil
}

func (r *MongoScheduleRepository) GetByMember(ctx context.Context, memberID string, from, to time.Time) ([]*domain.Schedule, error) {
	filter := bson.M{
		"member_id": memberID,
		"start_time": bson.M{
			"$gte": from,
			"$lte": to,
		},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schedules []*domain.Schedule
	if err := cursor.All(ctx, &schedules); err != nil {
		return nil, err
	}
	return schedules, nil
}

func (r *MongoScheduleRepository) List(ctx context.Context, tenantID string, filterOpts map[string]interface{}) ([]*domain.Schedule, error) {
	filter := bson.M{"tenant_id": tenantID}
	for k, v := range filterOpts {
		filter[k] = v
	}

	// Maybe add date range default?
	// If filter has start_time range, apply it.

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schedules []*domain.Schedule
	if err := cursor.All(ctx, &schedules); err != nil {
		return nil, err
	}
	return schedules, nil
}

func (r *MongoScheduleRepository) Update(ctx context.Context, schedule *domain.Schedule) error {
	oid, err := primitive.ObjectIDFromHex(schedule.ID)
	if err != nil {
		return domain.ErrInvalidID
	}
	schedule.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			"start_time": schedule.StartTime,
			"end_time":   schedule.EndTime,
			"status":     schedule.Status,
			"remarks":    schedule.Remarks,
			"updated_at": schedule.UpdatedAt,
		},
	}

	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	return err
}

func (r *MongoScheduleRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrInvalidID
	}

	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	})
	return err
}

func (r *MongoScheduleRepository) Delete(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrInvalidID
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}
	if result.DeletedCount == 0 {
		return domain.ErrScheduleNotFound
	}
	return nil
}

func (r *MongoScheduleRepository) CountByContractAndStatus(ctx context.Context, contractID string, statuses []string) (int64, error) {
	filter := bson.M{
		"contract_id": contractID,
		"status":      bson.M{"$in": statuses},
	}
	return r.collection.CountDocuments(ctx, filter)
}

// GetAttendanceByCoach fetches all schedules for a coach within the last N days
func (r *MongoScheduleRepository) GetAttendanceByCoach(ctx context.Context, coachID string, days int) ([]*domain.Schedule, error) {
	since := time.Now().AddDate(0, 0, -days)
	filter := bson.M{
		"coach_id":   coachID,
		"start_time": bson.M{"$gte": since},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schedules []*domain.Schedule
	if err := cursor.All(ctx, &schedules); err != nil {
		return nil, err
	}
	return schedules, nil
}
