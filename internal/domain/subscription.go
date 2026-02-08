package domain

import (
	"context"
	"time"
)

// Subscription represents the historical record of successful access
type Subscription struct {
	ID        string    `bson:"_id,omitempty" json:"id"`
	UserID    string    `bson:"user_id,omitempty" json:"user_id"`
	InvoiceID string    `bson:"invoice_id,omitempty" json:"invoice_id"`
	StartDate time.Time `bson:"start_date,omitempty" json:"start_date"`
	EndDate   time.Time `bson:"end_date,omitempty" json:"end_date"`
	CreatedAt time.Time `bson:"created_at,omitempty" json:"created_at"`
}

// CalculateNewEndDate calculates the new subscription end date based on stacking logic.
// If currentEnd is in the future, the new date extends from currentEnd.
// If currentEnd is in the past or nil, the new date starts from now.
// All time calculations use UTC for consistency across server environments.
func CalculateNewEndDate(currentEnd *time.Time, durationMonths int) time.Time {
	now := time.Now().UTC()

	if currentEnd != nil && currentEnd.After(now) {
		// Stack: extend from current end date
		return currentEnd.AddDate(0, durationMonths, 0)
	}

	// Fresh start: extend from now
	return now.AddDate(0, durationMonths, 0)
}

// SubscriptionRepository defines operations for managing subscriptions
type SubscriptionRepository interface {
	Create(ctx context.Context, subscription *Subscription) error
	GetByID(ctx context.Context, id string) (*Subscription, error)
	GetByUserID(ctx context.Context, userID string) ([]*Subscription, error)
	GetActiveByUserID(ctx context.Context, userID string) (*Subscription, error)
}
