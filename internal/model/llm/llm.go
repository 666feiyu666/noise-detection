// Package llm is the model-backed method of the modeling layer: it asks Google's
// Gemini to inspect the whole recording for clearly audible, unintended defects.
//
// It is one model among peers. It receives the features prepared by the
// data-preparation layer as context, and returns the model's structured
// findings; decisions are still computed in application code, and any API or
// parsing failure surfaces as an error report rather than a clean result.
package llm

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

	"github.com/666feiyu666/noise-detection/internal/features"
	"github.com/666feiyu666/noise-detection/internal/report"
)

// Environment variables the Gemini model reads.
const (
	EnvAPIKey = "GEMINI_API_KEY"
	EnvModel  = "GEMINI_MODEL"
)

// DefaultGeminiModel is used when GEMINI_MODEL is unset. It supports both audio
// input and structured (JSON schema) output. (gemini-2.5-flash was retired for
// new API keys; the API now points to this model.)
const DefaultGeminiModel = "gemini-3.8-flash"

// PromptVersion identifies the assessment prompt in reports.
const PromptVersion = "gemini-audio-v5"

// Default polling and retry bounds.
const (
	defaultPollInterval = 2 * time.Second
	defaultMaxWait      = 2 * time.Minute
	defaultMaxRetries   = 2
)

