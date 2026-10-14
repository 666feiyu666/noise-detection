package evaluation

import "github.com/666feiyu666/noise-detection/internal/report"

// Outcome is the decision computed for a completed assessment.
type Outcome struct {
	Decision         string
	ShouldRegenerate *bool
	Reason           string
}

// Decide applies the decision table:
//
//	No defects                       -> accept,             should_regenerate=false
//	Defects, attempt 1 or 2          -> regenerate,         should_regenerate=true
//	Defects, attempt >= maxAttempts  -> stop_limit_reached, should_regenerate=false
func Decide(hasDefect bool, attempt, maxAttempts int) Outcome {
	if !hasDefect {
		return Outcome{
			Decision:         report.DecisionAccept,
			ShouldRegenerate: report.Bool(false),
			Reason:           "No audible defect was detected.",
		}
	}
	if attempt < maxAttempts {
		return Outcome{
			Decision:         report.DecisionRegenerate,
			ShouldRegenerate: report.Bool(true),
			Reason:           "An audible defect was detected; another attempt is allowed.",
		}
	}
	return Outcome{
		Decision:         report.DecisionStopLimitReached,
		ShouldRegenerate: report.Bool(false),
		Reason:           "An audible defect was detected and the generation limit has been reached.",
	}
}
