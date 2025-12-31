package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoPTContractRepository struct {
	collection *mongo.Collection
}

func NewMongoPTContractRepository(db *mongo.Database) *MongoPTContractRepository {
	return &MongoPTContractRepository{
		collection: db.Collection("pt_contracts"),
	}
}

func (r *MongoPTContractRepository) Create(ctx context.Context, contract *domain.PTContract) error {
	contract.CreatedAt = time.Now()
	contract.UpdatedAt = time.Now()

	result, err := r.collection.InsertOne(ctx, contract)
	if err != nil {
		return fmt.Errorf("failed to create pt contract: %w", err)
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		contract.ID = oid.Hex()
	}
	return nil
}

func (r *MongoPTContractRepository) GetByID(ctx context.Context, id string) (*domain.PTContract, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrInvalidID
	}

	var contract domain.PTContract
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&contract)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrContractNotFound
		}
		return nil, err
	}
	return &contract, nil
}

func (r *MongoPTContractRepository) GetActiveByMember(ctx context.Context, memberID string) ([]*domain.PTContract, error) {
	filter := bson.M{
		"member_id":          memberID,
		"status":             domain.PackageStatusActive,
		"remaining_sessions": bson.M{"$gt": 0},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var contracts []*domain.PTContract
	if err := cursor.All(ctx, &contracts); err != nil {
		return nil, err
	}
	return contracts, nil
}

func (r *MongoPTContractRepository) GetByTenant(ctx context.Context, tenantID string) ([]*domain.PTContract, error) {
	filter := bson.M{"tenant_id": tenantID}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var contracts []*domain.PTContract
	if err := cursor.All(ctx, &contracts); err != nil {
		return nil, err
	}
	return contracts, nil
}

func (r *MongoPTContractRepository) GetActiveByCoach(ctx context.Context, coachID string) ([]*domain.PTContract, error) {
	filter := bson.M{
		"coach_id": coachID,
		"status":   domain.PackageStatusActive,
		// Should we filter remaining > 0? Usually yes for "Active" clients.
		"remaining_sessions": bson.M{"$gt": 0},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var contracts []*domain.PTContract
	if err := cursor.All(ctx, &contracts); err != nil {
		return nil, err
	}
	return contracts, nil
}

// DecrementSession atomically decrements remaining_sessions and updates status if needed
func (r *MongoPTContractRepository) DecrementSession(ctx context.Context, contractID string) error {
	oid, err := primitive.ObjectIDFromHex(contractID)
	if err != nil {
		return domain.ErrInvalidID
	}

	// 1. Atomically decrement if > 0
	filter := bson.M{
		"_id":                oid,
		"remaining_sessions": bson.M{"$gt": 0},
	}
	update := bson.M{
		"$inc": bson.M{"remaining_sessions": -1},
		"$set": bson.M{"updated_at": time.Now()},
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updatedContract domain.PTContract

	err = r.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updatedContract)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.ErrPackageDepleted // Effectively depleted or not found
		}
		return fmt.Errorf("failed to decrement session: %w", err)
	}

	// 2. If reached 0, update status to Depleted
	if updatedContract.RemainingSessions == 0 {
		_, err := r.collection.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{
			"$set": bson.M{"status": domain.PackageStatusDepleted},
		})
		if err != nil {
			fmt.Printf("Warning: Failed to update status to Depleted for contract %s: %v\n", contractID, err)
		}
	}

	return nil
}

func (r *MongoPTContractRepository) UpdateStatus(ctx context.Context, contractID string, status string) error {
	oid, err := primitive.ObjectIDFromHex(contractID)
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
