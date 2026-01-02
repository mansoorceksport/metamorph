package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
	"golang.org/x/sync/errgroup"
)

// DashboardService handles analytics aggregation for the Coach Command Center
type DashboardService struct {
	contractRepo domain.PTContractRepository
	schedRepo    domain.ScheduleRepository
	inbodyRepo   domain.InBodyRepository
	sessionRepo  domain.WorkoutSessionRepository
	userRepo     domain.UserRepository
}

// NewDashboardService creates a new DashboardService instance
func NewDashboardService(
	contractRepo domain.PTContractRepository,
	schedRepo domain.ScheduleRepository,
	inbodyRepo domain.InBodyRepository,
	sessionRepo domain.WorkoutSessionRepository,
	userRepo domain.UserRepository,
) *DashboardService {
	return &DashboardService{
		contractRepo: contractRepo,
		schedRepo:    schedRepo,
		inbodyRepo:   inbodyRepo,
		sessionRepo:  sessionRepo,
		userRepo:     userRepo,
	}
}

// GetCoachSummary retrieves aggregated analytics for a coach's Command Center
func (s *DashboardService) GetCoachSummary(ctx context.Context, coachID string) (*domain.DashboardSummary, error) {
	// 1. Get coach's assigned members from contracts
	contracts, err := s.contractRepo.GetActiveByCoach(ctx, coachID)
	if err != nil {
		return nil, fmt.Errorf("failed to get contracts: %w", err)
	}

	// Extract unique member IDs
	memberMap := make(map[string]bool)
	var memberIDs []string
	for _, contract := range contracts {
		if !memberMap[contract.MemberID] {
			memberMap[contract.MemberID] = true
			memberIDs = append(memberIDs, contract.MemberID)
		}
	}

	// Get user details for names
	users := make(map[string]*domain.User)
	for _, memberID := range memberIDs {
		user, err := s.userRepo.GetByID(ctx, memberID)
		if err == nil {
			users[memberID] = user
		}
	}

	summary := &domain.DashboardSummary{
		RisingStars:   []domain.MemberAnalytics{},
		ChurnRisk:     []domain.MemberAnalytics{},
		StrengthWins:  []domain.MemberAnalytics{},
		PackageHealth: []domain.MemberAnalytics{},
		Consistent:    []domain.MemberAnalytics{},
	}

	// Use errgroup for concurrent fetching
	g, gCtx := errgroup.WithContext(ctx)

	// Rising Stars (Recomposition Delta)
	g.Go(func() error {
		risingStars, err := s.calculateRisingStars(gCtx, memberIDs, users)
		if err != nil {
			return err
		}
		summary.RisingStars = risingStars
		return nil
	})

	// Churn Risk (Frequency Drift)
	g.Go(func() error {
		churnRisk, err := s.calculateChurnRisk(gCtx, coachID, memberIDs, users)
		if err != nil {
			return err
		}
		summary.ChurnRisk = churnRisk
		return nil
	})

	// Strength Wins (PR Detection)
	g.Go(func() error {
		strengthWins, err := s.calculateStrengthWins(gCtx, coachID, users)
		if err != nil {
			return err
		}
		summary.StrengthWins = strengthWins
		return nil
	})

	// Package Health (Low Sessions)
	g.Go(func() error {
		packageHealth, err := s.calculatePackageHealth(gCtx, coachID, users)
		if err != nil {
			return err
		}
		summary.PackageHealth = packageHealth
		return nil
	})

	// Consistent (100% Attendance)
	g.Go(func() error {
		consistent, err := s.calculateConsistent(gCtx, coachID, memberIDs, users)
		if err != nil {
			return err
		}
		summary.Consistent = consistent
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return summary, nil
}

// calculateRisingStars finds members with the best muscle/fat recomposition delta
func (s *DashboardService) calculateRisingStars(ctx context.Context, memberIDs []string, users map[string]*domain.User) ([]domain.MemberAnalytics, error) {
	if len(memberIDs) == 0 {
		return []domain.MemberAnalytics{}, nil
	}

	// Get 2 most recent scans per member
	scansMap, err := s.inbodyRepo.GetRecentScansByMembers(ctx, memberIDs, 2)
	if err != nil {
		return nil, err
	}

	type memberDelta struct {
		memberID    string
		muscleDelta float64
		fatDelta    float64
		totalScore  float64
	}

	var deltas []memberDelta
	for memberID, scans := range scansMap {
		// Must have at least 2 scans for comparison
		if len(scans) < 2 {
			continue
		}

		latest := scans[0]
		previous := scans[1]

		// Calculate delta: SMM gain + PBF loss
		muscleGain := latest.SMM - previous.SMM
		fatLoss := previous.PBF - latest.PBF

		// Only include positive recomposition
		if muscleGain > 0 || fatLoss > 0 {
			deltas = append(deltas, memberDelta{
				memberID:    memberID,
				muscleDelta: muscleGain,
				fatDelta:    fatLoss,
				totalScore:  muscleGain + fatLoss, // Simple weighted score
			})
		}
	}

	// Sort by totalScore descending
	sort.Slice(deltas, func(i, j int) bool {
		return deltas[i].totalScore > deltas[j].totalScore
	})

	// Take top 5
	result := make([]domain.MemberAnalytics, 0, 5)
	for i := 0; i < len(deltas) && i < 5; i++ {
		d := deltas[i]
		name := d.memberID
		if user, ok := users[d.memberID]; ok {
			name = user.Name
		}

		label := fmt.Sprintf("+%.1fkg Muscle", d.muscleDelta)
		if d.fatDelta > 0 {
			label += fmt.Sprintf(", -%.1f%% Fat", d.fatDelta)
		}

		result = append(result, domain.MemberAnalytics{
			MemberID: d.memberID,
			Name:     name,
			Value:    d.totalScore,
			Label:    label,
			Trend:    "rising",
		})
	}

	return result, nil
}

// calculateChurnRisk finds members with declining attendance
func (s *DashboardService) calculateChurnRisk(ctx context.Context, coachID string, memberIDs []string, users map[string]*domain.User) ([]domain.MemberAnalytics, error) {
	// Get attendance for last 30 days
	schedules, err := s.schedRepo.GetAttendanceByCoach(ctx, coachID, 30)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	sevenDaysAgo := now.AddDate(0, 0, -7)

	// Track attendance per member
	type memberAttendance struct {
		last30Days int
		last7Days  int
	}
	attendance := make(map[string]*memberAttendance)

	for _, sched := range schedules {
		if sched.Status == domain.ScheduleStatusCompleted {
			if attendance[sched.MemberID] == nil {
				attendance[sched.MemberID] = &memberAttendance{}
			}
			attendance[sched.MemberID].last30Days++
			if sched.StartTime.After(sevenDaysAgo) {
				attendance[sched.MemberID].last7Days++
			}
		}
	}

	var result []domain.MemberAnalytics
	for memberID, att := range attendance {
		if att.last30Days == 0 {
			continue
		}

		// Calculate weekly average over 30 days vs last 7 days
		avgWeekly := float64(att.last30Days) / 4.0 // ~4 weeks in 30 days
		lastWeek := float64(att.last7Days)

		if avgWeekly > 0 && lastWeek < avgWeekly*0.75 { // 25% drop
			dropPercent := (1 - lastWeek/avgWeekly) * 100

			name := memberID
			if user, ok := users[memberID]; ok {
				name = user.Name
			}

			result = append(result, domain.MemberAnalytics{
				MemberID: memberID,
				Name:     name,
				Value:    dropPercent,
				Label:    fmt.Sprintf("-%.0f%% vs Avg", dropPercent),
				Trend:    "declining",
			})
		}
	}

	// Sort by drop percentage descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Value > result[j].Value
	})

	// Limit to top 5
	if len(result) > 5 {
		result = result[:5]
	}

	return result, nil
}

