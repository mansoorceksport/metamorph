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

// MongoInvoiceRepository implements domain.InvoiceRepository
type MongoInvoiceRepository struct {
	collection *mongo.Collection
}

// NewMongoInvoiceRepository creates a new invoice repository
// Note: No index creation to ensure zero-impact deployment on existing collections
func NewMongoInvoiceRepository(db *mongo.Database) *MongoInvoiceRepository {
	coll := db.Collection("invoices")
	return &MongoInvoiceRepository{
		collection: coll,
	}
}

func (r *MongoInvoiceRepository) Create(ctx context.Context, invoice *domain.Invoice) error {
	now := time.Now().UTC()
	invoice.CreatedAt = now
	invoice.UpdatedAt = now

	objID := primitive.NewObjectID()
	invoice.ID = objID.Hex()

	doc := bson.M{
		"_id":                objID,
		"user_id":            invoice.UserID,
		"package_id":         invoice.PackageID,
		"amount":             invoice.Amount,
		"status":             invoice.Status,
		"va_number":          invoice.VANumber,
		"payment_method":     invoice.PaymentMethod,
		"payment_session_id": invoice.PaymentSessionID,
		"expiry_date":        invoice.ExpiryDate,
		"created_at":         invoice.CreatedAt,
		"updated_at":         invoice.UpdatedAt,
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create invoice: %w", err)
	}
	return nil
}

func (r *MongoInvoiceRepository) GetByID(ctx context.Context, id string) (*domain.Invoice, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid invoice id: %w", err)
	}

	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}
	return mapBsonToInvoice(raw), nil
}

func (r *MongoInvoiceRepository) GetByUserID(ctx context.Context, userID string) ([]*domain.Invoice, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to list invoices by user: %w", err)
	}
	defer cursor.Close(ctx)

	var invoices []*domain.Invoice
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		invoices = append(invoices, mapBsonToInvoice(raw))
	}
	return invoices, nil
}

// GetPendingByUserAndPackage finds an existing pending, non-expired invoice for reuse
func (r *MongoInvoiceRepository) GetPendingByUserAndPackage(ctx context.Context, userID, packageID string) (*domain.Invoice, error) {
	filter := bson.M{
		"user_id":    userID,
		"package_id": packageID,
		"status":     domain.InvoiceStatusPending,
		"expiry_date": bson.M{
			"$gt": time.Now().UTC(), // Not expired
		},
	}

	var raw bson.M
	if err := r.collection.FindOne(ctx, filter).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get pending invoice: %w", err)
	}
	return mapBsonToInvoice(raw), nil
}

func (r *MongoInvoiceRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid invoice id: %w", err)
	}

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now().UTC(),
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to update invoice status: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Update updates an invoice with all fields
func (r *MongoInvoiceRepository) Update(ctx context.Context, invoice *domain.Invoice) error {
	objID, err := primitive.ObjectIDFromHex(invoice.ID)
	if err != nil {
		return fmt.Errorf("invalid invoice id: %w", err)
	}

	invoice.UpdatedAt = time.Now().UTC()

	update := bson.M{
		"$set": bson.M{
			"va_number":          invoice.VANumber,
			"payment_method":     invoice.PaymentMethod,
			"payment_session_id": invoice.PaymentSessionID,
			"expiry_date":        invoice.ExpiryDate,
			"status":             invoice.Status,
			"updated_at":         invoice.UpdatedAt,
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GetByPaymentSessionID finds an invoice by its payment session ID
func (r *MongoInvoiceRepository) GetByPaymentSessionID(ctx context.Context, sessionID string) (*domain.Invoice, error) {
	var raw bson.M
	if err := r.collection.FindOne(ctx, bson.M{"payment_session_id": sessionID}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get invoice by session: %w", err)
	}
	return mapBsonToInvoice(raw), nil
}

func mapBsonToInvoice(raw bson.M) *domain.Invoice {
	invoice := &domain.Invoice{}

	if oid, ok := raw["_id"].(primitive.ObjectID); ok {
		invoice.ID = oid.Hex()
	}
	if userID, ok := raw["user_id"].(string); ok {
		invoice.UserID = userID
	}
	if pkgID, ok := raw["package_id"].(string); ok {
		invoice.PackageID = pkgID
	}
	if amount, ok := raw["amount"].(int64); ok {
		invoice.Amount = amount
	} else if amount, ok := raw["amount"].(int32); ok {
		invoice.Amount = int64(amount)
	}
	if status, ok := raw["status"].(string); ok {
		invoice.Status = status
	}
	if vaNum, ok := raw["va_number"].(string); ok {
		invoice.VANumber = vaNum
	}
	if paymentMethod, ok := raw["payment_method"].(string); ok {
		invoice.PaymentMethod = paymentMethod
	}
	if sessionID, ok := raw["payment_session_id"].(string); ok {
		invoice.PaymentSessionID = sessionID
	}
	if expiryDate, ok := raw["expiry_date"].(primitive.DateTime); ok {
		invoice.ExpiryDate = expiryDate.Time()
	}
	if created, ok := raw["created_at"].(primitive.DateTime); ok {
		invoice.CreatedAt = created.Time()
	}
	if updated, ok := raw["updated_at"].(primitive.DateTime); ok {
		invoice.UpdatedAt = updated.Time()
	}

	return invoice
}
