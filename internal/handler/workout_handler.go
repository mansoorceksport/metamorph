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

// ManageSessionExercises PATCH /v1/pro/sessions/:id/exercises
// Body: { "action": "add"|"remove", "exercise_id": "...", "index": ... }
func (h *WorkoutHandler) ManageSessionExercises(c *fiber.Ctx) error {
	sessionID := c.Params("id")
	var req struct {
		Action     string `json:"action"`      // "add", "remove"
		ExerciseID string `json:"exercise_id"` // required for add
		Index      int    `json:"index"`       // required for remove
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	if req.Action == "add" {
		if err := h.workoutService.AddExerciseToSession(c.Context(), sessionID, req.ExerciseID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	} else if req.Action == "remove" {
		if err := h.workoutService.RemoveExerciseFromSession(c.Context(), sessionID, req.Index); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid action"})
	}

	// Return updated session?
	updated, _ := h.workoutService.GetSession(c.Context(), sessionID)
	return c.JSON(updated)
}

// LogSessionSet PATCH /v1/pro/sessions/:id/log
func (h *WorkoutHandler) LogSessionSet(c *fiber.Ctx) error {
	sessionID := c.Params("id")
	var req struct {
		ExerciseIndex int     `json:"exercise_index"`
		SetIndex      int     `json:"set_index"` // 0-based relative to array, OR 1-based domain? Service uses array index logic.
		Weight        float64 `json:"weight"`
		Reps          int     `json:"reps"`
		Remarks       string  `json:"remarks"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	// Assuming SetIndex in body is 0-based for array access in service
	if err := h.workoutService.LogSet(c.Context(), sessionID, req.ExerciseIndex, req.SetIndex, req.Weight, req.Reps, req.Remarks); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "logged"})
}
