package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Metadata describes a probeable audio file.
type Metadata struct {
	DurationSeconds float64
	SampleRate      int
	Channels        int
	Codec           string
}

// FFmpeg is the default AudioSource backed by the ffprobe and ffmpeg binaries.
type FFmpeg struct{}

// Probe reads container and stream metadata with ffprobe.
func (FFmpeg) Probe(ctx context.Context, path string) (Metadata, error) {
	return Probe(ctx, path)
}

// Decode decodes the file to interleaved float32 PCM with ffmpeg.
func (FFmpeg) Decode(ctx context.Context, path string, sampleRate, channels int) (PCM, error) {
	return Decode(ctx, path, sampleRate, channels)
}

// Probe runs ffprobe and returns the first audio stream's metadata.
func Probe(ctx context.Context, path string) (Metadata, error) {
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		path,
	).Output()
	if err != nil {
		return Metadata{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var probe struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return Metadata{}, fmt.Errorf("parse ffprobe output: %w", err)
	}

	var meta Metadata
	if d, err := strconv.ParseFloat(strings.TrimSpace(probe.Format.Duration), 64); err == nil {
		meta.DurationSeconds = d
	}
	for _, s := range probe.Streams {
		if s.CodecType != "audio" {
			continue
		}
		meta.Codec = s.CodecName
		meta.Channels = s.Channels
		if r, err := strconv.Atoi(s.SampleRate); err == nil {
			meta.SampleRate = r
		}
		break
	}
	if meta.SampleRate == 0 || meta.Channels == 0 {
		return Metadata{}, fmt.Errorf("no audio stream found in %s", path)
	}
	return meta, nil
}
