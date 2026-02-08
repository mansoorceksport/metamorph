package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
)

// WebhookHandler handles external payment webhooks
type WebhookHandler struct {
	invoiceRepo      domain.InvoiceRepository
	packageRepo      domain.PackageRepository
	subscriptionRepo domain.SubscriptionRepository
	userRepo         domain.UserRepository
	apiKey           string
	vaNumber         string
}

// NewWebhookHandler creates a new WebhookHandler
func NewWebhookHandler(
	invoiceRepo domain.InvoiceRepository,
	packageRepo domain.PackageRepository,
	subscriptionRepo domain.SubscriptionRepository,
	userRepo domain.UserRepository,
	apiKey, vaNumber string,
) *WebhookHandler {
	return &WebhookHandler{
		invoiceRepo:      invoiceRepo,
		packageRepo:      packageRepo,
		subscriptionRepo: subscriptionRepo,
		userRepo:         userRepo,
		apiKey:           apiKey,
		vaNumber:         vaNumber,
	}
}

// IPAYMUWebhookRequest represents the webhook payload from iPaymu
type IPAYMUWebhookRequest struct {
	SID         string `json:"sid"`          // Session ID
	VA          string `json:"va"`           // Virtual Account number
	Status      string `json:"status"`       // Payment status: "berhasil", "pending", "expired"
	ReferenceID string `json:"reference_id"` // Our invoice ID
	TrxID       int64  `json:"trx_id"`       // iPaymu transaction ID
	Amount      int64  `json:"amount"`       // Payment amount
	Signature   string `json:"signature"`    // HMAC signature for verification
}

// IPAYMUWebhook handles POST /api/payments/webhook/ipaymu
// This is a public endpoint - no authentication required
func (h *WebhookHandler) IPAYMUWebhook(c *fiber.Ctx) error {
	ctx := c.UserContext()

	var req IPAYMUWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		log.Printf("[Webhook] Failed to parse body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid request body",
		})
	}

	log.Printf("[Webhook] Received callback: sid=%s, status=%s, va=%s, amount=%d",
		req.SID, req.Status, req.VA, req.Amount)

	// Verify signature
	if !h.verifySignature(req.VA, req.SID, req.Status, req.Signature) {
		log.Printf("[Webhook] Signature verification failed for sid=%s", req.SID)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "invalid signature",
		})
	}

	// Find invoice by payment session ID
	invoice, err := h.invoiceRepo.GetByPaymentSessionID(ctx, req.SID)
	if err != nil {
		log.Printf("[Webhook] Invoice not found for sid=%s: %v", req.SID, err)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "invoice not found",
		})
	}

	// Only process successful payments
	if req.Status != "berhasil" {
		log.Printf("[Webhook] Payment not successful: status=%s, sid=%s", req.Status, req.SID)
		return c.JSON(fiber.Map{
			"success": true,
			"message": "status acknowledged",
		})
	}

	// Prevent duplicate processing
	if invoice.Status == domain.InvoiceStatusPaid {
		log.Printf("[Webhook] Invoice already paid: id=%s", invoice.ID)
		return c.JSON(fiber.Map{
			"success": true,
			"message": "already processed",
		})
	}

	// Update invoice status to paid
	if err := h.invoiceRepo.UpdateStatus(ctx, invoice.ID, domain.InvoiceStatusPaid); err != nil {
		log.Printf("[Webhook] Failed to update invoice status: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to update invoice",
		})
	}

	// Get package to determine subscription duration
	pkg, err := h.packageRepo.GetByID(ctx, invoice.PackageID)
	if err != nil {
		log.Printf("[Webhook] Failed to get package: %v", err)
		// Continue - invoice is already marked as paid
	}

	// Get user to calculate new subscription end date
	user, err := h.userRepo.GetByID(ctx, invoice.UserID)
	if err != nil {
		log.Printf("[Webhook] Failed to get user: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to get user",
		})
	}

	// Calculate new subscription end date (stacking logic)
	durationMonths := 1 // Default to 1 month if package lookup failed
	if pkg != nil {
		durationMonths = pkg.DurationMonths
	}
	newEndDate := domain.CalculateNewEndDate(user.SubscriptionEndDate, durationMonths)

	// Create subscription record
	now := time.Now().UTC()
	subscription := &domain.Subscription{
		UserID:    invoice.UserID,
		InvoiceID: invoice.ID,
		StartDate: now,
		EndDate:   newEndDate,
	}

	if err := h.subscriptionRepo.Create(ctx, subscription); err != nil {
		log.Printf("[Webhook] Failed to create subscription: %v", err)
		// Continue - invoice is already marked as paid
	}

	// Update user's subscription end date
	user.SubscriptionEndDate = &newEndDate
	user.UpdatedAt = now
	if err := h.userRepo.Update(ctx, user); err != nil {
		log.Printf("[Webhook] Failed to update user subscription: %v", err)
		// Continue - subscription record was created
	}

	log.Printf("[Webhook] Payment processed successfully: invoice=%s, user=%s, newEndDate=%s",
		invoice.ID, invoice.UserID, newEndDate.Format(time.RFC3339))

	return c.JSON(fiber.Map{
		"success": true,
		"message": "payment processed",
	})
}

// verifySignature validates the HMAC-SHA256 signature from iPaymu
// Formula: hmac_sha256(apiKey, va + "." + sid + "." + status)
func (h *WebhookHandler) verifySignature(va, sid, status, providedSig string) bool {
	if providedSig == "" {
		return false
	}

	stringToSign := va + "." + sid + "." + status
	mac := hmac.New(sha256.New, []byte(h.apiKey))
	mac.Write([]byte(stringToSign))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(providedSig))
}
