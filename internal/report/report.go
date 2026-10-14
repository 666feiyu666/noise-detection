// Package report defines the input, output, and decision vocabulary shared
// by every stage of the audio-defect assessment pipeline, and the structured
// report returned to the caller.
//
// The types here mirror the external contract of the workflow: a caller
// supplies an Input, the pipeline produces a Report, and application code
// (never a model) computes the Decision.
package report

import "time"

// Assessment status values reported back to the caller.
const (
	StatusCompleted = "completed"
	StatusError     = "error"
)

// Decision values. These are computed in application code.
const (
	DecisionAccept           = "accept"
	DecisionRegenerate       = "regenerate"
	DecisionStopLimitReached = "stop_limit_reached"
	DecisionError            = "error"
)

// Generation limits. A creation request permits at most three generations.
const (
	MinGenerationAttempt = 1
	MaxGenerationAttempt = 3
)

// Input is the caller-supplied assessment request.
//
// This framework does not read or decode audio yet. AudioPath is validated for
// existence and format when file checking is enabled; DurationSeconds may be
// supplied as metadata so downstream stages can validate timestamps without
// touching the file contents.
type Input struct {
	RequestID          string
	GenerationAttempt  int
	AudioPath          string
	IntentionalEffects string
	ReportPath         string
	// DurationSeconds is optional metadata. When nil, timestamp bounds are not
	// checked against a duration.
	DurationSeconds *float64
}

// Source labels. Each analyzer tags its evidence with the family it comes from;
// an aggregated finding lists every source that observed the same event.
const (
	SourceDSP    = "dsp"
	SourceGemini = "gemini"
)

// Status values for aggregated findings, derived from the sources that
// observed an event.
const (
	StatusConfirmed   = "confirmed"   // two or more sources agree
	StatusUnconfirmed = "unconfirmed" // a single source observed the event
)

// Severity and confidence values for perceptual (timbre) findings. Severity and
// confidence are distinct: severity describes how audible or impactful the
// problem is, confidence describes how sure the assessment is. Neither is a
// calibrated probability.
const (
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"

	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// SignalObservation is a numeric signal measurement attached to an assessment
// as supporting evidence, for example a spectral or onset-envelope statistic.
//
// An observation is context, not a verdict: it is never, by itself, grounds
// for rejection, and it is deliberately kept out of the defect list so it
// cannot trigger a decision on its own.
type SignalObservation struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
	Note  string  `json:"note,omitempty"`
}

// Finding is one located, audible defect. Times are seconds from the start of
// the original file and are estimates, not sample-accurate measurements.
//
// Sources and Status are set when findings from several analyzers are
// aggregated: Sources lists every source (for example "dsp", "gemini") that
// observed the same event, and Status is "confirmed" when two or more sources
// agree or "unconfirmed" when only one did. They are empty for a single
// analyzer's raw output.
type Finding struct {
	StartSeconds float64  `json:"start_seconds"`
	EndSeconds   float64  `json:"end_seconds"`
	Type         string   `json:"type"`
	Description  string   `json:"description"`
	Sources      []string `json:"sources,omitempty"`
	Status       string   `json:"status,omitempty"`
	// Dimension names the perceptual quality the finding concerns (for
	// example "muffling", "loss_of_transients", "harshness"). It is set for
	// timbre findings and left empty for purely signal-level defects.
	Dimension string `json:"dimension,omitempty"`
	// Severity and Confidence describe how audible the problem is and how
	// certain the assessment is. They are distinct from one another.
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// Assessment is what an analyzer returns before the evidence from every
// analyzer is aggregated, the response is validated, and a decision is
// computed.
type Assessment struct {
	Findings      []Finding
	Model         string
	PromptVersion string
	// SignalObservations carries supporting numeric measurements (for example
	// spectral balance and onset-envelope statistics). Observations inform a
	// human or model reviewer but never trigger a decision by themselves.
	SignalObservations []SignalObservation
	// DurationSeconds is optional metadata an analyzer may report (for example,
	// the model's estimate of the recording length). When the caller did not
	// supply a duration, the pipeline uses this value for timestamp validation
	// and the report.
	DurationSeconds *float64
}

// Report is the machine-readable assessment result returned to the caller.
//
// HasDefect and ShouldRegenerate are pointers so that a null value can be
// emitted for error reports, distinguishing "unknown" from "false".
type Report struct {
	RequestID                string    `json:"request_id"`
	GenerationAttempt        int       `json:"generation_attempt"`
	AssessmentStatus         string    `json:"assessment_status"`
	HasDefect                *bool     `json:"has_defect"`
	Defects                  []Finding `json:"defects"`
	TimestampsAreApproximate bool      `json:"timestamps_are_approximate"`
	// SignalObservations are supporting numeric measurements (spectral
	// balance, onset-envelope statistics). They are evidence, not verdicts:
	// they never appear in Defects and never drive the decision on their own.
	SignalObservations []SignalObservation `json:"signal_observations,omitempty"`
	ShouldRegenerate   *bool               `json:"should_regenerate"`
	Decision           string              `json:"decision"`
	Reason             string              `json:"reason"`

	// Metadata describing the assessment.
	AudioFilename   string    `json:"audio_filename,omitempty"`
	DurationSeconds *float64  `json:"duration_seconds,omitempty"`
	Model           string    `json:"model,omitempty"`
	PromptVersion   string    `json:"prompt_version,omitempty"`
	AssessedAt      time.Time `json:"assessed_at"`
}

// Bool returns a pointer to v, for populating optional report fields.
func Bool(v bool) *bool { return &v }
