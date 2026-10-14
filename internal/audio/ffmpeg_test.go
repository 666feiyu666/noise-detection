package audio

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireFFmpeg skips the test when the ffmpeg/ffprobe binaries are unavailable.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
}

func TestProbeAndDecode(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sine.wav")

	// 2-second 440 Hz stereo sine.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-ac", "2", "-ar", "8000", "-c:a", "pcm_s16le", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test wav: %v: %s", err, out)
	}

	meta, err := Probe(ctx, path)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if meta.Channels != 2 || meta.SampleRate != 8000 {
		t.Fatalf("meta channels/rate = %d/%d, want 2/8000", meta.Channels, meta.SampleRate)
	}
	if meta.DurationSeconds < 1.9 || meta.DurationSeconds > 2.1 {
		t.Fatalf("duration = %v, want ~2", meta.DurationSeconds)
	}

	pcm, err := Decode(ctx, path, meta.SampleRate, meta.Channels)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if pcm.Channels != 2 || pcm.SampleRate != 8000 {
		t.Fatalf("pcm channels/rate = %d/%d, want 2/8000", pcm.Channels, pcm.SampleRate)
	}
	wantFrames := 2 * 8000
	if got := len(pcm.Samples) / pcm.Channels; got < wantFrames-1000 || got > wantFrames+1000 {
		t.Fatalf("decoded %d frames, want ~%d", got, wantFrames)
	}
}
