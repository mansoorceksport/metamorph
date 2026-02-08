package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/service"
)

// PaymentHandler handles payment-related API endpoints
type PaymentHandler struct {
	invoiceRepo     domain.InvoiceRepository
	packageRepo     domain.PackageRepository
	paymentProvider service.PaymentProvider
}

// NewPaymentHandler creates a new PaymentHandler
func NewPaymentHandler(
	invoiceRepo domain.InvoiceRepository,
	packageRepo domain.PackageRepository,
	paymentProvider service.PaymentProvider,
) *PaymentHandler {
	return &PaymentHandler{
		invoiceRepo:     invoiceRepo,
		packageRepo:     packageRepo,
		paymentProvider: paymentProvider,
	}
}

// CheckoutRequest represents the request body for checkout
type CheckoutRequest struct {
	PackageID     string `json:"package_id"`
	PaymentMethod string `json:"payment_method"` // BCA, Mandiri, BNI
}

// CheckoutResponse represents the checkout response with invoice details
type CheckoutResponse struct {
	ID            string `json:"id"`
	VANumber      string `json:"va_number"`
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	ExpiryDate    string `json:"expiry_date"` // ISO 8601 format
	Status        string `json:"status"`
}

// Checkout handles POST /api/member/payments/checkout
// Creates or returns existing pending invoice with VA number
func (h *PaymentHandler) Checkout(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "unauthorized",
		})
	}

	var req CheckoutRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid request body",
		})
	}

	// Validate package_id
	if req.PackageID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "package_id is required",
		})
	}

	// Validate payment_method
	validMethods := map[string]bool{"BCA": true, "Mandiri": true, "BNI": true}
	if !validMethods[req.PaymentMethod] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid payment_method, must be BCA, Mandiri, or BNI",
		})
	}

	ctx := c.UserContext()

	// Validate package exists and is active
	pkg, err := h.packageRepo.GetByID(ctx, req.PackageID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "package not found",
			})
		}
		log.Printf("[Checkout] Error fetching package: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch package",
		})
	}

	if !pkg.IsActive {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "package is not active",
		})
	}

	// Check for existing pending invoice (Active Session logic)
	existingInvoice, err := h.invoiceRepo.GetPendingByUserAndPackage(ctx, userID, req.PackageID)
	if err == nil && existingInvoice != nil {
		// Return existing invoice - no need to create new one
		return c.JSON(fiber.Map{
			"success": true,
			"data": CheckoutResponse{
				ID:            existingInvoice.ID,
				VANumber:      existingInvoice.VANumber,
				Amount:        existingInvoice.Amount,
				PaymentMethod: existingInvoice.PaymentMethod,
				ExpiryDate:    existingInvoice.ExpiryDate.Format("2006-01-02T15:04:05Z07:00"),
				Status:        existingInvoice.Status,
			},
		})
	}

	// If error is not "not found", it's a real error
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		log.Printf("[Checkout] Error checking existing invoice: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to check existing invoices",
		})
	}

	// No existing pending invoice - create new one
	// Step 1: Generate VA from payment provider
	vaResponse, err := h.paymentProvider.GenerateVA(ctx, req.PaymentMethod, pkg.Price, userID)
	if err != nil {
		log.Printf("[Checkout] Error generating VA: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "payment service unavailable, please try again later",
		})
	}

	// Step 2: Create invoice with VA details
	invoice := &domain.Invoice{
		UserID:           userID,
		PackageID:        req.PackageID,
		Amount:           pkg.Price,
		Status:           domain.InvoiceStatusPending,
		VANumber:         vaResponse.VANumber,
		PaymentMethod:    req.PaymentMethod,
		PaymentSessionID: vaResponse.SessionID,
		ExpiryDate:       vaResponse.ExpiresAt,
	}

	if err := h.invoiceRepo.Create(ctx, invoice); err != nil {
		log.Printf("[Checkout] Error creating invoice: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to create invoice",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data": CheckoutResponse{
			ID:            invoice.ID,
			VANumber:      invoice.VANumber,
			Amount:        invoice.Amount,
			PaymentMethod: invoice.PaymentMethod,
			ExpiryDate:    invoice.ExpiryDate.Format("2006-01-02T15:04:05Z07:00"),
			Status:        invoice.Status,
		},
	})
}

// GetInvoiceStatus handles GET /api/member/payments/status/:id
// Returns the current status of an invoice (for refresh functionality)
func (h *PaymentHandler) GetInvoiceStatus(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "unauthorized",
		})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invoice ID is required",
		})
	}

	ctx := c.UserContext()

	invoice, err := h.invoiceRepo.GetByID(ctx, invoiceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "invoice not found",
			})
		}
		log.Printf("[GetInvoiceStatus] Error fetching invoice: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch invoice",
		})
	}

	// Verify ownership
	if invoice.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "access denied",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": CheckoutResponse{
			ID:            invoice.ID,
			VANumber:      invoice.VANumber,
			Amount:        invoice.Amount,
			PaymentMethod: invoice.PaymentMethod,
			ExpiryDate:    invoice.ExpiryDate.Format("2006-01-02T15:04:05Z07:00"),
			Status:        invoice.Status,
		},
	})
}

// PackageResponse represents a payment package for the frontend
type PackageResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Price          int64  `json:"price"`
	DurationMonths int    `json:"duration_months"`
}

// ListPackages handles GET /api/member/payments/packages
// Returns active payment packages for subscription
func (h *PaymentHandler) ListPackages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	packages, err := h.packageRepo.GetActivePackages(ctx)
	if err != nil {
		log.Printf("[ListPackages] Error fetching packages: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch packages",
		})
	}

	// Map to response format
	var response []PackageResponse
	for _, pkg := range packages {
		response = append(response, PackageResponse{
			ID:             pkg.ID,
			Name:           pkg.Name,
			Description:    pkg.Description,
			Price:          pkg.Price,
			DurationMonths: pkg.DurationMonths,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    response,
	})
}
