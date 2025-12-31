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

type MongoPTPackageRepository struct {
	collection *mongo.Collection
}

func NewMongoPTPackageRepository(db *mongo.Database) *MongoPTPackageRepository {
	return &MongoPTPackageRepository{
		collection: db.Collection("pt_packages"), // Templates
	}
}

func (r *MongoPTPackageRepository) Create(ctx context.Context, pkg *domain.PTPackage) error {
	pkg.CreatedAt = time.Now()
	pkg.UpdatedAt = time.Now()
	pkg.Active = true // Default to active

	result, err := r.collection.InsertOne(ctx, pkg)
	if err != nil {
		return fmt.Errorf("failed to create pt package template: %w", err)
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		pkg.ID = oid.Hex()
	}
	return nil
}

func (r *MongoPTPackageRepository) GetByID(ctx context.Context, id string) (*domain.PTPackage, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var pkg domain.PTPackage
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&pkg)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrPackageTemplateNotFound
		}
		return nil, err
	}
	return &pkg, nil
}

func (r *MongoPTPackageRepository) GetByTenant(ctx context.Context, tenantID string) ([]*domain.PTPackage, error) {
	// Filter optionally by active? For admins maybe show all.
	filter := bson.M{"tenant_id": tenantID}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var packages []*domain.PTPackage
	if err := cursor.All(ctx, &packages); err != nil {
		return nil, err
	}
	return packages, nil
}

func (r *MongoPTPackageRepository) Update(ctx context.Context, pkg *domain.PTPackage) error {
	oid, err := primitive.ObjectIDFromHex(pkg.ID)
	if err != nil {
		return domain.ErrInvalidID
	}
	pkg.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			"name":           pkg.Name,
			"total_sessions": pkg.TotalSessions,
			"price":          pkg.Price,
			"active":         pkg.Active,
			"updated_at":     pkg.UpdatedAt,
		},
	}

	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": oid}, update)
	return err
}
