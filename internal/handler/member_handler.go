package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/service"
)

// Schedule status constants matching domain
const (
	ScheduleStatusScheduled  = "Scheduled"
	ScheduleStatusInProgress = "In Progress"
	ScanCacheTTL             = 1 * time.Hour // Cache scan details for 1 hour
)

// MemberHandler handles member-specific API endpoints
type MemberHandler struct {
	pbRepo         domain.PersonalBestRepository
	workoutService *service.WorkoutService
	ptService      *service.PTService
	scheduleRepo   domain.ScheduleRepository
	scanRepo       domain.InBodyRepository
	cacheRepo      domain.CacheRepository
}

// NewMemberHandler creates a new MemberHandler
func NewMemberHandler(
	pbRepo domain.PersonalBestRepository,
	workoutService *service.WorkoutService,
	ptService *service.PTService,
	scheduleRepo domain.ScheduleRepository,
	scanRepo domain.InBodyRepository,
	cacheRepo domain.CacheRepository,
) *MemberHandler {
	return &MemberHandler{
		pbRepo:         pbRepo,
		workoutService: workoutService,
		ptService:      ptService,
		scheduleRepo:   scheduleRepo,
		scanRepo:       scanRepo,
		cacheRepo:      cacheRepo,
	}
}

// GetMyPBs handles GET /v1/me/pbs
// Returns all personal bests for the authenticated member
func (h *MemberHandler) GetMyPBs(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)

	pbs, err := h.pbRepo.GetByMember(c.UserContext(), memberID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if pbs == nil {
		pbs = []*domain.PersonalBest{}
	}

	return c.JSON(pbs)
}

// GetMyVolumeHistory handles GET /v1/me/volume-history
// Returns DailyVolume records for the XP Mountain chart
func (h *MemberHandler) GetMyVolumeHistory(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)

	// Get limit from query param (default 30 days)
	limit := c.QueryInt("limit", 30)

	volumes, err := h.workoutService.GetMemberVolumeHistory(c.Context(), memberID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Build response
	type VolumePoint struct {
		Date          string  `json:"date"`
		TotalVolume   float64 `json:"total_volume"`
		TotalSets     int     `json:"total_sets"`
		TotalReps     int     `json:"total_reps"`
		TotalWeight   float64 `json:"total_weight"`
		ExerciseCount int     `json:"exercise_count"`
	}

	response := make([]VolumePoint, len(volumes))
	for i, v := range volumes {
		response[i] = VolumePoint{
			Date:          v.Date.Format("2006-01-02"),
			TotalVolume:   v.TotalVolume,
			TotalSets:     v.TotalSets,
			TotalReps:     v.TotalReps,
			TotalWeight:   v.TotalWeight,
			ExerciseCount: v.ExerciseCount,
		}
	}

	return c.JSON(fiber.Map{"volumes": response})
}

// GetMySchedules handles GET /v1/me/schedules
// Returns upcoming schedules for the authenticated member
func (h *MemberHandler) GetMySchedules(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)

	// Get schedules for the next 30 days by default
	from := time.Now()
	to := from.AddDate(0, 0, 30)

	schedules, err := h.scheduleRepo.GetByMember(c.UserContext(), memberID, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"schedules": schedules})
}

// GetMyDashboard handles GET /v1/me/dashboard
// Returns aggregated dashboard data for the member
func (h *MemberHandler) GetMyDashboard(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)

	// Get contracts to calculate remaining sessions
	contracts, err := h.ptService.GetActiveContractsByMember(c.UserContext(), memberID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Calculate total remaining sessions
	totalRemaining := 0
	totalSessions := 0
	for _, contract := range contracts {
		totalRemaining += contract.RemainingSessions
		totalSessions += contract.TotalSessions
	}

	// Get next upcoming schedule
	from := time.Now()
	to := from.AddDate(0, 0, 30)
	schedules, err := h.scheduleRepo.GetByMember(c.UserContext(), memberID, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	var nextSchedule *domain.Schedule
	for _, s := range schedules {
		if s.Status == ScheduleStatusScheduled || s.Status == ScheduleStatusInProgress {
			nextSchedule = s
			break
		}
	}

	// Get latest scan for AI recap
	scans, err := h.scanRepo.FindAllByUserID(c.UserContext(), memberID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	var latestScan *domain.InBodyRecord
	if len(scans) > 0 {
		latestScan = scans[0]
	}

	// Get top PBs (limit to 5)
	pbs, err := h.pbRepo.GetByMember(c.UserContext(), memberID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	topPBs := pbs
	if len(topPBs) > 5 {
		topPBs = topPBs[:5]
	}

	return c.JSON(fiber.Map{
		"remaining_sessions": totalRemaining,
		"total_sessions":     totalSessions,
		"next_schedule":      nextSchedule,
		"latest_scan":        latestScan,
		"top_pbs":            topPBs,
		"contracts":          contracts,
	})
}

// GetMyScans handles GET /v1/me/scans
// Returns a paginated, lightweight list of scans for the authenticated member
// Query params: limit (default 10), cursor, from, to
func (h *MemberHandler) GetMyScans(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)

	// Parse query parameters
	limit := c.QueryInt("limit", 10)
	cursor := c.Query("cursor", "")
	fromStr := c.Query("from", "")
	toStr := c.Query("to", "")

	// Parse date range
	var from, to time.Time
	if fromStr != "" {
		from, _ = time.Parse("2006-01-02", fromStr)
	}
	if toStr != "" {
		to, _ = time.Parse("2006-01-02", toStr)
		// Set to end of day
		to = to.Add(24*time.Hour - time.Second)
	}

	query := &domain.ScanListQuery{
		Limit:  limit,
		Cursor: cursor,
		From:   from,
		To:     to,
	}

	result, err := h.scanRepo.FindPaginatedByUserID(c.UserContext(), memberID, query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    result,
	})
}

// GetMyScan handles GET /v1/me/scans/:id
// Returns the full scan details with Redis caching
func (h *MemberHandler) GetMyScan(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)
	scanID := c.Params("id")

	if scanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "scan ID is required",
		})
	}

	// Try cache first
	if h.cacheRepo != nil {
		cached, err := h.cacheRepo.GetScanByID(c.UserContext(), scanID)
		if err == nil && cached != nil {
			// Verify ownership
			if cached.UserID.Hex() == memberID {
				return c.JSON(fiber.Map{
					"success": true,
					"data":    cached,
				})
			}
		}
	}

	// Cache miss - fetch from database
	scan, err := h.scanRepo.FindByID(c.UserContext(), scanID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "scan not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	// Verify ownership
	if scan.UserID.Hex() != memberID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "you don't have access to this scan",
		})
	}

	// Cache the result
	if h.cacheRepo != nil {
		_ = h.cacheRepo.SetScanByID(c.UserContext(), scanID, scan, ScanCacheTTL)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    scan,
	})
}
