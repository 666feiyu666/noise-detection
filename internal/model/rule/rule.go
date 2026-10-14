// Package rule is a filename-heuristic model: it decides "is a defect present?"
// from the candidate's filename only, and consumes no prepared features. It is
// a local placeholder so the pipeline can run end to end without a real signal
// or model.
package rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/666feiyu666/noise-detection/internal/features"
	"github.com/666feiyu666/noise-detection/internal/report"
)

// Rule is one local, filename-based heuristic.
//
// A rule matches when Match appears (case-insensitively) in the base filename
// of the candidate. A match produces one Finding at the given interval.
type Rule struct {
	Name         string  `json:"name"`
	Match        string  `json:"match"`
	Type         string  `json:"type"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Description  string  `json:"description"`
}

// Model is the local filename-rule detection method.
type Model struct {
	Rules []Rule
}

// ModelName, Source and PromptVersion identify this model in reports.
const (
	ModelName     = "local-rule"
	Source        = "rule"
	PromptVersion = "rule-v1"
)

// New returns a Model using the built-in default rules.
func New() *Model {
	return &Model{Rules: defaultRules()}
}

// NewFromFile loads rules from a JSON file (a JSON array of Rule).
func NewFromFile(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules file: %w", err)
	}
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("parse rules file: %w", err)
	}
	if len(rules) == 0 {
		return nil, errors.New("rules file contains no rules")
	}
	return &Model{Rules: rules}, nil
}

// defaultRules returns the built-in demonstration rules. They are examples for
// exercising the pipeline, not a real defect hypothesis.
func defaultRules() []Rule {
	return []Rule{
		{
			Name:         "digital-noise",
			Match:        "digital_noise",
			Type:         "digital_noise",
			StartSeconds: 0,
			EndSeconds:   1,
			Description:  "Placeholder rule: filename hints at harsh digital noise.",
		},
		{
			Name:         "click-pop",
			Match:        "click",
			Type:         "click_pop",
			StartSeconds: 0,
			EndSeconds:   1,
			Description:  "Placeholder rule: filename hints at an isolated click or pop.",
		},
		{
			Name:         "dropout",
			Match:        "dropout",
			Type:         "dropout",
			StartSeconds: 0,
			EndSeconds:   1,
			Description:  "Placeholder rule: filename hints at an unnatural dropout.",
		},
	}
}

// Name implements model.Model.
func (m *Model) Name() string { return ModelName }

// Source implements model.Model.
func (m *Model) Source() string { return Source }

// Predict implements model.Model using filename heuristics only.
func (m *Model) Predict(_ context.Context, in report.Input, _ features.Set) (report.Assessment, error) {
	filename := strings.ToLower(filepath.Base(in.AudioPath))
	findings := make([]report.Finding, 0, len(m.Rules))
	for _, r := range m.Rules {
		if r.Match == "" || !strings.Contains(filename, strings.ToLower(r.Match)) {
			continue
		}
		findings = append(findings, report.Finding{
			StartSeconds: r.StartSeconds,
			EndSeconds:   r.EndSeconds,
			Type:         r.Type,
			Description:  r.Description,
		})
	}
	return report.Assessment{
		Findings:      findings,
		Model:         ModelName,
		PromptVersion: PromptVersion,
	}, nil
}
