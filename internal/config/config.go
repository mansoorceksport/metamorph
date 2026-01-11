package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	Server     ServerConfig
	MongoDB    MongoDBConfig
	Redis      RedisConfig
	Firebase   FirebaseConfig
	OpenRouter OpenRouterConfig
	S3         S3Config
	JWT        JWTConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            string
	MaxUploadSizeMB int64
}

type S3Config struct {
	Endpoint  string
	PublicURL string // Public URL for serving files (e.g., https://storage.cek-sport.com)
	Region    string
	Bucket    string
}

// MongoDBConfig holds MongoDB connection configuration
type MongoDBConfig struct {
	URI      string
	Database string
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addr     string
	Password string
}

// FirebaseConfig holds Firebase Admin SDK configuration
type FirebaseConfig struct {
	ProjectID   string
	PrivateKey  string // Base64 encoded
	ClientEmail string
}

// OpenRouterConfig holds OpenRouter API configuration
type OpenRouterConfig struct {
	APIKey string
	Model  string
}

// JWTConfig holds JWT token configuration
type JWTConfig struct {
	Secret             string
	AccessTokenExpiry  time.Duration // Short-lived access token (15 min default)
	RefreshTokenExpiry time.Duration // Long-lived refresh token (7 days default)
}

// Load reads configuration from environment variables
// It attempts to load from .env file first, then falls back to system env vars
func Load() (*Config, error) {
	// Try to load .env file (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			MaxUploadSizeMB: getEnvAsInt64("MAX_UPLOAD_SIZE_MB", 5),
		},
		MongoDB: MongoDBConfig{
			URI:      getEnv("MONGODB_URI", "mongodb://localhost:27017"),
			Database: getEnv("MONGODB_DATABASE", "homgym"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
		},
		Firebase: FirebaseConfig{
			ProjectID:   getEnv("FIREBASE_PROJECT_ID", ""),
			PrivateKey:  getEnv("FIREBASE_PRIVATE_KEY", ""),
			ClientEmail: getEnv("FIREBASE_CLIENT_EMAIL", ""),
		},
		OpenRouter: OpenRouterConfig{
			APIKey: getEnv("OPENROUTER_API_KEY", ""),
			Model:  getEnv("OPENROUTER_MODEL", "google/gemini-2.0-flash-001"),
		},
		S3: S3Config{
			Endpoint:  getEnv("S3_ENDPOINT", "http://localhost:8333"),
			PublicURL: getEnv("S3_PUBLIC_URL", getEnv("S3_ENDPOINT", "http://localhost:8333")), // Falls back to Endpoint if not set
			Region:    getEnv("S3_REGION", "us-east-1"),
			Bucket:    getEnv("S3_BUCKET", "inbody-scans"),
		},
		JWT: JWTConfig{
			Secret:             getEnv("JWT_SECRET", "metamorph-dev-secret-change-in-production"),
			AccessTokenExpiry:  getDurationEnv("JWT_ACCESS_TOKEN_EXPIRY", 15*time.Minute),
			RefreshTokenExpiry: getDurationEnv("JWT_REFRESH_TOKEN_EXPIRY", 7*24*time.Hour),
		},
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required configuration is present
func (c *Config) Validate() error {
	if c.Firebase.ProjectID == "" {
		return fmt.Errorf("FIREBASE_PROJECT_ID is required")
	}
	if c.Firebase.PrivateKey == "" {
		return fmt.Errorf("FIREBASE_PRIVATE_KEY is required")
	}
	if c.Firebase.ClientEmail == "" {
		return fmt.Errorf("FIREBASE_CLIENT_EMAIL is required")
	}
	if c.OpenRouter.APIKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	return nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt64 retrieves an environment variable as int64 or returns a default value
func getEnvAsInt64(key string, defaultValue int64) int64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

// getDurationEnv retrieves an environment variable as time.Duration or returns a default value
// Accepts formats like "15m", "1h", "7d" (days are converted to hours)
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	// Handle custom "d" suffix for days
	if len(valueStr) > 1 && valueStr[len(valueStr)-1] == 'd' {
		days, err := strconv.Atoi(valueStr[:len(valueStr)-1])
		if err == nil {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	duration, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}
	return duration
}
