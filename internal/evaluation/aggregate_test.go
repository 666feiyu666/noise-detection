package evaluation

import (
	"testing"

	"github.com/666feiyu666/noise-detection/internal/report"
)

func TestAggregateBranches(t *testing.T) {
	evidence := []Evidence{
		{Source: report.SourceDSP, Findings: []report.Finding{
			{StartSeconds: 1, EndSeconds: 2, Type: "clipping", Description: "d"},
			{StartSeconds: 10, EndSeconds: 11, Type: "dropout", Description: "d"},
		}},
		{Source: report.SourceGemini, Findings: []report.Finding{
			{StartSeconds: 1.4, EndSeconds: 1.8, Type: "click_pop", Description: "g"},   // overlaps dsp[0]
			{StartSeconds: 20, EndSeconds: 21, Type: "digital_noise", Description: "g"}, // dsp has no support
		}},
	}

	out := Aggregate(evidence)
	if len(out) != 3 {
		t.Fatalf("got %d findings, want 3: %+v", len(out), out)
	}
	for i := 1; i < len(out); i++ {
		if out[i-1].StartSeconds > out[i].StartSeconds {
			t.Fatalf("not sorted by start: %+v", out)
		}
	}

	byType := map[string]report.Finding{}
	for _, f := range out {
		byType[f.Type] = f
	}

	if f := byType["clipping"]; f.Status != report.StatusConfirmed || len(f.Sources) != 2 {
		t.Errorf("confirmed branch: got status=%q sources=%v", f.Status, f.Sources)
	}
	if f := byType["dropout"]; f.Status != report.StatusUnconfirmed || len(f.Sources) != 1 {
		t.Errorf("unconfirmed branch: got status=%q sources=%v", f.Status, f.Sources)
	}
	if f := byType["digital_noise"]; f.Status != report.StatusUnconfirmed || f.Sources[0] != report.SourceGemini {
		t.Errorf("single-source branch: got status=%q sources=%v", f.Status, f.Sources)
	}
}

func TestAggregateDoesNotMergeSameSource(t *testing.T) {
	evidence := []Evidence{{Source: report.SourceDSP, Findings: []report.Finding{
		{StartSeconds: 1, EndSeconds: 2, Type: "clipping", Description: "d"},
		{StartSeconds: 1.5, EndSeconds: 2.5, Type: "dropout", Description: "d"},
	}}}
	out := Aggregate(evidence)
	if len(out) != 2 {
		t.Fatalf("same-source findings must stay separate, got %+v", out)
	}
}

func TestAggregateEmpty(t *testing.T) {
	if out := Aggregate(nil); len(out) != 0 {
		t.Fatalf("empty aggregate = %+v, want none", out)
	}
}
