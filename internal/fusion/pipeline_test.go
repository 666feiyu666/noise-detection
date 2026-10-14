package fusion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/detector"
	"github.com/666feiyu666/noise-detection/internal/report"
)

// fakeDetector lets tests inject findings or errors without a model.
type fakeDetector struct {
	findings []report.Finding
	err      error
}

func (f fakeDetector) Name() string { return "fake" }
func (f fakeDetector) Detect(context.Context, report.Input) (report.Assessment, error) {
	if f.err != nil {
		return report.Assessment{}, f.err
	}
	return report.Assessment{Findings: f.findings, Model: "fake", PromptVersion: "v0"}, nil
}

func newAudioFile(t *testing.T, name string) report.Input {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	return report.Input{RequestID: "req-1", GenerationAttempt: 1, AudioPath: path}
}

func TestRunClean(t *testing.T) {
	p := New(Config{Detector: detector.NewRuleDetector(), RequireAudioFile: true})
	rep, err := p.Run(context.Background(), newAudioFile(t, "song_clean.wav"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.AssessmentStatus != report.StatusCompleted {
		t.Fatalf("status = %q", rep.AssessmentStatus)
	}
	if rep.HasDefect == nil || *rep.HasDefect {
		t.Fatalf("has_defect = %v, want false", rep.HasDefect)
	}
	if rep.Decision != report.DecisionAccept {
		t.Fatalf("decision = %q, want %q", rep.Decision, report.DecisionAccept)
	}
	if rep.ShouldRegenerate == nil || *rep.ShouldRegenerate {
		t.Fatalf("should_regenerate = %v, want false", rep.ShouldRegenerate)
	}
}

func TestRunDefectAttemptBranches(t *testing.T) {
	cases := []struct {
		attempt      int
		wantDecision string
	}{
		{1, report.DecisionRegenerate},
		{2, report.DecisionRegenerate},
		{3, report.DecisionStopLimitReached},
	}
	for _, tc := range cases {
		in := newAudioFile(t, "cand_digital_noise.wav")
		in.GenerationAttempt = tc.attempt
		p := New(Config{Detector: detector.NewRuleDetector(), RequireAudioFile: true})
		rep, err := p.Run(context.Background(), in)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", tc.attempt, err)
		}
		if rep.Decision != tc.wantDecision {
			t.Fatalf("attempt %d: decision = %q, want %q", tc.attempt, rep.Decision, tc.wantDecision)
		}
		if rep.HasDefect == nil || !*rep.HasDefect {
			t.Fatalf("attempt %d: has_defect = %v, want true", tc.attempt, rep.HasDefect)
		}
	}
}

func TestRunDefaultAttempt(t *testing.T) {
	in := newAudioFile(t, "song_clean.wav")
	in.GenerationAttempt = 0 // defaulted to 1
	p := New(Config{Detector: detector.NewRuleDetector(), RequireAudioFile: true})
	rep, err := p.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.GenerationAttempt != report.MinGenerationAttempt {
		t.Fatalf("generation_attempt = %d, want %d", rep.GenerationAttempt, report.MinGenerationAttempt)
	}
}

func TestRunInvalidInputProducesErrorReport(t *testing.T) {
	p := New(Config{Detector: detector.NewRuleDetector(), RequireAudioFile: true})
	in := report.Input{RequestID: "", GenerationAttempt: 1, AudioPath: "x.wav"}
	rep, err := p.Run(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	assertErrorReport(t, rep)
}

func TestRunDetectorErrorProducesErrorReport(t *testing.T) {
	p := New(Config{Detector: fakeDetector{err: errors.New("boom")}, RequireAudioFile: false})
	in := report.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: "x.wav"}
	rep, err := p.Run(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	assertErrorReport(t, rep)
}

func TestRunInvalidFindingsProducesErrorReport(t *testing.T) {
	bad := fakeDetector{findings: []report.Finding{
		{StartSeconds: 2, EndSeconds: 2, Type: "t", Description: "d"},
	}}
	p := New(Config{Detector: bad, RequireAudioFile: false})
	in := report.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: "x.wav"}
	rep, err := p.Run(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	assertErrorReport(t, rep)
}

func TestRunHybridFusion(t *testing.T) {
	primary := fakeDetector{findings: []report.Finding{
		{StartSeconds: 1, EndSeconds: 2, Type: "clipping", Description: "d"},
	}}
	verifier := fakeDetector{findings: []report.Finding{
		{StartSeconds: 1.2, EndSeconds: 1.8, Type: "click_pop", Description: "g"},
	}}
	p := New(Config{Detector: primary, Verifier: verifier, RequireAudioFile: false})
	in := report.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: "x.wav"}
	rep, err := p.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.Decision != report.DecisionRegenerate {
		t.Fatalf("decision = %q, want %q", rep.Decision, report.DecisionRegenerate)
	}
	if len(rep.Defects) != 1 {
		t.Fatalf("fused defects = %+v, want 1 (overlapping)", rep.Defects)
	}
	if f := rep.Defects[0]; f.Source != report.SourceBoth || f.Status != report.StatusConfirmed {
		t.Fatalf("fused finding source/status = %q/%q, want both/confirmed", f.Source, f.Status)
	}
	if rep.Model != "fake+fake" {
		t.Fatalf("model = %q, want fake+fake", rep.Model)
	}
}

func assertErrorReport(t *testing.T, rep report.Report) {
	t.Helper()
	if rep.AssessmentStatus != report.StatusError {
		t.Fatalf("status = %q, want %q", rep.AssessmentStatus, report.StatusError)
	}
	if rep.HasDefect != nil {
		t.Fatalf("has_defect = %v, want nil", *rep.HasDefect)
	}
	if rep.ShouldRegenerate != nil {
		t.Fatalf("should_regenerate = %v, want nil", *rep.ShouldRegenerate)
	}
	if rep.Decision != report.DecisionError {
		t.Fatalf("decision = %q, want %q", rep.Decision, report.DecisionError)
	}
	if rep.Defects == nil || len(rep.Defects) != 0 {
		t.Fatalf("defects = %v, want empty slice", rep.Defects)
	}
}
