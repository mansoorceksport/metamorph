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

// MongoTenantRepository implements domain.TenantRepository
type MongoTenantRepository struct {
	collection *mongo.Collection
}

func NewMongoTenantRepository(db *mongo.Database) *MongoTenantRepository {
	collection := db.Collection("tenants")

	// Create unique index on join_code
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "join_code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		// Log warning but proceed (in a real app might want to fatal or handle better)
		fmt.Printf("Warning: failed to create index on join_code: %v\n", err)
	}

	return &MongoTenantRepository{
		collection: collection,
	}
}

func (r *MongoTenantRepository) Create(ctx context.Context, tenant *domain.Tenant) error {
	// Generate new ObjectID
	objID := primitive.NewObjectID()
	tenant.ID = objID.Hex()
	tenant.CreatedAt = time.Now()

	// Create a BSON document for insertion to ensure _id is an ObjectID
	doc := bson.M{
		"_id":         objID,
		"name":        tenant.Name,
		"join_code":   tenant.JoinCode,
		"logo_url":    tenant.LogoURL,
		"ai_settings": tenant.AISettings,
		"created_at":  tenant.CreatedAt,
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}
	return nil
}

func (r *MongoTenantRepository) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id format: %w", err)
	}

	// Fetch raw document
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	// Map back to domain
	return mapBsonToTenant(raw)
}

func (r *MongoTenantRepository) GetByJoinCode(ctx context.Context, code string) (*domain.Tenant, error) {
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"join_code": code}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant by code: %w", err)
	}
	return mapBsonToTenant(raw)
}

func (r *MongoTenantRepository) GetAll(ctx context.Context) ([]*domain.Tenant, error) {
	cursor, err := r.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer cursor.Close(ctx)

	var tenants []*domain.Tenant
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, fmt.Errorf("failed to decode tenant: %w", err)
		}
		tenant, err := mapBsonToTenant(raw)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, tenant)
	}
	return tenants, nil
}

func (r *MongoTenantRepository) Update(ctx context.Context, tenant *domain.Tenant) error {
	objID, err := primitive.ObjectIDFromHex(tenant.ID)
	if err != nil {
		return fmt.Errorf("invalid id format: %w", err)
	}

	update := bson.M{
		"$set": bson.M{
			"name":        tenant.Name,
			"logo_url":    tenant.LogoURL,
			"ai_settings": tenant.AISettings,
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Helper to map BSON to Tenant
func mapBsonToTenant(raw bson.M) (*domain.Tenant, error) {
	tenant := &domain.Tenant{}

	if oid, ok := raw["_id"].(primitive.ObjectID); ok {
		tenant.ID = oid.Hex()
	}
	if name, ok := raw["name"].(string); ok {
		tenant.Name = name
	}
	if code, ok := raw["join_code"].(string); ok {
		tenant.JoinCode = code
	}
	if logo, ok := raw["logo_url"].(string); ok {
		tenant.LogoURL = logo
	}
	if created, ok := raw["created_at"].(primitive.DateTime); ok {
		tenant.CreatedAt = created.Time()
	}

	// Handle AISettings
	if aiSettingsRaw, ok := raw["ai_settings"]; ok {
		// Use bson marshal/unmarshal for nested struct simplicity or map manually
		data, _ := bson.Marshal(aiSettingsRaw)
		bson.Unmarshal(data, &tenant.AISettings)
	}
	return tenant, nil
}

// MongoAssignmentRepository implements domain.AssignmentRepository
type MongoAssignmentRepository struct {
	collection     *mongo.Collection
	userCollection *mongo.Collection
}

func NewMongoAssignmentRepository(db *mongo.Database) *MongoAssignmentRepository {
	return &MongoAssignmentRepository{
		collection:     db.Collection("coach_assignments"),
		userCollection: db.Collection("users"),
	}
}

func (r *MongoAssignmentRepository) Create(ctx context.Context, assignment *domain.CoachAssignment) error {
	objID := primitive.NewObjectID()
	assignment.ID = objID.Hex()
	assignment.AssignedAt = time.Now()

	doc := bson.M{
		"_id":         objID,
		"coach_id":    assignment.CoachID,
		"member_id":   assignment.MemberID,
		"tenant_id":   assignment.TenantID,
		"assigned_at": assignment.AssignedAt,
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create assignment: %w", err)
	}
	return nil
}

func (r *MongoAssignmentRepository) GetByID(ctx context.Context, id string) (*domain.CoachAssignment, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to fetch assignment: %w", err)
	}

	return mapBsonToAssignment(raw), nil
}

func (r *MongoAssignmentRepository) Delete(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	_, err = r.collection.DeleteOne(ctx, bson.M{"_id": objID})
	return err
}

func (r *MongoAssignmentRepository) GetByMemberID(ctx context.Context, memberID string) (*domain.CoachAssignment, error) {
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"member_id": memberID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return mapBsonToAssignment(raw), nil
}

func (r *MongoAssignmentRepository) GetByCoachID(ctx context.Context, coachID string) ([]*domain.CoachAssignment, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"coach_id": coachID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var assignments []*domain.CoachAssignment
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		assignments = append(assignments, mapBsonToAssignment(raw))
	}
	return assignments, nil
}

func mapBsonToAssignment(raw bson.M) *domain.CoachAssignment {
	assignment := &domain.CoachAssignment{}
	if oid, ok := raw["_id"].(primitive.ObjectID); ok {
		assignment.ID = oid.Hex()
	}
	if cid, ok := raw["coach_id"].(string); ok {
		assignment.CoachID = cid
	}
	if mid, ok := raw["member_id"].(string); ok {
		assignment.MemberID = mid
	}
	if tid, ok := raw["tenant_id"].(string); ok {
		assignment.TenantID = tid
	}
	if assigned, ok := raw["assigned_at"].(primitive.DateTime); ok {
		assignment.AssignedAt = assigned.Time()
	}
	return assignment
}

func (r *MongoAssignmentRepository) FindMembersByCoach(ctx context.Context, coachID string) ([]*domain.User, error) {
	// 1. Get all assignments for coach
	assignments, err := r.GetByCoachID(ctx, coachID)
	if err != nil {
		return nil, err
	}
	if len(assignments) == 0 {
		return []*domain.User{}, nil
	}

	// 2. Extract Member IDs (these are MongoDB ObjectIDs)
	memberIDs := make([]primitive.ObjectID, 0, len(assignments))
	for _, a := range assignments {
		objID, err := primitive.ObjectIDFromHex(a.MemberID)
		if err != nil {
			// Skip invalid IDs
			continue
		}
		memberIDs = append(memberIDs, objID)
	}

	if len(memberIDs) == 0 {
		return []*domain.User{}, nil
	}

	// 3. Find Users where _id IN (memberIDs)
	filter := bson.M{
		"_id": bson.M{"$in": memberIDs},
	}

	cursor, err := r.userCollection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch assigned users: %w", err)
	}
	defer cursor.Close(ctx)

	var users []*domain.User
	for cursor.Next(ctx) {
		var user domain.User
		if err := cursor.Decode(&user); err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	return users, nil
}

// MongoBranchRepository implements domain.BranchRepository
type MongoBranchRepository struct {
	collection *mongo.Collection
}

func NewMongoBranchRepository(db *mongo.Database) *MongoBranchRepository {
	collection := db.Collection("branches")

	// Create unique index on join_code
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "join_code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		fmt.Printf("Warning: failed to create index on branch join_code: %v\n", err)
	}

	return &MongoBranchRepository{
		collection: collection,
	}
}

