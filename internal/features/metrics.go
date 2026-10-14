package features

import (
	"math"

	"github.com/666feiyu666/noise-detection/internal/audio"
)

// Analysis window and band parameters for the spectral metrics. These describe
// how the signal is measured, not a quality threshold: the results are
// supporting data only.
const (
	metricFrameSize    = 2048
	metricHop          = 1024
	metricHighCutoffHz = 6000.0
	metricSegmentSec   = 10.0
	onsetFluxSigmas    = 1.5
)

// metrics computes spectral-balance and onset-envelope measurements over the
// whole recording.
//
// Spectral balance and onset strength depend on instrument, arrangement, and
// production, so a reading becomes meaningful only next to context: a reference
// version, or the recording's own behaviour over time. Nothing here raises a
// defect on its own.
func metrics(pcm audio.PCM, duration float64) []Metric {
	if pcm.SampleRate <= 0 || pcm.Channels <= 0 || len(pcm.Samples) == 0 {
		return nil
	}
	if duration <= 0 {
		duration = float64(len(pcm.Samples)) / float64(pcm.SampleRate*pcm.Channels)
	}
	rate := float64(pcm.SampleRate)
	samples := mono(pcm.Samples, pcm.Channels)
	if len(samples) < metricFrameSize {
		return nil
	}

	window := hann(metricFrameSize)
	var (
		centroids []float64
		highRatio []float64
		times     []float64
		flux      []float64
		prevMag   []float64
	)
	for start := 0; start+metricFrameSize <= len(samples); start += metricHop {
		re := make([]float64, metricFrameSize)
		im := make([]float64, metricFrameSize)
		for i := 0; i < metricFrameSize; i++ {
			re[i] = float64(samples[start+i]) * window[i]
		}
		fft(re, im)

		half := metricFrameSize / 2
		mag := make([]float64, half)
		var sumMag, sumWeighted, totalEnergy, highEnergy float64
		for k := 1; k < half; k++ {
			m := math.Hypot(re[k], im[k])
			mag[k] = m
			freq := float64(k) * rate / float64(metricFrameSize)
			sumMag += m
			sumWeighted += freq * m
			e := m * m
			totalEnergy += e
			if freq >= metricHighCutoffHz {
				highEnergy += e
			}
		}
		c := 0.0
		if sumMag > 0 {
			c = sumWeighted / sumMag
		}
		hr := 0.0
		if totalEnergy > 0 {
			hr = highEnergy / totalEnergy
		}
		centroids = append(centroids, c)
		highRatio = append(highRatio, hr)
		times = append(times, float64(start+metricFrameSize/2)/rate)

		if prevMag != nil {
			var f float64
			for k := 1; k < half; k++ {
				if d := mag[k] - prevMag[k]; d > 0 {
					f += d
				}
			}
			flux = append(flux, f)
		}
		prevMag = mag
	}
	if len(centroids) == 0 {
		return nil
	}

	out := []Metric{
		{
			Name:  "spectral_centroid_hz",
			Value: mean(centroids),
			Unit:  "Hz",
			Note:  "energy-weighted mean frequency over the whole recording",
		},
		{
			Name:  "high_band_energy_db",
			Value: meanDB(highRatio),
			Unit:  "dB",
			Note:  "energy at or above 6000 Hz, relative to the full band",
		},
		{
			Name:  "onset_rate_per_sec",
			Value: onsetRate(flux, duration),
			Unit:  "1/s",
			Note:  "spectral-flux peaks per second (attack density)",
		},
	}
	if lo, hi, ok := segmentRangeDB(highRatio, times, metricSegmentSec); ok {
		out = append(out, Metric{
			Name:  "high_band_segment_range_db",
			Value: hi - lo,
			Unit:  "dB",
			Note:  "spread of high-band level across 10 s segments (temporal change)",
		})
	}
	return out
}

// mono collapses interleaved samples to a single channel by averaging.
func mono(samples []float32, channels int) []float32 {
	if channels <= 1 {
		return samples
	}
	out := make([]float32, len(samples)/channels)
	for i := range out {
		var sum float32
		for c := 0; c < channels; c++ {
			sum += samples[i*channels+c]
		}
		out[i] = sum / float32(channels)
	}
	return out
}

// hann returns a periodic Hann window of length n.
func hann(n int) []float64 {
	w := make([]float64, n)
	for i := 0; i < n; i++ {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	return w
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// meanDB converts the mean of an energy-fraction series to decibels, guarding
// against log(0).
func meanDB(fracs []float64) float64 {
	if m := mean(fracs); m > 0 {
		return 10 * math.Log10(m)
	}
	return -300
}

// onsetRate counts spectral-flux peaks (prominent attacks) per second.
func onsetRate(flux []float64, duration float64) float64 {
	if len(flux) < 3 || duration <= 0 {
		return 0
	}
	m := mean(flux)
	var variance float64
	for _, f := range flux {
		d := f - m
		variance += d * d
	}
	sd := math.Sqrt(variance / float64(len(flux)))
	threshold := m + onsetFluxSigmas*sd
	count := 0
	for i := 1; i < len(flux)-1; i++ {
		if flux[i] > threshold && flux[i] >= flux[i-1] && flux[i] > flux[i+1] {
			count++
		}
	}
	return float64(count) / duration
}

// segmentRangeDB buckets the high-band ratio into fixed segments and returns the
// min and max segment level in dB, describing how much the high band varies
// across the recording.
func segmentRangeDB(highRatio, times []float64, segSec float64) (lo, hi float64, ok bool) {
	if len(highRatio) != len(times) || len(highRatio) == 0 || segSec <= 0 {
		return 0, 0, false
	}
	sums := map[int]float64{}
	counts := map[int]int{}
	for i, r := range highRatio {
		seg := int(times[i] / segSec)
		sums[seg] += r
		counts[seg]++
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	for seg, s := range sums {
		if counts[seg] == 0 {
			continue
		}
		db := -300.0
		if m := s / float64(counts[seg]); m > 0 {
			db = 10 * math.Log10(m)
		}
		lo = math.Min(lo, db)
		hi = math.Max(hi, db)
	}
	if math.IsInf(lo, 1) {
		return 0, 0, false
	}
	return lo, hi, true
}

// fft is an in-place iterative radix-2 Cooley-Tukey FFT. len(re) must be a power
// of two and equal to len(im).
func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wr, wi := math.Cos(ang), math.Sin(ang)
		half := length / 2
		for i := 0; i < n; i += length {
			cr, ci := 1.0, 0.0
			for k := 0; k < half; k++ {
				a, b := i+k, i+k+half
				tr := cr*re[b] - ci*im[b]
				ti := cr*im[b] + ci*re[b]
				re[b] = re[a] - tr
				im[b] = im[a] - ti
				re[a] += tr
				im[a] += ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}
