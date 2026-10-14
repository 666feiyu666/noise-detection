package fusion

import (
	"testing"

	"github.com/666feiyu666/noise-detection/internal/report"
)

func TestDecide(t *testing.T) {
	cases := []struct {
		name         string
		hasDefect    bool
		attempt      int
		maxAttempts  int
		wantDecision string
		wantReGen    bool
	}{
		{"clean", false, 1, 3, report.DecisionAccept, false},
		{"defect at attempt 1", true, 1, 3, report.DecisionRegenerate, true},
		{"defect at attempt 2", true, 2, 3, report.DecisionRegenerate, true},
		{"defect at attempt 3", true, 3, 3, report.DecisionStopLimitReached, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.hasDefect, tc.attempt, tc.maxAttempts)
			if got.Decision != tc.wantDecision {
				t.Fatalf("decision = %q, want %q", got.Decision, tc.wantDecision)
			}
			if got.ShouldRegenerate == nil {
				t.Fatalf("should_regenerate is nil")
			}
			if *got.ShouldRegenerate != tc.wantReGen {
				t.Fatalf("should_regenerate = %v, want %v", *got.ShouldRegenerate, tc.wantReGen)
			}
			if got.Reason == "" {
				t.Fatalf("reason is empty")
			}
		})
	}
}
