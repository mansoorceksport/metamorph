package repository

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoPackageRepository implements domain.PackageRepository
type MongoPackageRepository struct {
	collection *mongo.Collection
}

// NewMongoPackageRepository creates a new package repository
// Note: No index creation to ensure zero-impact deployment on existing collections
func NewMongoPackageRepository(db *mongo.Database) *MongoPackageRepository {
	coll := db.Collection("packages")
	return &MongoPackageRepository{
		collection: coll,
	}
}

func (r *MongoPackageRepository) Create(ctx context.Context, pkg *domain.Package) error {
	now := time.Now().UTC()
	pkg.CreatedAt = now
	pkg.UpdatedAt = now

	doc := bson.M{
		"_id":             pkg.ID, // Using string ID (e.g., "pkg_annual_pro_12")
		"name":            pkg.Name,
		"description":     pkg.Description,
		"price":           pkg.Price,
		"duration_months": pkg.DurationMonths,
		"is_active":       pkg.IsActive,
		"created_at":      pkg.CreatedAt,
		"updated_at":      pkg.UpdatedAt,
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create package: %w", err)
	}
	return nil
}

func (r *MongoPackageRepository) GetByID(ctx context.Context, id string) (*domain.Package, error) {
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get package: %w", err)
	}
	return mapBsonToPackage(raw), nil
}

func (r *MongoPackageRepository) GetActivePackages(ctx context.Context) ([]*domain.Package, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"is_active": true})
	if err != nil {
		return nil, fmt.Errorf("failed to list active packages: %w", err)
	}
	defer cursor.Close(ctx)

	var packages []*domain.Package
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		packages = append(packages, mapBsonToPackage(raw))
	}
	return packages, nil
}

func (r *MongoPackageRepository) Update(ctx context.Context, pkg *domain.Package) error {
	pkg.UpdatedAt = time.Now().UTC()

	update := bson.M{
		"$set": bson.M{
			"name":            pkg.Name,
			"description":     pkg.Description,
			"price":           pkg.Price,
			"duration_months": pkg.DurationMonths,
			"is_active":       pkg.IsActive,
			"updated_at":      pkg.UpdatedAt,
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": pkg.ID}, update)
	if err != nil {
		return fmt.Errorf("failed to update package: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SeedDefaultPackages seeds the default packages if they don't exist
// Idempotency: checks by _id (not by name) to prevent duplicates
func (r *MongoPackageRepository) SeedDefaultPackages(ctx context.Context) error {
	const pkgID = "pkg_annual_pro_12"

	// Check if package already exists by ID
	_, err := r.GetByID(ctx, pkgID)
	if err == nil {
		// Package exists, no action needed
		log.Printf("[Seed] Package %s already exists, skipping", pkgID)
		return nil
	}

	if err != domain.ErrNotFound {
		return fmt.Errorf("failed to check package existence: %w", err)
	}

	// Package doesn't exist, create it
	pkg := &domain.Package{
		ID:             pkgID,
		Name:           "Annual Pro Transformation",
		Description:    "12 months of Pro access (10-for-12 deal)",
		Price:          490000, // IDR 490,000
		DurationMonths: 12,
		IsActive:       true,
	}

	if err := r.Create(ctx, pkg); err != nil {
		return fmt.Errorf("failed to seed package: %w", err)
	}

	log.Printf("[Seed] Created package: %s (%s) - Price: %d, Duration: %d months",
		pkg.ID, pkg.Name, pkg.Price, pkg.DurationMonths)

	return nil
}

func mapBsonToPackage(raw bson.M) *domain.Package {
	pkg := &domain.Package{}

	if id, ok := raw["_id"].(string); ok {
		pkg.ID = id
	}
	if name, ok := raw["name"].(string); ok {
		pkg.Name = name
	}
	if desc, ok := raw["description"].(string); ok {
		pkg.Description = desc
	}
	if price, ok := raw["price"].(int64); ok {
		pkg.Price = price
	} else if price, ok := raw["price"].(int32); ok {
		pkg.Price = int64(price)
	}
	if duration, ok := raw["duration_months"].(int32); ok {
		pkg.DurationMonths = int(duration)
	} else if duration, ok := raw["duration_months"].(int64); ok {
		pkg.DurationMonths = int(duration)
	}
	if isActive, ok := raw["is_active"].(bool); ok {
		pkg.IsActive = isActive
	}
	if created, ok := raw["created_at"].(interface{ Time() time.Time }); ok {
		pkg.CreatedAt = created.Time()
	}
	if updated, ok := raw["updated_at"].(interface{ Time() time.Time }); ok {
		pkg.UpdatedAt = updated.Time()
	}

	return pkg
}
