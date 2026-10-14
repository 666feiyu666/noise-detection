package detector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/666feiyu666/noise-detection/internal/report"
)

// Rule is one local, filename-based heuristic.
//
// A rule matches when Match appears (case-insensitively) in the base filename
// of the candidate. A match produces one Finding at the given interval. Rules
// are intentionally simple: this detector does not read, decode, or analyse
// audio content.
type Rule struct {
	Name         string  `json:"name"`
	Match        string  `json:"match"`
	Type         string  `json:"type"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Description  string  `json:"description"`
}

// RuleDetector is the local placeholder detector. It decides "is a defect
// detected?" from filename heuristics only, so the pipeline can run end to end
// without a model or audio input.
type RuleDetector struct {
	Rules []Rule
}

// RuleModel and RulePromptVersion identify this detector in reports.
const (
	RuleModel         = "local-rule"
	RulePromptVersion = "rule-v1"
)

// NewRuleDetector returns a RuleDetector using the built-in default rules.
func NewRuleDetector() *RuleDetector {
	return &RuleDetector{Rules: DefaultRules()}
}

// NewRuleDetectorFromFile loads rules from a JSON file (a JSON array of Rule).
func NewRuleDetectorFromFile(path string) (*RuleDetector, error) {
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
	return &RuleDetector{Rules: rules}, nil
}

// DefaultRules returns the built-in demonstration rules. They are examples for
// exercising the pipeline, not a real defect hypothesis.
func DefaultRules() []Rule {
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

// Name implements Detector.
func (d *RuleDetector) Name() string { return RuleModel }

// Detect implements Detector using filename heuristics only.
func (d *RuleDetector) Detect(_ context.Context, in report.Input) (report.Assessment, error) {
	filename := strings.ToLower(filepath.Base(in.AudioPath))
	findings := make([]report.Finding, 0, len(d.Rules))
	for _, r := range d.Rules {
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
		Model:         RuleModel,
		PromptVersion: RulePromptVersion,
	}, nil
}
