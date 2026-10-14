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
	"path/filepath"
	"strconv"
	"strings"

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
		requestIDs    stringList
		audioPaths    stringList
		attempt       = flag.Int("generation-attempt", 1, "generation attempt, 1..3")
		effects       = flag.String("intentional-effects", "", "optional description of deliberate effects")
		reportPath    = flag.String("report", "", "optional JSON report destination for a single audio file")
		reportsDir    = flag.String("reports-dir", "reports", "base directory for automatic per-run report directories")
		rulesPath     = flag.String("rules", "", "optional path to a JSON rules file for the local rule detector")
		detectorName  = flag.String("detector", "rule", "detector: rule (local), dsp (signal analysis), gemini (model-backed), or hybrid (dsp+gemini fusion)")
		geminiModel   = flag.String("gemini-model", "", "Gemini model id; overrides GEMINI_MODEL and the default")
		skipFileCheck = flag.Bool("skip-file-check", false, "do not require the audio file to exist")
		maxAttempts   = flag.Int("max-attempts", report.MaxGenerationAttempt, "generation limit")
	)
	flag.Var(&requestIDs, "request-id", "stable identifier for one candidate; repeat, once per -audio (required)")
	flag.Var(&audioPaths, "audio", "path to a candidate MP3/WAV file; repeat to assess several candidates in one run (required)")
	flag.Parse()

	ctx := context.Background()

	// One CLI invocation is one run: it assesses every candidate together and
	// writes them into a single numbered run directory.
	if len(audioPaths) == 0 {
		return fmt.Errorf("at least one -audio is required")
	}
	if len(requestIDs) != len(audioPaths) {
		return fmt.Errorf("got %d -request-id and %d -audio; provide one -request-id per -audio", len(requestIDs), len(audioPaths))
	}
	multi := len(audioPaths) > 1
	if multi && *reportPath != "" {
		return fmt.Errorf("-report names a single file; omit it when assessing several audio files")
	}

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

	// The run directory is created once per invocation, so runs never overwrite
	// one another. An explicit -report path (single audio only) overrides it.
	dest := *reportPath
	var runDir string
	if dest == "" {
		dir, err := nextRunDir(*reportsDir)
		if err != nil {
			return err
		}
		runDir = dir
		if !multi {
			dest = filepath.Join(dir, "report.json")
		}
	}

	var writeErr, runErr error
	for i, audio := range audioPaths {
		reportDest := dest
		if reportDest == "" {
			reportDest = filepath.Join(runDir, reportFileName(requestIDs[i]))
		}
		in := report.Input{
			RequestID:          requestIDs[i],
			GenerationAttempt:  *attempt,
			AudioPath:          audio,
			IntentionalEffects: *effects,
			ReportPath:         *reportPath,
		}
		rep, runErrI := p.Run(ctx, in)
		if err := writeReport(rep, reportDest); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			if writeErr == nil {
				writeErr = err
			}
			continue
		}
		fmt.Fprintln(os.Stderr, "report written to", reportDest)
		if runErrI != nil && runErr == nil {
			runErr = runErrI
		}
	}
	if writeErr != nil {
		return writeErr
	}
	if runErr != nil {
		return fmt.Errorf("assessment reported an error: %w", runErr)
	}
	return nil
}

// stringList collects a repeatable string flag in order.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// reportFileName derives a filesystem-safe report name from a request id, used
// when one run assesses several candidates.
func reportFileName(requestID string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, requestID)
	safe = strings.Trim(safe, ".")
	if safe == "" {
		safe = "report"
	}
	return safe + ".json"
}

// nextRunDir creates and returns the next numbered run directory under base
// (reports/run1, reports/run2, ...). Numbering continues past the highest
// existing runN so a run is never overwritten.
func nextRunDir(base string) (string, error) {
	entries, err := os.ReadDir(base)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read reports dir: %w", err)
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "run") {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), "run")); err == nil && n > max {
			max = n
		}
	}
	dir := filepath.Join(base, fmt.Sprintf("run%d", max+1))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create run dir: %w", err)
	}
	return dir, nil
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
