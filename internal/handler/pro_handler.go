package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/service"
)

type ProHandler struct {
	ptService        *service.PTService
	userRepo         domain.UserRepository // To fetch member details
	analyticsService domain.AnalyticsService
	dashboardService domain.DashboardService
}

func NewProHandler(
	ptService *service.PTService,
	userRepo domain.UserRepository,
	analyticsService domain.AnalyticsService,
	dashboardService domain.DashboardService,
) *ProHandler {
	return &ProHandler{
		ptService:        ptService,
		userRepo:         userRepo,
		analyticsService: analyticsService,
		dashboardService: dashboardService,
	}
}

// GetClients handles GET /v1/pro/clients
// GetClients handles GET /v1/pro/clients
func (h *ProHandler) GetClients(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string) // Ensure ID is correct (internal mongo ID or Firebase UID)

	// Fetch Active contracts for this coach
	contracts, err := h.ptService.GetActiveContractsByCoach(c.Context(), coachID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Extract Unique Member IDs
	memberMap := make(map[string]bool)
	var memberIDs []string
	for _, contract := range contracts {
		if !memberMap[contract.MemberID] {
			memberMap[contract.MemberID] = true
			memberIDs = append(memberIDs, contract.MemberID)
		}
	}

	// Fetch Member Profiles
	// This would require a Bulk Get or loop. For now loop.
	var clients []*domain.User
	for _, memberID := range memberIDs {
		user, err := h.userRepo.GetByID(c.Context(), memberID)
		if err == nil {
			clients = append(clients, user)
		}
	}

	return c.JSON(clients)
}

// GetClientHistory handles GET /v1/pro/clients/:id/history
// GetClientHistory handles GET /v1/pro/clients/:id/history
func (h *ProHandler) GetClientHistory(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string)
	clientID := c.Params("id") // Member ID

	// Verify access: Does coach have ANY active contract with this member?
	contracts, err := h.ptService.GetActiveContractsByCoach(c.Context(), coachID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	hasAccess := false
	for _, contract := range contracts {
		if contract.MemberID == clientID {
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized to view this client"})
	}

	// Get History
	// Limit default 12
	limit := 12
	if l := c.QueryInt("limit"); l > 0 {
		limit = l
	}

	history, err := h.analyticsService.GetHistory(c.Context(), clientID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(history)
}

// GetDashboardSummary handles GET /v1/pro/dashboard/summary
// Returns aggregated analytics for the Coach Command Center
func (h *ProHandler) GetDashboardSummary(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string)

	summary, err := h.dashboardService.GetCoachSummary(c.Context(), coachID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(summary)
}
