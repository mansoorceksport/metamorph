package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/service"
)

type ProHandler struct {
	ptService        *service.PTService
	userRepo         domain.UserRepository // To fetch member details
	analyticsService domain.AnalyticsService
	dashboardService domain.DashboardService
	pbRepo           domain.PersonalBestRepository // For fetching PBs
}

func NewProHandler(
	ptService *service.PTService,
	userRepo domain.UserRepository,
	analyticsService domain.AnalyticsService,
	dashboardService domain.DashboardService,
	pbRepo domain.PersonalBestRepository,
) *ProHandler {
	return &ProHandler{
		ptService:        ptService,
		userRepo:         userRepo,
		analyticsService: analyticsService,
		dashboardService: dashboardService,
		pbRepo:           pbRepo,
	}
}

// ClientResponse represents a client with contract info for the coach dashboard
type ClientResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	Avatar            string `json:"avatar,omitempty"`
	ActiveContractID  string `json:"active_contract_id"`
	RemainingSessions int    `json:"remaining_sessions"`
	ChurnScore        int    `json:"churn_score"`
	AttendanceTrend   string `json:"attendance_trend"`
	LastSessionDate   string `json:"last_session_date,omitempty"`
	TotalSessions     int    `json:"total_sessions"`
}

// GetClients handles GET /v1/pro/clients
// Returns clients with contract info for scheduling
func (h *ProHandler) GetClients(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string)

	// Use optimized aggregation to get contracts with member info
	contractsWithMembers, err := h.ptService.GetActiveContractsWithMembers(c.Context(), coachID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Deduplicate by member (a member may have multiple contracts)
	memberMap := make(map[string]*ClientResponse)
	for _, cwm := range contractsWithMembers {
		if cwm.Member == nil || cwm.Contract == nil {
			continue
		}

		// Calculate truly remaining sessions (Credit - Scheduled)
		activeScheduleCount, err := h.ptService.GetActiveScheduleCount(c.Context(), cwm.Contract.ID)
		if err != nil {
			// Log error but proceed with conservative estimate? Or just log.
			fmt.Printf("Error counting active schedules for contract %s: %v\n", cwm.Contract.ID, err)
			activeScheduleCount = 0
		}

		availableSessions := cwm.Contract.RemainingSessions - int(activeScheduleCount)
		if availableSessions < 0 {
			availableSessions = 0
		}

		memberID := cwm.Contract.MemberID
		existing, exists := memberMap[memberID]

		if !exists {
			// First contract for this member
			memberMap[memberID] = &ClientResponse{
				ID:                memberID,
				Name:              cwm.Member.Name,
				Email:             cwm.Member.Email,
				ActiveContractID:  cwm.Contract.ID,
				RemainingSessions: availableSessions,
				ChurnScore:        50, // TODO: Compute from attendance patterns
				AttendanceTrend:   "stable",
				TotalSessions:     cwm.Contract.TotalSessions,
			}
		} else {
			// Add remaining sessions from additional contracts
			existing.RemainingSessions += availableSessions
			existing.TotalSessions += cwm.Contract.TotalSessions
			// Keep the contract with most remaining sessions as active
			if availableSessions > existing.RemainingSessions {
				existing.ActiveContractID = cwm.Contract.ID
			}
		}
	}

	// Convert map to slice
	clients := make([]*ClientResponse, 0, len(memberMap))
	for _, client := range memberMap {
		clients = append(clients, client)
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

// ScheduleWithMemberName represents a schedule with denormalized member name
type ScheduleWithMemberName struct {
	*domain.Schedule
	MemberName string `json:"member_name"`
}

// GetMySchedules handles GET /v1/pro/schedules
// Returns coach's schedules for a date range, with member names
func (h *ProHandler) GetMySchedules(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string)

	// Parse date range from query params
	fromStr := c.Query("from")
	toStr := c.Query("to")

	var from, to time.Time
	var err error

	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid 'from' date format, use YYYY-MM-DD"})
		}
	} else {
		// Default to start of current day
		now := time.Now()
		from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}

	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid 'to' date format, use YYYY-MM-DD"})
		}
		// End of that day
		to = to.Add(24*time.Hour - time.Second)
	} else {
		// Default to 7 days from now
		to = from.Add(7 * 24 * time.Hour)
	}

	schedules, err := h.ptService.GetSchedules(c.Context(), "coach", coachID, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Fetch member names for each schedule
	result := make([]*ScheduleWithMemberName, 0, len(schedules))
	memberCache := make(map[string]string) // memberID -> name cache

	for _, schedule := range schedules {
		memberName := ""
		if name, ok := memberCache[schedule.MemberID]; ok {
			memberName = name
		} else {
			user, err := h.userRepo.GetByID(c.Context(), schedule.MemberID)
			if err == nil && user != nil {
				memberName = user.Name
				memberCache[schedule.MemberID] = memberName
			}
		}

		result = append(result, &ScheduleWithMemberName{
			Schedule:   schedule,
			MemberName: memberName,
		})
	}

	return c.JSON(result)
}

// GetMemberPBs handles GET /v1/pro/members/:member_id/pbs
// Returns all personal bests for a member
func (h *ProHandler) GetMemberPBs(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string)
	memberID := c.Params("member_id")

	// Verify access: Coach must have an active contract with this member
	contracts, err := h.ptService.GetActiveContractsByCoach(c.Context(), coachID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	hasAccess := false
	for _, contract := range contracts {
		if contract.MemberID == memberID {
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized to view this member's PBs"})
	}

	// Fetch PBs
	pbs, err := h.pbRepo.GetByMember(c.Context(), memberID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Return empty array if no PBs
	if pbs == nil {
		pbs = []*domain.PersonalBest{}
	}

	return c.JSON(pbs)
}
