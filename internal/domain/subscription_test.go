package domain

import (
	"testing"
	"time"
)

func TestCalculateNewEndDate(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name           string
		currentEnd     *time.Time
		durationMonths int
		expectAfter    time.Time // result should be after this time
		expectBefore   time.Time // result should be before this time
	}{
		{
			name:           "nil currentEnd - starts from now",
			currentEnd:     nil,
			durationMonths: 12,
			expectAfter:    now.AddDate(0, 12, -1), // allow 1 day tolerance
			expectBefore:   now.AddDate(0, 12, 1),
		},
		{
			name:           "past currentEnd - starts from now",
			currentEnd:     timePtr(now.AddDate(0, -1, 0)), // 1 month ago
			durationMonths: 12,
			expectAfter:    now.AddDate(0, 12, -1),
			expectBefore:   now.AddDate(0, 12, 1),
		},
		{
			name:           "future currentEnd - stacks from currentEnd",
			currentEnd:     timePtr(now.AddDate(0, 3, 0)), // 3 months in future
			durationMonths: 12,
			expectAfter:    now.AddDate(0, 15, -1), // 3 + 12 months
			expectBefore:   now.AddDate(0, 15, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateNewEndDate(tt.currentEnd, tt.durationMonths)

			if result.Before(tt.expectAfter) {
				t.Errorf("CalculateNewEndDate() = %v, want after %v", result, tt.expectAfter)
			}
			if result.After(tt.expectBefore) {
				t.Errorf("CalculateNewEndDate() = %v, want before %v", result, tt.expectBefore)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
