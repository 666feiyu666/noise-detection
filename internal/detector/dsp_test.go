package detector

import (
	"context"
	"math"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/audio"
	"github.com/666feiyu666/noise-detection/internal/report"
)

const testRate = 8000

func hasType(fs []report.Finding, typ string) bool {
	for _, f := range fs {
		if f.Type == typ {
			return true
		}
	}
	return false
}

// validIntervals checks the timestamp contract used by the pipeline.
func validIntervals(t *testing.T, fs []report.Finding, duration float64) {
	t.Helper()
	for i, f := range fs {
		if f.Type == "" || f.Description == "" {
			t.Fatalf("finding %d missing type/description: %+v", i, f)
		}
		if f.StartSeconds < 0 || f.EndSeconds <= f.StartSeconds || f.EndSeconds > duration {
			t.Fatalf("finding %d interval [%v,%v] invalid for duration %v", i, f.StartSeconds, f.EndSeconds, duration)
		}
	}
}

func TestAnalyzeClean(t *testing.T) {
	n := testRate / 2
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(0.5 * math.Sin(2*math.Pi*440*float64(i)/testRate))
	}
	duration := float64(n) / testRate
	fs := Analyze(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultDSPThresholds())
	if len(fs) != 0 {
		t.Fatalf("clean signal: got %d findings: %+v", len(fs), fs)
	}
}

func TestAnalyzeClipping(t *testing.T) {
	s := make([]float32, testRate) // 1s pinned at full scale
	for i := range s {
		s[i] = 1.0
	}
	duration := 1.0
	fs := Analyze(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultDSPThresholds())
	if len(fs) != 1 || fs[0].Type != "clipping" {
		t.Fatalf("want one clipping finding, got %+v", fs)
	}
	validIntervals(t, fs, duration)
}

func TestAnalyzeSilence(t *testing.T) {
	// half-second tone, one second of silence, half-second tone
	half := testRate / 2
	s := make([]float32, 0, testRate*2)
	for i := 0; i < half; i++ {
		s = append(s, float32(0.5*math.Sin(2*math.Pi*440*float64(i)/testRate)))
	}
	s = append(s, make([]float32, testRate)...) // 1s silence in the middle
	for i := 0; i < half; i++ {
		s = append(s, float32(0.5*math.Sin(2*math.Pi*440*float64(i)/testRate)))
	}
	duration := float64(len(s)) / testRate
	fs := Analyze(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultDSPThresholds())
	if len(fs) != 1 || fs[0].Type != "dropout" {
		t.Fatalf("want one dropout finding, got %+v", fs)
	}
	if fs[0].StartSeconds < 0.4 || fs[0].StartSeconds > 0.6 {
		t.Fatalf("dropout start = %v, want ~0.5", fs[0].StartSeconds)
	}
	validIntervals(t, fs, duration)
}

func TestAnalyzeLeadingSilenceIgnored(t *testing.T) {
	// Leading silence is normal and must not be reported.
	s := make([]float32, testRate)
	duration := 1.0
	fs := Analyze(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultDSPThresholds())
	if len(fs) != 0 {
		t.Fatalf("leading silence should be ignored, got %+v", fs)
	}
}

func TestAnalyzeTransient(t *testing.T) {
	s := make([]float32, testRate)
	for i := range s {
		s[i] = 0.1
	}
	s[4000] = 0.95 // single abrupt discontinuity
	duration := 1.0
	fs := Analyze(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultDSPThresholds())
	if !hasType(fs, "click_pop") {
		t.Fatalf("want a click_pop finding, got %+v", fs)
	}
	validIntervals(t, fs, duration)
}

func TestAnalyzeImbalance(t *testing.T) {
	n := testRate / 2
	s := make([]float32, n*2)
	for i := 0; i < n; i++ {
		tone := math.Sin(2 * math.Pi * 440 * float64(i) / testRate)
		s[2*i] = float32(0.5 * tone)
		s[2*i+1] = float32(0.05 * tone)
	}
	duration := float64(n) / testRate
	fs := Analyze(audio.PCM{Samples: s, Channels: 2, SampleRate: testRate}, duration, DefaultDSPThresholds())
	if !hasType(fs, "channel_imbalance") {
		t.Fatalf("want a channel_imbalance finding, got %+v", fs)
	}
	validIntervals(t, fs, duration)
}

// fakeSource lets Detect be exercised without ffmpeg.
type fakeSource struct {
	meta audio.Metadata
	pcm  audio.PCM
	err  error
}

func (f fakeSource) Probe(_ context.Context, _ string) (audio.Metadata, error) {
	return f.meta, f.err
}

func (f fakeSource) Decode(_ context.Context, _ string, _, _ int) (audio.PCM, error) {
	return f.pcm, f.err
}

func TestDSPDetectorUsesProbedDuration(t *testing.T) {
	src := fakeSource{
		meta: audio.Metadata{DurationSeconds: 3.5, SampleRate: testRate, Channels: 1},
		pcm:  audio.PCM{Samples: make([]float32, testRate), Channels: 1, SampleRate: testRate},
	}
	d := NewDSPDetector(DSPConfig{Source: src})
	got, err := d.Detect(context.Background(), report.Input{AudioPath: "x.wav"})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got.Model != DSPModel || got.PromptVersion != DSPPromptVersion {
		t.Fatalf("model/prompt = %q/%q", got.Model, got.PromptVersion)
	}
	if got.DurationSeconds == nil || *got.DurationSeconds != 3.5 {
		t.Fatalf("duration = %v, want 3.5", got.DurationSeconds)
	}
}
