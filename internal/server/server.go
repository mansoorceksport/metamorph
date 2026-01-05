package server

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/mansoorceksport/metamorph/internal/config"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/handler"
	"github.com/mansoorceksport/metamorph/internal/middleware"
	"github.com/mansoorceksport/metamorph/internal/repository"
	"github.com/mansoorceksport/metamorph/internal/service"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// AppDependencies holds the dependencies required to start the application
type AppDependencies struct {
	Config      *config.Config
	MongoDB     *mongo.Database
	RedisClient *redis.Client
	AuthClient  service.FirebaseAuthClient
}

// NewApp creates and configures the Fiber application with the given dependencies
func NewApp(deps AppDependencies) *fiber.App {
	// Initialize repositories
	mongoRepo := repository.NewMongoInBodyRepository(deps.MongoDB)
	redisRepo := repository.NewRedisCacheRepository(deps.RedisClient)
	tenantRepo := repository.NewMongoTenantRepository(deps.MongoDB)
	userRepo := repository.NewMongoUserRepository(deps.MongoDB)
	branchRepo := repository.NewMongoBranchRepository(deps.MongoDB)
	pkgRepo := repository.NewMongoPTPackageRepository(deps.MongoDB)
	contractRepo := repository.NewMongoPTContractRepository(deps.MongoDB)
	schedRepo := repository.NewMongoScheduleRepository(deps.MongoDB)
	exerciseRepo := repository.NewMongoExerciseRepository(deps.MongoDB)
	templateRepo := repository.NewMongoTemplateRepository(deps.MongoDB)
	workoutSessionRepo := repository.NewMongoWorkoutSessionRepository(deps.MongoDB)

	// S3 Init (Optional/Mockable in future, for now using config if available)
	// For tests, we might want to mock this too, but for now we'll create it directly
	// or skip if config is missing/test mode.
	// In a real refactor we'd pass S3Repo as dependency too.
	// For this pass, we'll try to init it.
	s3Repo, err := repository.NewSeaweedS3Repository(context.Background(), deps.Config.S3)
	if err != nil {
		log.Printf("Warning: Failed to initialize S3 repository: %v", err)
	}

	// Initialize services
	digitizerService := service.NewOpenRouterDigitizer(
		deps.Config.OpenRouter.APIKey,
		deps.Config.OpenRouter.Model,
		userRepo,
		tenantRepo,
	)

	scanService := service.NewScanService(
		digitizerService,
		mongoRepo,
		redisRepo,
		s3Repo,
	)

	// Initialize analytics service
	analyticsService := service.NewAnalyticsService(mongoRepo)

	// Initialize trend service
	trendService := service.NewTrendService(mongoRepo, redisRepo)

	// Initialize auth service
	authService := service.NewAuthService(userRepo, tenantRepo, deps.AuthClient, deps.Config.JWT.Secret)
	ptService := service.NewPTService(pkgRepo, contractRepo, schedRepo, workoutSessionRepo)
	workoutService := service.NewWorkoutService(exerciseRepo, templateRepo, workoutSessionRepo, schedRepo)

	// Initialize dashboard service
	dashboardService := service.NewDashboardService(contractRepo, schedRepo, mongoRepo, workoutSessionRepo, userRepo)

	// Initialize handlers
	scanHandler := handler.NewScanHandler(scanService, deps.Config.Server.MaxUploadSizeMB)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsService, trendService)
	authHandler := handler.NewAuthHandler(authService)
	saasHandler := handler.NewSaaSHandler(tenantRepo, userRepo, branchRepo)
	proHandler := handler.NewProHandler(ptService, userRepo, analyticsService, dashboardService)
	ptHandler := handler.NewPTHandler(ptService, branchRepo, userRepo)
	workoutHandler := handler.NewWorkoutHandler(workoutService, exerciseRepo, templateRepo)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "HOM Gym Digitizer API",
		BodyLimit:    int(deps.Config.Server.MaxUploadSizeMB * 1024 * 1024),
		ErrorHandler: customErrorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Correlation-ID",
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

	// Auth endpoints (public)
	auth := v1.Group("/auth")
	auth.Post("/login", authHandler.LoginOrRegister)

	// ===========================================
	// MEMBER API - /v1/me/* (requires 'member' role)
	// ===========================================
	me := v1.Group("/me")
	me.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	me.Use(middleware.TenantScope())
	me.Use(middleware.AuthorizeRole(domain.RoleMember))

	meScans := me.Group("/scans")
	meScans.Post("/digitize", scanHandler.DigitizeScan)
	meScans.Get("/", scanHandler.ListScans)
	meScans.Get("/:id", scanHandler.GetScan)
	meScans.Patch("/:id", scanHandler.UpdateScan)
	meScans.Delete("/:id", scanHandler.DeleteScan)

	me.Post("/join-tenant", saasHandler.JoinTenant)
	me.Get("/contracts", ptHandler.GetMyContracts)

	meAnalytics := me.Group("/analytics")
	meAnalytics.Get("/history", analyticsHandler.GetHistory)
	meAnalytics.Get("/recap", analyticsHandler.GetRecap)

	// ===========================================
	// PRO API - /v1/pro/* (requires 'coach' or 'tenant_admin' role)
	// ===========================================
	pro := v1.Group("/pro")
	pro.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	pro.Use(middleware.TenantScope())
	pro.Use(middleware.AuthorizeRole(domain.RoleCoach, domain.RoleTenantAdmin))

	pro.Get("/clients", proHandler.GetClients)
	pro.Get("/clients/:id/history", proHandler.GetClientHistory)
	pro.Get("/dashboard/summary", proHandler.GetDashboardSummary)
	pro.Get("/schedules", proHandler.GetMySchedules) // Get coach's schedules for date range

	pro.Post("/schedules", ptHandler.CreateSchedule)
	pro.Post("/schedules/:id/complete", ptHandler.CompleteSession)
	pro.Delete("/schedules/:id", ptHandler.DeleteSchedule)

	// ===========================================
	// PLATFORM API - /v1/platform/* (requires 'super_admin' role)
	// ===========================================
	platform := v1.Group("/platform")
	platform.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	platform.Use(middleware.TenantScope())
	platform.Use(middleware.AuthorizeRole(domain.RoleSuperAdmin))

	platformTenants := platform.Group("/tenants")
	platformTenants.Post("/", saasHandler.CreateTenant)
	platformTenants.Get("/:id", saasHandler.GetTenant)
	platformTenants.Put("/:id", saasHandler.UpdateTenant)

	// Deprecated: Assignments replaced by Contracts
	// platformAssignments := platform.Group("/assignments")
	// platformAssignments.Post("/", saasHandler.AssignCoach)
	// platformAssignments.Delete("/:id", saasHandler.RemoveAssignment)

	platformTenantAdmins := platform.Group("/tenant-admins")
	platformTenantAdmins.Post("/", saasHandler.CreateTenantAdmin)
	platformTenantAdmins.Get("/", saasHandler.ListTenantAdmins)
	platformTenantAdmins.Get("/:id", saasHandler.GetUser)
	platformTenantAdmins.Put("/:id", saasHandler.UpdateUser)
	platformTenantAdmins.Delete("/:id", saasHandler.DeleteUser)

	platformBranches := platform.Group("/branches")
	platformBranches.Post("/", saasHandler.CreateBranch)
	platformBranches.Get("/", saasHandler.ListBranches)
	platformBranches.Get("/:id", saasHandler.GetBranch)
	platformBranches.Put("/:id", saasHandler.UpdateBranch)
	platformBranches.Delete("/:id", saasHandler.DeleteBranch)

	// ===========================================
	// TENANT-ADMIN API - /v1/tenant-admin/* (requires 'tenant_admin' role)
	// ===========================================
	tenantAdmin := v1.Group("/tenant-admin")
	tenantAdmin.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	tenantAdmin.Use(middleware.TenantScope())
	tenantAdmin.Use(middleware.AuthorizeRole(domain.RoleTenantAdmin))

	// Deprecated: Assignments replaced by Contracts
	// tenantAssignments := tenantAdmin.Group("/assignments")
	// tenantAssignments.Post("/", saasHandler.AssignCoach)
	// tenantAssignments.Delete("/:id", saasHandler.RemoveAssignment)

	tenantAdminUsers := tenantAdmin.Group("/users")
	tenantAdminUsers.Get("/", saasHandler.ListUsers)
	tenantAdminUsers.Post("/", saasHandler.CreateUser)
	tenantAdminUsers.Get("/:id", saasHandler.GetUser)
	tenantAdminUsers.Put("/:id", saasHandler.UpdateUser)
	tenantAdminUsers.Delete("/:id", saasHandler.DeleteUser)

	tenantAdminCoaches := tenantAdmin.Group("/coaches")
	tenantAdminCoaches.Get("/", saasHandler.ListCoaches)
	tenantAdminCoaches.Post("/", saasHandler.CreateCoach)
	tenantAdminCoaches.Get("/:id", saasHandler.GetCoach)
	tenantAdminCoaches.Put("/:id", saasHandler.UpdateCoach)
	tenantAdminCoaches.Delete("/:id", saasHandler.DeleteCoach)

	tenantAdminBranches := tenantAdmin.Group("/branches")
	tenantAdminBranches.Post("/", saasHandler.CreateBranch)
	tenantAdminBranches.Get("/", saasHandler.ListBranches)
	tenantAdminBranches.Get("/:id", saasHandler.GetBranch)
	tenantAdminBranches.Put("/:id", saasHandler.UpdateBranch)
	tenantAdminBranches.Delete("/:id", saasHandler.DeleteBranch)

	tenantAdminPackages := tenantAdmin.Group("/packages")
	tenantAdminPackages.Post("/", ptHandler.CreatePackageTemplate)
	tenantAdminPackages.Get("/", ptHandler.ListPackageTemplates)
	tenantAdminPackages.Get("/:id", ptHandler.GetPackageTemplate)
	tenantAdminPackages.Put("/:id", ptHandler.UpdatePackageTemplate)

	tenantAdminContracts := tenantAdmin.Group("/contracts")
	tenantAdminContracts.Post("/", ptHandler.CreateContract)
	tenantAdminContracts.Get("/", ptHandler.ListContracts)

	// ===========================================
	// SHARED /schedules & /contracts API (Coach & Member & Admin)
	// ===========================================
	schedules := v1.Group("/schedules")
	schedules.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	schedules.Use(middleware.TenantScope())
	schedules.Get("/", ptHandler.ListSchedules) // Filter by coach_id/member_id
	schedules.Get("/:id", ptHandler.GetSchedule)
	// Reschedule: Coach or Member
	schedules.Patch("/:id/reschedule", middleware.AuthorizeRole(domain.RoleCoach, domain.RoleMember), ptHandler.RescheduleSession)

	// Shared Contracts details (for getting by ID)
	contracts := v1.Group("/contracts")
	contracts.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	contracts.Use(middleware.TenantScope())
	contracts.Get("/:id", ptHandler.GetContract)

	// ===========================================
	// EXERCISES & TEMPLATES API (Shared)
	// ===========================================
	// Public Read, Admin Write

	// Exercises
	v1.Get("/exercises", workoutHandler.ListExercises)
	// Admin CRUD
	adminEx := v1.Group("/exercises")
	adminEx.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	adminEx.Use(middleware.TenantScope()) // Ensure tenant context? Exercises are GLOBAL. But Auth middleware usually requires it.
	// Actually, Exercises are Global. Middleware TenantScope might fail if user has no tenant?
	// But SuperAdmin might not be in a tenant.
	// Let's assume SuperAdmin or TenantAdmin can manage.
	// For "Global", strictly SuperAdmin.
	adminEx.Use(middleware.AuthorizeRole(domain.RoleSuperAdmin))
	adminEx.Post("/", workoutHandler.CreateExercise)
	adminEx.Put("/:id", workoutHandler.UpdateExercise)
	adminEx.Delete("/:id", workoutHandler.DeleteExercise)

	// Templates
	v1.Get("/templates", workoutHandler.ListTemplates)
	adminTpl := v1.Group("/templates")
	adminTpl.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	adminTpl.Use(middleware.TenantScope())
	adminTpl.Use(middleware.AuthorizeRole(domain.RoleSuperAdmin))
	adminTpl.Post("/", workoutHandler.CreateTemplate)
	adminTpl.Put("/:id", workoutHandler.UpdateTemplate)
	adminTpl.Delete("/:id", workoutHandler.DeleteTemplate)

	// ===========================================
	// WORKOUT SESSIONS (Pro)
	// ===========================================
	// Added to existing 'pro' group
	pro.Post("/sessions/initialize", workoutHandler.InitializeSession)
	pro.Patch("/sessions/:id/log-ulid", workoutHandler.LogSessionSetByULID) // ULID-first atomic

	pro.Post("/schedules/:schedule_id/exercises", workoutHandler.AddExercise)
	pro.Delete("/exercises/:id", workoutHandler.RemoveExercise)
	pro.Put("/exercises/:id", workoutHandler.UpdatePlannedExercise)

	return app
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	log.Printf("Error: %v", err)
	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
	})
}
