package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	firebase "firebase.google.com/go/v4"
	"github.com/gofiber/fiber/v2"
	"google.golang.org/api/option"
)

const (
	userIDKey = "userID"
)

// FirebaseAuth creates a Fiber middleware that validates Firebase JWT tokens
func FirebaseAuth(firebaseApp *firebase.App) fiber.Handler {
	// Get Auth client
	authClient, err := firebaseApp.Auth(context.Background())
	if err != nil {
		panic("failed to initialize Firebase Auth client: " + err.Error())
	}

	return func(c *fiber.Ctx) error {
		// Extract token from Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "missing authorization header",
			})
		}

		// Extract Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "invalid authorization header format, expected 'Bearer <token>'",
			})
		}

		token := parts[1]

		// Verify the token
		decodedToken, err := authClient.VerifyIDToken(context.Background(), token)
		if err != nil {
			// Check for specific error types
			if strings.Contains(err.Error(), "expired") {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"success": false,
					"error":   "token expired",
				})
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "invalid token",
			})
		}

		// Extract UID and attach to context
		userID := decodedToken.UID
		c.Locals(userIDKey, userID)

		// Continue to next handler
		return c.Next()
	}
}

// InitFirebase initializes Firebase Admin SDK with environment variables
func InitFirebase(projectID, privateKeyB64, clientEmail string) (*firebase.App, error) {
	// Decode base64 private key
	privateKey, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return nil, err
	}

	// Create credentials JSON
	credentialsJSON := map[string]interface{}{
		"type":         "service_account",
		"project_id":   projectID,
		"private_key":  string(privateKey),
		"client_email": clientEmail,
	}

	// Initialize Firebase app
	app, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsJSON(mustMarshalJSON(credentialsJSON)))
	if err != nil {
		return nil, err
	}

	return app, nil
}

// GetUserID extracts the user ID from Fiber context
// Should only be called after FirebaseAuth middleware
func GetUserID(c *fiber.Ctx) string {
	userID, ok := c.Locals(userIDKey).(string)
	if !ok {
		return ""
	}
	return userID
}

// mustMarshalJSON is a helper to marshal JSON or panic
func mustMarshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
