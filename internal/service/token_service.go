package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mansoorceksport/metamorph/internal/config"
	"github.com/mansoorceksport/metamorph/internal/domain"
)

// TokenService handles JWT access/refresh token generation and validation
type TokenService struct {
	jwtConfig        config.JWTConfig
	refreshTokenRepo domain.RefreshTokenRepository
	userRepo         domain.UserRepository
}

// NewTokenService creates a new token service
func NewTokenService(
	jwtConfig config.JWTConfig,
	refreshTokenRepo domain.RefreshTokenRepository,
	userRepo domain.UserRepository,
) *TokenService {
	return &TokenService{
		jwtConfig:        jwtConfig,
		refreshTokenRepo: refreshTokenRepo,
		userRepo:         userRepo,
	}
}

// TokenPair contains both access and refresh tokens
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // Seconds until access token expires
}

// GenerateTokenPair creates both access and refresh tokens for a user
func (s *TokenService) GenerateTokenPair(ctx context.Context, user *domain.User, userAgent, ipAddress string) (*TokenPair, error) {
	// Generate access token (short-lived JWT)
	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token (random string, stored in DB)
	refreshToken, err := s.generateAndStoreRefreshToken(ctx, user.ID, userAgent, ipAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.jwtConfig.AccessTokenExpiry.Seconds()),
	}, nil
}

// RefreshAccessToken validates refresh token and returns new token pair
func (s *TokenService) RefreshAccessToken(ctx context.Context, refreshToken, userAgent, ipAddress string) (*TokenPair, error) {
	// Hash the provided refresh token
	tokenHash := hashToken(refreshToken)

	// Find the refresh token in DB
	storedToken, err := s.refreshTokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("failed to find refresh token: %w", err)
	}
	if storedToken == nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	// Validate token
	if !storedToken.IsValid() {
		return nil, fmt.Errorf("refresh token expired or revoked")
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Revoke old refresh token (token rotation for security)
	if err := s.refreshTokenRepo.RevokeByHash(ctx, tokenHash); err != nil {
		return nil, fmt.Errorf("failed to revoke old token: %w", err)
	}

	// Generate new token pair
	return s.GenerateTokenPair(ctx, user, userAgent, ipAddress)
}

// RevokeRefreshToken invalidates a specific refresh token (logout)
func (s *TokenService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	tokenHash := hashToken(refreshToken)
	return s.refreshTokenRepo.RevokeByHash(ctx, tokenHash)
}

// RevokeAllUserTokens invalidates all refresh tokens for a user (force logout)
func (s *TokenService) RevokeAllUserTokens(ctx context.Context, userID string) error {
	return s.refreshTokenRepo.RevokeAllByUserID(ctx, userID)
}

// generateAccessToken creates a short-lived JWT access token
func (s *TokenService) generateAccessToken(user *domain.User) (string, error) {
	claims := domain.MetamorphClaims{
		UserID:       user.ID,
		Roles:        user.Roles,
		TenantID:     user.TenantID,
		HomeBranchID: user.HomeBranchID,
		BranchAccess: user.BranchAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.jwtConfig.AccessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtConfig.Secret))
}

// generateAndStoreRefreshToken creates a random refresh token and stores its hash
func (s *TokenService) generateAndStoreRefreshToken(ctx context.Context, userID, userAgent, ipAddress string) (string, error) {
	// Generate random 32-byte token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	rawToken := hex.EncodeToString(tokenBytes)

	// Store hash in database (never store raw token)
	tokenHash := hashToken(rawToken)
	refreshToken := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.jwtConfig.RefreshTokenExpiry),
		UserAgent: userAgent,
		IPAddress: ipAddress,
	}

	if err := s.refreshTokenRepo.Create(ctx, refreshToken); err != nil {
		return "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	return rawToken, nil
}

// hashToken creates a SHA256 hash of the token
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
