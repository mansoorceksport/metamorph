# HOM Gym Digitizer Service Implementation Plan

This plan outlines the implementation of a Digitizer Service for the HOM Gym app that uses OpenRouter AI to extract InBody scan metrics, persists data to MongoDB, and caches results in Redis with Firebase authentication.

## Configuration Decisions

> [!NOTE]
> **OpenRouter API - Cost Analysis**
> - **Gemini 1.5 Pro** selected (cheapest option with maximum results)
>   - Input: $1.25/M tokens vs Claude 3.5 Sonnet: $6/M tokens (~5x cheaper)
>   - Output: $2.50/M tokens vs Claude 3.5 Sonnet: $30/M tokens (~12x cheaper)
> - API key will be read from `.env` file
> 
> **Upload Configuration**
> - Max file size: **5MB** (typical iPhone photo is ~2.3MB)
> - Supported formats: JPEG, PNG
> 
> **Firebase Authentication**
> - Credentials configured via **environment variables** in `.env` (no JSON file to commit)
> - Required env vars: `FIREBASE_PROJECT_ID`, `FIREBASE_PRIVATE_KEY`, `FIREBASE_CLIENT_EMAIL`
> 
> **HTTP Framework**
> - Using **Fiber v2** (fast, Express-like Go framework)

## Proposed Changes

### Project Structure

```
metamorph/
├── cmd/
│   └── main.go                          # Application entry point
├── docs/
│   └── implementation_plan.md           # This architecture documentation
├── internal/
│   ├── config/
│   │   └── config.go                    # Configuration management
│   ├── domain/
│   │   └── inbody.go                    # InBodyRecord model + interfaces
│   ├── middleware/
│   │   └── firebase_auth.go             # Firebase JWT validation middleware
│   ├── repository/
│   │   ├── mongo_inbody.go              # MongoDB repository implementation
│   │   └── redis_cache.go               # Redis cache implementation
│   ├── service/
│   │   ├── digitizer.go                 # OpenRouter AI service
│   │   └── scan.go                      # Business logic for scans
│   └── handler/
│       └── scan_handler.go              # HTTP handlers
├── docker-compose.yaml                  # Already exists, will update
├── .env.example                         # Environment variables template
├── .gitignore                           # Update to ignore credentials
└── go.mod                               # Dependency management
```

---

### Domain Layer

#### [NEW] [inbody.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/domain/inbody.go)

**Purpose**: Define the core domain model and repository/service interfaces following dependency inversion principle.

```go
// InBodyRecord - the provided struct
// InBodyRepository - interface for data persistence
// CacheRepository - interface for Redis caching
// DigitizerService - interface for AI-based extraction
// ScanService - interface for business logic
```

This ensures the domain layer has NO dependencies on external packages (MongoDB, Redis, Firebase, etc.).

---

### Repository Layer

#### [NEW] [mongo_inbody.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/repository/mongo_inbody.go)

**Purpose**: Implements `InBodyRepository` interface using MongoDB official driver.

**Key Methods**:
- `Create(ctx context.Context, record *InBodyRecord) error`
- `GetLatestByUserID(ctx context.Context, userID string) (*InBodyRecord, error)`
- `GetByUserID(ctx context.Context, userID string, limit int) ([]*InBodyRecord, error)`

**Dependencies**: `go.mongodb.org/mongo-driver/mongo`

---

#### [NEW] [redis_cache.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/repository/redis_cache.go)

**Purpose**: Implements `CacheRepository` interface for caching latest scans.

**Key Methods**:
- `SetLatestScan(ctx context.Context, userID string, record *InBodyRecord, ttl time.Duration) error`
- `GetLatestScan(ctx context.Context, userID string) (*InBodyRecord, error)`
- `InvalidateUserCache(ctx context.Context, userID string) error`

**Dependencies**: `github.com/redis/go-redis/v9`

---

### Service Layer

#### [NEW] [digitizer.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/service/digitizer.go)

**Purpose**: Implements `DigitizerService` interface to interact with OpenRouter API.

**Key Methods**:
- `ExtractMetrics(ctx context.Context, imageData []byte) (*InBodyMetrics, error)`

**Model Configuration**:
- Using `google/gemini-1.5-pro` (most cost-effective: $1.25/M input, $2.50/M output)
- Vision capability enabled for image analysis

**Prompt Template**:
```
Extract weight, smm, body_fat_mass, pbf, bmi, bmr, and test_date from this InBody 270 scan. 
Return ONLY JSON in this exact format:
{
  "weight": 0.0,
  "smm": 0.0,
  "body_fat_mass": 0.0,
  "pbf": 0.0,
  "bmi": 0.0,
  "bmr": 0,
  "test_date": "2025-12-24T10:00:00Z"
}
```

**Dependencies**: OpenRouter HTTP client

---

#### [NEW] [scan.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/service/scan.go)

**Purpose**: Implements `ScanService` interface for orchestrating the entire digitization workflow.

**Key Methods**:
- `ProcessScan(ctx context.Context, userID string, imageData []byte, imageURL string) (*InBodyRecord, error)`

**Business Logic**:
1. Call `DigitizerService.ExtractMetrics()` to get AI-parsed data
2. Build `InBodyRecord` with userID from JWT context
3. Save to MongoDB via `InBodyRepository.Create()`
4. Cache in Redis via `CacheRepository.SetLatestScan()` with 24h TTL
5. Return the saved record

---

### Middleware Layer

#### [NEW] [firebase_auth.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/middleware/firebase_auth.go)

