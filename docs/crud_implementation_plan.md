# InBody Scans CRUD Implementation Plan

## Overview
Implement complete CRUD operations for the `inbody_records` collection with user ownership verification and SeaweedFS integration.

## Endpoints to Implement

### 1. List All Scans
- **Route**: `GET /v1/scans`
- **Auth**: Required (Firebase JWT)
- **Logic**: Return all scans for authenticated user, sorted by `test_date_time` DESC
- **Response**: Array of InBodyRecord objects

### 2. Get Single Scan
- **Route**: `GET /v1/scans/:id`
- **Auth**: Required (Firebase JWT)
- **Logic**: Return single scan, verify ownership
- **Response**: Single InBodyRecord object
- **Error**: 404 if not found, 403 if not owned by user

### 3. Update Scan Metrics
- **Route**: `PATCH /v1/scans/:id`
- **Auth**: Required (Firebase JWT)
- **Logic**: Update specific metrics (Weight, SMM, BodyFatMass, PBF, BMI, BMR), verify ownership
- **Request Body**:
  ```json
  {
    "weight": 75.5,
    "smm": 35.2,
    "body_fat_mass": 12.8,
    "pbf": 16.9,
    "bmi": 22.5,
    "bmr": 1650
  }
  ```
- **Response**: Updated InBodyRecord
- **Error**: 404 if not found, 403 if not owned by user

### 4. Delete Scan
- **Route**: `DELETE /v1/scans/:id`
- **Auth**: Required (Firebase JWT)
- **Logic**: Delete record from MongoDB AND image from SeaweedFS, verify ownership
- **Response**: Success message
- **Error**: 404 if not found, 403 if not owned by user

---

## Implementation Steps

### Phase 1: Domain Layer Updates

#### File: `internal/domain/inbody.go`

Add methods to `InBodyRepository` interface:
```go
type InBodyRepository interface {
    Create(ctx context.Context, record *InBodyRecord) error
    FindLatestByUserID(ctx context.Context, userID string) (*InBodyRecord, error)
    
    // New methods
    FindAllByUserID(ctx context.Context, userID string) ([]*InBodyRecord, error)
    FindByID(ctx context.Context, id string) (*InBodyRecord, error)
    Update(ctx context.Context, id string, record *InBodyRecord) error
    Delete(ctx context.Context, id string) error
}
```

Add methods to `ScanService` interface:
```go
type ScanService interface {
    ProcessScan(ctx context.Context, userID string, imageData []byte, imageURL string) (*InBodyRecord, error)
    
    // New methods
    GetAllScans(ctx context.Context, userID string) ([]*InBodyRecord, error)
    GetScanByID(ctx context.Context, userID string, scanID string) (*InBodyRecord, error)
    UpdateScan(ctx context.Context, userID string, scanID string, updates map[string]interface{}) (*InBodyRecord, error)
    DeleteScan(ctx context.Context, userID string, scanID string) error
}
```

---

### Phase 2: Repository Layer Implementation

#### File: `internal/repository/mongo_inbody.go`

Implement new repository methods:

```go
func (r *MongoInBodyRepository) FindAllByUserID(ctx context.Context, userID string) ([]*domain.InBodyRecord, error) {
    // Query with user_id filter
    // Sort by test_date_time descending
    // Return array of records
}

func (r *MongoInBodyRepository) FindByID(ctx context.Context, id string) (*domain.InBodyRecord, error) {
    // Convert id to ObjectID
    // Find by _id
    // Return single record or error if not found
}

func (r *MongoInBodyRepository) Update(ctx context.Context, id string, record *domain.InBodyRecord) error {
    // Convert id to ObjectID
    // Use UpdateOne with $set operator
    // Return error if not found
}

func (r *MongoInBodyRepository) Delete(ctx context.Context, id string) error {
    // Convert id to ObjectID
    // Use DeleteOne
    // Return error if not found
}
```

---

### Phase 3: Service Layer Implementation

#### File: `internal/service/scan.go`

Implement new service methods with ownership verification:

