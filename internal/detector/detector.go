// Package detector defines the pluggable audio-defect detector contract.
//
// A Detector inspects one candidate recording and returns the defects it
// believes are present. The initial framework ships with a purely local rule
// detector; a model-backed detector (for example the gemini package) only needs
// to implement this interface, without changing the pipeline.
package detector

import (
	"context"

	"github.com/666feiyu666/noise-detection/internal/report"
)

// Detector inspects one candidate and reports its findings.
//
// Implementations must not denoise, normalise, or modify the input. An error
// return means the candidate could not be assessed and must surface as an
// error report, never as a clean result.
type Detector interface {
	// Name identifies the detector in reports.
	Name() string
	// Detect assesses the candidate described by in.
	Detect(ctx context.Context, in report.Input) (report.Assessment, error)
}

// Sourced detectors report the family their findings come from (report.SourceDSP
// or report.SourceGemini) so fusion can label evidence by origin.
type Sourced interface {
	Source() string
}
