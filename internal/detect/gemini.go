// Gemini-backed Detector.
//
// This detector uploads the candidate recording through Google's Files API,
// asks Gemini to inspect the whole recording for clearly audible, unintended
// defects, and returns the model's structured findings. It implements the
// detect.Detector interface so the pipeline is unchanged: decisions are still
// computed in application code, and any API or parsing failure surfaces as an
// error report rather than a clean result.
package detect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/666feiyu666/noise-detection/internal/contract"
)

// Environment variables the Gemini detector reads.
const (
	EnvAPIKey = "GEMINI_API_KEY"
	EnvModel  = "GEMINI_MODEL"
)

// DefaultGeminiModel is used when GEMINI_MODEL is unset. It supports both audio
// input and structured (JSON schema) output. (gemini-2.5-flash was retired for
// new API keys; the API now points to this model.)
const DefaultGeminiModel = "gemini-3.8-flash"

// GeminiPromptVersion identifies the assessment prompt in reports.
const GeminiPromptVersion = "gemini-audio-v1"

// Default polling and retry bounds.
const (
	defaultPollInterval = 2 * time.Second
	defaultMaxWait      = 2 * time.Minute
	defaultMaxRetries   = 2
)

// GeminiConfig configures a GeminiDetector.
type GeminiConfig struct {
	// APIKey overrides the GEMINI_API_KEY environment variable.
	APIKey string
	// Model overrides the GEMINI_MODEL environment variable and the default.
	Model string
	// Client injects an already-built client (for tests). When nil, a client is
	// created from the resolved API key.
	Client *genai.Client
	// PollInterval and MaxWait bound how long to wait for an uploaded file to
	// become ACTIVE. Zero values select defaults.
	PollInterval time.Duration
	MaxWait      time.Duration
	// MaxRetries bounds transient API retries (in addition to the first call).
	// Negative values disable retries; zero selects the default.
	MaxRetries int
}

// GeminiDetector assesses a recording with Google's Gemini API.
type GeminiDetector struct {
	client       *genai.Client
	model        string
	pollInterval time.Duration
	maxWait      time.Duration
	maxRetries   int
}

// NewGeminiDetector resolves configuration and returns a ready detector.
func NewGeminiDetector(ctx context.Context, cfg GeminiConfig) (*GeminiDetector, error) {
	model := firstNonEmpty(cfg.Model, os.Getenv(EnvModel), DefaultGeminiModel)

	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = defaultMaxWait
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}

	client := cfg.Client
	if client == nil {
		apiKey := firstNonEmpty(cfg.APIKey, os.Getenv(EnvAPIKey))
		if apiKey == "" {
			return nil, fmt.Errorf("no API key: set %s or GeminiConfig.APIKey", EnvAPIKey)
		}
		c, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			return nil, fmt.Errorf("create gemini client: %w", err)
		}
		client = c
	}

	return &GeminiDetector{
		client:       client,
		model:        model,
		pollInterval: cfg.PollInterval,
		maxWait:      cfg.MaxWait,
		maxRetries:   cfg.MaxRetries,
	}, nil
}

// Name implements Detector.
func (d *GeminiDetector) Name() string { return d.model }

// Detect implements Detector by uploading the file, assessing it, and parsing
// the structured response.
func (d *GeminiDetector) Detect(ctx context.Context, in contract.Input) (contract.Assessment, error) {
	mimeType, err := audioMIMEType(in.AudioPath)
	if err != nil {
		return contract.Assessment{}, err
	}

	uploaded, err := d.upload(ctx, in.AudioPath, mimeType)
	if err != nil {
		return contract.Assessment{}, err
	}
	// Best-effort cleanup of the temporary remote upload, on every path.
	defer func() {
		_, _ = d.client.Files.Delete(context.WithoutCancel(ctx), uploaded.Name, nil)
	}()

	if err := d.waitActive(ctx, uploaded); err != nil {
		return contract.Assessment{}, err
	}

	result, err := d.assess(ctx, uploaded, in)
	if err != nil {
		return contract.Assessment{}, err
	}

	findings, err := result.findings()
	if err != nil {
		return contract.Assessment{}, err
	}

	var duration *float64
	if result.DurationSeconds > 0 {
		duration = &result.DurationSeconds
	}

	return contract.Assessment{
		Findings:        findings,
		Model:           d.model,
		PromptVersion:   GeminiPromptVersion,
		DurationSeconds: duration,
	}, nil
}

// upload sends the recording through the Files API, retrying transient errors.
func (d *GeminiDetector) upload(ctx context.Context, path, mimeType string) (*genai.File, error) {
	var file *genai.File
	err := d.withRetry(ctx, "upload", func() error {
		f, err := d.client.Files.UploadFromPath(ctx, path, &genai.UploadFileConfig{MIMEType: mimeType})
		if err != nil {
			return err
		}
		file = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return file, nil
}

// waitActive polls until the uploaded file reaches the ACTIVE state.
func (d *GeminiDetector) waitActive(ctx context.Context, file *genai.File) error {
	deadline := time.Now().Add(d.maxWait)
	for {
		switch file.State {
		case genai.FileStateActive:
			return nil
		case genai.FileStateFailed:
			return fmt.Errorf("uploaded file processing failed: %s", fileError(file))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("uploaded file did not become active within %s (state %q)", d.maxWait, file.State)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d.pollInterval):
		}
		updated, err := d.client.Files.Get(ctx, file.Name, nil)
		if err != nil {
			return fmt.Errorf("poll uploaded file: %w", err)
		}
		file = updated
	}
}

// assess sends the complete recording with the assessment prompt and response
// schema, and parses the model's structured output.
func (d *GeminiDetector) assess(ctx context.Context, file *genai.File, in contract.Input) (geminiResult, error) {
	var result geminiResult

	err := d.withRetry(ctx, "generateContent", func() error {
		resp, err := d.client.Models.GenerateContent(ctx, d.model, []*genai.Content{{
			Role: genai.RoleUser,
			Parts: []*genai.Part{
				{FileData: &genai.FileData{FileURI: file.URI, MIMEType: file.MIMEType}},
				{Text: buildPrompt(in)},
			},
		}}, &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: systemInstruction}}},
			Temperature:       genai.Ptr[float32](0),
			ResponseMIMEType:  "application/json",
			ResponseSchema:    responseSchema(),
		})
		if err != nil {
			return err
		}
		text := resp.Text()
		if strings.TrimSpace(text) == "" {
			return errors.New("model returned an empty response")
		}
		var parsed geminiResult
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			return fmt.Errorf("parse structured response: %w", err)
		}
		result = parsed
		return nil
	})
	if err != nil {
		return geminiResult{}, err
	}
	return result, nil
}