```go
func (s *ScanServiceImpl) GetAllScans(ctx context.Context, userID string) ([]*domain.InBodyRecord, error) {
    // Call repository.FindAllByUserID
    // Return results (already filtered by user)
}

func (s *ScanServiceImpl) GetScanByID(ctx context.Context, userID string, scanID string) (*domain.InBodyRecord, error) {
    // Call repository.FindByID
    // Verify record.UserID == userID (ownership check)
    // Return 403 error if mismatch
}

func (s *ScanServiceImpl) UpdateScan(ctx context.Context, userID string, scanID string, updates map[string]interface{}) (*domain.InBodyRecord, error) {
    // Fetch existing record
    // Verify ownership
    // Apply updates to allowed fields (weight, smm, body_fat_mass, pbf, bmi, bmr)
    // Call repository.Update
    // Return updated record
}

func (s *ScanServiceImpl) DeleteScan(ctx context.Context, userID string, scanID string) error {
    // Fetch existing record
    // Verify ownership
    // Extract image URL from metadata
    // Delete from SeaweedFS (if fileRepository available)
    // Delete from MongoDB
    // Return success
}
```

**Note**: For DeleteScan, we'll need to add a Delete method to FileRepository interface.

---

### Phase 4: Handler Layer Implementation

#### File: `internal/handler/scan_handler.go`

Add new handler methods:

```go
func (h *ScanHandler) ListScans(c *fiber.Ctx) error {
    // Extract userID from locals (set by AuthMiddleware)
    // Call service.GetAllScans
    // Return JSON array
}

func (h *ScanHandler) GetScan(c *fiber.Ctx) error {
    // Extract userID from locals
    // Extract scanID from params
    // Call service.GetScanByID
    // Handle 403/404 errors
    // Return JSON object
}

func (h *ScanHandler) UpdateScan(c *fiber.Ctx) error {
    // Extract userID from locals
    // Extract scanID from params
    // Parse request body (updates map)
    // Call service.UpdateScan
    // Handle 403/404 errors
    // Return updated JSON object
}

func (h *ScanHandler) DeleteScan(c *fiber.Ctx) error {
    // Extract userID from locals
    // Extract scanID from params
    // Call service.DeleteScan
    // Handle 403/404 errors
    // Return success message
}
```

---

### Phase 5: Routing Updates

#### File: `cmd/main.go`

Wire up new routes under `/v1/scans`:

```go
scans := v1.Group("/scans")
scans.Use(authMiddleware.Authenticate) // All routes require auth

scans.Post("/digitize", scanHandler.DigitizeScan)  // Existing
scans.Get("/", scanHandler.ListScans)               // New
scans.Get("/:id", scanHandler.GetScan)              // New
scans.Patch("/:id", scanHandler.UpdateScan)         // New
scans.Delete("/:id", scanHandler.DeleteScan)        // New
```

---

## Additional Considerations

### FileRepository Delete Method

Add to `internal/domain/file.go`:
```go
type FileRepository interface {
    Upload(ctx context.Context, file []byte, filename string, contentType string) (string, error)
    Delete(ctx context.Context, fileURL string) error  // New method
}
```

Implement in `internal/repository/seaweed_s3.go`:
```go
func (r *SeaweedS3Repository) Delete(ctx context.Context, fileURL string) error {
    // Parse URL to extract bucket and key
    // Call s3.DeleteObject
    // Return error if failed
}
```

### Error Handling

Create custom errors in `internal/domain/errors.go`:
```go
var (
    ErrNotFound = errors.New("record not found")
    ErrForbidden = errors.New("access forbidden")
)
```

### Testing Strategy

1. Unit tests for repository methods
2. Integration tests for service layer (ownership logic)
3. E2E tests for handlers with valid/invalid tokens
4. Test SeaweedFS cleanup on delete

---

## Summary

This implementation follows Clean Architecture principles:
- **Domain**: Interfaces define contracts
- **Repository**: Data access implementation
- **Service**: Business logic with ownership verification
- **Handler**: HTTP-specific logic
- **Routes**: Wire everything together

All endpoints are protected by Firebase Auth middleware, ensuring only authenticated users can access their own data.
