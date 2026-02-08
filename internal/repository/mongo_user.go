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

// MongoUserRepository implements domain.UserRepository
type MongoUserRepository struct {
	collection *mongo.Collection
}

func NewMongoUserRepository(db *mongo.Database) *MongoUserRepository {
	coll := db.Collection("users")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create unique indexes on email and firebase_uid
	// firebase_uid is sparse (allows empty values, only indexes non-empty)
	_, _ = coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "firebase_uid", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "tenant_id", Value: 1}}},
		{Keys: bson.D{{Key: "roles", Value: 1}}},
	})

	return &MongoUserRepository{
		collection: coll,
	}
}

func (r *MongoUserRepository) Create(ctx context.Context, user *domain.User) error {
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	objID := primitive.NewObjectID()
	user.ID = objID.Hex()

	doc := bson.M{
		"_id":            objID,
		"email":          user.Email,
		"name":           user.Name,
		"roles":          user.Roles,
		"tenant_id":      user.TenantID,
		"branch_access":  user.BranchAccess,
		"home_branch_id": user.HomeBranchID,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	}

	if user.FirebaseUID != "" {
		doc["firebase_uid"] = user.FirebaseUID
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *MongoUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return mapBsonToUser(raw), nil
}

func (r *MongoUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"email": email}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return mapBsonToUser(raw), nil
}

func (r *MongoUserRepository) GetByFirebaseUID(ctx context.Context, uid string) (*domain.User, error) {
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"firebase_uid": uid}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by uid: %w", err)
	}
	return mapBsonToUser(raw), nil
}

func (r *MongoUserRepository) Update(ctx context.Context, user *domain.User) error {
	objID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	user.UpdatedAt = time.Now()
	// Update method
	update := bson.M{
		"$set": bson.M{
			"name":           user.Name,
			"email":          user.Email,
			"roles":          user.Roles,
			"tenant_id":      user.TenantID,
			"branch_access":  user.BranchAccess,
			"home_branch_id": user.HomeBranchID,
			"updated_at":     user.UpdatedAt,
		},
	}

	if user.FirebaseUID != "" {
		update["$set"].(bson.M)["firebase_uid"] = user.FirebaseUID
	}

	// Include entitlement fields if set
	if user.TrialEndDate != nil {
		update["$set"].(bson.M)["trial_end_date"] = user.TrialEndDate
	}
	if user.SubscriptionEndDate != nil {
		update["$set"].(bson.M)["subscription_end_date"] = user.SubscriptionEndDate
	}
	if user.FirstLoginAt != nil {
		update["$set"].(bson.M)["first_login_at"] = user.FirstLoginAt
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MongoUserRepository) Delete(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	_, err = r.collection.DeleteOne(ctx, bson.M{"_id": objID})
	return err
}

func (r *MongoUserRepository) UpsertByFirebaseUID(ctx context.Context, user *domain.User) error {
	filter := bson.M{"firebase_uid": user.FirebaseUID}

	// Generate ObjectID for potential insert
	objID := primitive.NewObjectID()
	now := time.Now()

	update := bson.M{
		"$setOnInsert": bson.M{
			"_id":          objID,
			"firebase_uid": user.FirebaseUID,
			"created_at":   now,
		},
		"$set": bson.M{
			"email":      user.Email,
			"name":       user.Name,
			"roles":      user.Roles,
			"updated_at": now,
		},
	}

	// Only set tenant_id if provided
	if user.TenantID != "" {
		update["$set"].(bson.M)["tenant_id"] = user.TenantID
	}

	opts := options.Update().SetUpsert(true)
	result, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert user: %w", err)
	}

	if result.UpsertedID != nil {
		user.ID = objID.Hex()
	} else {
		// Fetch to get current state
		existing, err := r.GetByFirebaseUID(ctx, user.FirebaseUID)
		if err == nil {
			user.ID = existing.ID
			user.TenantID = existing.TenantID
		}
	}

	return nil
}

