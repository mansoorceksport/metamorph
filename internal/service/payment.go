package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mansoorceksport/metamorph/internal/infrastructure/ipaymu"
	"github.com/oklog/ulid/v2"
)

// VAResponse represents the response from a payment provider
type VAResponse struct {
	VANumber  string
	SessionID string
	ExpiresAt time.Time
}

// PaymentProvider defines the interface for payment gateway integrations
type PaymentProvider interface {
	// GenerateVA creates a Virtual Account for the given bank and amount
	GenerateVA(ctx context.Context, bank string, amount int64, userID string) (*VAResponse, error)
}

// MockIPaymuClient is a mock implementation of PaymentProvider for development
type MockIPaymuClient struct{}

// IPaymuClientAdapter adapts the ipaymu.Client to PaymentProvider interface
type IPaymuClientAdapter struct {
	client *ipaymu.Client
}

// NewPaymentProvider returns the appropriate PaymentProvider based on environment config
// If IPAYMU_API_KEY is empty, returns a mock client for development
func NewPaymentProvider() PaymentProvider {
	apiKey := os.Getenv("IPAYMU_API_KEY")
	va := os.Getenv("IPAYMU_VA")
	baseURL := os.Getenv("IPAYMU_BASE_URL")
	notifyURL := os.Getenv("PAYMENT_NOTIFY_URL")

	if apiKey == "" || va == "" {
		log.Println("[Payment] Using mock iPaymu client (no credentials configured)")
		return &MockIPaymuClient{}
	}

	if baseURL == "" {
		baseURL = "https://sandbox.ipaymu.com" // Default to sandbox
	}

	// Build full webhook URL
	webhookURL := ""
	if notifyURL != "" {
		webhookURL = notifyURL + "/api/payments/webhook/ipaymu"
	}

	log.Printf("[Payment] Using real iPaymu client (base: %s, notify: %s)", baseURL, webhookURL)
	client := ipaymu.NewClient(ipaymu.Config{
		VA:        va,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		NotifyURL: webhookURL,
	})

	return &IPaymuClientAdapter{client: client}
}

// GenerateVA generates a mock Virtual Account number
func (m *MockIPaymuClient) GenerateVA(ctx context.Context, bank string, amount int64, userID string) (*VAResponse, error) {
	// Generate a unique session ID using ULID
	sessionID := ulid.Make().String()

	// Generate mock VA number based on bank
	var vaNumber string
	switch bank {
	case "BCA":
		vaNumber = fmt.Sprintf("8888-MOCK-BCA-%s", sessionID[:8])
	case "Mandiri":
		vaNumber = fmt.Sprintf("8888-MOCK-MDR-%s", sessionID[:8])
	case "BNI":
		vaNumber = fmt.Sprintf("8888-MOCK-BNI-%s", sessionID[:8])
	default:
		vaNumber = fmt.Sprintf("8888-MOCK-GEN-%s", sessionID[:8])
	}

	return &VAResponse{
		VANumber:  vaNumber,
		SessionID: sessionID,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}, nil
}

// GenerateVA creates a real Virtual Account via iPaymu API
func (a *IPaymuClientAdapter) GenerateVA(ctx context.Context, bank string, amount int64, userID string) (*VAResponse, error) {
	bankCode := ipaymu.MapBankCodeToIPAYMU(bank)

	// Create temporary invoice ID for the request
	invoiceID := ulid.Make().String()

	// Call iPaymu API
	resp, err := a.client.CreateDirectVA(
		ctx,
		invoiceID,
		amount,
		bankCode,
		"Member",               // Default name
		"member@metamorph.app", // Default email
		"081234567890",         // Default phone
	)
	if err != nil {
		log.Printf("[Payment] iPaymu API error: %v", err)
		return nil, fmt.Errorf("payment provider error: %w", err)
	}

	return &VAResponse{
		VANumber:  resp.VANumber,
		SessionID: resp.SessionID,
		ExpiresAt: resp.ExpiresAt,
	}, nil
}