// Config configures a Gemini model.
type Config struct {
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

// Gemini assesses a recording with Google's Gemini API.
type Gemini struct {
	client       *genai.Client
	model        string
	pollInterval time.Duration
	maxWait      time.Duration
	maxRetries   int
}

// NewGemini resolves configuration and returns a ready model.
func NewGemini(ctx context.Context, cfg Config) (*Gemini, error) {
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
			return nil, fmt.Errorf("no API key: set %s or Config.APIKey", EnvAPIKey)
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

	return &Gemini{
		client:       client,
		model:        model,
		pollInterval: cfg.PollInterval,
		maxWait:      cfg.MaxWait,
		maxRetries:   cfg.MaxRetries,
	}, nil
}

// Name implements model.Model.
func (d *Gemini) Name() string { return d.model }

// Source implements model.Model.
func (d *Gemini) Source() string { return report.SourceGemini }

// Predict implements model.Model by uploading the file, assessing it with the
// prepared features as context, and parsing the structured response.
func (d *Gemini) Predict(ctx context.Context, in report.Input, prepared features.Set) (report.Assessment, error) {
	mimeType, err := audioMIMEType(in.AudioPath)
	if err != nil {
		return report.Assessment{}, err
	}

	uploaded, err := d.upload(ctx, in.AudioPath, mimeType)
	if err != nil {
		return report.Assessment{}, err
	}
	// Best-effort cleanup of the temporary remote upload, on every path.
	defer func() {
		_, _ = d.client.Files.Delete(context.WithoutCancel(ctx), uploaded.Name, nil)
	}()

	if err := d.waitActive(ctx, uploaded); err != nil {
		return report.Assessment{}, err
	}

	result, err := d.assess(ctx, uploaded, in, prepared)
	if err != nil {
		return report.Assessment{}, err
	}

	findings, err := result.findings()
	if err != nil {
		return report.Assessment{}, err
	}

	var duration *float64
	if result.DurationSeconds > 0 {
		duration = &result.DurationSeconds
	}

	return report.Assessment{
		Findings:        findings,
		Model:           d.model,
		PromptVersion:   PromptVersion,
		DurationSeconds: duration,
	}, nil
}

// upload sends the recording through the Files API, retrying transient errors.
func (d *Gemini) upload(ctx context.Context, path, mimeType string) (*genai.File, error) {
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
func (d *Gemini) waitActive(ctx context.Context, file *genai.File) error {
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
func (d *Gemini) assess(ctx context.Context, file *genai.File, in report.Input, prepared features.Set) (geminiResult, error) {
	var result geminiResult

	err := d.withRetry(ctx, "generateContent", func() error {
		resp, err := d.client.Models.GenerateContent(ctx, d.model, []*genai.Content{{
			Role: genai.RoleUser,
			Parts: []*genai.Part{
				{FileData: &genai.FileData{FileURI: file.URI, MIMEType: file.MIMEType}},
				{Text: buildPrompt(in, prepared)},
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
func (d *Gemini) withRetry(ctx context.Context, label string, op func() error) error {
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
	Dimension    string  `json:"dimension"`
	Description  string  `json:"description"`
	Severity     string  `json:"severity"`
	Confidence   string  `json:"confidence"`
}

// findings converts the model output to contract findings, rejecting the
// inconsistent shapes the schema is meant to prevent.
func (r geminiResult) findings() ([]report.Finding, error) {
	if r.HasDefect && len(r.Defects) == 0 {
		return nil, errors.New("model reported has_defect=true but returned no defects")
	}
	if !r.HasDefect && len(r.Defects) > 0 {
		return nil, errors.New("model reported has_defect=false but returned defects")
	}
	findings := make([]report.Finding, 0, len(r.Defects))
	for _, f := range r.Defects {
		findings = append(findings, report.Finding{
			StartSeconds: f.StartSeconds,
			EndSeconds:   f.EndSeconds,
			Type:         f.Type,
			Description:  f.Description,
			Dimension:    f.Dimension,
			Severity:     f.Severity,
			Confidence:   f.Confidence,
		})
	}
	return findings, nil
}

const systemInstruction = "You are a devoted music lover and high-end audio listener with an exceptionally critical ear. " +
	"You care above all about timbre and sound quality: faithful, refined tone delights you, while dull, veiled, " +
	"coarse, smeared, or degraded timbre bothers you immediately and you hold every recording to that standard. " +
	"You report clearly audible, unintended problems, including timbre and perceptual quality, " +
	"and never comment on artistic intent or musical taste; your high standards concern sound quality alone, " +
	"never genre, melody, harmony, or mood."

// buildPrompt assembles the assessment prompt for one candidate, including the
// deterministic features prepared upstream as context.
func buildPrompt(in report.Input, prepared features.Set) string {
	var b strings.Builder
	b.WriteString("Listen to the attached recording the way a demanding, timbre-obsessed listener would: ")
	b.WriteString("you hold a high standard for tone, care about colour, clarity, and texture, ")
	b.WriteString("and you notice at once when the sound is dull, veiled, coarse, smeared, or otherwise less than clean.\n\n")
	b.WriteString("Inspect the ENTIRE recording (the attached audio file) for clearly audible, unintended problems. ")
	b.WriteString("Look for isolated clicks or pops, harsh digital noise, broken or corrupted sound, ")
	b.WriteString("unnatural dropouts, and abrupt audio discontinuities.\n\n")
	b.WriteString("Also assess timbre and perceptual quality across the passage, not only isolated artifacts. ")
	b.WriteString("Consider these overlapping dimensions: dullness (missing upper-frequency detail relative to an appropriate comparison); ")
	b.WriteString("muffling (a veiled quality, as though the sound passes through a barrier); loss of clarity (reduced note definition or separation); ")
	b.WriteString("loss of transients (weakened attack detail); smearing (blurred attacks that make successive sounds less distinct); ")
	b.WriteString("harshness (unnaturally abrasive or piercing sound); distortion (breakup, roughness, or added tonal components); ")
	b.WriteString("and suspected compression artifacts (warbling, metallic texture, or blurred detail possibly from lossy encoding).\n\n")
	b.WriteString("State what you hear before suggesting any cause. A listening judgment alone must not assert that a codec, processing stage, or model caused the problem. ")
	b.WriteString("Do NOT judge genre, melody, harmony, mood, or personal preference. ")
	b.WriteString("Do NOT flag normal percussion transients, jazz dissonance, intentional pauses, ")
	b.WriteString("or deliberate production effects merely because they sound unusual. ")
	b.WriteString("A problem that spans the whole recording is a valid finding: use the full interval when it is not localized.\n\n")
	b.WriteString("Trust your ear above all: if any part of the sound makes the listening experience unpleasant — ")
	b.WriteString("grating, harsh, tiring, dull, veiled, muddy, smeared, thin, or otherwise uncomfortable to the ear — report it, ")
	b.WriteString("even when you are unsure of the cause or whether it is objectively a defect. ")
	b.WriteString("Do not withhold a finding merely because it is subtle, ambiguous, or you cannot prove it; ")
	b.WriteString("an unpleasant timbre alone is a valid reason to report.\n\n")
	b.WriteString("For each finding return: a start and end time, a defect type, the perceptual dimension it concerns ")
	b.WriteString("(click_pop, digital_noise, dropout, dullness, muffling, loss_of_clarity, loss_of_transients, smearing, harshness, distortion, or codec_artifacts), ")
	b.WriteString("a short description of the audible evidence, a severity (low, medium, or high), and a confidence (low, medium, or high). ")
	b.WriteString("Severity and confidence are separate: severity is how audible or impactful the problem is, confidence is how sure you are. ")
	b.WriteString("Times are seconds from the beginning of the original file, including any leading silence; they are estimates, not sample-accurate. ")
	b.WriteString("Set has_defect to false and return an empty defects list when no clearly supported problem is present. ")
	b.WriteString("Also report duration_seconds, your best estimate of the recording length in seconds.\n")

	// Deterministic context from the data-preparation layer. It is evidence to
	// weigh, not a verdict to copy.
	if len(prepared.Metrics) > 0 {
		b.WriteString("\nLocal signal measurements taken over the recording (context only; a reading is not a defect by itself):\n")
		for _, m := range prepared.Metrics {
			fmt.Fprintf(&b, "- %s = %.3f %s", m.Name, m.Value, m.Unit)
			if m.Note != "" {
				fmt.Fprintf(&b, " (%s)", m.Note)
			}
			b.WriteString("\n")
		}
	}
	if len(prepared.Candidates) > 0 {
		b.WriteString("Deterministic signal candidates found by local analysis (each may be a false positive; confirm or dismiss them by ear):\n")
		for _, c := range prepared.Candidates {
			fmt.Fprintf(&b, "- %.2f-%.2fs %s: %s\n", c.StartSeconds, c.EndSeconds, c.Type, c.Description)
		}
	}
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
				Description: "True if any part of the sound is unpleasant or uncomfortable to the ear, even if subtle or hard to attribute.",
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
						"start_seconds": {Type: genai.TypeNumber, Description: "Estimated problem start in seconds from the file start."},
						"end_seconds":   {Type: genai.TypeNumber, Description: "Estimated problem end in seconds from the file start."},
						"type":          {Type: genai.TypeString, Description: "Short category, e.g. click_pop, digital_noise, dropout, muffling, harshness."},
						"dimension":     {Type: genai.TypeString, Description: "Perceptual dimension: dullness, muffling, loss_of_clarity, loss_of_transients, smearing, harshness, distortion, codec_artifacts, or an artifact type such as click_pop."},
						"description":   {Type: genai.TypeString, Description: "Short description of the audible evidence, stating what is heard."},
						"severity":      {Type: genai.TypeString, Enum: []string{"low", "medium", "high"}, Description: "How audible or impactful the problem is."},
						"confidence":    {Type: genai.TypeString, Enum: []string{"low", "medium", "high"}, Description: "How certain the audible assessment is; not a probability."},
					},
					Required: []string{"start_seconds", "end_seconds", "type", "dimension", "description", "severity", "confidence"},
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