**Purpose**: Fiber middleware that validates Firebase JWT tokens from the `Authorization: Bearer <token>` header.

**Flow**:
1. Extract token from `Authorization` header
2. Verify using Firebase Admin SDK
3. Extract UID from token
4. Attach UID to Fiber context: `ctx.Locals("userID", uid)`
5. Call `ctx.Next()`

**Error Handling**:
- Missing token → 401 Unauthorized
- Invalid token → 401 Unauthorized
- Expired token → 401 Unauthorized

**Dependencies**: `firebase.google.com/go/v4/auth`

---

### HTTP Handler Layer

#### [NEW] [scan_handler.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/handler/scan_handler.go)

**Purpose**: HTTP handlers for scan-related endpoints.

**Endpoints**:

**POST /v1/scans/digitize**
- Accepts: `multipart/form-data` with `image` field
- Middleware: Firebase Auth (required)
- Max upload size: **5MB**
- Validation: 
  - Image file exists
  - File size ≤ 5MB
  - MIME type is `image/jpeg` or `image/png`
- Flow:
  1. Get userID from `ctx.Locals("userID")`
  2. Read image bytes from form file
  3. Call `ScanService.ProcessScan()`
  4. Return JSON response with saved record

**Response Format**:
```json
{
  "success": true,
  "data": {
    "id": "...",
    "user_id": "...",
    "test_date_time": "...",
    "weight": 75.5,
    ...
  }
}
```

---

### Configuration

#### [NEW] [config.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/internal/config/config.go)

**Purpose**: Centralized configuration management using environment variables.

**Configuration Struct**:
```go
type Config struct {
    Server ServerConfig
    MongoDB MongoDBConfig
    Redis RedisConfig
    Firebase FirebaseConfig
    OpenRouter OpenRouterConfig
}
```

**Environment Variables**:
- `PORT` (default: 8080)
- `MONGODB_URI` (default: mongodb://localhost:27017)
- `MONGODB_DATABASE` (default: homgym)
- `REDIS_ADDR` (default: localhost:6379)
- `REDIS_PASSWORD` (optional)
- `FIREBASE_PROJECT_ID` (required)
- `FIREBASE_PRIVATE_KEY` (required, base64 encoded)
- `FIREBASE_CLIENT_EMAIL` (required)
- `OPENROUTER_API_KEY` (required)
- `OPENROUTER_MODEL` (default: google/gemini-1.5-pro)
- `MAX_UPLOAD_SIZE_MB` (default: 5)

---

#### [MODIFY] [main.go](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/cmd/main.go)

**Changes**:
- Load configuration
- Initialize Firebase Admin SDK
- Connect to MongoDB
- Connect to Redis
- Initialize repositories
- Initialize services
- Initialize handlers
- Setup Fiber app with routes and middleware
- Start HTTP server with graceful shutdown

**Dependency Injection Flow**:
```
Config → Clients (MongoDB, Redis, Firebase) → Repositories → Services → Handlers → Fiber App
```

---

#### [MODIFY] [go.mod](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/go.mod)

**Add Dependencies**:
- `github.com/gofiber/fiber/v2` - HTTP framework
- `go.mongodb.org/mongo-driver` - MongoDB driver
- `github.com/redis/go-redis/v9` - Redis client
- `firebase.google.com/go/v4` - Firebase Admin SDK
- `github.com/joho/godotenv` - Environment variables loader
- `google.golang.org/api` - Required by Firebase SDK

---

#### [NEW] [.env.example](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/.env.example)

**Purpose**: Template for environment variables.

Contains all required and optional environment variables with example values and descriptions.

---

#### [MODIFY] [.gitignore](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/.gitignore)

**Add entries**:
- `.env`

---

### Infrastructure

#### [MODIFY] [docker-compose.yaml](file:///Users/mansoor/go/src/github.com/mansoorceksport/metamorph/docker-compose.yaml)

**Changes**:
- Add MongoDB authentication (optional but recommended)
- Add environment variables for MongoDB and Redis
- Potentially add the Go app service for full containerization (optional)

---

## Verification Plan

### Automated Tests

I will create a basic structure that allows for testing, but comprehensive unit tests can be added later:

1. **Domain Tests**: Verify interfaces compile correctly
2. **Service Tests**: Mock-based tests for `ScanService` (optional, can be added later)

### Manual Verification

After implementation, you'll need to:

1. **Environment Setup**:
   ```bash
   cp .env.example .env
   # Edit .env with your actual Firebase credentials and API keys
   ```

2. **Start Dependencies**:
   ```bash
   docker-compose up -d
   ```

3. **Run Application**:
   ```bash
   go run cmd/main.go
   ```

4. **Test Endpoint**:
   ```bash
   # Get a Firebase token from your frontend/client
   curl -X POST http://localhost:8080/v1/scans/digitize \
     -H "Authorization: Bearer YOUR_FIREBASE_TOKEN" \
     -F "image=@path/to/inbody-scan.jpg"
   ```

5. **Verify MongoDB**: Check that records are saved in the `inbody_records` collection
6. **Verify Redis**: Check that the latest scan is cached with 24h TTL
7. **Test Token Validation**: Try requests without token or with invalid token to verify 401 responses

---

## Next Steps

Once you approve this plan, I will:
1. Generate all interface definitions in the domain layer
2. Implement all repository, service, middleware, and handler code
3. Wire everything together in `main.go`
4. Update configuration files and dependencies
5. Create a comprehensive walkthrough
