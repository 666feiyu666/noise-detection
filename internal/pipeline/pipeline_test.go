package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/contract"
	"github.com/666feiyu666/noise-detection/internal/detect"
)

// fakeDetector lets tests inject findings or errors without a model.
type fakeDetector struct {
	findings []contract.Finding
	err      error
}

func (f fakeDetector) Name() string { return "fake" }
func (f fakeDetector) Detect(context.Context, contract.Input) (contract.Assessment, error) {
	if f.err != nil {
		return contract.Assessment{}, f.err
	}
	return contract.Assessment{Findings: f.findings, Model: "fake", PromptVersion: "v0"}, nil
}

func newAudioFile(t *testing.T, name string) contract.Input {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	return contract.Input{RequestID: "req-1", GenerationAttempt: 1, AudioPath: path}
}

func TestRunClean(t *testing.T) {
	p := New(Config{Detector: detect.NewRuleDetector(), RequireAudioFile: true})
	report, err := p.Run(context.Background(), newAudioFile(t, "song_clean.wav"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.AssessmentStatus != contract.StatusCompleted {
		t.Fatalf("status = %q", report.AssessmentStatus)
	}
	if report.HasDefect == nil || *report.HasDefect {
		t.Fatalf("has_defect = %v, want false", report.HasDefect)
	}
	if report.Decision != contract.DecisionAccept {
		t.Fatalf("decision = %q, want %q", report.Decision, contract.DecisionAccept)
	}
	if report.ShouldRegenerate == nil || *report.ShouldRegenerate {
		t.Fatalf("should_regenerate = %v, want false", report.ShouldRegenerate)
	}
}

func TestRunDefectAttemptBranches(t *testing.T) {
	cases := []struct {
		attempt      int
		wantDecision string
	}{
		{1, contract.DecisionRegenerate},
		{2, contract.DecisionRegenerate},
		{3, contract.DecisionStopLimitReached},
	}
	for _, tc := range cases {
		in := newAudioFile(t, "cand_digital_noise.wav")
		in.GenerationAttempt = tc.attempt
		p := New(Config{Detector: detect.NewRuleDetector(), RequireAudioFile: true})
		report, err := p.Run(context.Background(), in)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", tc.attempt, err)
		}
		if report.Decision != tc.wantDecision {
			t.Fatalf("attempt %d: decision = %q, want %q", tc.attempt, report.Decision, tc.wantDecision)
		}
		if report.HasDefect == nil || !*report.HasDefect {
			t.Fatalf("attempt %d: has_defect = %v, want true", tc.attempt, report.HasDefect)
		}
	}
}

func TestRunDefaultAttempt(t *testing.T) {
	in := newAudioFile(t, "song_clean.wav")
	in.GenerationAttempt = 0 // defaulted to 1
	p := New(Config{Detector: detect.NewRuleDetector(), RequireAudioFile: true})
	report, err := p.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.GenerationAttempt != contract.MinGenerationAttempt {
		t.Fatalf("generation_attempt = %d, want %d", report.GenerationAttempt, contract.MinGenerationAttempt)
	}
}

func TestRunInvalidInputProducesErrorReport(t *testing.T) {
	p := New(Config{Detector: detect.NewRuleDetector(), RequireAudioFile: true})
	in := contract.Input{RequestID: "", GenerationAttempt: 1, AudioPath: "x.wav"}
	report, err := p.Run(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	assertErrorReport(t, report)
}

func TestRunDetectorErrorProducesErrorReport(t *testing.T) {
	p := New(Config{Detector: fakeDetector{err: errors.New("boom")}, RequireAudioFile: false})
	in := contract.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: "x.wav"}
	report, err := p.Run(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	assertErrorReport(t, report)
}

func TestRunInvalidFindingsProducesErrorReport(t *testing.T) {
	bad := fakeDetector{findings: []contract.Finding{
		{StartSeconds: 2, EndSeconds: 2, Type: "t", Description: "d"},
	}}
	p := New(Config{Detector: bad, RequireAudioFile: false})
	in := contract.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: "x.wav"}
	report, err := p.Run(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	assertErrorReport(t, report)
}

func assertErrorReport(t *testing.T, report contract.Report) {
	t.Helper()
	if report.AssessmentStatus != contract.StatusError {
		t.Fatalf("status = %q, want %q", report.AssessmentStatus, contract.StatusError)
	}
	if report.HasDefect != nil {
		t.Fatalf("has_defect = %v, want nil", *report.HasDefect)
	}
	if report.ShouldRegenerate != nil {
		t.Fatalf("should_regenerate = %v, want nil", *report.ShouldRegenerate)
	}
	if report.Decision != contract.DecisionError {
		t.Fatalf("decision = %q, want %q", report.Decision, contract.DecisionError)
	}
	if report.Defects == nil || len(report.Defects) != 0 {
		t.Fatalf("defects = %v, want empty slice", report.Defects)
	}
}
