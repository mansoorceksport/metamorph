package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/mansoorceksport/metamorph/internal/config"
	"github.com/mansoorceksport/metamorph/internal/server"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Define request/response structs inline or reuse from domain/dto if available.
// For E2E, inline or map[string]interface{} is often easier to decoupling from internal changes.
// But using internal structs ensures type safety. relying on map for flexibility.

func TestGoldenPath(t *testing.T) {
	// 1. Setup Infrastructure
	// MongoDB (Container)
	db, cleanupDB := SetupTestDB(t)
	defer cleanupDB()

	// Redis (Miniredis for speed/simplicity, or Container)
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Mock Auth
	mockAuth := NewMockAuthClient()

	// Config (Minimal)
	cfg := &config.Config{}
	cfg.Server.MaxUploadSizeMB = 10
	cfg.JWT.Secret = "test-secret-key-123"
	// ... other config defaults ...

	// 2. Initialize App
	app := server.NewApp(server.AppDependencies{
		Config:      cfg,
		MongoDB:     db,
		RedisClient: redisClient,
		AuthClient:  mockAuth,
	})

	// Helper for requests
	request := func(method, path, token string, body interface{}) *http.Response {
		var bodyReader io.Reader
		if body != nil {
			jsonBytes, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(jsonBytes)
		}
		req, _ := http.NewRequest(method, path, bodyReader)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := app.Test(req, -1) // -1 disables timeout (or use large value)
		require.NoError(t, err)
		return resp
	}

	// ==========================================
	// STEP 1: Super Admin Login & Setup
	// ==========================================
	// We need to bypass login endpoint slightly because LoginOrRegister verifies Firebase Token AND then checks DB.
	// Since DB is empty, we must "register" the Super Admin first?
	// The system usually bootstraps a super admin or we allow first user?
	// Actually, LoginOrRegister CREATES the user if not exists.
	// So we pass a mock token for Super Admin.
	// WAIT: Super Admin role is guarded. Normal registration gives "member".
	// WE NEED to seed a Super Admin in the DB manually for the test to start.

	// Seed Super Admin
	_, err = db.Collection("users").InsertOne(context.Background(), map[string]interface{}{
		"email":        "super@admin.com",
		"firebase_uid": "uid_super_admin", // Matches logic in LoginOrRegister possibly
		"roles":        []string{"super_admin"},
		"name":         "Super Admin",
	})
	require.NoError(t, err)

	// Add to Mock Auth
	mockAuth.AddMockUser("token_super_admin", "uid_super_admin", "super@admin.com")

	// Super Admin Login
	// LoginOrRegister expects Firebase Token in Authorization Header
	resp := request("POST", "/v1/auth/login", "token_super_admin", nil)

	assert.Equal(t, 200, resp.StatusCode)

	var loginData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginData)
	superToken := loginData["token"].(string)
	require.NotEmpty(t, superToken)

	fmt.Println("✓ Super Admin Logged In")

	// ==========================================
	// STEP 2: Create Tenant
	// ==========================================
	resp = request("POST", "/v1/platform/tenants", superToken, map[string]string{
		"name":      "Golden Gym",
		"slug":      "golden-gym",
		"join_code": "GOLDEN123",
	})
	assert.Equal(t, 201, resp.StatusCode)

	var tenantData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tenantData)
	tenantID := tenantData["id"].(string)
	require.NotEmpty(t, tenantID)

	fmt.Println("✓ Tenant Created:", tenantID)

	// ==========================================
	// STEP 3: Create Branch
	// ==========================================
	resp = request("POST", "/v1/platform/branches", superToken, map[string]interface{}{
		"name":      "Downtown Branch",
		"tenant_id": tenantID,
		"location":  "Downtown",
	})
	assert.Equal(t, 201, resp.StatusCode)

	var branchData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&branchData)
	branchID := branchData["id"].(string)
	require.NotEmpty(t, branchID)

	fmt.Println("✓ Branch Created:", branchID)

	// ==========================================
	// STEP 4: Create Tenant Admin
	// ==========================================
	// This user doesn't exist yet, we create them via Platform endpoint
	resp = request("POST", "/v1/platform/tenant-admins", superToken, map[string]interface{}{
		"email":     "admin@goldengym.com",
		"name":      "Gym Owner",
		"tenant_id": tenantID,
	})
	assert.Equal(t, 201, resp.StatusCode)

	// Now this user needs to "Login".
	// In real life, they'd sign up with Firebase. E2E: We mock their firebase token.
	// But since we created them in DB with an email, we need to link them.
	// LoginOrRegister will find by EMAIL and update Firebase UID.
	mockAuth.AddMockUser("token_tenant_admin", "uid_tenant_admin", "admin@goldengym.com")

	// ==========================================
	// STEP 5: Tenant Admin Login
	// ==========================================
	resp = request("POST", "/v1/auth/login", "token_tenant_admin", nil)
	assert.Equal(t, 200, resp.StatusCode)

	json.NewDecoder(resp.Body).Decode(&loginData)
	adminToken := loginData["token"].(string)
	require.NotEmpty(t, adminToken)

	fmt.Println("✓ Tenant Admin Logged In")

	// ==========================================
	// STEP 6: Create Coach
	// ==========================================
	resp = request("POST", "/v1/tenant-admin/coaches", adminToken, map[string]interface{}{
		"email":          "coach@goldengym.com",
		"name":           "Coach Mike",
		"home_branch_id": branchID,
	})
	assert.Equal(t, 201, resp.StatusCode)

	var coachData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&coachData)
	coachID := coachData["id"].(string)

	fmt.Println("✓ Coach Created")

	// ==========================================
	// STEP 7: Create Member
	// ==========================================
	resp = request("POST", "/v1/tenant-admin/users", adminToken, map[string]interface{}{
		"email": "member@goldengym.com",
		"name":  "John Member",
	})
	assert.Equal(t, 201, resp.StatusCode)

	var memberData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&memberData)
	memberID := memberData["id"].(string)

	fmt.Println("✓ Member Created")

	// ==========================================
	// STEP 8: Create Package Template (New Flow)
	// ==========================================
	resp = request("POST", "/v1/tenant-admin/packages", adminToken, map[string]interface{}{
		"name":           "10 Pack Promo",
		"total_sessions": 10,
		"price":          2000000,
		"branch_id":      branchID,
	})
	assert.Equal(t, 201, resp.StatusCode)

	var pkgData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&pkgData)
	pkgID := pkgData["id"].(string)
	fmt.Println("✓ Package Template Created:", pkgID)

	// ==========================================
	// STEP 9: Create Contract (Assign Package & Coach)
	// ==========================================
	resp = request("POST", "/v1/tenant-admin/contracts", adminToken, map[string]interface{}{
		"package_id": pkgID,
		"member_id":  memberID,
		"coach_id":   coachID,
		"branch_id":  branchID,
	})
	assert.Equal(t, 201, resp.StatusCode)

	var contractData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&contractData)
	contractID := contractData["id"].(string)

	fmt.Println("✓ Contract Created (Coach Assigned):", contractID)

	// ==========================================
	// STEP 9: Verify Data Persistence (The Bug Fix)
	// ==========================================
	// Coach logs in for the first time.
	// 1. Mock Coach Auth
	mockAuth.AddMockUser("token_coach", "uid_coach", "coach@goldengym.com")

	// 2. Login
	resp = request("POST", "/v1/auth/login", "token_coach", nil)
	assert.Equal(t, 200, resp.StatusCode)

	json.NewDecoder(resp.Body).Decode(&loginData)

	loggedInUser := loginData["user"].(map[string]interface{})
	loggedInID := loggedInUser["id"].(string)

	// CRITICAL CHECK: Ensure we logged into the SAME coach account we created earlier
	assert.Equal(t, coachID, loggedInID, "Logged in Coach ID must match previously created Coach ID. Link-by-email might have failed.")

	// 3. Check DB directly (or via API) to ensure home_branch_id is still set
	// Let's verify via API if possible, or direct DB check is robust for E2E

	var coachDoc map[string]interface{}
	err = db.Collection("users").FindOne(context.Background(), map[string]interface{}{
		"email": "coach@goldengym.com",
	}).Decode(&coachDoc)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, "uid_coach", coachDoc["firebase_uid"]) // Linked successfully
	assert.Equal(t, branchID, coachDoc["home_branch_id"])  // CRITICAL: NOT DELETED

	fmt.Println("✓ Data Persistence Verified: home_branch_id preserved intact!")

	coachToken := loginData["token"].(string)

	// ==========================================
	// STEP 10: Verify Coach sees Client (via Contract)
	// ==========================================
	resp = request("GET", "/v1/pro/clients", coachToken, nil)
	assert.Equal(t, 200, resp.StatusCode)

	var clients []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&clients)

	found := false
	for _, c := range clients {
		if c["id"] == memberID {
			found = true
			break
		}
	}
	assert.True(t, found, "Member should appear in Coach's client list due to active contract")
	fmt.Println("✓ Coach Client List Verified")

	// ==========================================
	// STEP 11: Create Schedule
	// ==========================================
	resp = request("POST", "/v1/pro/schedules", coachToken, map[string]interface{}{
		"contract_id": contractID,
		"member_id":   memberID,
		"start_time":  "2025-01-01T10:00:00Z",
		"end_time":    "2025-01-01T11:00:00Z",
		"remarks":     "First Session",
	})
	// Expect success
	assert.Equal(t, 201, resp.StatusCode)
	var scheduleData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&scheduleData)
	scheduleID := scheduleData["id"].(string)
	fmt.Println("✓ Schedule Created:", scheduleID)

	// ==========================================
	// STEP 11a: Create Master Data (Exercises & Templates)
	// ==========================================
	// Use Super Admin Token
	resp = request("POST", "/v1/exercises", superToken, map[string]interface{}{
		"name":         "Burpee",
		"muscle_group": "Full Body",
		"equipment":    "None",
		"video_url":    "https://youtube.com/burpee",
	})
	assert.Equal(t, 201, resp.StatusCode)
	var exerciseData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&exerciseData)
	exID := exerciseData["id"].(string)

	resp = request("POST", "/v1/templates", superToken, map[string]interface{}{
		"name":         "Power HIIT",
		"gender":       "All",
		"exercise_ids": []string{exID},
	})
	assert.Equal(t, 201, resp.StatusCode)
	var templateData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&templateData)
	tplID := templateData["id"].(string)

	fmt.Println("✓ Master Data Created")

	// ==========================================
	// STEP 11b: Workout Session Flow
	// ==========================================
	// 1. Initialize Session
	resp = request("POST", "/v1/pro/sessions/initialize", coachToken, map[string]string{
		"schedule_id": scheduleID,
		"template_id": tplID,
	})
	assert.Equal(t, 201, resp.StatusCode)
	var sessionData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sessionData)
	sessionID := sessionData["id"].(string)

	// Verify Deep Copy
	planned := sessionData["planned_exercises"].([]interface{})
	assert.Len(t, planned, 1) // One exercise from template

	// 2. Add Exercise Dynamic (Add "Burpee" again or another if we had one. Let's add same one)
	resp = request("PATCH", "/v1/pro/sessions/"+sessionID+"/exercises", coachToken, map[string]interface{}{
		"action":      "add",
		"exercise_id": exID,
	})
	assert.Equal(t, 200, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&sessionData)
	planned = sessionData["planned_exercises"].([]interface{})
	assert.Len(t, planned, 2) // Added one

	// 3. Log a Set
	resp = request("PATCH", "/v1/pro/sessions/"+sessionID+"/log", coachToken, map[string]interface{}{
		"exercise_index": 0,
		"set_index":      0,
		"weight":         50,
		"reps":           15,
		"remarks":        "Good form",
	})
	assert.Equal(t, 200, resp.StatusCode)

	fmt.Println("✓ Workout Session Flow Verified")

	// ==========================================
	// STEP 12: Complete Schedule
	// ==========================================
	resp = request("POST", "/v1/pro/schedules/"+scheduleID+"/complete", coachToken, nil)
	assert.Equal(t, 200, resp.StatusCode)
	fmt.Println("✓ Schedule Completed")

	// Verify Contract Decrement
	var contractDoc map[string]interface{}
	objID, _ := primitive.ObjectIDFromHex(contractID)
	err = db.Collection("pt_contracts").FindOne(context.Background(), map[string]interface{}{
		"_id": objID,
	}).Decode(&contractDoc)
	require.NoError(t, err)

	// Initial was 10. Should be 9.
	// Note: mongo-driver decodes numbers as int32 or float64 depending on setup.
	// But let's check remaining_sessions.
	remSessions := contractDoc["remaining_sessions"]
	// Use assertions robust to types
	assert.EqualValues(t, 9, remSessions, "Contract should have deduced 1 session")

	fmt.Println("✓ Contract Decremented")
}
