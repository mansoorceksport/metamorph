package domain

import (
	"context"
	"time"
)

// Invoice status constants
const (
	InvoiceStatusPending = "pending"
	InvoiceStatusPaid    = "paid"
	InvoiceStatusExpired = "expired"
	InvoiceStatusFailed  = "failed"
)

// Invoice represents a payment intent (specifically for iPaymu VA)
type Invoice struct {
	ID               string    `bson:"_id,omitempty" json:"id"`
	UserID           string    `bson:"user_id,omitempty" json:"user_id"`
	PackageID        string    `bson:"package_id,omitempty" json:"package_id"`
	Amount           int64     `bson:"amount,omitempty" json:"amount"` // Amount in smallest currency unit
	Status           string    `bson:"status,omitempty" json:"status"` // pending, paid, expired, failed
	VANumber         string    `bson:"va_number,omitempty" json:"va_number"`
	PaymentMethod    string    `bson:"payment_method,omitempty" json:"payment_method"` // BCA, Mandiri, BNI
	PaymentSessionID string    `bson:"payment_session_id,omitempty" json:"payment_session_id"`
	ExpiryDate       time.Time `bson:"expiry_date,omitempty" json:"expiry_date"` // VA expires after 24h
	CreatedAt        time.Time `bson:"created_at,omitempty" json:"created_at"`
	UpdatedAt        time.Time `bson:"updated_at,omitempty" json:"updated_at"`
}

// InvoiceRepository defines operations for managing invoices
type InvoiceRepository interface {
	Create(ctx context.Context, invoice *Invoice) error
	GetByID(ctx context.Context, id string) (*Invoice, error)
	GetByUserID(ctx context.Context, userID string) ([]*Invoice, error)
	GetPendingByUserAndPackage(ctx context.Context, userID, packageID string) (*Invoice, error)
	GetByPaymentSessionID(ctx context.Context, sessionID string) (*Invoice, error)
	UpdateStatus(ctx context.Context, id string, status string) error
	Update(ctx context.Context, invoice *Invoice) error
}
