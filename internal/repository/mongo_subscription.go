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

// MongoSubscriptionRepository implements domain.SubscriptionRepository
type MongoSubscriptionRepository struct {
	collection *mongo.Collection
}

// NewMongoSubscriptionRepository creates a new subscription repository
// Note: No index creation to ensure zero-impact deployment on existing collections
func NewMongoSubscriptionRepository(db *mongo.Database) *MongoSubscriptionRepository {
	coll := db.Collection("subscriptions")
	return &MongoSubscriptionRepository{
		collection: coll,
	}
}

func (r *MongoSubscriptionRepository) Create(ctx context.Context, subscription *domain.Subscription) error {
	now := time.Now().UTC()
	subscription.CreatedAt = now

	objID := primitive.NewObjectID()
	subscription.ID = objID.Hex()

	doc := bson.M{
		"_id":        objID,
		"user_id":    subscription.UserID,
		"invoice_id": subscription.InvoiceID,
		"start_date": subscription.StartDate,
		"end_date":   subscription.EndDate,
		"created_at": subscription.CreatedAt,
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	return nil
}

func (r *MongoSubscriptionRepository) GetByID(ctx context.Context, id string) (*domain.Subscription, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid subscription id: %w", err)
	}

	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}
	return mapBsonToSubscription(raw), nil
}

func (r *MongoSubscriptionRepository) GetByUserID(ctx context.Context, userID string) ([]*domain.Subscription, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by user: %w", err)
	}
	defer cursor.Close(ctx)

	var subscriptions []*domain.Subscription
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, mapBsonToSubscription(raw))
	}
	return subscriptions, nil
}

func (r *MongoSubscriptionRepository) GetActiveByUserID(ctx context.Context, userID string) (*domain.Subscription, error) {
	now := time.Now().UTC()

	filter := bson.M{
		"user_id":  userID,
		"end_date": bson.M{"$gt": now},
	}

	var raw bson.M
	if err := r.collection.FindOne(ctx, filter).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get active subscription: %w", err)
	}
	return mapBsonToSubscription(raw), nil
}

func mapBsonToSubscription(raw bson.M) *domain.Subscription {
	sub := &domain.Subscription{}

	if oid, ok := raw["_id"].(primitive.ObjectID); ok {
		sub.ID = oid.Hex()
	}
	if userID, ok := raw["user_id"].(string); ok {
		sub.UserID = userID
	}
	if invoiceID, ok := raw["invoice_id"].(string); ok {
		sub.InvoiceID = invoiceID
	}
	if startDate, ok := raw["start_date"].(primitive.DateTime); ok {
		sub.StartDate = startDate.Time()
	}
	if endDate, ok := raw["end_date"].(primitive.DateTime); ok {
		sub.EndDate = endDate.Time()
	}
	if created, ok := raw["created_at"].(primitive.DateTime); ok {
		sub.CreatedAt = created.Time()
	}

	return sub
}
