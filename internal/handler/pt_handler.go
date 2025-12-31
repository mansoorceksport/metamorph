package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/service"
)

type PTHandler struct {
	ptService  *service.PTService
	branchRepo domain.BranchRepository
	userRepo   domain.UserRepository
}

func NewPTHandler(ptService *service.PTService, branchRepo domain.BranchRepository, userRepo domain.UserRepository) *PTHandler {
	return &PTHandler{
		ptService:  ptService,
		branchRepo: branchRepo,
		userRepo:   userRepo,
	}
}

// --- Tenant Admin: Package Templates ---

// CreatePackageTemplate POST /v1/tenant-admin/packages
func (h *PTHandler) CreatePackageTemplate(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(string)
	if !ok || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing tenant context"})
	}

	var req struct {
		Name          string  `json:"name"`
		TotalSessions int     `json:"total_sessions"`
		Price         float64 `json:"price"`
		BranchID      string  `json:"branch_id"` // Optional? Or required? Usually required for packages.
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Package name is required"})
	}

	// Validate Branch (if provided)
	if req.BranchID != "" {
		branch, err := h.branchRepo.GetByID(c.Context(), req.BranchID)
		if err != nil {
			if err == domain.ErrNotFound {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Branch not found"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to validate branch"})
		}
		if branch.TenantID != tenantID {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Branch does not belong to this tenant"})
		}
	}

	pkg := &domain.PTPackage{
		Name:          req.Name,
		TenantID:      tenantID,
		BranchID:      req.BranchID,
		TotalSessions: req.TotalSessions,
		Price:         req.Price,
	}

	if err := h.ptService.CreatePackageTemplate(c.Context(), pkg); err != nil {
		if err == domain.ErrInvalidSessionAmount {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(pkg)
}

// ListPackageTemplates GET /v1/tenant-admin/packages
func (h *PTHandler) ListPackageTemplates(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(string)
	if !ok || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing tenant context"})
	}

	packages, err := h.ptService.GetPackageTemplatesByTenant(c.Context(), tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(packages)
}

// GetPackageTemplate GET /v1/tenant-admin/packages/:id
func (h *PTHandler) GetPackageTemplate(c *fiber.Ctx) error {
	id := c.Params("id")
	pkg, err := h.ptService.GetPackageTemplate(c.Context(), id)
	if err != nil {
		if err == domain.ErrPackageTemplateNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Package not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(pkg)
}

// UpdatePackageTemplate PUT /v1/tenant-admin/packages/:id
func (h *PTHandler) UpdatePackageTemplate(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.PTPackage
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	req.ID = id
	if err := h.ptService.UpdatePackageTemplate(c.Context(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(req)
}

// --- Tenant Admin: Contracts (Assignment) ---

// CreateContract POST /v1/tenant-admin/contracts
func (h *PTHandler) CreateContract(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(string)
	if !ok || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing tenant context"})
	}

	var req struct {
		PackageID string `json:"package_id"`
		MemberID  string `json:"member_id"`
		CoachID   string `json:"coach_id"`
		BranchID  string `json:"branch_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	contract := &domain.PTContract{
		PackageID: req.PackageID,
		MemberID:  req.MemberID,
		CoachID:   req.CoachID,
		BranchID:  req.BranchID,
		TenantID:  tenantID,
	}

	if err := h.ptService.CreateContract(c.Context(), contract); err != nil {
		if err == domain.ErrBranchMismatch {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(contract)
}

// ListContracts GET /v1/tenant-admin/contracts
func (h *PTHandler) ListContracts(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(string)
	if !ok || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing tenant context"})
	}
	// Future: Filters from query params
	contracts, err := h.ptService.GetContractsByTenant(c.Context(), tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(contracts)
}

// GetMyContracts GET /v1/me/contracts
func (h *PTHandler) GetMyContracts(c *fiber.Ctx) error {
	memberID, ok := c.Locals("userID").(string)
	if !ok || memberID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing user context"})
	}

	contracts, err := h.ptService.GetActiveContractsByMember(c.Context(), memberID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(contracts)
}

// GetContract GET /v1/contracts/:id (Admin/Coach/Member)
func (h *PTHandler) GetContract(c *fiber.Ctx) error {
	id := c.Params("id")
	contract, err := h.ptService.GetContract(c.Context(), id)
	if err != nil {
		if err == domain.ErrContractNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Contract not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	// Todo: Auth check ownership?
	return c.JSON(contract)
}

// --- Pro/Member: Schedules ---

// CreateSchedule POST /v1/pro/schedules
func (h *PTHandler) CreateSchedule(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing user context"})
	}
	tenantID, _ := c.Locals("tenant_id").(string)
	// Fetch user to get current HomeBranchID (dynamic lookup)
	user, err := h.userRepo.GetByID(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Failed to fetch user profile"})
	}
	homeBranchID := user.HomeBranchID

	if homeBranchID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Coach must be assigned to a Home Branch"})
	}

	var req struct {
		ContractID string    `json:"contract_id"`
		MemberID   string    `json:"member_id"`
		StartTime  time.Time `json:"start_time"`
		EndTime    time.Time `json:"end_time"`
		Remarks    string    `json:"remarks"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	schedule := &domain.Schedule{
		ContractID: req.ContractID,
		CoachID:    userID, // The creator (Pro) is the coach
		MemberID:   req.MemberID,
		TenantID:   tenantID,
		BranchID:   homeBranchID,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Remarks:    req.Remarks,
	}

	if err := h.ptService.CreateSchedule(c.Context(), schedule); err != nil {
		if err == domain.ErrPackageDepleted {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if err == domain.ErrBranchMismatch {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(schedule)
}

// RescheduleSession PATCH /v1/schedules/:id/reschedule
func (h *PTHandler) RescheduleSession(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Determine role
	rolesInterface := c.Locals("roles")
	var roles []string
	if r, ok := rolesInterface.([]string); ok {
		roles = r
	} else if rComp, ok := rolesInterface.([]interface{}); ok {
		for _, v := range rComp {
			if s, ok := v.(string); ok {
				roles = append(roles, s)
			}
		}
	}

	actorRole := "member"
	for _, r := range roles {
		if r == "coach" {
			actorRole = "coach"
			break
		}
	}

	scheduleID := c.Params("id")
	var req struct {
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	err := h.ptService.RescheduleSession(c.Context(), scheduleID, req.StartTime, req.EndTime, actorRole, userID)
	if err != nil {
		if err == domain.ErrUnauthorizedReschedule {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Reschedule processed", "status": "updated"})
}

// CompleteSession POST /v1/pro/schedules/:id/complete
func (h *PTHandler) CompleteSession(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	scheduleID := c.Params("id")

	if err := h.ptService.CompleteSession(c.Context(), scheduleID, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Session completed"})
}

// ListSchedules GET /v1/schedules
func (h *PTHandler) ListSchedules(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(string)
	if !ok || tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing tenant context"})
	}

	// Parse Filters
	filters := make(map[string]interface{})
	if memberID := c.Query("member_id"); memberID != "" {
		filters["member_id"] = memberID
	}
	if coachID := c.Query("coach_id"); coachID != "" {
		filters["coach_id"] = coachID
	}
	// Add more filters if needed (from, to)

	schedules, err := h.ptService.ListSchedules(c.Context(), tenantID, filters)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(schedules)
}

// GetSchedule GET /v1/schedules/:id
func (h *PTHandler) GetSchedule(c *fiber.Ctx) error {
	id := c.Params("id")
	schedule, err := h.ptService.GetSchedule(c.Context(), id)
	if err != nil {
		if err == domain.ErrScheduleNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(schedule)
}
