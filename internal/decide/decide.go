// Package decide computes the assessment decision in application code, never
// in a model.
package decide

import "github.com/666feiyu666/noise-detection/internal/contract"

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
			Decision:         contract.DecisionAccept,
			ShouldRegenerate: contract.Bool(false),
			Reason:           "No audible defect was detected.",
		}
	}
	if attempt < maxAttempts {
		return Outcome{
			Decision:         contract.DecisionRegenerate,
			ShouldRegenerate: contract.Bool(true),
			Reason:           "An audible defect was detected; another attempt is allowed.",
		}
	}
	return Outcome{
		Decision:         contract.DecisionStopLimitReached,
		ShouldRegenerate: contract.Bool(false),
		Reason:           "An audible defect was detected and the generation limit has been reached.",
	}
}
