package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

type PTService struct {
	pkgRepo      domain.PTPackageRepository
	contractRepo domain.PTContractRepository
	schedRepo    domain.ScheduleRepository
}

func NewPTService(
	pkgRepo domain.PTPackageRepository,
	contractRepo domain.PTContractRepository,
	schedRepo domain.ScheduleRepository,
) *PTService {
	return &PTService{
		pkgRepo:      pkgRepo,
		contractRepo: contractRepo,
		schedRepo:    schedRepo,
	}
}

// --- Package (Template) Management ---

func (s *PTService) CreatePackageTemplate(ctx context.Context, pkg *domain.PTPackage) error {
	// Validate Sessions Tier
	validTiers := map[int]bool{10: true, 20: true, 30: true, 40: true, 50: true}
	if !validTiers[pkg.TotalSessions] {
		return domain.ErrInvalidSessionAmount
	}

	pkg.Active = true
	return s.pkgRepo.Create(ctx, pkg)
}

func (s *PTService) GetPackageTemplatesByTenant(ctx context.Context, tenantID string) ([]*domain.PTPackage, error) {
	return s.pkgRepo.GetByTenant(ctx, tenantID)
}

func (s *PTService) GetPackageTemplate(ctx context.Context, id string) (*domain.PTPackage, error) {
	return s.pkgRepo.GetByID(ctx, id)
}

func (s *PTService) UpdatePackageTemplate(ctx context.Context, pkg *domain.PTPackage) error {
	// Optional: basic validation if fields present
	if pkg.TotalSessions > 0 {
		validTiers := map[int]bool{10: true, 20: true, 30: true, 40: true, 50: true}
		if !validTiers[pkg.TotalSessions] {
			return domain.ErrInvalidSessionAmount
		}
	}
	return s.pkgRepo.Update(ctx, pkg)
}

// --- Contract Management (Purchasing/Assigning) ---

func (s *PTService) CreateContract(ctx context.Context, contractReq *domain.PTContract) error {
	// 1. Fetch Template
	template, err := s.pkgRepo.GetByID(ctx, contractReq.PackageID)
	if err != nil {
		return err
	}
	if !template.Active {
		return errors.New("cannot create contract from inactive package template")
	}

	// 2. Validate Branch Consistency (Optional strict mode: Template Branch == Member Branch == Coach Branch)
	// For now, simpler check: Template Branch must match assigned Branch if template has one.
	if template.BranchID != "" && template.BranchID != contractReq.BranchID {
		return domain.ErrBranchMismatch
	}

	// 3. Hydrate Contract from Template
	contractReq.TotalSessions = template.TotalSessions
	contractReq.RemainingSessions = template.TotalSessions
	contractReq.Price = template.Price
	contractReq.Status = domain.PackageStatusActive

	return s.contractRepo.Create(ctx, contractReq)
}

func (s *PTService) GetContractsByTenant(ctx context.Context, tenantID string) ([]*domain.PTContract, error) {
	return s.contractRepo.GetByTenant(ctx, tenantID)
}

func (s *PTService) GetActiveContractsByMember(ctx context.Context, memberID string) ([]*domain.PTContract, error) {
	return s.contractRepo.GetActiveByMember(ctx, memberID)
}

func (s *PTService) GetActiveContractsByCoach(ctx context.Context, coachID string) ([]*domain.PTContract, error) {
	return s.contractRepo.GetActiveByCoach(ctx, coachID)
}

func (s *PTService) GetContract(ctx context.Context, id string) (*domain.PTContract, error) {
	return s.contractRepo.GetByID(ctx, id)
}

// --- Scheduling ---

func (s *PTService) CreateSchedule(ctx context.Context, schedule *domain.Schedule) error {
	// 1. Verify Contract exists and has remaining sessions
	contract, err := s.contractRepo.GetByID(ctx, schedule.ContractID)
	if err != nil {
		return err
	}

	if contract.Status != domain.PackageStatusActive || contract.RemainingSessions <= 0 {
		return domain.ErrPackageDepleted
	}

	// 2. Overbooking Protection: Check pending schedules
	// RemainingSessions tracks *uncompleted* sessions.
	// We must ensure we don't schedule more sessions than remaining.
	scheduledCount, err := s.schedRepo.CountByContractAndStatus(ctx, contract.ID, []string{domain.ScheduleStatusScheduled, domain.ScheduleStatusPendingConfirmation})
	if err != nil {
		return fmt.Errorf("failed to check existing schedules: %w", err)
	}

	if int(scheduledCount) >= contract.RemainingSessions {
		return errors.New("cannot schedule session: package limit reached (pending sessions use all remaining credits)")
	}

	if contract.MemberID != schedule.MemberID {
		return errors.New("contract does not belong to this member")
	}

	// Ensure schedule branch matches contract branch?
	// Usually yes.
	if contract.BranchID != schedule.BranchID {
		return domain.ErrBranchMismatch
	}

	// 2. Set defaults
	schedule.Status = domain.ScheduleStatusScheduled

	// 3. Create
	return s.schedRepo.Create(ctx, schedule)
}

func (s *PTService) RescheduleSession(ctx context.Context, scheduleID string, newStart, newEnd time.Time, actorRole string, actorID string) error {
	schedule, err := s.schedRepo.GetByID(ctx, scheduleID)
	if err != nil {
		return err
	}

	// Authorization Check
	if actorRole == "coach" && schedule.CoachID != actorID {
		return domain.ErrUnauthorizedReschedule
	}
	if actorRole == "member" && schedule.MemberID != actorID {
		return domain.ErrUnauthorizedReschedule
	}

	schedule.StartTime = newStart
	schedule.EndTime = newEnd

	if actorRole == "coach" {
		schedule.Status = domain.ScheduleStatusScheduled
	} else if actorRole == "member" {
		schedule.Status = domain.ScheduleStatusPendingConfirmation
	} else {
		schedule.Status = domain.ScheduleStatusScheduled
	}

	return s.schedRepo.Update(ctx, schedule)
}

func (s *PTService) CompleteSession(ctx context.Context, scheduleID string, coachID string) error {
	schedule, err := s.schedRepo.GetByID(ctx, scheduleID)
	if err != nil {
		return err
	}

	if schedule.CoachID != coachID {
		return domain.ErrForbidden
	}

	if schedule.Status == domain.ScheduleStatusCompleted {
		return errors.New("session already completed")
	}

	// 1. Mark Schedule as Completed
	if err := s.schedRepo.UpdateStatus(ctx, scheduleID, domain.ScheduleStatusCompleted); err != nil {
		return fmt.Errorf("failed to complete schedule: %w", err)
	}

	// 2. Atomically Decrement Contract
	if err := s.contractRepo.DecrementSession(ctx, schedule.ContractID); err != nil {
		return fmt.Errorf("session completed but failed to decrement contract: %w", err)
	}

	return nil
}

func (s *PTService) GetSchedules(ctx context.Context, role, userID string, from, to time.Time) ([]*domain.Schedule, error) {
	if role == "coach" {
		return s.schedRepo.GetByCoach(ctx, userID, from, to)
	} else if role == "member" {
		return s.schedRepo.GetByMember(ctx, userID, from, to)
	}
	return nil, errors.New("invalid role for schedule listing")
}

func (s *PTService) ListSchedules(ctx context.Context, tenantID string, filter map[string]interface{}) ([]*domain.Schedule, error) {
	return s.schedRepo.List(ctx, tenantID, filter)
}

func (s *PTService) GetSchedule(ctx context.Context, id string) (*domain.Schedule, error) {
	return s.schedRepo.GetByID(ctx, id)
}
