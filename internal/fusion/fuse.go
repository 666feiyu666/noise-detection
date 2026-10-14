package fusion

import (
	"sort"

	"github.com/666feiyu666/noise-detection/internal/report"
)

// FusionToleranceSec is how far apart two findings may be and still be treated
// as describing the same event.
const FusionToleranceSec = 0.5

// Fuse combines a primary detector's findings (for example DSP candidates) with
// a secondary detector's findings (for example Gemini's perceptual assessment).
//
//	A primary finding overlapping a secondary one -> confirmed ("both")
//	A primary finding with no secondary support   -> unconfirmed (primary source)
//	A secondary finding with no primary support   -> suspected (secondary source)
//
// The result is sorted by start time. Fuse never invents, drops, or moves an
// interval; it only labels evidence and merges the two lists.
func Fuse(primary, secondary []report.Finding, primarySource, secondarySource string) []report.Finding {
	used := make([]bool, len(secondary))
	out := make([]report.Finding, 0, len(primary)+len(secondary))

	for _, p := range primary {
		match := -1
		for i, s := range secondary {
			if used[i] || !overlaps(p, s) {
				continue
			}
			match = i
			break
		}
		if match >= 0 {
			used[match] = true
			p.Source = report.SourceBoth
			p.Status = report.StatusConfirmed
		} else {
			p.Source = primarySource
			p.Status = report.StatusUnconfirmed
		}
		out = append(out, p)
	}

	for i, s := range secondary {
		if used[i] {
			continue
		}
		s.Source = secondarySource
		s.Status = report.StatusSuspected
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].StartSeconds < out[j].StartSeconds })
	return out
}

// overlaps reports whether two findings describe the same event, within
// FusionToleranceSec.
func overlaps(a, b report.Finding) bool {
	return a.StartSeconds <= b.EndSeconds+FusionToleranceSec &&
		b.StartSeconds <= a.EndSeconds+FusionToleranceSec
}
