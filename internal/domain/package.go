package domain

import (
	"context"
	"time"
)

// Package represents a purchasable plan/subscription offering
type Package struct {
	ID             string    `bson:"_id,omitempty" json:"id"`
	Name           string    `bson:"name,omitempty" json:"name"`
	Description    string    `bson:"description,omitempty" json:"description"`
	Price          int64     `bson:"price,omitempty" json:"price"` // Price in smallest currency unit (e.g., IDR)
	DurationMonths int       `bson:"duration_months,omitempty" json:"duration_months"`
	IsActive       bool      `bson:"is_active,omitempty" json:"is_active"`
	CreatedAt      time.Time `bson:"created_at,omitempty" json:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at,omitempty" json:"updated_at"`
}

// PackageRepository defines operations for managing packages
type PackageRepository interface {
	Create(ctx context.Context, pkg *Package) error
	GetByID(ctx context.Context, id string) (*Package, error)
	GetActivePackages(ctx context.Context) ([]*Package, error)
	Update(ctx context.Context, pkg *Package) error
}
