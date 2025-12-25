package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/middleware"
	"github.com/mansoorceksport/metamorph/internal/service"
)

// AnalyticsHandler handles HTTP requests for analytics operations
type AnalyticsHandler struct {
	analyticsService *service.AnalyticsService
	trendService     *service.TrendService
}

// NewAnalyticsHandler creates a new analytics handler
func NewAnalyticsHandler(analyticsService *service.AnalyticsService, trendService *service.TrendService) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
		trendService:     trendService,
	}
}

// GetHistory handles GET /v1/analytics/history
func (h *AnalyticsHandler) GetHistory(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "user not authenticated",
		})
	}

	// Parse limit query parameter (default: 10)
	limitStr := c.Query("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}

	// Cap maximum limit to prevent abuse
	if limit > 100 {
		limit = 100
	}

	history, err := h.analyticsService.GetHistory(c.Context(), userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to retrieve analytics history: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    history,
	})
}

// GetRecap handles GET /v1/analytics/recap
func (h *AnalyticsHandler) GetRecap(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "user not authenticated",
		})
	}

	recap, err := h.trendService.GenerateTrendRecap(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to generate trend recap: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    recap,
	})
}
