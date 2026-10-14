package features

import (
	"math"
	"testing"

	"github.com/666feiyu666/noise-detection/internal/audio"
)

func metricValue(t *testing.T, ms []Metric, name string) float64 {
	t.Helper()
	for _, m := range ms {
		if m.Name == name {
			return m.Value
		}
	}
	t.Fatalf("missing metric %q in %+v", name, ms)
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

func TestMetricsSpectralBalance(t *testing.T) {
	const rate = 44100
	low := ExtractPCM(tone(300, rate, rate), 1, DefaultParams()).Metrics
	high := ExtractPCM(tone(10000, rate, rate), 1, DefaultParams()).Metrics

	lowDB := metricValue(t, low, "high_band_energy_db")
	highDB := metricValue(t, high, "high_band_energy_db")
	if !(lowDB < highDB-20) {
		t.Fatalf("low-tone high band %v dB should be far below high-tone %v dB", lowDB, highDB)
	}

	lowC := metricValue(t, low, "spectral_centroid_hz")
	highC := metricValue(t, high, "spectral_centroid_hz")
	if !(highC > lowC*5) {
		t.Fatalf("high-tone centroid %v should far exceed low-tone %v", highC, lowC)
	}
}

func TestMetricsEmpty(t *testing.T) {
	if got := ExtractPCM(audio.PCM{}, 0, DefaultParams()).Metrics; got != nil {
		t.Fatalf("empty PCM: want nil metrics, got %+v", got)
	}
}
