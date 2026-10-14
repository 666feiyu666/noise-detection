package detector

import (
	"math"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/audio"
	"github.com/666feiyu666/noise-detection/internal/report"
)

func observation(t *testing.T, obs []report.SignalObservation, name string) float64 {
	t.Helper()
	for _, o := range obs {
		if o.Name == name {
			return o.Value
		}
	}
	t.Fatalf("missing observation %q in %+v", name, obs)
	return 0
}

// tone builds a mono PCM holding a single sine tone.
func tone(freq float64, rate, n int) audio.PCM {
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
	}
	return audio.PCM{Samples: s, Channels: 1, SampleRate: rate}
}

func TestAnalyzeTimbreSpectralBalance(t *testing.T) {
	const rate = 44100
	low := AnalyzeTimbre(tone(300, rate, rate), 1)
	high := AnalyzeTimbre(tone(10000, rate, rate), 1)

	lowDB := observation(t, low, "high_band_energy_db")
	highDB := observation(t, high, "high_band_energy_db")
	if !(lowDB < highDB-20) {
		t.Fatalf("low-tone high band %v dB should be far below high-tone %v dB", lowDB, highDB)
	}

	lowC := observation(t, low, "spectral_centroid_hz")
	highC := observation(t, high, "spectral_centroid_hz")
	if !(highC > lowC*5) {
		t.Fatalf("high-tone centroid %v should far exceed low-tone %v", highC, lowC)
	}
}

func TestAnalyzeTimbreEmpty(t *testing.T) {
	if got := AnalyzeTimbre(audio.PCM{}, 0); got != nil {
		t.Fatalf("empty PCM: want nil, got %+v", got)
	}
}