func (r *MongoBranchRepository) Create(ctx context.Context, branch *domain.Branch) error {
	objID := primitive.NewObjectID()
	branch.ID = objID.Hex()
	branch.CreatedAt = time.Now()

	doc := bson.M{
		"_id":        objID,
		"tenant_id":  branch.TenantID,
		"name":       branch.Name,
		"join_code":  branch.JoinCode,
		"created_at": branch.CreatedAt,
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	return nil
}

func (r *MongoBranchRepository) GetByID(ctx context.Context, id string) (*domain.Branch, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return mapBsonToBranch(raw), nil
}

func (r *MongoBranchRepository) GetByJoinCode(ctx context.Context, code string) (*domain.Branch, error) {
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"join_code": code}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return mapBsonToBranch(raw), nil
}

func (r *MongoBranchRepository) GetByTenantID(ctx context.Context, tenantID string) ([]*domain.Branch, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"tenant_id": tenantID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var branches []*domain.Branch
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		branches = append(branches, mapBsonToBranch(raw))
	}
	return branches, nil
}

func mapBsonToBranch(raw bson.M) *domain.Branch {
	branch := &domain.Branch{}
	if oid, ok := raw["_id"].(primitive.ObjectID); ok {
		branch.ID = oid.Hex()
	}
	if tid, ok := raw["tenant_id"].(string); ok {
		branch.TenantID = tid
	}
	if name, ok := raw["name"].(string); ok {
		branch.Name = name
	}
	if code, ok := raw["join_code"].(string); ok {
		branch.JoinCode = code
	}
	if created, ok := raw["created_at"].(primitive.DateTime); ok {
		branch.CreatedAt = created.Time()
	}
	if updated, ok := raw["updated_at"].(primitive.DateTime); ok {
		branch.UpdatedAt = updated.Time()
	}
	return branch
}

// Update updates an existing branch
func (r *MongoBranchRepository) Update(ctx context.Context, branch *domain.Branch) error {
	objID, err := primitive.ObjectIDFromHex(branch.ID)
	if err != nil {
		return fmt.Errorf("invalid branch id: %w", err)
	}

	branch.UpdatedAt = time.Now()
	update := bson.M{
		"$set": bson.M{
			"name":       branch.Name,
			"join_code":  branch.JoinCode,
			"updated_at": branch.UpdatedAt,
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to update branch: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete deletes a branch by ID
func (r *MongoBranchRepository) Delete(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid branch id: %w", err)
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": objID})
	if err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}
	if result.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GetAll retrieves all branches (for super_admin)
func (r *MongoBranchRepository) GetAll(ctx context.Context) ([]*domain.Branch, error) {
	cursor, err := r.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	defer cursor.Close(ctx)

	var branches []*domain.Branch
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		branches = append(branches, mapBsonToBranch(raw))
	}
	return branches, nil
}
