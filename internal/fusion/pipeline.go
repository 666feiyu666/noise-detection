// Package fusion wires the assessment stages together and computes the final
// decision:
//
//	Validate input -> Assess -> (Fuse) -> Validate response -> Decide -> Report
//
// A single detector produces a plain assessment. When a Verifier is configured,
// its findings are fused with the primary detector's to form a hybrid
// assessment. Validation, assessment, or response-validation failures produce
// an error report rather than a clean result.
package fusion

import (
	"context"
	"path/filepath"
	"time"

	"github.com/666feiyu666/noise-detection/internal/audio"
	"github.com/666feiyu666/noise-detection/internal/detector"
	"github.com/666feiyu666/noise-detection/internal/report"
)

// Config configures a Pipeline.
type Config struct {
	// Detector is required.
	Detector detector.Detector
	// Verifier is optional. When set, its findings are fused with Detector's,
	// forming a hybrid assessment (for example DSP candidates verified by
	// Gemini). Detector is treated as the primary source.
	Verifier detector.Detector
	// MaxAttempts caps generation attempts; defaults to report.MaxGenerationAttempt.
	MaxAttempts int
	// RequireAudioFile enforces that AudioPath points to an existing file.
	RequireAudioFile bool
	// Now supplies the assessment time; defaults to time.Now.
	Now func() time.Time
}

// Pipeline runs the assessment flow.
type Pipeline struct {
	cfg Config
}

// New creates a Pipeline, applying defaults.
func New(cfg Config) *Pipeline {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = report.MaxGenerationAttempt
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Pipeline{cfg: cfg}
}

// Run executes one assessment and always returns a Report. A non-nil error
// accompanies reports whose decision is an error.
func (p *Pipeline) Run(ctx context.Context, in report.Input) (report.Report, error) {
	in = p.normalize(in)

	if err := audio.Input(in, audio.InputOptions{RequireAudioFile: p.cfg.RequireAudioFile}); err != nil {
		return p.errorReport(in, err), err
	}

	assessment, err := p.cfg.Detector.Detect(ctx, in)
	if err != nil {
		return p.errorReport(in, err), err
	}

	findings := assessment.Findings
	model := assessment.Model
	promptVersion := assessment.PromptVersion
	duration := assessment.DurationSeconds
	observations := assessment.SignalObservations

	// Hybrid mode: fuse the verifier's findings with the primary detector's.
	if p.cfg.Verifier != nil {
		second, err := p.cfg.Verifier.Detect(ctx, in)
		if err != nil {
			return p.errorReport(in, err), err
		}
		if duration == nil {
			duration = second.DurationSeconds
		}
		findings = Fuse(findings, second.Findings, sourceOf(p.cfg.Detector), sourceOf(p.cfg.Verifier))
		model = join(model, second.Model)
		promptVersion = join(promptVersion, second.PromptVersion)
		observations = append(observations, second.SignalObservations...)
	}

	// A detector may report the duration it observed; use it only when the
	// caller supplied none, so caller metadata always wins.
	if in.DurationSeconds == nil {
		in.DurationSeconds = duration
	}

	if err := audio.Findings(findings, in.DurationSeconds); err != nil {
		return p.errorReport(in, err), err
	}

	hasDefect := len(findings) > 0
	outcome := Decide(hasDefect, in.GenerationAttempt, p.cfg.MaxAttempts)

	return report.Report{
		RequestID:                in.RequestID,
		GenerationAttempt:        in.GenerationAttempt,
		AssessmentStatus:         report.StatusCompleted,
		HasDefect:                report.Bool(hasDefect),
		Defects:                  nonNil(findings),
		TimestampsAreApproximate: true,
		SignalObservations:       observations,
		ShouldRegenerate:         outcome.ShouldRegenerate,
		Decision:                 outcome.Decision,
		Reason:                   outcome.Reason,
		AudioFilename:            baseOrEmpty(in.AudioPath),
		DurationSeconds:          in.DurationSeconds,
		Model:                    model,
		PromptVersion:            promptVersion,
		AssessedAt:               p.cfg.Now(),
	}, nil
}

// normalize applies input defaults before validation.
func (p *Pipeline) normalize(in report.Input) report.Input {
	if in.GenerationAttempt == 0 {
		in.GenerationAttempt = report.MinGenerationAttempt
	}
	return in
}

// errorReport builds the report emitted for any failed stage.
func (p *Pipeline) errorReport(in report.Input, err error) report.Report {
	return report.Report{
		RequestID:                in.RequestID,
		GenerationAttempt:        in.GenerationAttempt,
		AssessmentStatus:         report.StatusError,
		HasDefect:                nil,
		Defects:                  []report.Finding{},
		TimestampsAreApproximate: true,
		ShouldRegenerate:         nil,
		Decision:                 report.DecisionError,
		Reason:                   err.Error(),
		AudioFilename:            baseOrEmpty(in.AudioPath),
		DurationSeconds:          in.DurationSeconds,
		AssessedAt:               p.cfg.Now(),
	}
}

// sourceOf returns a detector's evidence label.
func sourceOf(d detector.Detector) string {
	if s, ok := d.(detector.Sourced); ok {
		return s.Source()
	}
	return d.Name()
}

func join(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out == "" {
			out = p
		} else {
			out += "+" + p
		}
	}
	return out
}

func nonNil(f []report.Finding) []report.Finding {
	if f == nil {
		return []report.Finding{}
	}
	return f
}

func baseOrEmpty(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
