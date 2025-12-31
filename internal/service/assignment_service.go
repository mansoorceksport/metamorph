package service

import (
	"context"
	"fmt"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

// AssignmentService handles logic for assigning members to coaches
type AssignmentService struct {
	assignmentRepo domain.AssignmentRepository
	userRepo       domain.UserRepository
}

func NewAssignmentService(
	assignmentRepo domain.AssignmentRepository,
	userRepo domain.UserRepository,
) *AssignmentService {
	return &AssignmentService{
		assignmentRepo: assignmentRepo,
		userRepo:       userRepo,
	}
}

// AssignMemberToCoach securely assigns a member to a coach
// Enforces Tenant Isolation and Branch Specificity
func (s *AssignmentService) AssignMemberToCoach(ctx context.Context, coachID, memberID string) (*domain.CoachAssignment, error) {
	// 1. Fetch Coach
	coach, err := s.userRepo.GetByID(ctx, coachID)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, fmt.Errorf("coach not found")
		}
		return nil, err
	}

	// Verify user is actually a coach
	if !coach.HasRole(domain.RoleCoach) {
		return nil, fmt.Errorf("user %s is not a coach", coachID)
	}

	// 2. Fetch Member
	member, err := s.userRepo.GetByID(ctx, memberID)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, fmt.Errorf("member not found")
		}
		return nil, err
	}

	// 3. Strict Tenant Isolation
	if coach.TenantID != member.TenantID {
		return nil, fmt.Errorf("cross-tenant assignment forbidden: coach and member belong to different tenants")
	}

	// 4. Strict Branch Specificity
	// If coach has a HomeBranchID, member MUST have access to it
	if coach.HomeBranchID != "" {
		hasAccess := false
		for _, branchID := range member.BranchAccess {
			if branchID == coach.HomeBranchID {
				hasAccess = true
				break
			}
		}

		if !hasAccess {
			return nil, fmt.Errorf("member must join the coach's branch (ID: %s) before assignment", coach.HomeBranchID)
		}
	}

	// 5. Create Assignment
	assignment := &domain.CoachAssignment{
		CoachID:  coachID,
		MemberID: memberID,
		TenantID: coach.TenantID,
		// In a real system you might store BranchID too if assignments are branch-specific
	}

	if err := s.assignmentRepo.Create(ctx, assignment); err != nil {
		return nil, fmt.Errorf("failed to create assignment: %w", err)
	}

	return assignment, nil
}