// calculateStrengthWins detects personal records from session logs
func (s *DashboardService) calculateStrengthWins(ctx context.Context, coachID string, users map[string]*domain.User) ([]domain.MemberAnalytics, error) {
	// Get sessions from last 7 days
	now := time.Now()
	sevenDaysAgo := now.AddDate(0, 0, -7)
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	recentSessions, err := s.sessionRepo.GetSessionsByCoachAndDateRange(ctx, coachID, sevenDaysAgo, now)
	if err != nil {
		return nil, err
	}

	// Get historical sessions for comparison (last 30 days before the recent period)
	historicalSessions, err := s.sessionRepo.GetSessionsByCoachAndDateRange(ctx, coachID, thirtyDaysAgo, sevenDaysAgo)
	if err != nil {
		return nil, err
	}

	// Build historical max weights per member/exercise
	type exerciseKey struct {
		memberID   string
		exerciseID string
	}
	historicalMax := make(map[exerciseKey]float64)

	for _, session := range historicalSessions {
		for _, ex := range session.PlannedExercises {
			key := exerciseKey{memberID: session.MemberID, exerciseID: ex.ExerciseID}
			for _, set := range ex.Sets {
				if set.Completed && set.Weight > historicalMax[key] {
					historicalMax[key] = set.Weight
				}
			}
		}
	}

	// Find PRs in recent sessions
	type prRecord struct {
		memberID    string
		exercise    string
		weight      float64
		improvement float64
	}
	var prs []prRecord

	for _, session := range recentSessions {
		for _, ex := range session.PlannedExercises {
			key := exerciseKey{memberID: session.MemberID, exerciseID: ex.ExerciseID}
			for _, set := range ex.Sets {
				if set.Completed && set.Weight > historicalMax[key] {
					improvement := set.Weight - historicalMax[key]
					prs = append(prs, prRecord{
						memberID:    session.MemberID,
						exercise:    ex.Name,
						weight:      set.Weight,
						improvement: improvement,
					})
					// Update max to avoid duplicate PRs
					historicalMax[key] = set.Weight
				}
			}
		}
	}

	// Sort by most recent (we'll just take the ones with best improvement)
	sort.Slice(prs, func(i, j int) bool {
		return prs[i].improvement > prs[j].improvement
	})

	result := make([]domain.MemberAnalytics, 0, 5)
	for i := 0; i < len(prs) && i < 5; i++ {
		pr := prs[i]
		name := pr.memberID
		if user, ok := users[pr.memberID]; ok {
			name = user.Name
		}

		result = append(result, domain.MemberAnalytics{
			MemberID: pr.memberID,
			Name:     name,
			Value:    pr.weight,
			Label:    fmt.Sprintf("%.1fkg %s (+%.1fkg PR)", pr.weight, pr.exercise, pr.improvement),
			Trend:    "rising",
		})
	}

	return result, nil
}

