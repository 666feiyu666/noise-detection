// Package model is the modeling layer of the pipeline (CRISP-DM: Modeling).
//
// A Model is one detection method — a local rule pass, a deterministic DSP
// decision, a machine-learning model, or an LLM — that turns prepared features
// into findings. Every model shares the one prepared feature set and never
// re-reads the signal itself; new methods are added by implementing Model and
// registering a factory, without changing the pipeline.
package model

import (
	"context"

	"github.com/666feiyu666/noise-detection/internal/features"
	"github.com/666feiyu666/noise-detection/internal/report"
)

// Model turns prepared features into findings.
//
// Implementations must not denoise, normalise, or modify the input. An error
// return means the candidate could not be assessed and must surface as an error
// report, never as a clean result.
type Model interface {
	// Name identifies the model in reports.
	Name() string
	// Source labels the evidence family this model produces (for example
	// report.SourceDSP or report.SourceGemini).
	Source() string
	// Predict assesses the candidate from its prepared features. in carries the
	// caller request, including the audio path, for models that also read the
	// original file.
	Predict(ctx context.Context, in report.Input, prepared features.Set) (report.Assessment, error)
}
