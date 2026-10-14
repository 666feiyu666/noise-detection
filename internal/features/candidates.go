package features

import (
	"fmt"
	"math"
	"sort"
)

// Params holds the tunable limits for local signal analysis. Levels are linear
// amplitudes in [0, 1]; a threshold identifies a candidate anomaly, not a
// confirmed defect.
type Params struct {
	ClipLevel        float64 // |sample| >= this counts as near full scale
	ClipMinRunFrames int     // shortest run of clipped frames reported
	ClipMergeSec     float64 // merge clipped runs separated by a shorter gap
	SilenceLevel     float64 // window RMS below this counts as silent
	SilenceWindowSec float64 // analysis window for silence
	MinSilenceSec    float64 // shortest silent run reported
	TransientDelta   float64 // |x[n]-x[n-1]| above this counts as an isolated discontinuity
	BurstDelta       float64 // |x[n]-x[n-1]| above this is a moderate jump that may cluster
	BurstWindowSec   float64 // half-window (s) searched around a sample for a jump cluster
	BurstMinHits     int     // moderate jumps within the window that make a burst a click/pop
	MinImbalanceDB   float64 // minimum channel RMS difference reported
}

// DefaultParams returns conservative starting parameters.
func DefaultParams() Params {
	return Params{
		ClipLevel:        0.999,
		ClipMinRunFrames: 3,
		ClipMergeSec:     0.005,
		SilenceLevel:     0.0003,
		SilenceWindowSec: 0.02,
		MinSilenceSec:    0.5,
		TransientDelta:   0.8,
		BurstDelta:       0.35,
		BurstWindowSec:   0.02,
		BurstMinHits:     5,
		MinImbalanceDB:   6,
	}
}

// candidates runs every signal check and returns the anomalous intervals,
// sorted by start time.
func candidates(samples []float32, channels, rate int, duration float64, p Params) []Candidate {
	var out []Candidate
	out = append(out, clipCandidates(samples, channels, rate, duration, p)...)
	out = append(out, silenceCandidates(samples, channels, rate, duration, p)...)
	out = append(out, transientCandidates(samples, channels, rate, duration, p)...)
	out = append(out, imbalanceCandidates(samples, channels, rate, duration, p)...)
	sort.Slice(out, func(i, j int) bool { return out[i].StartSeconds < out[j].StartSeconds })
	return out
}

func clipCandidates(samples []float32, channels, rate int, duration float64, p Params) []Candidate {
	minSamples := p.ClipMinRunFrames * channels
	if minSamples < channels {
		minSamples = channels
	}
	mergeGap := int(p.ClipMergeSec*float64(rate)) * channels

	// Collect runs of consecutive near-full-scale samples. A short isolated peak
	// is normal; a sustained run indicates clipping.
	var spans [][2]int
	for i := 0; i < len(samples); {
		if math.Abs(float64(samples[i])) < p.ClipLevel {
			i++
			continue
		}
		j := i
		for j < len(samples) && math.Abs(float64(samples[j])) >= p.ClipLevel {
			j++
		}
		if j-i >= minSamples {
			spans = append(spans, [2]int{i, j})
		}
		i = j
	}
	if len(spans) == 0 {
		return nil
	}

	// Merge runs separated by only a short gap.
	merged := [][2]int{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s[0]-last[1] <= mergeGap {
			last[1] = s[1]
		} else {
			merged = append(merged, s)
		}
	}

	out := make([]Candidate, 0, len(merged))
	for _, s := range merged {
		out = append(out, intervalCandidate(s[0], s[1], channels, rate, duration, 0.01,
			"clipping", "Sustained near-full-scale samples indicate clipping."))
	}
	return out
}

func silenceCandidates(samples []float32, channels, rate int, duration float64, p Params) []Candidate {
	win := int(p.SilenceWindowSec*float64(rate)) * channels
	if win < channels {
		win = channels
	}
	var out []Candidate
	runStart := -1
	flush := func(start, end int) {
		// Leading and trailing silence is normal; only interruptions inside the
		// recording are reported as dropout candidates.
		if start <= 0 || end >= len(samples) {
			return
		}
		c := intervalCandidate(start, end, channels, rate, duration, 0.01,
			"dropout", "An unexpected low-energy interval suggests silence or a dropout.")
		if c.EndSeconds-c.StartSeconds >= p.MinSilenceSec {
			out = append(out, c)
		}
	}
	for start := 0; start < len(samples); start += win {
		end := min(start+win, len(samples))
		var sum float64
		for i := start; i < end; i++ {
			sum += float64(samples[i]) * float64(samples[i])
		}
		quiet := math.Sqrt(sum/float64(end-start)) < p.SilenceLevel
		switch {
		case quiet && runStart < 0:
			runStart = start
		case !quiet && runStart >= 0:
			flush(runStart, start)
			runStart = -1
		}
	}
	flush(runStart, len(samples))
	return out
}

