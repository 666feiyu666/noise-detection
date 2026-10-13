// Package detect defines the pluggable audio-defect detector contract.
//
// A Detector inspects one candidate recording and returns the defects it
// believes are present. The initial framework ships with a purely local rule
// detector; a future model-backed detector (for example Gemini) only needs to
// implement this interface, without changing the pipeline.
package detect

import (
	"context"

	"github.com/666feiyu666/noise-detection/internal/contract"
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
	Detect(ctx context.Context, in contract.Input) (contract.Assessment, error)
}
