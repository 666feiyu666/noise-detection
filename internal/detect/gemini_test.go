package detect

import (
	"testing"
)

func TestAudioMIMEType(t *testing.T) {
	cases := map[string]string{
		"a.mp3": "audio/mp3",
		"A.MP3": "audio/mp3",
		"b.wav": "audio/wav",
	}
	for path, want := range cases {
		got, err := audioMIMEType(path)
		if err != nil {
			t.Fatalf("audioMIMEType(%q): unexpected error: %v", path, err)
		}
		if got != want {
			t.Errorf("audioMIMEType(%q) = %q, want %q", path, got, want)
		}
	}
	for _, bad := range []string{"a.flac", "a.ogg", "noext"} {
		if _, err := audioMIMEType(bad); err == nil {
			t.Errorf("audioMIMEType(%q): expected error", bad)
		}
	}
}

func TestGeminiResultFindings(t *testing.T) {
	clean := geminiResult{HasDefect: false, DurationSeconds: 12}
	f, err := clean.findings()
	if err != nil {
		t.Fatalf("clean result: unexpected error: %v", err)
	}
	if len(f) != 0 {
		t.Errorf("clean result: got %d findings, want 0", len(f))
	}

	defective := geminiResult{
		HasDefect: true,
		Defects: []geminiFinding{{
			StartSeconds: 1.5, EndSeconds: 2.0, Type: "click_pop", Description: "a click",
		}},
	}
	f, err = defective.findings()
	if err != nil {
		t.Fatalf("defective result: unexpected error: %v", err)
	}
	if len(f) != 1 || f[0].Type != "click_pop" {
		t.Errorf("defective result: got %+v, want one click_pop finding", f)
	}

	inconsistent := []geminiResult{
		{HasDefect: true},
		{HasDefect: false, Defects: []geminiFinding{{Type: "click_pop"}}},
	}
	for i, r := range inconsistent {
		if _, err := r.findings(); err == nil {
			t.Errorf("inconsistent case %d: expected error, got nil", i)
		}
	}
}

// geminiDetector must satisfy the Detector interface.
var _ Detector = (*GeminiDetector)(nil)
