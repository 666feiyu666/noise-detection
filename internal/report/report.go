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

// Source labels for fused findings.
const (
	SourceDSP    = "dsp"
	SourceGemini = "gemini"
	SourceBoth   = "both"
)

// Status values for fused findings.
const (
	StatusConfirmed   = "confirmed"   // DSP and Gemini agree
	StatusUnconfirmed = "unconfirmed" // DSP candidate with no Gemini support
	StatusSuspected   = "suspected"   // Gemini-only perceptual defect
)

// Finding is one located, audible defect. Times are seconds from the start of
// the original file and are estimates, not sample-accurate measurements.
//
// Source and Status are set when findings are fused: source is "dsp", "gemini",
// or "both"; status is "confirmed", "unconfirmed", or "suspected". They are
// empty for a single detector's raw output.
type Finding struct {
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Type         string  `json:"type"`
	Description  string  `json:"description"`
	Source       string  `json:"source,omitempty"`
	Status       string  `json:"status,omitempty"`
}

// Assessment is what a Detector returns before the response is validated and a
// decision is computed.
type Assessment struct {
	Findings      []Finding
	Model         string
	PromptVersion string
	// DurationSeconds is optional metadata a Detector may report (for example,
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
	ShouldRegenerate         *bool     `json:"should_regenerate"`
	Decision                 string    `json:"decision"`
	Reason                   string    `json:"reason"`

	// Metadata describing the assessment.
	AudioFilename   string    `json:"audio_filename,omitempty"`
	DurationSeconds *float64  `json:"duration_seconds,omitempty"`
	Model           string    `json:"model,omitempty"`
	PromptVersion   string    `json:"prompt_version,omitempty"`
	AssessedAt      time.Time `json:"assessed_at"`
}

// Bool returns a pointer to v, for populating optional report fields.
func Bool(v bool) *bool { return &v }
