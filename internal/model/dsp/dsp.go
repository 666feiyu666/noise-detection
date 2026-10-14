// Package dsp is the deterministic signal-decision model. It turns the
// candidate anomaly intervals prepared by the feature layer into findings, and
// does no signal processing of its own: the measuring happens in the data-
// preparation layer, the deciding happens here.
package dsp

import (
	"context"

	"github.com/666feiyu666/noise-detection/internal/features"
	"github.com/666feiyu666/noise-detection/internal/report"
)

// ModelName and PromptVersion identify this model in reports.
const (
	ModelName     = "dsp"
	PromptVersion = "dsp-v2"
)

// Model reports one finding per candidate anomaly interval prepared by the
// feature layer.
type Model struct{}

// New returns a ready DSP model.
func New() *Model { return &Model{} }

// Name implements model.Model.
func (m *Model) Name() string { return ModelName }

// Source implements model.Model.
func (m *Model) Source() string { return report.SourceDSP }

// Predict implements model.Model by mapping each prepared candidate to a
// finding.
func (m *Model) Predict(_ context.Context, _ report.Input, prepared features.Set) (report.Assessment, error) {
	findings := make([]report.Finding, 0, len(prepared.Candidates))
	for _, c := range prepared.Candidates {
		findings = append(findings, report.Finding{
			StartSeconds: c.StartSeconds,
			EndSeconds:   c.EndSeconds,
			Type:         c.Type,
			Description:  c.Description,
		})
	}
	return report.Assessment{
		Findings:      findings,
		Model:         ModelName,
		PromptVersion: PromptVersion,
	}, nil
}
