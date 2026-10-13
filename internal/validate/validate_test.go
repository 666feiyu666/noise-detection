package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/contract"
)

func TestInput(t *testing.T) {
	valid := []struct {
		name string
		in   contract.Input
	}{
		{"minimal", contract.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: "a.wav"}},
		{"max attempt", contract.Input{RequestID: "r", GenerationAttempt: 3, AudioPath: "a.wav"}},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := Input(tc.in, InputOptions{}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	invalid := []struct {
		name string
		in   contract.Input
	}{
		{"missing request_id", contract.Input{GenerationAttempt: 1, AudioPath: "a.wav"}},
		{"missing audio_path", contract.Input{RequestID: "r", GenerationAttempt: 1}},
		{"attempt too low", contract.Input{RequestID: "r", GenerationAttempt: 0, AudioPath: "a.wav"}},
		{"attempt too high", contract.Input{RequestID: "r", GenerationAttempt: 4, AudioPath: "a.wav"}},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			if err := Input(tc.in, InputOptions{}); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestInputRequireAudioFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.wav")
	if err := Input(contract.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: missing},
		InputOptions{RequireAudioFile: true}); err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}

	existing := filepath.Join(dir, "ok.wav")
	if err := os.WriteFile(existing, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Input(contract.Input{RequestID: "r", GenerationAttempt: 1, AudioPath: existing},
		InputOptions{RequireAudioFile: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindings(t *testing.T) {
	dur := 10.0
	valid := []contract.Finding{{StartSeconds: 1, EndSeconds: 2, Type: "click_pop", Description: "d"}}
	if err := Findings(valid, &dur); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	invalid := []struct {
		name    string
		finding contract.Finding
	}{
		{"missing type", contract.Finding{StartSeconds: 1, EndSeconds: 2, Description: "d"}},
		{"missing description", contract.Finding{StartSeconds: 1, EndSeconds: 2, Type: "t"}},
		{"negative start", contract.Finding{StartSeconds: -1, EndSeconds: 2, Type: "t", Description: "d"}},
		{"end not after start", contract.Finding{StartSeconds: 2, EndSeconds: 2, Type: "t", Description: "d"}},
		{"end beyond duration", contract.Finding{StartSeconds: 9, EndSeconds: 20, Type: "t", Description: "d"}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := Findings([]contract.Finding{tc.finding}, &dur); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}
