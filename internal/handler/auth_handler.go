package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/service"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// LoginOrRegister handles POST /v1/auth/login
func (h *AuthHandler) LoginOrRegister(c *fiber.Ctx) error {
	// Get Firebase token from Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing Authorization header",
		})
	}

	// Extract token (format: "Bearer <token>")
	token := authHeader
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}

	// Call auth service
	resp, err := h.authService.LoginOrRegister(c.Context(), service.LoginOrRegisterRequest{
		FirebaseToken: token,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Return response
	return c.JSON(fiber.Map{
		"token":       resp.Token,
		"is_new_user": resp.IsNewUser,
		"message":     h.getWelcomeMessage(resp),
		"user": fiber.Map{
			"id":        resp.User.ID,
			"roles":     resp.User.Roles,
			"tenant_id": resp.User.TenantID,
		},
	})
}

func (h *AuthHandler) getWelcomeMessage(resp *service.LoginOrRegisterResponse) string {
	if resp.IsNewUser {
		return "Welcome! Your account has been created."
	}
	if resp.RoleAdded {
		return "Role added successfully to your existing account."
	}
	return "Welcome back!"
}
