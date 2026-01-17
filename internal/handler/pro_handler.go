package handler

import (
	"fmt"
	"strings"
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
	scanService      domain.ScanService            // For digitizing scans
	inbodyRepo       domain.InBodyRepository       // For fetching scan records
	workoutService   *service.WorkoutService       // For volume history
	maxUploadMB      int64
}

func NewProHandler(
	ptService *service.PTService,
	userRepo domain.UserRepository,
	analyticsService domain.AnalyticsService,
	dashboardService domain.DashboardService,
	pbRepo domain.PersonalBestRepository,
	scanService domain.ScanService,
	inbodyRepo domain.InBodyRepository,
	workoutService *service.WorkoutService,
	maxUploadMB int64,
) *ProHandler {
	return &ProHandler{
		ptService:        ptService,
		userRepo:         userRepo,
		analyticsService: analyticsService,
		dashboardService: dashboardService,
		pbRepo:           pbRepo,
		scanService:      scanService,
		inbodyRepo:       inbodyRepo,
		workoutService:   workoutService,
		maxUploadMB:      maxUploadMB,
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

	// Use optimized aggregation to get contracts with member info AND schedule counts
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

		// Use pre-computed schedule count from aggregation (eliminates N+1 queries)
		availableSessions := cwm.Contract.RemainingSessions - cwm.ActiveScheduleCount
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

// CreateMember handles POST /v1/pro/members
// Allows a Coach to create a new member in their tenant
// If package_id is provided, also creates a contract
func (h *ProHandler) CreateMember(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string)

	var req struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		PackageID string `json:"package_id"` // Optional: if provided, creates contract
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email is required"})
	}
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name is required"})
	}

	// Get Coach's TenantID from JWT context
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	// Create user with strictly 'member' role
	user := &domain.User{
		Email:    req.Email,
		Name:     req.Name,
		Roles:    []string{domain.RoleMember},
		TenantID: tID,
	}

	if err := h.userRepo.Create(c.Context(), user); err != nil {
		// Check for duplicate key error (email already exists)
		if strings.Contains(err.Error(), "E11000") || strings.Contains(err.Error(), "duplicate key") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "A member with this email already exists"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// If package_id provided, create contract
	var contract *domain.PTContract
	if req.PackageID != "" {
		// Validate package belongs to tenant
		pkg, err := h.ptService.GetPackageTemplate(c.Context(), req.PackageID)
		if err != nil {
			// Member created but package not found - return member with warning
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{
				"member":  user,
				"warning": "Package not found, member created without contract",
			})
		}
		if pkg.TenantID != tID {
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{
				"member":  user,
				"warning": "Package does not belong to your tenant, member created without contract",
			})
		}

		// Create contract
		contract = &domain.PTContract{
			PackageID: req.PackageID,
			MemberID:  user.ID,
			CoachID:   coachID,
			BranchID:  pkg.BranchID,
			TenantID:  tID,
		}

		if err := h.ptService.CreateContract(c.Context(), contract); err != nil {
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{
				"member":  user,
				"warning": fmt.Sprintf("Failed to create contract: %s", err.Error()),
			})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"member":   user,
		"contract": contract,
	})
}

// DigitizeMemberScan handles POST /v1/pro/members/:id/scans
// Allows a Coach to upload and digitize an InBody scan for a member
func (h *ProHandler) DigitizeMemberScan(c *fiber.Ctx) error {
	memberID := c.Params("id")
	if memberID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Member ID is required"})
	}

	// Verify member exists and belongs to same tenant
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	member, err := h.userRepo.GetByID(c.Context(), memberID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Member not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if member.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Member does not belong to your tenant"})
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid multipart form: " + err.Error()})
	}

	// Get image file
	files := form.File["image"]
	if len(files) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing 'image' field in form data"})
	}

	imageFile := files[0]

	// Validate file size
	maxBytes := h.maxUploadMB * 1024 * 1024
	if imageFile.Size > maxBytes {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("File size exceeds maximum of %dMB", h.maxUploadMB)})
	}

	// Validate MIME type
	if !isValidImageType(imageFile) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid file type, only JPEG, PNG, and HEIC images are allowed"})
	}

	// Read file contents
	fileHandle, err := imageFile.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to open uploaded file"})
	}
	defer fileHandle.Close()

	imageData := make([]byte, imageFile.Size)
	_, err = fileHandle.Read(imageData)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to read uploaded file"})
	}

	imageURL := imageFile.Filename

	// Process the scan for the MEMBER (not the coach)
	record, err := h.scanService.ProcessScan(c.Context(), memberID, imageData, imageURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to process scan: " + err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    record,
	})
}

