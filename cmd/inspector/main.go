// Command inspector runs one audio-defect assessment.
//
// It validates input, runs a detector (local rule, DSP, Gemini, or a hybrid
// DSP+Gemini fusion), validates the response, computes a decision, and emits a
// JSON report to stdout or a file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/666feiyu666/noise-detection/internal/detector"
	"github.com/666feiyu666/noise-detection/internal/fusion"
	"github.com/666feiyu666/noise-detection/internal/gemini"
	"github.com/666feiyu666/noise-detection/internal/report"
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
		detectorName  = flag.String("detector", "rule", "detector: rule (local), dsp (signal analysis), gemini (model-backed), or hybrid (dsp+gemini fusion)")
		geminiModel   = flag.String("gemini-model", "", "Gemini model id; overrides GEMINI_MODEL and the default")
		skipFileCheck = flag.Bool("skip-file-check", false, "do not require the audio file to exist")
		maxAttempts   = flag.Int("max-attempts", report.MaxGenerationAttempt, "generation limit")
	)
	flag.Parse()

	ctx := context.Background()

	primary, verifier, err := buildDetectors(ctx, *detectorName, *rulesPath, *geminiModel)
	if err != nil {
		return err
	}

	p := fusion.New(fusion.Config{
		Detector:         primary,
		Verifier:         verifier,
		MaxAttempts:      *maxAttempts,
		RequireAudioFile: !*skipFileCheck,
	})

	in := report.Input{
		RequestID:          *requestID,
		GenerationAttempt:  *attempt,
		AudioPath:          *audioPath,
		IntentionalEffects: *effects,
		ReportPath:         *reportPath,
	}

	rep, runErr := p.Run(ctx, in)
	if err := writeReport(rep, *reportPath); err != nil {
		return err
	}
	if runErr != nil {
		return fmt.Errorf("assessment reported an error: %w", runErr)
	}
	return nil
}

// buildDetectors selects the detector(s) named on the command line. The second
// return value is the optional verifier used for hybrid fusion.
func buildDetectors(ctx context.Context, name, rulesPath, geminiModel string) (detector.Detector, detector.Detector, error) {
	switch name {
	case "", "rule":
		if rulesPath != "" {
			d, err := detector.NewRuleDetectorFromFile(rulesPath)
			return d, nil, err
		}
		return detector.NewRuleDetector(), nil, nil
	case "dsp":
		return detector.NewDSPDetector(detector.DSPConfig{}), nil, nil
	case "gemini":
		g, err := gemini.NewGeminiDetector(ctx, gemini.GeminiConfig{Model: geminiModel})
		return g, nil, err
	case "hybrid":
		g, err := gemini.NewGeminiDetector(ctx, gemini.GeminiConfig{Model: geminiModel})
		if err != nil {
			return nil, nil, err
		}
		return detector.NewDSPDetector(detector.DSPConfig{}), g, nil
	default:
		return nil, nil, fmt.Errorf("unknown detector %q: use rule, dsp, gemini, or hybrid", name)
	}
}

func writeReport(r report.Report, path string) error {
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
