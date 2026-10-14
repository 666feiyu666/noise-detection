package dsp

import (
	"context"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/features"
	"github.com/666feiyu666/noise-detection/internal/report"
)

func TestPredictMapsCandidates(t *testing.T) {
	m := New()
	prepared := features.Set{Candidates: []features.Candidate{
		{Type: "clipping", StartSeconds: 1, EndSeconds: 2, Description: "d"},
		{Type: "click_pop", StartSeconds: 5, EndSeconds: 5.1, Description: "c"},
	}}
	got, err := m.Predict(context.Background(), report.Input{AudioPath: "x.wav"}, prepared)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(got.Findings) != 2 {
		t.Fatalf("findings = %d, want 2: %+v", len(got.Findings), got.Findings)
	}
	if got.Findings[0].Type != "clipping" || got.Findings[1].Type != "click_pop" {
		t.Fatalf("unexpected finding types: %+v", got.Findings)
	}
	if got.Model != ModelName || got.PromptVersion != PromptVersion {
		t.Fatalf("model/prompt = %q/%q", got.Model, got.PromptVersion)
	}
	if m.Source() != report.SourceDSP {
		t.Fatalf("source = %q, want %q", m.Source(), report.SourceDSP)
	}
}

func TestPredictNoCandidates(t *testing.T) {
	got, err := New().Predict(context.Background(), report.Input{AudioPath: "x.wav"}, features.Set{})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(got.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", got.Findings)
	}
}
