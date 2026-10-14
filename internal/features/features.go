// Package features is the data-preparation layer of the pipeline (CRISP-DM:
// Data Preparation).
//
// It decodes one candidate exactly once and extracts the deterministic signal
// features and candidate anomaly intervals that every model then consumes.
// Nothing here is a verdict: measurements and candidates are prepared data,
// produced by signal analysis rather than by a judgement, and it is the
// modeling layer that decides what they mean.
package features

import (
	"context"

	"github.com/666feiyu666/noise-detection/internal/audio"
)

// AudioSource supplies metadata and PCM for a candidate. The default
// implementation shells out to ffprobe and ffmpeg.
type AudioSource interface {
	Probe(ctx context.Context, path string) (audio.Metadata, error)
	Decode(ctx context.Context, path string, sampleRate, channels int) (audio.PCM, error)
}

// DefaultSource is the AudioSource used when a caller supplies none.
var DefaultSource AudioSource = audio.FFmpeg{}

// Metric is one named, numeric feature measured over a recording. A metric is
// context, not a verdict: it never rejects a recording on its own.
type Metric struct {
	Name  string
	Value float64
	Unit  string
	Note  string
}

// Candidate is one interval of the signal that looks anomalous, before any
// model decides whether it is an audible defect.
type Candidate struct {
	Type         string
	StartSeconds float64
	EndSeconds   float64
	Description  string
}

// Set is the prepared data for one candidate: measurements, a stable numeric
// feature vector, and the candidate anomaly intervals.
type Set struct {
	DurationSeconds float64
	SampleRate      int
	Channels        int

	// Metrics are the named measurements taken over the whole recording.
	Metrics []Metric

	// Vector is a stable-order numeric representation for machine-learning
	// models; VectorNames labels each element.
	Vector      []float64
	VectorNames []string

	// Candidates are the anomalous intervals found by signal analysis, sorted
	// by start time.
	Candidates []Candidate
}

// Extract probes and decodes one candidate, then prepares its features. The
// decode happens here, once, so no model repeats it.
func Extract(ctx context.Context, src AudioSource, path string) (Set, error) {
	if src == nil {
		src = DefaultSource
	}
	meta, err := src.Probe(ctx, path)
	if err != nil {
		return Set{}, err
	}
	pcm, err := src.Decode(ctx, path, meta.SampleRate, meta.Channels)
	if err != nil {
		return Set{}, err
	}

	duration := meta.DurationSeconds
	if duration <= 0 && pcm.SampleRate > 0 && pcm.Channels > 0 {
		duration = float64(len(pcm.Samples)) / float64(pcm.SampleRate*pcm.Channels)
	}
	return ExtractPCM(pcm, duration, DefaultParams()), nil
}

// ExtractPCM prepares features from decoded PCM. It is pure, so it is
// straightforward to test.
func ExtractPCM(pcm audio.PCM, duration float64, params Params) Set {
	set := Set{DurationSeconds: duration, SampleRate: pcm.SampleRate, Channels: pcm.Channels}
	if pcm.SampleRate <= 0 || pcm.Channels <= 0 || len(pcm.Samples) == 0 {
		return set
	}
	set.Candidates = candidates(pcm.Samples, pcm.Channels, pcm.SampleRate, duration, params)
	set.Metrics = metrics(pcm, duration)
	set.Vector, set.VectorNames = featureVector(set.Metrics, set.Candidates)
	return set
}

// candidateTypes fixes the order candidate counts appear in the feature vector.
var candidateTypes = []string{"clipping", "dropout", "click_pop", "channel_imbalance"}

// featureVector flattens the measurements and per-type candidate counts into a
// stable-order numeric vector for machine-learning models.
func featureVector(ms []Metric, cs []Candidate) ([]float64, []string) {
	names := make([]string, 0, len(ms)+len(candidateTypes))
	values := make([]float64, 0, len(ms)+len(candidateTypes))
	for _, m := range ms {
		names = append(names, m.Name)
		values = append(values, m.Value)
	}
	for _, t := range candidateTypes {
		n := 0
		for _, c := range cs {
			if c.Type == t {
				n++
			}
		}
		names = append(names, "candidate_count_"+t)
		values = append(values, float64(n))
	}
	return values, names
}
