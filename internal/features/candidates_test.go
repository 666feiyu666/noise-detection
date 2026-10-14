package features

import (
	"context"
	"math"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/audio"
)

const testRate = 8000

func hasCandidateType(cs []Candidate, typ string) bool {
	for _, c := range cs {
		if c.Type == typ {
			return true
		}
	}
	return false
}

// validCandidates checks the timestamp contract used downstream.
func validCandidates(t *testing.T, cs []Candidate, duration float64) {
	t.Helper()
	for i, c := range cs {
		if c.Type == "" || c.Description == "" {
			t.Fatalf("candidate %d missing type/description: %+v", i, c)
		}
		if c.StartSeconds < 0 || c.EndSeconds <= c.StartSeconds || c.EndSeconds > duration {
			t.Fatalf("candidate %d interval [%v,%v] invalid for duration %v", i, c.StartSeconds, c.EndSeconds, duration)
		}
	}
}

func TestExtractPCMClean(t *testing.T) {
	n := testRate / 2
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(0.5 * math.Sin(2*math.Pi*440*float64(i)/testRate))
	}
	duration := float64(n) / testRate
	set := ExtractPCM(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultParams())
	if len(set.Candidates) != 0 {
		t.Fatalf("clean signal: got %d candidates: %+v", len(set.Candidates), set.Candidates)
	}
}

func TestExtractPCMClipping(t *testing.T) {
	s := make([]float32, testRate) // 1s pinned at full scale
	for i := range s {
		s[i] = 1.0
	}
	duration := 1.0
	set := ExtractPCM(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultParams())
	if len(set.Candidates) != 1 || set.Candidates[0].Type != "clipping" {
		t.Fatalf("want one clipping candidate, got %+v", set.Candidates)
	}
	validCandidates(t, set.Candidates, duration)
}

func TestExtractPCMSilence(t *testing.T) {
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
	set := ExtractPCM(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultParams())
	if len(set.Candidates) != 1 || set.Candidates[0].Type != "dropout" {
		t.Fatalf("want one dropout candidate, got %+v", set.Candidates)
	}
	if set.Candidates[0].StartSeconds < 0.4 || set.Candidates[0].StartSeconds > 0.6 {
		t.Fatalf("dropout start = %v, want ~0.5", set.Candidates[0].StartSeconds)
	}
	validCandidates(t, set.Candidates, duration)
}

func TestExtractPCMLeadingSilenceIgnored(t *testing.T) {
	// Leading silence is normal and must not be reported.
	s := make([]float32, testRate)
	duration := 1.0
	set := ExtractPCM(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultParams())
	if len(set.Candidates) != 0 {
		t.Fatalf("leading silence should be ignored, got %+v", set.Candidates)
	}
}

func TestExtractPCMTransient(t *testing.T) {
	s := make([]float32, testRate)
	for i := range s {
		s[i] = 0.1
	}
	s[4000] = 0.95 // single abrupt discontinuity
	duration := 1.0
	set := ExtractPCM(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, duration, DefaultParams())
	if !hasCandidateType(set.Candidates, "click_pop") {
		t.Fatalf("want a click_pop candidate, got %+v", set.Candidates)
	}
	validCandidates(t, set.Candidates, duration)
}

func TestExtractPCMImbalance(t *testing.T) {
	n := testRate / 2
	s := make([]float32, n*2)
	for i := 0; i < n; i++ {
		tone := math.Sin(2 * math.Pi * 440 * float64(i) / testRate)
		s[2*i] = float32(0.5 * tone)
		s[2*i+1] = float32(0.05 * tone)
	}
	duration := float64(n) / testRate
	set := ExtractPCM(audio.PCM{Samples: s, Channels: 2, SampleRate: testRate}, duration, DefaultParams())
	if !hasCandidateType(set.Candidates, "channel_imbalance") {
		t.Fatalf("want a channel_imbalance candidate, got %+v", set.Candidates)
	}
	validCandidates(t, set.Candidates, duration)
}

// TestExtractFeatureVector checks the stable numeric vector mirrors the metrics
// and candidate counts.
func TestExtractFeatureVector(t *testing.T) {
	n := testRate
	s := make([]float32, n)
	for i := range s {
		s[i] = 1.0 // clipping for the whole second
	}
	set := ExtractPCM(audio.PCM{Samples: s, Channels: 1, SampleRate: testRate}, 1.0, DefaultParams())
	if len(set.Vector) != len(set.VectorNames) {
		t.Fatalf("vector %d and names %d mismatch", len(set.Vector), len(set.VectorNames))
	}
	idx := -1
	for i, name := range set.VectorNames {
		if name == "candidate_count_clipping" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("missing candidate_count_clipping in %v", set.VectorNames)
	}
	if set.Vector[idx] != 1 {
		t.Fatalf("candidate_count_clipping = %v, want 1", set.Vector[idx])
	}
}

// fakeSource lets Extract be exercised without ffmpeg.
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

func TestExtractUsesProbedDuration(t *testing.T) {
	src := fakeSource{
		meta: audio.Metadata{DurationSeconds: 3.5, SampleRate: testRate, Channels: 1},
		pcm:  audio.PCM{Samples: make([]float32, testRate), Channels: 1, SampleRate: testRate},
	}
	set, err := Extract(context.Background(), src, "x.wav")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if set.DurationSeconds != 3.5 {
		t.Fatalf("duration = %v, want 3.5", set.DurationSeconds)
	}
}