func transientCandidates(samples []float32, channels, rate int, duration float64, p Params) []Candidate {
	gap := int(0.005 * float64(rate))
	if gap < 1 {
		gap = 1
	}
	win := int(p.BurstWindowSec * float64(rate))
	if win < 1 {
		win = 1
	}
	minHits := p.BurstMinHits
	if minHits < 2 {
		minHits = 2
	}

	var out []Candidate
	for _, ch := range deinterleave(samples, channels) {
		// Per-sample jump magnitude, plus a prefix count of moderate jumps so a
		// sliding window's jump density can be read in O(1).
		diffs := make([]float64, len(ch))
		pref := make([]int, len(ch)+1)
		for i := 1; i < len(ch); i++ {
			diffs[i] = math.Abs(float64(ch[i] - ch[i-1]))
			pref[i+1] = pref[i]
			if diffs[i] >= p.BurstDelta {
				pref[i+1]++
			}
		}

		var hits []int
		for i := 1; i < len(ch); i++ {
			switch {
			case diffs[i] >= p.TransientDelta:
				// A single jump this large is a discontinuity on its own.
				hits = append(hits, i)
			case diffs[i] >= p.BurstDelta:
				// A moderate jump counts only inside a dense cluster, which
				// distinguishes a click/pop burst from a normal attack.
				lo, hi := i-win, i+win
				if lo < 1 {
					lo = 1
				}
				if hi > len(ch) {
					hi = len(ch)
				}
				if pref[hi]-pref[lo] >= minHits {
					hits = append(hits, i)
				}
			}
		}

		for i := 0; i < len(hits); {
			j := i
			for j+1 < len(hits) && hits[j+1]-hits[j] <= gap {
				j++
			}
			out = append(out, frameCandidate(hits[i]-1, hits[j], rate, duration, 0.002,
				"click_pop", "A cluster of abrupt waveform discontinuities suggests a click or pop."))
			i = j + 1
		}
	}
	return out
}

func imbalanceCandidates(samples []float32, channels, rate int, duration float64, p Params) []Candidate {
	if channels < 2 || duration <= 0 {
		return nil
	}
	chans := deinterleave(samples, channels)
	lo, hi := math.Inf(1), 0.0
	for _, ch := range chans {
		r := rms(ch)
		lo = math.Min(lo, r)
		hi = math.Max(hi, r)
	}
	if hi == 0 {
		return nil // fully silent, no imbalance to report
	}
	var db float64
	if lo <= 0 {
		db = math.Inf(1)
	} else {
		db = 20 * math.Log10(hi/lo)
	}
	if db < p.MinImbalanceDB {
		return nil
	}
	return []Candidate{{
		Type:         "channel_imbalance",
		StartSeconds: 0,
		EndSeconds:   duration,
		Description:  fmt.Sprintf("Channel RMS differs by %.1f dB.", db),
	}}
}

// deinterleave splits interleaved samples into one slice per channel.
func deinterleave(samples []float32, channels int) [][]float32 {
	out := make([][]float32, channels)
	for c := range out {
		out[c] = make([]float32, 0, len(samples)/channels+1)
	}
	for i, s := range samples {
		c := i % channels
		out[c] = append(out[c], s)
	}
	return out
}

func rms(ch []float32) float64 {
	if len(ch) == 0 {
		return 0
	}
	var sum float64
	for _, s := range ch {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(ch)))
}

// intervalCandidate converts interleaved sample indices to a clamped candidate.
func intervalCandidate(startIdx, endIdx, channels, rate int, duration, minWidth float64, typ, desc string) Candidate {
	return frameCandidate(startIdx/channels, endIdx/channels, rate, duration, minWidth, typ, desc)
}

// frameCandidate converts frame indices to a clamped candidate with end > start
// and end <= duration.
func frameCandidate(startFrame, endFrame, rate int, duration, minWidth float64, typ, desc string) Candidate {
	start := float64(startFrame) / float64(rate)
	end := float64(endFrame) / float64(rate)
	if duration > 0 && end > duration {
		end = duration
	}
	if end <= start {
		end = start + minWidth
	}
	if duration > 0 && end > duration {
		start = math.Max(0, duration-minWidth)
		end = duration
	}
	return Candidate{Type: typ, StartSeconds: start, EndSeconds: end, Description: desc}
}
