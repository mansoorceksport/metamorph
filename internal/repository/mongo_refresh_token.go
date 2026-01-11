package repository

import (
	"context"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoRefreshTokenRepository implements RefreshTokenRepository using MongoDB
type MongoRefreshTokenRepository struct {
	collection *mongo.Collection
}

// NewMongoRefreshTokenRepository creates a new MongoDB refresh token repository
func NewMongoRefreshTokenRepository(db *mongo.Database) *MongoRefreshTokenRepository {
	collection := db.Collection("refresh_tokens")

	// Create indexes for efficient queries
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Index on token_hash for fast lookups
	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "token_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	// Index on user_id for revoking all tokens
	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}},
	})

	// TTL index for automatic cleanup of expired tokens
	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0), // Expire at the expires_at time
	})

	return &MongoRefreshTokenRepository{
		collection: collection,
	}
}

// Create stores a new refresh token
func (r *MongoRefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	token.CreatedAt = time.Now()
	_, err := r.collection.InsertOne(ctx, token)
	return err
}

// FindByHash retrieves a token by its hash
func (r *MongoRefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	var token domain.RefreshToken
	err := r.collection.FindOne(ctx, bson.M{
		"token_hash": hash,
		"revoked":    false,
	}).Decode(&token)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &token, nil
}

// RevokeByHash revokes a specific token
func (r *MongoRefreshTokenRepository) RevokeByHash(ctx context.Context, hash string) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.M{"token_hash": hash},
		bson.M{"$set": bson.M{"revoked": true}},
	)
	return err
}

// RevokeAllByUserID revokes all refresh tokens for a user (force logout)
func (r *MongoRefreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID string) error {
	_, err := r.collection.UpdateMany(ctx,
		bson.M{"user_id": userID, "revoked": false},
		bson.M{"$set": bson.M{"revoked": true}},
	)
	return err
}

// DeleteExpired removes expired tokens (manual cleanup if TTL index not working)
func (r *MongoRefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	_, err := r.collection.DeleteMany(ctx, bson.M{
		"expires_at": bson.M{"$lt": time.Now()},
	})
	return err
}
