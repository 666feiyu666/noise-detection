// Package validate checks the caller request and detector response before they
// flow onward. Validation failures must surface as errors, never as a clean
// result.
package validate

import (
	"fmt"
	"os"

	"github.com/666feiyu666/noise-detection/internal/contract"
)

// InputOptions controls input validation.
type InputOptions struct {
	// RequireAudioFile enforces that AudioPath points to an existing regular
	// file. Set it false to run the pipeline on metadata only.
	RequireAudioFile bool
}

// Input validates the caller request. It does not read or decode audio.
func Input(in contract.Input, opts InputOptions) error {
	if in.RequestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if in.GenerationAttempt < contract.MinGenerationAttempt || in.GenerationAttempt > contract.MaxGenerationAttempt {
		return fmt.Errorf("generation_attempt must be between %d and %d, got %d",
			contract.MinGenerationAttempt, contract.MaxGenerationAttempt, in.GenerationAttempt)
	}
	if in.AudioPath == "" {
		return fmt.Errorf("audio_path is required")
	}
	if in.DurationSeconds != nil && *in.DurationSeconds <= 0 {
		return fmt.Errorf("duration_seconds must be positive, got %v", *in.DurationSeconds)
	}
	if opts.RequireAudioFile {
		info, err := os.Stat(in.AudioPath)
		if err != nil {
			return fmt.Errorf("audio_path is not readable: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("audio_path must be a file, got a directory")
		}
		if info.Size() == 0 {
			return fmt.Errorf("audio_path is empty")
		}
	}
	return nil
}
