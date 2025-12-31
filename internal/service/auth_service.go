package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mansoorceksport/metamorph/internal/domain"
)

// FirebaseAuthClient defines the interface for Firebase Auth operations
// This allows mocking for tests
type FirebaseAuthClient interface {
	VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error)
}

// AuthService handles authentication and user registration
type AuthService struct {
	userRepo   domain.UserRepository
	tenantRepo domain.TenantRepository
	authClient FirebaseAuthClient
	jwtSecret  string
}

// NewAuthService creates a new auth service
func NewAuthService(
	userRepo domain.UserRepository,
	tenantRepo domain.TenantRepository,
	authClient FirebaseAuthClient,
	jwtSecret string,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		authClient: authClient,
		jwtSecret:  jwtSecret,
	}
}

// LoginOrRegisterRequest contains the request params
type LoginOrRegisterRequest struct {
	FirebaseToken string
}

// LoginOrRegisterResponse contains the user and whether they were newly created
type LoginOrRegisterResponse struct {
	User      *domain.User
	Token     string
	IsNewUser bool
	RoleAdded bool
	TenantID  string
}

// LoginOrRegister handles smart authentication and registration
func (s *AuthService) LoginOrRegister(ctx context.Context, req LoginOrRegisterRequest) (*LoginOrRegisterResponse, error) {
	// Step 1: Verify Firebase token and extract user info
	token, err := s.authClient.VerifyIDToken(ctx, req.FirebaseToken)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Get user info from token
	firebaseUID := token.UID
	email := token.Claims["email"].(string)
	name, _ := token.Claims["name"].(string)

	// Default name if not provided
	if name == "" {
		name = email
	}

	// Step 2: Search for existing user by firebase_uid
	existingUser, err := s.userRepo.GetByFirebaseUID(ctx, firebaseUID)

	// Step 3: If not found by firebase_uid, try email (for pre-provisioned accounts)
	if err != nil && err == domain.ErrNotFound {
		emailUser, emailErr := s.userRepo.GetByEmail(ctx, email)
		if emailErr == nil && emailUser != nil {
			// Found pre-provisioned user by email - link firebase_uid
			if emailUser.FirebaseUID == "" {
				// Link the Firebase account to this user
				if updateErr := s.userRepo.UpdateFirebaseUID(ctx, emailUser.ID, firebaseUID); updateErr != nil {
					return nil, fmt.Errorf("failed to link firebase account: %w", updateErr)
				}
				emailUser.FirebaseUID = firebaseUID
				// Use this user for subsequent logic
				existingUser = emailUser
				err = nil
			} else {
				// Email exists but already linked to different firebase_uid
				return nil, fmt.Errorf("email already linked to different account")
			}
		}
	}

	// Step 4: Login existing user
	if err == nil && existingUser != nil {
		// Generate JWT token using stored Roles and TenantID
		token, err := s.GenerateMetamorphToken(existingUser)
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}

		return &LoginOrRegisterResponse{
			User:      existingUser,
			Token:     token,
			IsNewUser: false,
			RoleAdded: false,
			TenantID:  existingUser.TenantID,
		}, nil
	}

	// Step 5: New user - create with DEFAULT role (Member)
	// WE DO NOT ALLOW REQUESTED ROLE HERE anymore for security.
	// Only admins can create coaches/admins via specialized endpoints.
	if err == domain.ErrNotFound {
		newUser := &domain.User{
			FirebaseUID: firebaseUID,
			Email:       email,
			Name:        name,
			Roles:       []string{domain.RoleMember},
			// TenantID is empty for generic members until they join a gym/tenant
		}

		// Create the user
		if err := s.userRepo.Create(ctx, newUser); err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}

		// Generate JWT token
		token, err := s.GenerateMetamorphToken(newUser)
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}

		return &LoginOrRegisterResponse{
			User:      newUser,
			Token:     token,
			IsNewUser: true,
			RoleAdded: false,
			TenantID:  newUser.TenantID,
		}, nil
	}

	// Other error occurred
	return nil, fmt.Errorf("failed to fetch user: %w", err)
}

// createTenantForCoach creates a tenant for a coach
func (s *AuthService) createTenantForCoach(ctx context.Context, user *domain.User) (string, error) {
	// Generate a Join Code (simple random string for now)
	joinCode, err := generateJoinCode()
	if err != nil {
		return "", fmt.Errorf("failed to generate join code: %w", err)
	}

	// Create a default tenant for the coach
	tenant := &domain.Tenant{
		Name:     fmt.Sprintf("%s's Gym", user.Name),
		JoinCode: joinCode,
		LogoURL:  "",
		AISettings: domain.AISettings{
			Tone:    "Encouraging",
			Style:   "Detailed",
			Persona: "Supportive Head Coach",
		},
	}

	if err := s.tenantRepo.Create(ctx, tenant); err != nil {
		return "", err
	}

	// Set tenant_id on the user object (caller will save it)
	user.TenantID = tenant.ID

	return tenant.ID, nil
}

// generateJoinCode creates a random 6-character code
func generateJoinCode() (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return "HOM-" + string(b), nil
}

// GenerateMetamorphToken creates a JWT token with custom claims
func (s *AuthService) GenerateMetamorphToken(user *domain.User) (string, error) {
	// Create claims with user data
	claims := domain.MetamorphClaims{
		UserID:       user.ID,
		Roles:        user.Roles, // Multi-role support - includes all roles
		TenantID:     user.TenantID,
		HomeBranchID: user.HomeBranchID,
		BranchAccess: user.BranchAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with secret
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}
