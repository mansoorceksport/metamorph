package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// IdempotencyMiddleware provides idempotency for POST/PATCH requests using X-Correlation-ID
// If the same correlation ID is received within the TTL, it returns the cached response
func IdempotencyMiddleware(redisClient *redis.Client, ttl time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Only apply to mutating methods
		if c.Method() != "POST" && c.Method() != "PATCH" && c.Method() != "PUT" {
			return c.Next()
		}

		correlationID := c.Get("X-Correlation-ID")
		if correlationID == "" {
			// No correlation ID = no idempotency check
			return c.Next()
		}

		key := fmt.Sprintf("idempotency:%s", correlationID)
		ctx := context.Background()

		// Check if we have a cached response
		cached, err := redisClient.Get(ctx, key).Bytes()
		if err == nil && len(cached) > 0 {
			// Return cached response
			c.Set("X-Idempotent-Replay", "true")
			c.Set("Content-Type", "application/json")
			return c.Send(cached)
		}

		// Process the request
		if err := c.Next(); err != nil {
			return err
		}

		// Cache successful responses (2xx status codes)
		statusCode := c.Response().StatusCode()
		if statusCode >= 200 && statusCode < 300 {
			body := c.Response().Body()
			if len(body) > 0 {
				// Cache with TTL (fire and forget)
				go func() {
					bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					redisClient.Set(bgCtx, key, body, ttl)
				}()
			}
		}

		return nil
	}
}
