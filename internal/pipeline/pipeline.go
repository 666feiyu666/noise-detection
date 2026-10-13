// Package pipeline wires the assessment stages together:
//
//	Validate input -> Assess -> Validate response -> Decide -> Report
//
// Validation or assessment failures produce an error report rather than a
// clean result.
package pipeline

import (
	"context"
	"path/filepath"
	"time"

	"github.com/666feiyu666/noise-detection/internal/contract"
	"github.com/666feiyu666/noise-detection/internal/decide"
	"github.com/666feiyu666/noise-detection/internal/detect"
	"github.com/666feiyu666/noise-detection/internal/validate"
)

// Config configures a Pipeline.
type Config struct {
	// Detector is required.
	Detector detect.Detector
	// MaxAttempts caps generation attempts; defaults to contract.MaxGenerationAttempt.
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
		cfg.MaxAttempts = contract.MaxGenerationAttempt
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Pipeline{cfg: cfg}
}

// Run executes one assessment and always returns a Report. A non-nil error
// accompanies reports whose decision is an error.
func (p *Pipeline) Run(ctx context.Context, in contract.Input) (contract.Report, error) {
	in = p.normalize(in)

	if err := validate.Input(in, validate.InputOptions{RequireAudioFile: p.cfg.RequireAudioFile}); err != nil {
		return p.errorReport(in, err), err
	}

	assessment, err := p.cfg.Detector.Detect(ctx, in)
	if err != nil {
		return p.errorReport(in, err), err
	}

	if err := validate.Findings(assessment.Findings, in.DurationSeconds); err != nil {
		return p.errorReport(in, err), err
	}

	hasDefect := len(assessment.Findings) > 0
	outcome := decide.Decide(hasDefect, in.GenerationAttempt, p.cfg.MaxAttempts)

	return contract.Report{
		RequestID:                in.RequestID,
		GenerationAttempt:        in.GenerationAttempt,
		AssessmentStatus:         contract.StatusCompleted,
		HasDefect:                contract.Bool(hasDefect),
		Defects:                  nonNil(assessment.Findings),
		TimestampsAreApproximate: true,
		ShouldRegenerate:         outcome.ShouldRegenerate,
		Decision:                 outcome.Decision,
		Reason:                   outcome.Reason,
		AudioFilename:            baseOrEmpty(in.AudioPath),
		DurationSeconds:          in.DurationSeconds,
		Model:                    assessment.Model,
		PromptVersion:            assessment.PromptVersion,
		AssessedAt:               p.cfg.Now(),
	}, nil
}

// normalize applies input defaults before validation.
func (p *Pipeline) normalize(in contract.Input) contract.Input {
	if in.GenerationAttempt == 0 {
		in.GenerationAttempt = contract.MinGenerationAttempt
	}
	return in
}

// errorReport builds the report emitted for any failed stage.
func (p *Pipeline) errorReport(in contract.Input, err error) contract.Report {
	return contract.Report{
		RequestID:                in.RequestID,
		GenerationAttempt:        in.GenerationAttempt,
		AssessmentStatus:         contract.StatusError,
		HasDefect:                nil,
		Defects:                  []contract.Finding{},
		TimestampsAreApproximate: true,
		ShouldRegenerate:         nil,
		Decision:                 contract.DecisionError,
		Reason:                   err.Error(),
		AudioFilename:            baseOrEmpty(in.AudioPath),
		DurationSeconds:          in.DurationSeconds,
		AssessedAt:               p.cfg.Now(),
	}
}

func nonNil(f []contract.Finding) []contract.Finding {
	if f == nil {
		return []contract.Finding{}
	}
	return f
}

func baseOrEmpty(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
