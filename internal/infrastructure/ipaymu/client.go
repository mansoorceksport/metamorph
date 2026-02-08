package ipaymu

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// BankCode represents supported bank codes for VA
type BankCode string

const (
	BankBCA     BankCode = "bca"
	BankMandiri BankCode = "mandiri"
	BankBNI     BankCode = "bni"
	BankBRI     BankCode = "bri"
	BankCIMB    BankCode = "cimb"
)

// Config holds iPaymu API configuration
type Config struct {
	VA        string // Virtual Account number (merchant VA)
	APIKey    string // API Key from iPaymu
	BaseURL   string // Base URL (sandbox or production)
	NotifyURL string // Webhook URL for payment notifications
}

// Client is the iPaymu API client
type Client struct {
	config     Config
	httpClient *http.Client
}

// VAResponse represents the response from VA creation
type VAResponse struct {
	VANumber  string
	SessionID string
	ExpiresAt time.Time
}

// DirectPaymentRequest represents the request body for direct VA payment
type DirectPaymentRequest struct {
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	Amount         int64  `json:"amount"`
	NotifyURL      string `json:"notifyUrl"`
	Expired        int    `json:"expired"` // Expiry in hours
	Comments       string `json:"comments"`
	ReferenceID    string `json:"referenceId"`
	PaymentMethod  string `json:"paymentMethod"`
	PaymentChannel string `json:"paymentChannel"`
}

// DirectPaymentResponse represents the iPaymu API response
type DirectPaymentResponse struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
	Data    struct {
		SessionID     string `json:"SessionId"`
		TransactionID int64  `json:"TransactionId"`
		ReferenceID   string `json:"ReferenceId"`
		Via           string `json:"Via"`
		Channel       string `json:"Channel"`
		PaymentNo     string `json:"PaymentNo"` // This is the VA number
		PaymentName   string `json:"PaymentName"`
		Total         int64  `json:"Total"`
		Fee           int64  `json:"Fee"`
		Expired       string `json:"Expired"` // ISO date string
	} `json:"Data"`
}

// NewClient creates a new iPaymu client
func NewClient(cfg Config) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// generateSignature creates the HMAC-SHA256 signature for iPaymu API
// Step 1: bodyHash = lowercase(sha256(jsonBody))
// Step 2: stringToSign = METHOD + ":" + va + ":" + bodyHash + ":" + apiKey
// Step 3: signature = lowercase(hmacSha256(apiKey, stringToSign))
func (c *Client) generateSignature(jsonBody []byte, method string) string {
	// Step 1: SHA256 hash of body
	bodyHashBytes := sha256.Sum256(jsonBody)
	bodyHash := strings.ToLower(hex.EncodeToString(bodyHashBytes[:]))

	// Step 2: Create string to sign
	// Format: METHOD:VA:BODY_HASH:API_KEY
	stringToSign := fmt.Sprintf("%s:%s:%s:%s", method, c.config.VA, bodyHash, c.config.APIKey)

	// Step 3: HMAC-SHA256
	h := hmac.New(sha256.New, []byte(c.config.APIKey))
	h.Write([]byte(stringToSign))
	signature := strings.ToLower(hex.EncodeToString(h.Sum(nil)))

	return signature
}

// CreateDirectVA creates a Virtual Account for direct payment
func (c *Client) CreateDirectVA(ctx context.Context, invoiceID string, amount int64, bankCode BankCode, userName, userEmail, userPhone string) (*VAResponse, error) {
	endpoint := "/api/v2/payment/direct"
	url := c.config.BaseURL + endpoint

	// Build request body
	reqBody := DirectPaymentRequest{
		Name:           userName,
		Phone:          userPhone,
		Email:          userEmail,
		Amount:         amount,
		NotifyURL:      c.config.NotifyURL,
		Expired:        24, // 24 hours
		Comments:       fmt.Sprintf("Invoice: %s", invoiceID),
		ReferenceID:    invoiceID,
		PaymentMethod:  "va",
		PaymentChannel: string(bankCode),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Generate signature
	signature := c.generateSignature(jsonBody, "POST")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("va", c.config.VA)
	req.Header.Set("signature", signature)
	req.Header.Set("timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	log.Printf("[iPaymu] Calling %s with bank: %s, amount: %d", url, bankCode, amount)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[iPaymu] Response status: %d, body: %s", resp.StatusCode, string(respBody))

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iPaymu API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var apiResp DirectPaymentResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check API status
	if apiResp.Status != 200 {
		return nil, fmt.Errorf("iPaymu API error: %s", apiResp.Message)
	}

	// Parse expiry date
	expiresAt, _ := time.Parse(time.RFC3339, apiResp.Data.Expired)
	if expiresAt.IsZero() {
		// Fallback to 24 hours from now
		expiresAt = time.Now().UTC().Add(24 * time.Hour)
	}

	return &VAResponse{
		VANumber:  apiResp.Data.PaymentNo,
		SessionID: apiResp.Data.SessionID,
		ExpiresAt: expiresAt,
	}, nil
}

// MapBankCodeToIPAYMU converts frontend bank name to iPaymu bank code
func MapBankCodeToIPAYMU(bank string) BankCode {
	switch strings.ToUpper(bank) {
	case "BCA":
		return BankBCA
	case "MANDIRI":
		return BankMandiri
	case "BNI":
		return BankBNI
	case "BRI":
		return BankBRI
	case "CIMB":
		return BankCIMB
	default:
		return BankBCA // Default to BCA
	}
}
