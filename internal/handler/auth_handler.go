package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/mansoorceksport/metamorph/internal/service"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService  *service.AuthService
	tokenService *service.TokenService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authService *service.AuthService, tokenService *service.TokenService) *AuthHandler {
	return &AuthHandler{
		authService:  authService,
		tokenService: tokenService,
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

	// Call auth service to login/register
	resp, err := h.authService.LoginOrRegister(c.Context(), service.LoginOrRegisterRequest{
		FirebaseToken: token,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Generate token pair (access + refresh)
	userAgent := c.Get("User-Agent")
	ipAddress := c.IP()

	tokenPair, err := h.tokenService.GenerateTokenPair(c.Context(), resp.User, userAgent, ipAddress)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to generate tokens: " + err.Error(),
		})
	}

	// Set refresh token as httpOnly cookie
	c.Cookie(&fiber.Cookie{
		Name:     "metamorph-refresh-token",
		Value:    tokenPair.RefreshToken,
		Expires:  time.Now().Add(7 * 24 * time.Hour), // 7 days
		HTTPOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: "Lax", // Allow cross-site with top-level navigation
		Path:     "/",   // Available on all paths
	})

	// Return response with access token
	return c.JSON(fiber.Map{
		"token":       tokenPair.AccessToken,
		"expires_in":  tokenPair.ExpiresIn,
		"is_new_user": resp.IsNewUser,
		"message":     h.getWelcomeMessage(resp),
		"user": fiber.Map{
			"id":        resp.User.ID,
			"roles":     resp.User.Roles,
			"tenant_id": resp.User.TenantID,
		},
	})
}

// RefreshToken handles POST /v1/auth/refresh
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	// Get refresh token from httpOnly cookie
	refreshToken := c.Cookies("metamorph-refresh-token")
	if refreshToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "No refresh token provided",
		})
	}

	userAgent := c.Get("User-Agent")
	ipAddress := c.IP()

	// Refresh the token pair
	tokenPair, err := h.tokenService.RefreshAccessToken(c.Context(), refreshToken, userAgent, ipAddress)
	if err != nil {
		// Clear the invalid cookie
		c.Cookie(&fiber.Cookie{
			Name:     "metamorph-refresh-token",
			Value:    "",
			Expires:  time.Now().Add(-1 * time.Hour),
			HTTPOnly: true,
			Secure:   false,
			SameSite: "Lax",
			Path:     "/",
		})

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired refresh token",
		})
	}

	// Set new refresh token cookie
	c.Cookie(&fiber.Cookie{
		Name:     "metamorph-refresh-token",
		Value:    tokenPair.RefreshToken,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
		Path:     "/",
	})

	// Return new access token
	return c.JSON(fiber.Map{
		"token":      tokenPair.AccessToken,
		"expires_in": tokenPair.ExpiresIn,
	})
}

// Logout handles POST /v1/auth/logout
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	// Get refresh token from cookie
	refreshToken := c.Cookies("metamorph-refresh-token")
	if refreshToken != "" {
		// Revoke the refresh token
		_ = h.tokenService.RevokeRefreshToken(c.Context(), refreshToken)
	}

	// Clear the cookie
	c.Cookie(&fiber.Cookie{
		Name:     "metamorph-refresh-token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
		Path:     "/",
	})

	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
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
