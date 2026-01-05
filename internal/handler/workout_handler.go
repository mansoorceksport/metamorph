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
	exs, err := h.exerciseRepo.List(c.Context(), filter)
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
	if err := h.exerciseRepo.Create(c.Context(), &req); err != nil {
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
	if err := h.exerciseRepo.Update(c.Context(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(req)
}

func (h *WorkoutHandler) DeleteExercise(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.exerciseRepo.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// --- Templates CRUD ---

func (h *WorkoutHandler) ListTemplates(c *fiber.Ctx) error {
	tmps, err := h.templateRepo.List(c.Context())
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
	if err := h.templateRepo.Create(c.Context(), &req); err != nil {
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
	if err := h.templateRepo.Update(c.Context(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(req)
}

func (h *WorkoutHandler) DeleteTemplate(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.templateRepo.Delete(c.Context(), id); err != nil {
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

	session, err := h.workoutService.InitializeSession(c.Context(), req.ScheduleID, req.TemplateID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(session)
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

	planned, err := h.workoutService.AddExerciseToSession(c.Context(), scheduleID, req.ExerciseID, req.ClientID, req.TargetSets, req.TargetReps, req.RestSeconds, req.Notes, req.Order)
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
	if err := h.workoutService.RemovePlannedExercise(c.Context(), id); err != nil {
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

	if err := h.workoutService.UpdatePlannedExercise(c.Context(), ex); err != nil {
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
	if err := h.workoutService.LogSetByULID(c.Context(), sessionID, req.ExerciseID, setLog); err != nil {
		if err == domain.ErrExerciseULIDNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "logged", "set_ulid": setLog.ULID})
}
