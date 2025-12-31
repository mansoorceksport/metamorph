package handler

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/middleware"
)

// ScanHandler handles HTTP requests for scan operations
type ScanHandler struct {
	scanService domain.ScanService
	maxUploadMB int64
}

// NewScanHandler creates a new scan handler
func NewScanHandler(scanService domain.ScanService, maxUploadMB int64) *ScanHandler {
	return &ScanHandler{
		scanService: scanService,
		maxUploadMB: maxUploadMB,
	}
}

// DigitizeScan handles POST /v1/scans/digitize
func (h *ScanHandler) DigitizeScan(c *fiber.Ctx) error {
	// Get user ID from context (set by FirebaseAuth middleware)
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "user not authenticated",
		})
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid multipart form: " + err.Error(),
		})
	}

	// Get image file
	files := form.File["image"]
	if len(files) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "missing 'image' field in form data",
		})
	}

	imageFile := files[0]

	// Validate file size
	maxBytes := h.maxUploadMB * 1024 * 1024
	if imageFile.Size > maxBytes {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("file size exceeds maximum of %dMB", h.maxUploadMB),
		})
	}

	// Validate MIME type
	if !isValidImageType(imageFile) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid file type, only JPEG, PNG, and HEIC images are allowed",
		})
	}

	// Read file contents
	fileHandle, err := imageFile.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to open uploaded file",
		})
	}
	defer fileHandle.Close()

	imageData := make([]byte, imageFile.Size)
	_, err = fileHandle.Read(imageData)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to read uploaded file",
		})
	}

	// For now, we'll use the filename as imageURL
	// In production, you'd upload to cloud storage (S3, GCS, etc.) and get a URL
	imageURL := imageFile.Filename

	// Process the scan
	record, err := h.scanService.ProcessScan(c.Context(), userID, imageData, imageURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to process scan: " + err.Error(),
		})
	}

	// Return success response
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    record,
	})
}

// isValidImageType checks if the uploaded file is a valid image type
func isValidImageType(file *multipart.FileHeader) bool {
	// Check by content type
	contentType := file.Header.Get("Content-Type")
	if contentType == "image/jpeg" ||
		contentType == "image/jpg" ||
		contentType == "image/png" ||
		contentType == "image/heic" ||
		contentType == "image/heif" {
		return true
	}

	// Fallback: check by file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".heic" || ext == ".heif"
}

// ListScans handles GET /v1/scans
func (h *ScanHandler) ListScans(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "user not authenticated",
		})
	}

	records, err := h.scanService.GetAllScans(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to retrieve scans: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    records,
	})
}

// GetScan handles GET /v1/scans/:id
func (h *ScanHandler) GetScan(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "user not authenticated",
		})
	}

	scanID := c.Params("id")
	if scanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "scan id is required",
		})
	}

	record, err := h.scanService.GetScanByID(c.Context(), userID, scanID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "scan not found",
			})
		}
		if err == domain.ErrForbidden {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"error":   "you don't have access to this scan",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to retrieve scan: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    record,
	})
}

// UpdateScan handles PATCH /v1/scans/:id
func (h *ScanHandler) UpdateScan(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "user not authenticated",
		})
	}

	scanID := c.Params("id")
	if scanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "scan id is required",
		})
	}

	// Parse request body
	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid request body: " + err.Error(),
		})
	}

	record, err := h.scanService.UpdateScan(c.Context(), userID, scanID, updates)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "scan not found",
			})
		}
		if err == domain.ErrForbidden {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"error":   "you don't have access to this scan",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to update scan: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    record,
	})
}

// DeleteScan handles DELETE /v1/scans/:id
func (h *ScanHandler) DeleteScan(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "user not authenticated",
		})
	}

	scanID := c.Params("id")
	if scanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "scan id is required",
		})
	}

	err := h.scanService.DeleteScan(c.Context(), userID, scanID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "scan not found",
			})
		}
		if err == domain.ErrForbidden {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"error":   "you don't have access to this scan",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to delete scan: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "scan deleted successfully",
	})
}
