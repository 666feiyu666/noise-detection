package evaluation

import (
	"context"
	"path/filepath"
	"time"

	"github.com/666feiyu666/noise-detection/internal/audio"
	"github.com/666feiyu666/noise-detection/internal/features"
	"github.com/666feiyu666/noise-detection/internal/model"
	"github.com/666feiyu666/noise-detection/internal/report"
)

// Config configures a Pipeline.
type Config struct {
	// Models is the ordered set of detection methods to run. Every model
	// receives the same prepared features and their evidence is aggregated. At
	// least one is required.
	Models []model.Model
	// Source prepares the features. Defaults to features.DefaultSource.
	Source features.AudioSource
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

	// Data preparation: decode once and extract the features every model shares.
	prepared, err := features.Extract(ctx, p.cfg.Source, in.AudioPath)
	if err != nil {
		return p.errorReport(in, err), err
	}

	duration := in.DurationSeconds
	if duration == nil && prepared.DurationSeconds > 0 {
		d := prepared.DurationSeconds
		duration = &d
	}

	// Modeling: run every selected model over the prepared features.
	evidence := make([]Evidence, 0, len(p.cfg.Models))
	var models, promptVersions []string
	var modelObservations []report.SignalObservation
	for _, m := range p.cfg.Models {
		assessment, err := m.Predict(ctx, in, prepared)
		if err != nil {
			return p.errorReport(in, err), err
		}
		evidence = append(evidence, Evidence{Source: m.Source(), Findings: assessment.Findings})
		models = append(models, assessment.Model)
		promptVersions = append(promptVersions, assessment.PromptVersion)
		modelObservations = append(modelObservations, assessment.SignalObservations...)
		if duration == nil {
			duration = assessment.DurationSeconds
		}
	}

	// Evaluation: aggregate the evidence, then validate and decide.
	findings := Aggregate(evidence)

	if in.DurationSeconds == nil {
		in.DurationSeconds = duration
	}
	if err := audio.Findings(findings, in.DurationSeconds); err != nil {
		return p.errorReport(in, err), err
	}

	hasDefect := len(findings) > 0
	outcome := Decide(hasDefect, in.GenerationAttempt, p.cfg.MaxAttempts)

	observations := append(metricObservations(prepared.Metrics), modelObservations...)

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
		Model:                    join(models...),
		PromptVersion:            join(promptVersions...),
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

// metricObservations exposes the prepared measurements as report observations.
func metricObservations(ms []features.Metric) []report.SignalObservation {
	if len(ms) == 0 {
		return nil
	}
	out := make([]report.SignalObservation, 0, len(ms))
	for _, m := range ms {
		out = append(out, report.SignalObservation{
			Name:  m.Name,
			Value: m.Value,
			Unit:  m.Unit,
			Note:  m.Note,
		})
	}
	return out
}

// join concatenates non-empty labels with "+", used for model and prompt
// version strings.
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
