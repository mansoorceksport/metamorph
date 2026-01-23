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
	exerciseRepo   domain.ExerciseRepository
	userRepo       domain.UserRepository
}

// NewMemberHandler creates a new MemberHandler
func NewMemberHandler(
	pbRepo domain.PersonalBestRepository,
	workoutService *service.WorkoutService,
	ptService *service.PTService,
	scheduleRepo domain.ScheduleRepository,
	scanRepo domain.InBodyRepository,
	cacheRepo domain.CacheRepository,
	exerciseRepo domain.ExerciseRepository,
	userRepo domain.UserRepository,
) *MemberHandler {
	return &MemberHandler{
		pbRepo:         pbRepo,
		workoutService: workoutService,
		ptService:      ptService,
		scheduleRepo:   scheduleRepo,
		scanRepo:       scanRepo,
		cacheRepo:      cacheRepo,
		exerciseRepo:   exerciseRepo,
		userRepo:       userRepo,
	}
}

// PBWithExerciseName is a PersonalBest enriched with the exercise name
type PBWithExerciseName struct {
	ID           string    `json:"id"`
	ExerciseID   string    `json:"exercise_id"`
	ExerciseName string    `json:"exercise_name"`
	Weight       float64   `json:"weight"`
	Reps         int       `json:"reps"`
	AchievedAt   time.Time `json:"achieved_at"`
}

