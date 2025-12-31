package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mansoorceksport/metamorph/internal/domain"
)

// Context keys for storing user info
// Using same keys as Firebase middleware for compatibility
const (
	UserIDKey       = "userID" // matches firebase_auth.go
	RolesKey        = "roles"
	TenantIDKey     = "tenant_id"
	HomeBranchIDKey = "home_branch_id"
	BranchAccessKey = "branch_access"
)

// VerifyMetamorphToken validates the JWT and extracts claims
func VerifyMetamorphToken(jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get token from Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing authorization token",
			})
		}

		// Extract token (format: "Bearer <token>")
		tokenString := authHeader
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		}

		// Parse and validate token
		token, err := jwt.ParseWithClaims(tokenString, &domain.MetamorphClaims{}, func(token *jwt.Token) (interface{}, error) {
			// Verify signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid signing method")
			}
			return []byte(jwtSecret), nil
		})

		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		// Extract claims
		claims, ok := token.Claims.(*domain.MetamorphClaims)
		if !ok || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token claims",
			})
		}

		// Store claims in context
		c.Locals(UserIDKey, claims.UserID)
		c.Locals(RolesKey, claims.Roles)
		c.Locals(TenantIDKey, claims.TenantID)
		c.Locals(HomeBranchIDKey, claims.HomeBranchID)
		c.Locals(BranchAccessKey, claims.BranchAccess)

		return c.Next()
	}
}

// AuthorizeRole checks if user has at least one of the required roles
func AuthorizeRole(allowedRoles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get roles from context
		rolesInterface := c.Locals(RolesKey)
		if rolesInterface == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No roles found in token",
			})
		}

		userRoles, ok := rolesInterface.([]string)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid roles format",
			})
		}

		// Check if user has any of the allowed roles
		for _, userRole := range userRoles {
			for _, allowedRole := range allowedRoles {
				if userRole == allowedRole {
					return c.Next() // User has required role
				}
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":          "Insufficient permissions",
			"required_roles": allowedRoles,
		})
	}
}

// TenantScope ensures user_id is present and validates tenant_id based on role
// - Members: tenant_id is optional (solo users allowed)
// - Coaches/Admins: tenant_id is required (must belong to a gym/tenant)
func TenantScope() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Verify user_id exists (required for all)
		userID := c.Locals(UserIDKey)
		if userID == nil || userID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing user context",
			})
		}

		// Get roles to determine tenant_id requirement
		rolesInterface := c.Locals(RolesKey)
		if rolesInterface != nil {
			roles, ok := rolesInterface.([]string)
			if ok {
				// Check role-based tenant requirements
				isSuperAdmin := false
				isTenantAdmin := false
				isCoach := false

				for _, role := range roles {
					switch role {
					case "super_admin":
						isSuperAdmin = true
					case "tenant_admin":
						isTenantAdmin = true
					case "coach":
						isCoach = true
					}
				}

				// super_admin: Global access, no tenant required
				if isSuperAdmin {
					return c.Next()
				}

				// tenant_admin and coaches MUST have a tenant
				if isTenantAdmin || isCoach {
					tenantID := c.Locals(TenantIDKey)
					if tenantID == nil || tenantID == "" {
						roleType := "Tenant admin"
						if isCoach {
							roleType = "Coach"
						}
						return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
							"error": roleType + " must belong to a tenant",
						})
					}

					// Coach Specific: Must have HomeBranchID
					// REMOVED: Strict check in middleware to allow dynamic DB lookup in handlers
					// This supports changing assignments without re-login.
				}
			}
		}

		// Members can proceed without tenant_id (solo users)
		return c.Next()
	}
}