// GetMember handles GET /v1/pro/members/:id
// Returns member details with contract info
func (h *ProHandler) GetMember(c *fiber.Ctx) error {
	memberID := c.Params("id")
	if memberID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Member ID is required"})
	}

	// Get tenant from JWT
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	// Fetch member
	member, err := h.userRepo.GetByID(c.Context(), memberID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Member not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate tenant
	if member.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Member does not belong to your tenant"})
	}

	// Get contracts for this member with the coach
	coachID := c.Locals("userID").(string)
	contracts, err := h.ptService.GetContractsByMemberAndCoach(c.Context(), memberID, coachID)
	if err != nil {
		contracts = []*domain.PTContract{} // Default to empty if error
	}

	// Calculate total remaining sessions across all contracts
	totalRemaining := 0
	for _, contract := range contracts {
		totalRemaining += contract.RemainingSessions
	}

	return c.JSON(fiber.Map{
		"id":                 member.ID,
		"name":               member.Name,
		"email":              member.Email,
		"contracts":          contracts,
		"remaining_sessions": totalRemaining,
	})
}

// ListPackages handles GET /v1/pro/packages
// Returns available PT packages for the coach's tenant
func (h *ProHandler) ListPackages(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	packages, err := h.ptService.GetPackageTemplatesByTenant(c.Context(), tID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(packages)
}

// CreateContract handles POST /v1/pro/contracts
// Allows coach to create a contract (assign package) for a member
func (h *ProHandler) CreateContract(c *fiber.Ctx) error {
	coachID := c.Locals("userID").(string)
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	var req struct {
		MemberID  string `json:"member_id"`
		PackageID string `json:"package_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.MemberID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Member ID is required"})
	}
	if req.PackageID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Package ID is required"})
	}

	// Validate member belongs to tenant
	member, err := h.userRepo.GetByID(c.Context(), req.MemberID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Member not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if member.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Member does not belong to your tenant"})
	}

	// Validate package belongs to tenant
	pkg, err := h.ptService.GetPackageTemplate(c.Context(), req.PackageID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Package not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if pkg.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Package does not belong to your tenant"})
	}

	// Create contract
	contract := &domain.PTContract{
		PackageID: req.PackageID,
		MemberID:  req.MemberID,
		CoachID:   coachID,
		BranchID:  pkg.BranchID, // Use package's branch
		TenantID:  tID,
	}

	if err := h.ptService.CreateContract(c.Context(), contract); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(contract)
}

// GetMemberScans handles GET /v1/pro/members/:id/scans
// Returns InBody scan records for a member
func (h *ProHandler) GetMemberScans(c *fiber.Ctx) error {
	memberID := c.Params("id")
	if memberID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Member ID is required"})
	}

	// Get tenant from JWT
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	// Validate member belongs to tenant
	member, err := h.userRepo.GetByID(c.Context(), memberID)
	if err != nil {
		if err == domain.ErrNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Member not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if member.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Member does not belong to your tenant"})
	}

	// Fetch scans for member
	scans, err := h.inbodyRepo.GetByUserID(c.Context(), memberID, 50) // Limit to 50 scans
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(scans)
}

// GetScan handles GET /v1/pro/scans/:id
// Returns a single InBody scan record by ID
func (h *ProHandler) GetScan(c *fiber.Ctx) error {
	scanID := c.Params("id")
	if scanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Scan ID is required"})
	}

	// Fetch scan
	scan, err := h.inbodyRepo.FindByID(c.Context(), scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Scan not found"})
	}

	// Get tenant from JWT and validate member belongs to tenant
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	member, err := h.userRepo.GetByID(c.Context(), scan.UserID.Hex())
	if err != nil || member.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
	}

	return c.JSON(scan)
}

// UpdateScan handles PUT /v1/pro/scans/:id
// Allows coach to update scan data (fix AI extraction errors)
func (h *ProHandler) UpdateScan(c *fiber.Ctx) error {
	scanID := c.Params("id")
	if scanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Scan ID is required"})
	}

	// Fetch existing scan
	scan, err := h.inbodyRepo.FindByID(c.Context(), scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Scan not found"})
	}

	// Validate tenant ownership
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	member, err := h.userRepo.GetByID(c.Context(), scan.UserID.Hex())
	if err != nil || member.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
	}

	// Parse update request - allow partial updates
	var req domain.InBodyRecord
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Update only non-zero values from request
	// Core metrics
	if req.Weight > 0 {
		scan.Weight = req.Weight
	}
	if req.SMM > 0 {
		scan.SMM = req.SMM
	}
	if req.PBF > 0 {
		scan.PBF = req.PBF
	}
	if req.BMI > 0 {
		scan.BMI = req.BMI
	}
	if req.BodyFatMass > 0 {
		scan.BodyFatMass = req.BodyFatMass
	}
	// Health indicators
	if req.VisceralFatLevel > 0 {
		scan.VisceralFatLevel = req.VisceralFatLevel
	}
	if req.BMR > 0 {
		scan.BMR = req.BMR
	}
	if req.FatFreeMass > 0 {
		scan.FatFreeMass = req.FatFreeMass
	}
	if req.InBodyScore > 0 {
		scan.InBodyScore = req.InBodyScore
	}
	if req.WaistHipRatio > 0 {
		scan.WaistHipRatio = req.WaistHipRatio
	}
	if req.ObesityDegree > 0 {
		scan.ObesityDegree = req.ObesityDegree
	}
	// Control guide
	if req.TargetWeight > 0 {
		scan.TargetWeight = req.TargetWeight
	}
	// These can be negative, so check if they were explicitly set
	scan.WeightControl = req.WeightControl
	scan.FatControl = req.FatControl
	scan.MuscleControl = req.MuscleControl
	if req.RecommendedCalorieIntake > 0 {
		scan.RecommendedCalorieIntake = req.RecommendedCalorieIntake
	}
	// Segmental data
	if req.SegmentalLean != nil {
		scan.SegmentalLean = req.SegmentalLean
	}
	if req.SegmentalFat != nil {
		scan.SegmentalFat = req.SegmentalFat
	}

	// Save updates
	if err := h.inbodyRepo.Update(c.Context(), scanID, scan); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(scan)
}

// DeleteScan handles DELETE /v1/pro/scans/:id
// Removes a scan record
func (h *ProHandler) DeleteScan(c *fiber.Ctx) error {
	scanID := c.Params("id")
	if scanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Scan ID is required"})
	}

	// Fetch scan to validate ownership
	scan, err := h.inbodyRepo.FindByID(c.Context(), scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Scan not found"})
	}

	// Validate tenant ownership
	tenantID := c.Locals("tenant_id")
	if tenantID == nil || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "TenantID not found in token"})
	}
	tID := tenantID.(string)

	member, err := h.userRepo.GetByID(c.Context(), scan.UserID.Hex())
	if err != nil || member.TenantID != tID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
	}

	// Delete scan
	if err := h.inbodyRepo.Delete(c.Context(), scanID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Scan deleted"})
}

// GetMemberVolumeHistory handles GET /v1/pro/members/:id/volume-history
// Returns DailyVolume records for the XP Mountain chart
func (h *ProHandler) GetMemberVolumeHistory(c *fiber.Ctx) error {
	memberID := c.Params("id")
	if memberID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Member ID required"})
	}

	// Get limit from query param (default 30 days)
	limit := c.QueryInt("limit", 30)

	// Get volume history
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
