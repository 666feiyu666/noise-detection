package detect

import (
	"context"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/contract"
)

func TestRuleDetectorDetect(t *testing.T) {
	cases := []struct {
		name      string
		audioPath string
		want      int
	}{
		{"clean filename", "song_clean_take.wav", 0},
		{"digital noise match", "cand_digital_noise_01.mp3", 1},
		{"click match is case-insensitive", "Take_CLICK_02.wav", 1},
		{"dropout match", "final_dropout.wav", 1},
	}
	d := NewRuleDetector()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.Detect(context.Background(), contract.Input{AudioPath: tc.audioPath})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.Findings) != tc.want {
				t.Fatalf("findings = %d, want %d", len(got.Findings), tc.want)
			}
			if got.Model != RuleModel {
				t.Fatalf("model = %q, want %q", got.Model, RuleModel)
			}
		})
	}
}