// calculatePackageHealth finds contracts with low remaining sessions
func (s *DashboardService) calculatePackageHealth(ctx context.Context, coachID string, users map[string]*domain.User) ([]domain.MemberAnalytics, error) {
	// Get contracts with < 3 remaining sessions
	contracts, err := s.contractRepo.GetLowSessionsByCoach(ctx, coachID, 3)
	if err != nil {
		return nil, err
	}

	result := make([]domain.MemberAnalytics, 0, len(contracts))
	for _, contract := range contracts {
		name := contract.MemberID
		if user, ok := users[contract.MemberID]; ok {
			name = user.Name
		}

		label := fmt.Sprintf("%d sessions left", contract.RemainingSessions)
		trend := "declining"
		if contract.RemainingSessions <= 1 {
			label = "Last session!"
		}

		result = append(result, domain.MemberAnalytics{
			MemberID: contract.MemberID,
			Name:     name,
			Value:    float64(contract.RemainingSessions),
			Label:    label,
			Trend:    trend,
		})
	}

	// Sort by remaining sessions ascending (lowest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Value < result[j].Value
	})

	// Limit to 5
	if len(result) > 5 {
		result = result[:5]
	}

	return result, nil
}

// calculateConsistent finds members with 100% attendance over last 30 days
func (s *DashboardService) calculateConsistent(ctx context.Context, coachID string, memberIDs []string, users map[string]*domain.User) ([]domain.MemberAnalytics, error) {
	// Get attendance for last 30 days
	schedules, err := s.schedRepo.GetAttendanceByCoach(ctx, coachID, 30)
	if err != nil {
		return nil, err
	}

	// Track attendance per member
	type attendance struct {
		scheduled int
		completed int
	}
	att := make(map[string]*attendance)

	for _, sched := range schedules {
		if att[sched.MemberID] == nil {
			att[sched.MemberID] = &attendance{}
		}
		att[sched.MemberID].scheduled++
		if sched.Status == domain.ScheduleStatusCompleted {
			att[sched.MemberID].completed++
		}
	}

	var result []domain.MemberAnalytics
	for memberID, a := range att {
		// Must have at least 1 scheduled session
		if a.scheduled == 0 {
			continue
		}

		rate := float64(a.completed) / float64(a.scheduled) * 100
		if rate >= 100 { // 100% attendance
			name := memberID
			if user, ok := users[memberID]; ok {
				name = user.Name
			}

			result = append(result, domain.MemberAnalytics{
				MemberID: memberID,
				Name:     name,
				Value:    rate,
				Label:    fmt.Sprintf("%d/%d sessions", a.completed, a.scheduled),
				Trend:    "rising",
			})
		}
	}

	// Sort by number of sessions completed (more sessions = more consistent)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Value > result[j].Value
	})

	// Limit to 5
	if len(result) > 5 {
		result = result[:5]
	}

	return result, nil
}
