package fusion

import (
	"testing"

	"github.com/666feiyu666/noise-detection/internal/report"
)

func TestFuseBranches(t *testing.T) {
	primary := []report.Finding{
		{StartSeconds: 1, EndSeconds: 2, Type: "clipping", Description: "d"},
		{StartSeconds: 10, EndSeconds: 11, Type: "dropout", Description: "d"},
	}
	secondary := []report.Finding{
		{StartSeconds: 1.4, EndSeconds: 1.8, Type: "click_pop", Description: "g"},    // overlaps primary[0]
		{StartSeconds: 20, EndSeconds: 21, Type: "digital_noise", Description: "g"}, // no primary support
	}

	out := Fuse(primary, secondary, report.SourceDSP, report.SourceGemini)
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

	if f := byType["clipping"]; f.Source != report.SourceBoth || f.Status != report.StatusConfirmed {
		t.Errorf("confirmed branch: got source=%q status=%q", f.Source, f.Status)
	}
	if f := byType["dropout"]; f.Source != report.SourceDSP || f.Status != report.StatusUnconfirmed {
		t.Errorf("unconfirmed branch: got source=%q status=%q", f.Source, f.Status)
	}
	if f := byType["digital_noise"]; f.Source != report.SourceGemini || f.Status != report.StatusSuspected {
		t.Errorf("suspected branch: got source=%q status=%q", f.Source, f.Status)
	}
}

func TestFuseEmpty(t *testing.T) {
	if out := Fuse(nil, nil, report.SourceDSP, report.SourceGemini); len(out) != 0 {
		t.Fatalf("empty fusion = %+v, want none", out)
	}
}
