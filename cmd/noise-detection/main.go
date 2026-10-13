// Command noise-detection runs one audio-defect assessment.
//
// This is the framework stage: it validates input, runs a local rule detector
// (no model calls, no audio decoding), validates the response, computes a
// decision, and emits a JSON report to stdout or a file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/666feiyu666/noise-detection/internal/contract"
	"github.com/666feiyu666/noise-detection/internal/detect"
	"github.com/666feiyu666/noise-detection/internal/pipeline"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		requestID     = flag.String("request-id", "", "stable identifier shared by all candidates of one request (required)")
		audioPath     = flag.String("audio", "", "path to the candidate MP3/WAV file (required)")
		attempt       = flag.Int("generation-attempt", 1, "generation attempt, 1..3")
		effects       = flag.String("intentional-effects", "", "optional description of deliberate effects")
		reportPath    = flag.String("report", "", "optional JSON report destination; stdout when empty")
		rulesPath     = flag.String("rules", "", "optional path to a JSON rules file for the local rule detector")
		skipFileCheck = flag.Bool("skip-file-check", false, "do not require the audio file to exist")
		maxAttempts   = flag.Int("max-attempts", contract.MaxGenerationAttempt, "generation limit")
	)
	flag.Parse()

	var detector detect.Detector = detect.NewRuleDetector()
	if *rulesPath != "" {
		d, err := detect.NewRuleDetectorFromFile(*rulesPath)
		if err != nil {
			return err
		}
		detector = d
	}

	p := pipeline.New(pipeline.Config{
		Detector:         detector,
		MaxAttempts:      *maxAttempts,
		RequireAudioFile: !*skipFileCheck,
	})

	in := contract.Input{
		RequestID:          *requestID,
		GenerationAttempt:  *attempt,
		AudioPath:          *audioPath,
		IntentionalEffects: *effects,
		ReportPath:         *reportPath,
	}

	report, runErr := p.Run(context.Background(), in)
	if err := writeReport(report, *reportPath); err != nil {
		return err
	}
	if runErr != nil {
		return fmt.Errorf("assessment reported an error: %w", runErr)
	}
	return nil
}

func writeReport(r contract.Report, path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
