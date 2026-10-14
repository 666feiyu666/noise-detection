// Package evaluation is the evaluation layer of the pipeline (CRISP-DM:
// Evaluation). It turns the evidence produced by every model into one result:
// it aggregates findings and observations, labels each finding with the sources
// that observed it, validates the response, and computes the decision in
// application code.
package evaluation

import (
	"sort"

	"github.com/666feiyu666/noise-detection/internal/report"
)

// EvidenceToleranceSec is how far apart two findings may be and still be
// treated as describing the same event.
const EvidenceToleranceSec = 0.5

// Evidence is the findings one model observed for a candidate.
type Evidence struct {
	// Source is the model's evidence family (for example report.SourceDSP).
	Source string
	// Findings are the events that model observed, unlabelled.
	Findings []report.Finding
}

// Aggregate merges findings from several sources into one event list.
//
// Overlapping findings from different sources are combined into a single
// event: the first source's finding supplies the event's type and description,
// and every source that observed it is recorded in Sources. Status is then
// "confirmed" when two or more sources observed an event and "unconfirmed"
// when only one did. Findings from the same source are never merged with each
// other. The result is sorted by start time.
func Aggregate(evidence []Evidence) []report.Finding {
	out := make([]report.Finding, 0)
	for _, e := range evidence {
		for _, f := range e.Findings {
			if i := matchEvent(out, f, e.Source); i >= 0 {
				out[i].Sources = addSource(out[i].Sources, e.Source)
				continue
			}
			f.Sources = addSource(nil, e.Source)
			out = append(out, f)
		}
	}
	for i := range out {
		if len(out[i].Sources) >= 2 {
			out[i].Status = report.StatusConfirmed
		} else {
			out[i].Status = report.StatusUnconfirmed
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartSeconds < out[j].StartSeconds })
	return out
}

// matchEvent returns the index of the first event that overlaps f and has not
// already been observed by source, or -1 when there is none.
func matchEvent(events []report.Finding, f report.Finding, source string) int {
	for i := range events {
		if containsSource(events[i].Sources, source) {
			continue
		}
		if overlaps(events[i], f) {
			return i
		}
	}
	return -1
}

// overlaps reports whether two findings describe the same event, within
// EvidenceToleranceSec.
func overlaps(a, b report.Finding) bool {
	return a.StartSeconds <= b.EndSeconds+EvidenceToleranceSec &&
		b.StartSeconds <= a.EndSeconds+EvidenceToleranceSec
}

// addSource appends source to sources unless it is empty or already present.
func addSource(sources []string, source string) []string {
	if source == "" {
		return sources
	}
	if containsSource(sources, source) {
		return sources
	}
	return append(sources, source)
}

func containsSource(sources []string, source string) bool {
	for _, s := range sources {
		if s == source {
			return true
		}
	}
	return false
}
