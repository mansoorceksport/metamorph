package domain

import (
	"context"
	"time"
)

// RefreshToken represents a stored refresh token for session management
type RefreshToken struct {
	ID        string    `bson:"_id,omitempty" json:"id"`
	UserID    string    `bson:"user_id" json:"user_id"`
	TokenHash string    `bson:"token_hash" json:"-"` // SHA256 hash, never expose
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UserAgent string    `bson:"user_agent" json:"user_agent"` // Device tracking
	IPAddress string    `bson:"ip_address" json:"ip_address"`
	Revoked   bool      `bson:"revoked" json:"revoked"`
}

// IsExpired checks if the refresh token has expired
func (r *RefreshToken) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// IsValid checks if the token is valid (not expired and not revoked)
func (r *RefreshToken) IsValid() bool {
	return !r.IsExpired() && !r.Revoked
}

// RefreshTokenRepository defines the interface for refresh token storage
type RefreshTokenRepository interface {
	// Create stores a new refresh token
	Create(ctx context.Context, token *RefreshToken) error

	// FindByHash retrieves a token by its hash
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)

	// RevokeByHash revokes a specific token
	RevokeByHash(ctx context.Context, hash string) error

	// RevokeAllByUserID revokes all refresh tokens for a user (force logout)
	RevokeAllByUserID(ctx context.Context, userID string) error

	// DeleteExpired removes expired tokens (cleanup job)
	DeleteExpired(ctx context.Context) error
}
