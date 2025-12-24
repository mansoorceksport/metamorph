package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	Server     ServerConfig
	MongoDB    MongoDBConfig
	Redis      RedisConfig
	Firebase   FirebaseConfig
	OpenRouter OpenRouterConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            string
	MaxUploadSizeMB int64
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
