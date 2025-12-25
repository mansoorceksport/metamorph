package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/mansoorceksport/metamorph/internal/config"
	"github.com/mansoorceksport/metamorph/internal/handler"
	"github.com/mansoorceksport/metamorph/internal/middleware"
	"github.com/mansoorceksport/metamorph/internal/repository"
	"github.com/mansoorceksport/metamorph/internal/service"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Println("Starting HOM Gym Digitizer Service...")

	// Initialize Firebase
	firebaseApp, err := middleware.InitFirebase(
		cfg.Firebase.ProjectID,
		cfg.Firebase.PrivateKey,
		cfg.Firebase.ClientEmail,
	)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase: %v", err)
	}
	log.Println("✓ Firebase initialized")

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDB.URI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := mongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
		}
	}()

	// Ping MongoDB to verify connection
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}
	log.Println("✓ MongoDB connected")

	mongoDB := mongoClient.Database(cfg.MongoDB.Database)

	// Connect to Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       0,
	})
	defer redisClient.Close()

	// Ping Redis to verify connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✓ Redis connected")

	// Initialize repositories
	mongoRepo := repository.NewMongoInBodyRepository(mongoDB)
	redisRepo := repository.NewRedisCacheRepository(redisClient)

	s3Repo, err := repository.NewSeaweedS3Repository(ctx, cfg.S3)
	if err != nil {
		log.Printf("Warning: Failed to initialize S3 repository: %v", err)
		// We don't fatal here because app can technically run without upload if we wanted,
		// but given requirements it implies critical failure.
		// For now we'll log warning and s3Repo will be nil (service handles nil)
	} else {
		log.Println("✓ SeaweedFS S3 repository initialized")
	}

	// Initialize services
	digitizerService := service.NewOpenRouterDigitizer(
		cfg.OpenRouter.APIKey,
		cfg.OpenRouter.Model,
	)

	scanService := service.NewScanService(
		digitizerService,
		mongoRepo,
		redisRepo,
		s3Repo,
	)
	log.Println("✓ Services initialized")

	// Initialize handlers
	scanHandler := handler.NewScanHandler(scanService, cfg.Server.MaxUploadSizeMB)
	log.Println("✓ Handlers initialized")

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "HOM Gym Digitizer API",
		BodyLimit:    int(cfg.Server.MaxUploadSizeMB * 1024 * 1024),
		ErrorHandler: customErrorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"service": "hom-gym-digitizer",
		})
	})

	// API v1 routes
	v1 := app.Group("/v1")

	// Scans endpoints - all protected by Firebase Auth
	scans := v1.Group("/scans")
	scans.Use(middleware.FirebaseAuth(firebaseApp)) // Apply auth to all scan routes

	scans.Post("/digitize", scanHandler.DigitizeScan) // POST /v1/scans/digitize
	scans.Get("/", scanHandler.ListScans)             // GET /v1/scans
	scans.Get("/:id", scanHandler.GetScan)            // GET /v1/scans/:id
	scans.Patch("/:id", scanHandler.UpdateScan)       // PATCH /v1/scans/:id
	scans.Delete("/:id", scanHandler.DeleteScan)      // DELETE /v1/scans/:id

	// Start server in goroutine
	port := cfg.Server.Port
	go func() {
		log.Printf("🚀 Server starting on port %s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	if err := app.Shutdown(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Server stopped gracefully")
}

// customErrorHandler handles Fiber errors
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   message,
	})
}
