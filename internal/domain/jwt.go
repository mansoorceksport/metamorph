package domain

import (
	"github.com/golang-jwt/jwt/v5"
)

// MetamorphClaims represents custom JWT claims for Metamorph auth
type MetamorphClaims struct {
	UserID       string   `json:"user_id"`
	Roles        []string `json:"roles"`
	TenantID     string   `json:"tenant_id"`
	HomeBranchID string   `json:"home_branch_id,omitempty"`
	BranchAccess []string `json:"branch_access,omitempty"`
	jwt.RegisteredClaims
}