// GetMyPBs handles GET /v1/me/pbs
// Returns personal bests for the authenticated member enriched with exercise names
// Query params: limit (default 5)
func (h *MemberHandler) GetMyPBs(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)
	limit := c.QueryInt("limit", 5)

	if limit > 20 {
		limit = 20
	}

	pbs, err := h.pbRepo.GetByMember(c.UserContext(), memberID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if pbs == nil || len(pbs) == 0 {
		return c.JSON([]PBWithExerciseName{})
	}

	// Limit the results
	if len(pbs) > limit {
		pbs = pbs[:limit]
	}

	// Collect exercise IDs for batch lookup
	exerciseIDs := make([]string, len(pbs))
	for i, pb := range pbs {
		exerciseIDs[i] = pb.ExerciseID
	}

	// Batch fetch exercises using $in query (prevents N+1)
	exercises, err := h.exerciseRepo.GetByIDs(c.UserContext(), exerciseIDs)
	if err != nil {
		// Log error but continue with empty names
		exercises = []*domain.Exercise{}
	}

	// Build lookup map
	exerciseMap := make(map[string]string)
	for _, ex := range exercises {
		exerciseMap[ex.ID] = ex.Name
	}

	// Build enriched response
	result := make([]PBWithExerciseName, len(pbs))
	for i, pb := range pbs {
		result[i] = PBWithExerciseName{
			ID:           pb.ID,
			ExerciseID:   pb.ExerciseID,
			ExerciseName: exerciseMap[pb.ExerciseID],
			Weight:       pb.Weight,
			Reps:         pb.Reps,
			AchievedAt:   pb.AchievedAt,
		}
	}

	return c.JSON(result)
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

	// Try cache first
	if h.cacheRepo != nil {
		var cached map[string]interface{}
		if err := h.cacheRepo.GetMemberSchedules(c.UserContext(), memberID, &cached); err == nil {
			return c.JSON(cached)
		}
	}

	// Get schedules for the next 30 days by default
	from := time.Now()
	to := from.AddDate(0, 0, 30)

	schedules, err := h.scheduleRepo.GetByMember(c.UserContext(), memberID, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	response := fiber.Map{"schedules": schedules}

	// Cache the result (10 minutes TTL)
	if h.cacheRepo != nil {
		_ = h.cacheRepo.SetMemberSchedules(c.UserContext(), memberID, response, 10*time.Minute)
	}

	return c.JSON(response)
}

// WorkoutHistoryItem represents a completed workout session for the member history view
type WorkoutHistoryItem struct {
	ID            string    `json:"id"`
	Date          time.Time `json:"date"`
	SessionGoal   string    `json:"session_goal"`
	TotalVolume   float64   `json:"total_volume"`
	TotalSets     int       `json:"total_sets"`
	ExerciseCount int       `json:"exercise_count"`
	HasNewPB      bool      `json:"has_new_pb"`
}

// WorkoutHistoryResponse represents the paginated response
type WorkoutHistoryResponse struct {
	Workouts   []WorkoutHistoryItem `json:"workouts"`
	Total      int                  `json:"total"`
	HasMore    bool                 `json:"has_more"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

// GetMyWorkoutHistory handles GET /v1/me/workouts/history
// Returns completed workout sessions for the authenticated member (DESC order - latest first)
// Query params: limit (default 10), cursor (for pagination)
func (h *MemberHandler) GetMyWorkoutHistory(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)

	// Parse query params
	limit := c.QueryInt("limit", 10)
	cursor := c.Query("cursor", "")

	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 10
	}

	// Get schedules from past 365 days
	to := time.Now()
	from := to.AddDate(-1, 0, 0)

	schedules, err := h.scheduleRepo.GetByMember(c.UserContext(), memberID, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	// Filter to only completed sessions
	var completedSchedules []*domain.Schedule
	for _, s := range schedules {
		if s.Status == domain.ScheduleStatusCompleted {
			completedSchedules = append(completedSchedules, s)
		}
	}

	// Sort by date descending (most recent first)
	// Note: The repo may return sorted data, but let's ensure DESC order
	for i := 0; i < len(completedSchedules)-1; i++ {
		for j := i + 1; j < len(completedSchedules); j++ {
			if completedSchedules[i].StartTime.Before(completedSchedules[j].StartTime) {
				completedSchedules[i], completedSchedules[j] = completedSchedules[j], completedSchedules[i]
			}
		}
	}

	// Handle cursor-based pagination
	startIdx := 0
	if cursor != "" {
		for i, s := range completedSchedules {
			if s.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Apply pagination
	endIdx := startIdx + limit + 1 // +1 to check if there's more
	if endIdx > len(completedSchedules) {
		endIdx = len(completedSchedules)
	}

	paginatedSchedules := completedSchedules[startIdx:endIdx]
	hasMore := len(paginatedSchedules) > limit
	if hasMore {
		paginatedSchedules = paginatedSchedules[:limit]
	}

	// Fetch volume history once (not per schedule)
	volumes, _ := h.workoutService.GetMemberVolumeHistory(c.UserContext(), memberID, 365)
	// Map by ScheduleID, not date (date can have multiple workouts)
	volumeMap := make(map[string]*domain.DailyVolume)
	for _, v := range volumes {
		volumeMap[v.ScheduleID] = v
	}

	// Build response
	history := make([]WorkoutHistoryItem, len(paginatedSchedules))
	for i, s := range paginatedSchedules {
		var totalVolume float64
		var totalSets int
		var exerciseCount int

		// Match by ScheduleID for accurate volume per workout
		if v, ok := volumeMap[s.ID]; ok {
			totalVolume = v.TotalVolume
			totalSets = v.TotalSets
			exerciseCount = v.ExerciseCount
		}

		history[i] = WorkoutHistoryItem{
			ID:            s.ID,
			Date:          s.StartTime,
			SessionGoal:   s.SessionGoal,
			TotalVolume:   totalVolume,
			TotalSets:     totalSets,
			ExerciseCount: exerciseCount,
			HasNewPB:      false, // TODO: Track if any PB was set on this date
		}
	}

	// Build next cursor
	var nextCursor string
	if hasMore && len(paginatedSchedules) > 0 {
		nextCursor = paginatedSchedules[len(paginatedSchedules)-1].ID
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": WorkoutHistoryResponse{
			Workouts:   history,
			Total:      len(completedSchedules),
			HasMore:    hasMore,
			NextCursor: nextCursor,
		},
	})
}

// GetMyDashboard handles GET /v1/me/dashboard
// Returns aggregated dashboard data for the member
func (h *MemberHandler) GetMyDashboard(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)

	// Try cache first
	if h.cacheRepo != nil {
		var cached map[string]interface{}
		if err := h.cacheRepo.GetMemberDashboard(c.UserContext(), memberID, &cached); err == nil {
			return c.JSON(cached)
		}
	}

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

	// Get user's first_login_at for trial calculation
	var firstLoginAt *time.Time
	if h.userRepo != nil {
		if user, err := h.userRepo.GetByID(c.UserContext(), memberID); err == nil && user != nil {
			firstLoginAt = user.FirstLoginAt
		}
	}

	response := fiber.Map{
		"remaining_sessions": totalRemaining,
		"total_sessions":     totalSessions,
		"next_schedule":      nextSchedule,
		"latest_scan":        latestScan,
		"top_pbs":            topPBs,
		"contracts":          contracts,
		"first_login_at":     firstLoginAt,
	}

	// Cache the result (5 minutes TTL)
	if h.cacheRepo != nil {
		_ = h.cacheRepo.SetMemberDashboard(c.UserContext(), memberID, response, 5*time.Minute)
	}

	return c.JSON(response)
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

// ExerciseWithSets represents an exercise with its sets for workout detail
type ExerciseWithSets struct {
	ExerciseID   string      `json:"exercise_id"`
	ExerciseName string      `json:"exercise_name"`
	Sets         []SetDetail `json:"sets"`
	IsPR         bool        `json:"is_pr"` // Whether this exercise has a PB from this session
}

// SetDetail represents a single set in the workout detail
type SetDetail struct {
	SetIndex  int     `json:"set_index"`
	Weight    float64 `json:"weight"`
	Reps      int     `json:"reps"`
	Completed bool    `json:"completed"`
}

// WorkoutDetailResponse represents the full workout detail
type WorkoutDetailResponse struct {
	ID            string             `json:"id"`
	Date          time.Time          `json:"date"`
	SessionGoal   string             `json:"session_goal"`
	TotalVolume   float64            `json:"total_volume"`
	TotalSets     int                `json:"total_sets"`
	ExerciseCount int                `json:"exercise_count"`
	Exercises     []ExerciseWithSets `json:"exercises"`
}

// GetMyWorkoutDetail handles GET /v1/me/workouts/:id
// Returns detailed workout data including exercises, sets, and PR info
func (h *MemberHandler) GetMyWorkoutDetail(c *fiber.Ctx) error {
	memberID := c.Locals("userID").(string)
	scheduleID := c.Params("id")

	if scheduleID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "workout ID is required"})
	}

	// Get the schedule
	schedule, err := h.scheduleRepo.GetByID(c.UserContext(), scheduleID)
	if err != nil {
		// Try by client_id
		schedule, err = h.scheduleRepo.GetByClientID(c.UserContext(), scheduleID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "workout not found"})
		}
	}

	// Verify ownership
	if schedule.MemberID != memberID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "you don't have access to this workout"})
	}

	// Get set logs for this schedule
	setLogs, err := h.workoutService.GetSetsBySchedule(c.UserContext(), schedule.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Get member's PBs to mark exercises with PRs
	pbs, _ := h.pbRepo.GetByMember(c.UserContext(), memberID)
	pbScheduleMap := make(map[string]string) // exerciseID -> scheduleID where PB was achieved
	for _, pb := range pbs {
		pbScheduleMap[pb.ExerciseID] = pb.ScheduleID
	}

	// Group sets by exercise
	exerciseMap := make(map[string]*ExerciseWithSets)
	exerciseOrder := []string{}

	for _, log := range setLogs {
		if log.Weight <= 0 || log.Reps <= 0 {
			continue // Skip empty sets
		}

		if _, exists := exerciseMap[log.ExerciseID]; !exists {
			exerciseMap[log.ExerciseID] = &ExerciseWithSets{
				ExerciseID: log.ExerciseID,
				Sets:       []SetDetail{},
				IsPR:       pbScheduleMap[log.ExerciseID] == schedule.ID,
			}
			exerciseOrder = append(exerciseOrder, log.ExerciseID)
		}

		exerciseMap[log.ExerciseID].Sets = append(exerciseMap[log.ExerciseID].Sets, SetDetail{
			SetIndex:  log.SetIndex,
			Weight:    log.Weight,
			Reps:      log.Reps,
			Completed: log.Completed,
		})
	}

	// Batch fetch exercise names
	if len(exerciseOrder) > 0 {
		exercises, _ := h.exerciseRepo.GetByIDs(c.UserContext(), exerciseOrder)
		nameMap := make(map[string]string)
		for _, ex := range exercises {
			nameMap[ex.ID] = ex.Name
		}
		for _, exID := range exerciseOrder {
			if name, ok := nameMap[exID]; ok {
				exerciseMap[exID].ExerciseName = name
			}
		}
	}

	// Build ordered exercise list
	exerciseList := make([]ExerciseWithSets, 0, len(exerciseOrder))
	for _, exID := range exerciseOrder {
		exerciseList = append(exerciseList, *exerciseMap[exID])
	}

	// Calculate totals
	var totalVolume float64
	var totalSets int
	for _, ex := range exerciseList {
		for _, set := range ex.Sets {
			totalVolume += set.Weight * float64(set.Reps)
			totalSets++
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": WorkoutDetailResponse{
			ID:            schedule.ID,
			Date:          schedule.StartTime,
			SessionGoal:   schedule.SessionGoal,
			TotalVolume:   totalVolume,
			TotalSets:     totalSets,
			ExerciseCount: len(exerciseList),
			Exercises:     exerciseList,
		},
	})
}
