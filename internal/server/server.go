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
	"github.com/mansoorceksport/metamorph/internal/telemetry"
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
	schedMongoRepo := repository.NewMongoScheduleRepository(deps.MongoDB)
	schedRepo := repository.NewCachedScheduleRepository(schedMongoRepo, redisRepo)
	exerciseRepo := repository.NewMongoExerciseRepository(deps.MongoDB)
	templateRepo := repository.NewMongoTemplateRepository(deps.MongoDB)
	workoutSessionRepo := repository.NewMongoWorkoutSessionRepository(deps.MongoDB)
	setLogRepo := repository.NewMongoSetLogRepository(deps.MongoDB)
	pbRepo := repository.NewMongoPersonalBestRepository(deps.MongoDB)
	dailyVolumeRepo := repository.NewMongoDailyVolumeRepository(deps.MongoDB)
	refreshTokenRepo := repository.NewMongoRefreshTokenRepository(deps.MongoDB)

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
	tokenService := service.NewTokenService(deps.Config.JWT, refreshTokenRepo, userRepo)
	ptService := service.NewPTService(pkgRepo, contractRepo, schedRepo, workoutSessionRepo, setLogRepo, pbRepo)
	workoutService := service.NewWorkoutService(exerciseRepo, templateRepo, workoutSessionRepo, schedRepo, setLogRepo, pbRepo, dailyVolumeRepo)

	// Initialize dashboard service
	dashboardService := service.NewDashboardService(contractRepo, schedRepo, mongoRepo, workoutSessionRepo, userRepo, pbRepo)

	// Initialize handlers
	scanHandler := handler.NewScanHandler(scanService, deps.Config.Server.MaxUploadSizeMB)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsService, trendService)
	authHandler := handler.NewAuthHandler(authService, tokenService)
	saasHandler := handler.NewSaaSHandler(tenantRepo, userRepo, branchRepo)
	proHandler := handler.NewProHandler(ptService, userRepo, analyticsService, dashboardService, pbRepo, scanService, mongoRepo, workoutService, schedRepo, deps.Config.Server.MaxUploadSizeMB)
	ptHandler := handler.NewPTHandler(ptService, branchRepo, userRepo, workoutService)
	workoutHandler := handler.NewWorkoutHandler(workoutService, exerciseRepo, templateRepo)
	memberHandler := handler.NewMemberHandler(pbRepo, workoutService, ptService, schedRepo, mongoRepo, redisRepo, exerciseRepo, userRepo)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "HOM Gym Digitizer API",
		BodyLimit:    int(deps.Config.Server.MaxUploadSizeMB * 1024 * 1024),
		ErrorHandler: customErrorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(logger.New())

	// OpenTelemetry tracing middleware (before other middleware)
	if deps.Config.OTEL.Enabled {
		app.Use(telemetry.FiberMiddleware())
	}

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:3000, http://localhost:3001, http://192.168.1.10:3000, https://pt.cek-sport.com",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Correlation-ID",
		AllowMethods:     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		AllowCredentials: true, // Required for httpOnly cookie refresh tokens
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
	auth.Post("/refresh", authHandler.RefreshToken)
	auth.Post("/logout", authHandler.Logout)

	// ===========================================
	// MEMBER API - /v1/me/* (requires 'member' role)
	// ===========================================
	me := v1.Group("/me")
	me.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	me.Use(middleware.TenantScope())
	me.Use(middleware.AuthorizeRole(domain.RoleMember))

	// Member dashboard and data endpoints
	me.Get("/dashboard", memberHandler.GetMyDashboard)
	me.Get("/pbs", memberHandler.GetMyPBs)
	me.Get("/volume-history", memberHandler.GetMyVolumeHistory)
	me.Get("/schedules", memberHandler.GetMySchedules)

	// Workouts hub endpoints
	meWorkouts := me.Group("/workouts")
	meWorkouts.Get("/history", memberHandler.GetMyWorkoutHistory)
	meWorkouts.Get("/:id", memberHandler.GetMyWorkoutDetail)

	meScans := me.Group("/scans")
	meScans.Post("/digitize", scanHandler.DigitizeScan)
	meScans.Get("/", memberHandler.GetMyScans)   // Optimized: paginated, lightweight list
	meScans.Get("/:id", memberHandler.GetMyScan) // Optimized: cached detail
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
	pro.Get("/clients/simple", proHandler.GetClientsSimple) // Lightweight for /members list
	pro.Get("/clients/:id/history", proHandler.GetClientHistory)
	pro.Get("/dashboard/summary", proHandler.GetDashboardSummary)
	pro.Get("/schedules", proHandler.GetMySchedules)                          // Get coach's schedules for date range
	pro.Get("/schedules/hydrate", proHandler.HydrateSchedules)                // Login hydration - all statuses including cancelled
	pro.Get("/members/:member_id/pbs", proHandler.GetMemberPBs)               // Get member's personal bests
	pro.Get("/members/:id", proHandler.GetMember)                             // Get member details
	pro.Get("/members/:id/scans", proHandler.GetMemberScans)                  // Get member's scan records
	pro.Get("/members/:id/volume-history", proHandler.GetMemberVolumeHistory) // Get member's workout volume history
	pro.Get("/packages", proHandler.ListPackages)                             // List available packages
	pro.Get("/scans/:id", proHandler.GetScan)                                 // Get single scan by ID
	pro.Post("/members", proHandler.CreateMember)                             // Coach creates new member
	pro.Post("/members/:id/scans", proHandler.DigitizeMemberScan)             // Coach uploads scan for member
	pro.Post("/contracts", proHandler.CreateContract)                         // Coach creates contract for member
	pro.Put("/scans/:id", proHandler.UpdateScan)                              // Update scan data
	pro.Delete("/scans/:id", proHandler.DeleteScan)                           // Delete scan

	pro.Post("/schedules", ptHandler.CreateSchedule)
	pro.Post("/schedules/:id/complete", ptHandler.CompleteSession)
	pro.Put("/schedules/:id/status", ptHandler.UpdateScheduleStatus)
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
	// Exercise CRUD (Coach and Admin can create/update/delete)
	adminEx := v1.Group("/exercises")
	adminEx.Use(middleware.VerifyMetamorphToken(deps.Config.JWT.Secret))
	adminEx.Use(middleware.TenantScope())
	// Allow Coach to manage exercises (will restrict to SuperAdmin later via Metamorph Dashboard)
	adminEx.Use(middleware.AuthorizeRole(domain.RoleSuperAdmin, domain.RoleCoach, domain.RoleTenantAdmin))
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

	// Atomic set operations (new set_logs collection)
	pro.Put("/sets/:id", workoutHandler.UpdateSetLog)
	pro.Delete("/sets/:id", workoutHandler.DeleteSetLog)
	pro.Post("/exercises/:id/sets", workoutHandler.AddSetToExercise)
	pro.Get("/schedules/:schedule_id/sets", workoutHandler.ListScheduleSets)
	pro.Get("/schedules/:schedule_id/exercises", workoutHandler.ListScheduleExercises)

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
