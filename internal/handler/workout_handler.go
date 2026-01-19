package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/service"
)

type WorkoutHandler struct {
	workoutService *service.WorkoutService
	exerciseRepo   domain.ExerciseRepository // Exposed for simple CRUD
	templateRepo   domain.TemplateRepository // Exposed for simple CRUD
	// In strict layered arch, these CRUDs should go through service too.
	// But for scaffolding valid simple persistence, direct repo is acceptable for now.
}

func NewWorkoutHandler(
	workoutService *service.WorkoutService,
	exerciseRepo domain.ExerciseRepository,
	templateRepo domain.TemplateRepository,
) *WorkoutHandler {
	return &WorkoutHandler{
		workoutService: workoutService,
		exerciseRepo:   exerciseRepo,
		templateRepo:   templateRepo,
	}
}

// --- Exercises CRUD ---

func (h *WorkoutHandler) ListExercises(c *fiber.Ctx) error {
	nameFilter := c.Query("name")
	filter := make(map[string]interface{})
	if nameFilter != "" {
		filter["name"] = nameFilter
	}
	// public
	exs, err := h.exerciseRepo.List(c.UserContext(), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(exs)
}

func (h *WorkoutHandler) CreateExercise(c *fiber.Ctx) error {
	// Admin Only (Middleware check outside)
	var req domain.Exercise
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}
	if err := h.exerciseRepo.Create(c.UserContext(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(req)
}

func (h *WorkoutHandler) UpdateExercise(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.Exercise
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}
	req.ID = id
	if err := h.exerciseRepo.Update(c.UserContext(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(req)
}

func (h *WorkoutHandler) DeleteExercise(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.exerciseRepo.Delete(c.UserContext(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// --- Templates CRUD ---

func (h *WorkoutHandler) ListTemplates(c *fiber.Ctx) error {
	tmps, err := h.templateRepo.List(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tmps)
}

func (h *WorkoutHandler) CreateTemplate(c *fiber.Ctx) error {
	var req domain.WorkoutTemplate
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}
	if err := h.templateRepo.Create(c.UserContext(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(req)
}

func (h *WorkoutHandler) UpdateTemplate(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.WorkoutTemplate
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}
	req.ID = id
	if err := h.templateRepo.Update(c.UserContext(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(req)
}

func (h *WorkoutHandler) DeleteTemplate(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.templateRepo.Delete(c.UserContext(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// --- Session Management ---

// InitializeSession POST /v1/pro/sessions/initialize
func (h *WorkoutHandler) InitializeSession(c *fiber.Ctx) error {
	// TODO: Verify Coach Permission (HomeBranch vs Schedule Branch) - done in Service?
	// Service only gets IDs. Handler should probably verify or Service should fetch and verify.
	// For now, simple implementation.
	var req struct {
		ScheduleID string `json:"schedule_id"`
		TemplateID string `json:"template_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	session, err := h.workoutService.InitializeSession(c.UserContext(), req.ScheduleID, req.TemplateID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(session)
}

// ListScheduleSets GET /v1/pro/schedules/:schedule_id/sets - List all set logs for a schedule
func (h *WorkoutHandler) ListScheduleSets(c *fiber.Ctx) error {
	scheduleID := c.Params("schedule_id")

	sets, err := h.workoutService.GetSetsBySchedule(c.UserContext(), scheduleID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(sets)
}

// ListScheduleExercises GET /v1/pro/schedules/:schedule_id/exercises - List all planned exercises for a schedule
func (h *WorkoutHandler) ListScheduleExercises(c *fiber.Ctx) error {
	scheduleID := c.Params("schedule_id")

	exercises, err := h.workoutService.GetExercisesBySchedule(c.UserContext(), scheduleID)
	if err != nil {
		if err == domain.ErrSessionNotFound {
			return c.JSON([]interface{}{})
		}
		// If schedule itself is not found, resolveScheduleID returns error
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(exercises)
}

// AddExercise POST /v1/pro/schedules/:schedule_id/exercises
func (h *WorkoutHandler) AddExercise(c *fiber.Ctx) error {
	scheduleID := c.Params("schedule_id")
	var req struct {
		ClientID    string `json:"client_id"` // Frontend ULID for identity handshake
		ExerciseID  string `json:"exercise_id"`
		TargetSets  int    `json:"target_sets"`
		TargetReps  int    `json:"target_reps"`
		RestSeconds int    `json:"rest_seconds"`
		Notes       string `json:"notes"`
		Order       int    `json:"order"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	planned, err := h.workoutService.AddExerciseToSession(c.UserContext(), scheduleID, req.ExerciseID, req.ClientID, req.TargetSets, req.TargetReps, req.RestSeconds, req.Notes, req.Order)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Return planned exercise with client_id for dual-identity handshake
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":           planned.ID,
		"client_id":    req.ClientID,
		"schedule_id":  planned.ScheduleID,
		"exercise_id":  planned.ExerciseID,
		"name":         planned.Name,
		"target_sets":  planned.TargetSets,
		"target_reps":  planned.TargetReps,
		"rest_seconds": planned.RestSeconds,
		"notes":        planned.Notes,
		"order":        planned.Order,
	})
}

// RemoveExercise DELETE /v1/pro/exercises/:id
func (h *WorkoutHandler) RemoveExercise(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.workoutService.RemovePlannedExercise(c.UserContext(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// UpdatePlannedExercise PUT /v1/pro/exercises/:id
func (h *WorkoutHandler) UpdatePlannedExercise(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		TargetSets  int    `json:"target_sets"`
		TargetReps  int    `json:"target_reps"`
		RestSeconds int    `json:"rest_seconds"`
		Notes       string `json:"notes"`
		Order       int    `json:"order"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	ex := &domain.PlannedExercise{
		ID:          id,
		TargetSets:  req.TargetSets,
		TargetReps:  req.TargetReps,
		RestSeconds: req.RestSeconds,
		Notes:       req.Notes,
		Order:       req.Order,
	}

	if err := h.workoutService.UpdatePlannedExercise(c.UserContext(), ex); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

// LogSessionSetByULID PATCH /v1/pro/sessions/:id/log-ulid
func (h *WorkoutHandler) LogSessionSetByULID(c *fiber.Ctx) error {
	sessionID := c.Params("id")

	var req struct {
		ExerciseID string `json:"exercise_ulid"` // Legacy param name from frontend, now maps to _id
		SetLog     struct {
			ULID      string  `json:"ulid"`
			SetIndex  int     `json:"set_index"`
			Weight    float64 `json:"weight"`
			Reps      int     `json:"reps"`
			Remarks   string  `json:"remarks"`
			Completed bool    `json:"completed"`
		} `json:"set_log"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	setLog := &domain.SetLog{
		ULID:      req.SetLog.ULID,
		SetIndex:  req.SetLog.SetIndex,
		Weight:    req.SetLog.Weight,
		Reps:      req.SetLog.Reps,
		Remarks:   req.SetLog.Remarks,
		Completed: req.SetLog.Completed,
	}

	// req.ExerciseID matches "exercise_ulid" json tag which frontend sends safely
	if err := h.workoutService.LogSetByULID(c.UserContext(), sessionID, req.ExerciseID, setLog); err != nil {
		if err == domain.ErrExerciseULIDNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "logged", "set_ulid": setLog.ULID})
}

// UpdateSetLog PUT /v1/pro/sets/:id - Atomic update of a set log document
func (h *WorkoutHandler) UpdateSetLog(c *fiber.Ctx) error {
	id := c.Params("id") // Parse request (optional remarks or partial update)
	var req struct {
		Weight    *float64 `json:"weight"`
		Reps      *int     `json:"reps"`
		Remarks   *string  `json:"remarks"`
		Completed *bool    `json:"completed"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// For partial updates, we might need a Get then Update pattern, but service handles it?
	// Service expects explicit values.
	// For now assume full update or frontend sends current values.
	// IF frontend sends partial, we need to handle defaults.
	// Simplification: Service UpdateSetLog signatures takes args.
	// We need to fetch current if partial?
	// The frontend `updateSetLogWithSync` sends ALL fields. So we are good.

	weight := 0.0
	if req.Weight != nil {
		weight = *req.Weight
	}
	reps := 0
	if req.Reps != nil {
		reps = *req.Reps
	}
	remarks := ""
	if req.Remarks != nil {
		remarks = *req.Remarks
	}
	completed := false
	if req.Completed != nil {
		completed = *req.Completed
	}

	err := h.workoutService.UpdateSetLog(c.UserContext(), id, weight, reps, remarks, completed)
	if err != nil {
		if err == domain.ErrSessionNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Set log not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

// DeleteSetLog DELETE /v1/pro/sets/:id - Delete a set log
func (h *WorkoutHandler) DeleteSetLog(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Set log ID is required"})
	}

	err := h.workoutService.DeleteSetLog(c.UserContext(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

// AddSetToExercise POST /v1/pro/exercises/:id/sets - Add a new set to an exercise
func (h *WorkoutHandler) AddSetToExercise(c *fiber.Ctx) error {
	exerciseID := c.Params("id") // PlannedExercise ID (MongoDB or client_id)

	var req struct {
		ClientID string `json:"client_id"` // Frontend ULID for dual-identity
		SetIndex int    `json:"set_index"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	setLog, err := h.workoutService.AddSetToExercise(c.UserContext(), exerciseID, req.ClientID, req.SetIndex)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":        setLog.ID,
		"client_id": setLog.ClientID,
		"set_index": setLog.SetIndex,
	})
}
