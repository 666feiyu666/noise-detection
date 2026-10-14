package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"strconv"
)

// PCM is interleaved float32 audio in the range [-1, 1].
type PCM struct {
	Samples    []float32
	Channels   int
	SampleRate int
}

// Decode decodes the file to interleaved little-endian float32 PCM with ffmpeg.
// When sampleRate or channels is zero the source value is used.
func Decode(ctx context.Context, path string, sampleRate, channels int) (PCM, error) {
	args := []string{"-v", "error", "-i", path, "-f", "f32le", "-acodec", "pcm_f32le"}
	if sampleRate > 0 {
		args = append(args, "-ar", strconv.Itoa(sampleRate))
	}
	if channels > 0 {
		args = append(args, "-ac", strconv.Itoa(channels))
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return PCM{}, fmt.Errorf("ffmpeg decode %s: %w: %s", path, err, bytes.TrimSpace(stderr.Bytes()))
	}

	data := stdout.Bytes()
	n := len(data) / 4
	if n == 0 {
		return PCM{}, fmt.Errorf("ffmpeg produced no audio for %s", path)
	}
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return PCM{Samples: samples, Channels: max(channels, 1), SampleRate: sampleRate}, nil
}