// withRetry runs op, retrying only failures that look transient.
func (d *GeminiDetector) withRetry(ctx context.Context, label string, op func() error) error {
	var lastErr error
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		if err := op(); err != nil {
			lastErr = err
			if !isTransient(err) {
				return err
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("%s failed after %d attempts: %w", label, d.maxRetries+1, lastErr)
}

// geminiResult is the model's structured output.
type geminiResult struct {
	HasDefect       bool            `json:"has_defect"`
	DurationSeconds float64         `json:"duration_seconds"`
	Defects         []geminiFinding `json:"defects"`
}

type geminiFinding struct {
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Type         string  `json:"type"`
	Description  string  `json:"description"`
}

// findings converts the model output to contract findings, rejecting the
// inconsistent shapes the schema is meant to prevent.
func (r geminiResult) findings() ([]contract.Finding, error) {
	if r.HasDefect && len(r.Defects) == 0 {
		return nil, errors.New("model reported has_defect=true but returned no defects")
	}
	if !r.HasDefect && len(r.Defects) > 0 {
		return nil, errors.New("model reported has_defect=false but returned defects")
	}
	findings := make([]contract.Finding, 0, len(r.Defects))
	for _, f := range r.Defects {
		findings = append(findings, contract.Finding{
			StartSeconds: f.StartSeconds,
			EndSeconds:   f.EndSeconds,
			Type:         f.Type,
			Description:  f.Description,
		})
	}
	return findings, nil
}

const systemInstruction = "You are an expert audio-quality inspector for generated music. " +
	"You report only clearly audible, unintended defects and never comment on artistic intent."

// buildPrompt assembles the assessment prompt for one candidate.
func buildPrompt(in contract.Input) string {
	var b strings.Builder
	b.WriteString("Inspect the ENTIRE recording (the attached audio file) for clearly audible, unintended defects. ")
	b.WriteString("Look specifically for: isolated clicks or pops, harsh digital noise, broken or corrupted sound, ")
	b.WriteString("unnatural dropouts, and abrupt audio discontinuities.\n\n")
	b.WriteString("Do NOT judge genre, melody, harmony, mood, or personal preference. ")
	b.WriteString("Do NOT flag normal percussion transients, jazz dissonance, intentional pauses, ")
	b.WriteString("or deliberate production effects merely because they sound unusual. ")
	b.WriteString("Report only clearly supported defects; an ambiguous sound alone is not a defect.\n\n")
	b.WriteString("For each defect return a start time, an end time, a defect type, and a short description of the audible evidence. ")
	b.WriteString("Times are seconds from the beginning of the original file, including any leading silence; they are estimates, not sample-accurate. ")
	b.WriteString("Set has_defect to false and return an empty defects list when no clear defect is present. ")
	b.WriteString("Also report duration_seconds, your best estimate of the recording length in seconds.\n")
	if in.IntentionalEffects != "" {
		fmt.Fprintf(&b, "\nThe creator declared these intentional effects; do not treat them as defects: %s\n", in.IntentionalEffects)
	}
	return b.String()
}

// responseSchema describes the structured output the model must produce.
func responseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"has_defect": {
				Type:        genai.TypeBoolean,
				Description: "True only if at least one clearly audible, unintended defect is present.",
			},
			"duration_seconds": {
				Type:        genai.TypeNumber,
				Description: "Estimated length of the recording in seconds.",
			},
			"defects": {
				Type:        genai.TypeArray,
				Description: "Clearly audible defects; empty when has_defect is false.",
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"start_seconds": {Type: genai.TypeNumber, Description: "Estimated defect start in seconds from the file start."},
						"end_seconds":   {Type: genai.TypeNumber, Description: "Estimated defect end in seconds from the file start."},
						"type":          {Type: genai.TypeString, Description: "Short defect category, e.g. click_pop, digital_noise, dropout."},
						"description":   {Type: genai.TypeString, Description: "Short description of the audible evidence."},
					},
					Required: []string{"start_seconds", "end_seconds", "type", "description"},
				},
			},
		},
		Required: []string{"has_defect", "duration_seconds", "defects"},
	}
}

// audioMIMEType maps a supported audio extension to the MIME type the API
// expects, rejecting anything that is not MP3 or WAV.
func audioMIMEType(path string) (string, error) {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".mp3":
		return "audio/mp3", nil
	case ".wav":
		return "audio/wav", nil
	default:
		return "", fmt.Errorf("unsupported audio format %q: only .mp3 and .wav are accepted", ext)
	}
}

// isTransient reports whether an error is worth retrying.
func isTransient(err error) bool {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	// Network-level failures surface as generic errors; retry those.
	return true
}

func fileError(f *genai.File) string {
	if f != nil && f.Error != nil && f.Error.Message != "" {
		return f.Error.Message
	}
	return "unknown error"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
