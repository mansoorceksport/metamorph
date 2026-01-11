package main

import (
	"context"
	"encoding/base64"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mansoorceksport/metamorph/internal/config"
	"github.com/mansoorceksport/metamorph/internal/middleware"
	"github.com/mansoorceksport/metamorph/internal/server"
	"github.com/mansoorceksport/metamorph/internal/telemetry"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Println("Starting HOM Gym Digitizer Service...")

	// Initialize OpenTelemetry (for Grafana Cloud)
	ctx := context.Background()

	// Grafana Cloud requires Basic auth with instanceId:apiToken base64 encoded
	authString := cfg.OTEL.InstanceID + ":" + cfg.OTEL.Token
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))

	otelProvider, err := telemetry.Initialize(ctx, telemetry.Config{
		ServiceName:    cfg.OTEL.ServiceName,
		ServiceVersion: cfg.OTEL.ServiceVersion,
		Environment:    cfg.OTEL.Environment,
		OTLPEndpoint:   cfg.OTEL.Endpoint,
		OTLPHeaders: map[string]string{
			"Authorization": "Basic " + authEncoded,
		},
		Enabled: cfg.OTEL.Enabled,
	})
	if err != nil {
		log.Printf("Warning: Failed to initialize OpenTelemetry: %v", err)
	}
	if otelProvider != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			otelProvider.Shutdown(shutdownCtx)
		}()
	}

	// Initialize Firebase
	firebaseApp, err := middleware.InitFirebase(
		cfg.Firebase.ProjectID,
		cfg.Firebase.PrivateKey,
		cfg.Firebase.ClientEmail,
	)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase: %v", err)
	}

	authClient, err := firebaseApp.Auth(ctx)
	if err != nil {
		log.Fatalf("Failed to get Firebase Auth client: %v", err)
	}
	log.Println("✓ Firebase initialized")

	// Connect to MongoDB with OpenTelemetry instrumentation
	ctxMongo, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoOpts := options.Client().ApplyURI(cfg.MongoDB.URI)
	// Add OTEL monitor for MongoDB tracing
	if cfg.OTEL.Enabled {
		mongoOpts.SetMonitor(otelmongo.NewMonitor())
	}

	mongoClient, err := mongo.Connect(ctxMongo, mongoOpts)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := mongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
		}
	}()

	// Ping MongoDB to verify connection
	if err := mongoClient.Ping(ctxMongo, nil); err != nil {
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

	// Initialize App using Server package
	app := server.NewApp(server.AppDependencies{
		Config:      cfg,
		MongoDB:     mongoDB,
		RedisClient: redisClient,
		AuthClient:  authClient,
	})

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down gracefully...")
		app.Shutdown()
	}()

	// Start server
	log.Printf("🚀 Server starting on port %s", cfg.Server.Port)
	if err := app.Listen(":" + cfg.Server.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
