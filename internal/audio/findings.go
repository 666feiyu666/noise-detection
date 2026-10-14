package audio

import (
	"fmt"

	"github.com/666feiyu666/noise-detection/internal/report"
)

// Findings validates detector findings against the timestamp contract.
//
// Every interval must satisfy 0 <= start < end, and end must not exceed the
// known duration when one is supplied. Inconsistent or incomplete findings are
// rejected rather than silently treated as success.
func Findings(findings []report.Finding, duration *float64) error {
	for i, f := range findings {
		if f.Type == "" {
			return fmt.Errorf("finding %d: type is required", i)
		}
		if f.Description == "" {
			return fmt.Errorf("finding %d: description is required", i)
		}
		if f.StartSeconds < 0 {
			return fmt.Errorf("finding %d: start_seconds must be >= 0, got %v", i, f.StartSeconds)
		}
		if f.EndSeconds <= f.StartSeconds {
			return fmt.Errorf("finding %d: require start_seconds < end_seconds, got %v >= %v",
				i, f.StartSeconds, f.EndSeconds)
		}
		if duration != nil && f.EndSeconds > *duration {
			return fmt.Errorf("finding %d: end_seconds %v exceeds duration %v",
				i, f.EndSeconds, *duration)
		}
	}
	return nil
}