func (r *MongoUserRepository) AddRole(ctx context.Context, userID string, role string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	// Use $addToSet to prevent duplicate roles
	update := bson.M{
		"$addToSet": bson.M{"roles": role},
		"$set":      bson.M{"updated_at": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to add role: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MongoUserRepository) UpdateFirebaseUID(ctx context.Context, userID string, firebaseUID string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	update := bson.M{
		"$set": bson.M{
			"firebase_uid": firebaseUID,
			"updated_at":   time.Now(),
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to update firebase uid: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MongoUserRepository) RemoveRole(ctx context.Context, userID string, role string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	update := bson.M{
		"$pull": bson.M{"roles": role},
		"$set":  bson.M{"updated_at": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// RecordLogin updates first_login_at (only if not set), last_login_at, and increments login_count
func (r *MongoUserRepository) RecordLogin(ctx context.Context, userID string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	now := time.Now()

	// Use $setOnInsert-like behavior with conditional update
	// First, try to set first_login_at if it doesn't exist
	update := bson.M{
		"$set": bson.M{
			"last_login_at": now,
			"updated_at":    now,
		},
		"$inc": bson.M{"login_count": 1},
	}

	// First update: always update last_login_at and increment count
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to record login: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}

	// Second update: set first_login_at only if it doesn't exist
	_, _ = r.collection.UpdateOne(ctx, bson.M{
		"_id":            objID,
		"first_login_at": bson.M{"$exists": false},
	}, bson.M{
		"$set": bson.M{"first_login_at": now},
	})

	return nil
}

func (r *MongoUserRepository) GetAll(ctx context.Context) ([]*domain.User, error) {
	cursor, err := r.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer cursor.Close(ctx)

	var users []*domain.User
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		users = append(users, mapBsonToUser(raw))
	}
	return users, nil
}

func (r *MongoUserRepository) GetByRole(ctx context.Context, role string) ([]*domain.User, error) {
	filter := bson.M{"roles": role}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list users by role: %w", err)
	}
	defer cursor.Close(ctx)

	var users []*domain.User
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		users = append(users, mapBsonToUser(raw))
	}
	return users, nil
}

func (r *MongoUserRepository) GetByTenant(ctx context.Context, tenantID string) ([]*domain.User, error) {
	filter := bson.M{"tenant_id": tenantID}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list users by tenant: %w", err)
	}
	defer cursor.Close(ctx)

	var users []*domain.User
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		users = append(users, mapBsonToUser(raw))
	}
	return users, nil
}

func (r *MongoUserRepository) GetByTenantAndRole(ctx context.Context, tenantID string, role string) ([]*domain.User, error) {
	filter := bson.M{
		"tenant_id": tenantID,
		"roles":     role,
	}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list users by tenant and role: %w", err)
	}
	defer cursor.Close(ctx)

	var users []*domain.User
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		users = append(users, mapBsonToUser(raw))
	}
	return users, nil
}

func mapBsonToUser(raw bson.M) *domain.User {
	user := &domain.User{}
	if oid, ok := raw["_id"].(primitive.ObjectID); ok {
		user.ID = oid.Hex()
	}
	if uid, ok := raw["firebase_uid"].(string); ok {
		user.FirebaseUID = uid
	}
	if email, ok := raw["email"].(string); ok {
		user.Email = email
	}
	if name, ok := raw["name"].(string); ok {
		user.Name = name
	}
	if tid, ok := raw["tenant_id"].(string); ok {
		user.TenantID = tid
	}
	if hbid, ok := raw["home_branch_id"].(string); ok {
		user.HomeBranchID = hbid
	}
	if ba, ok := raw["branch_access"].(primitive.A); ok {
		user.BranchAccess = make([]string, 0, len(ba))
		for _, b := range ba {
			if id, ok := b.(string); ok {
				user.BranchAccess = append(user.BranchAccess, id)
			}
		}
	}
	if created, ok := raw["created_at"].(primitive.DateTime); ok {
		user.CreatedAt = created.Time()
	}
	if updated, ok := raw["updated_at"].(primitive.DateTime); ok {
		user.UpdatedAt = updated.Time()
	}

	// Handle roles array
	if rolesRaw, ok := raw["roles"]; ok {
		if rolesArray, ok := rolesRaw.(primitive.A); ok {
			user.Roles = make([]string, 0, len(rolesArray))
			for _, r := range rolesArray {
				if roleStr, ok := r.(string); ok {
					user.Roles = append(user.Roles, roleStr)
				}
			}
		}
	}

	// Handle activity tracking fields
	if firstLogin, ok := raw["first_login_at"].(primitive.DateTime); ok {
		t := firstLogin.Time()
		user.FirstLoginAt = &t
	}
	if lastLogin, ok := raw["last_login_at"].(primitive.DateTime); ok {
		t := lastLogin.Time()
		user.LastLoginAt = &t
	}
	if count, ok := raw["login_count"].(int32); ok {
		user.LoginCount = int(count)
	} else if count, ok := raw["login_count"].(int64); ok {
		user.LoginCount = int(count)
	}

	// Handle entitlement fields
	if trialEnd, ok := raw["trial_end_date"].(primitive.DateTime); ok {
		t := trialEnd.Time()
		user.TrialEndDate = &t
	}
	if subEnd, ok := raw["subscription_end_date"].(primitive.DateTime); ok {
		t := subEnd.Time()
		user.SubscriptionEndDate = &t
	}

	return user
}
