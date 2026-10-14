# Audio Defect Detector: Technical Design

**Core idea:** hybrid detection — deterministic **DSP** finds measurable
signal anomalies; an **LLM (Gemini)** judges perceptual defects; the two are
**fused** into one structured report. Decisions are computed in code, never by
the model.

> Aligned with the `noise-detection` repository.

## Architecture

```text
audio input
    |
    v
preprocessing ── ffprobe metadata + ffmpeg PCM decode
    |
    +-----------------------------+
    |                             |
    v                             v
DSP analysis                  LLM analysis
(measurable anomalies)        (perceptual defects)
    |                             |
    +-------------+---------------+
                  v
              fusion (dedup + label)
                  |
                  v
           report (JSON)
```

| Module | Package | Responsibility |
|---|---|---|
| Preprocessing | `internal/audio` | validate input; ffprobe metadata; ffmpeg PCM decode |
| DSP | `internal/detector` | clipping, silence/dropout, click/pop bursts, channel balance; spectral-balance and onset-envelope observations |
| LLM | `internal/gemini` | Gemini assessment via `google.golang.org/genai` (Files API, structured JSON), including timbre/perceptual dimensions |
| Fusion | `internal/fusion` | orchestrate stages, fuse DSP + LLM evidence, carry observations, compute the decision |
| Report | `internal/report` | the JSON contract; findings carry `source`, `status`, `dimension`, `severity`, `confidence`; reports carry `signal_observations` |

## Pipeline

1. **Preprocess** — validate the file; read metadata with ffprobe; decode to
   interleaved float32 PCM with ffmpeg.
2. **DSP** — flag candidate anomalies from signal statistics (thresholds give
   *candidates*, not verdicts), and attach spectral-balance and onset-envelope
   measurements as `signal_observations` (evidence, never a verdict).
3. **LLM** — assess the whole recording with Gemini for audible clicks/pops,
   harsh digital noise, dropouts, and abrupt discontinuities, and for timbre
   and perceptual quality (dullness, muffling, loss of clarity, loss of
   transients, smearing, harshness, distortion, codec artifacts). Ignore genre,
   mood, normal percussion, and declared intentional effects. Return `type`,
   `start`, `end`, `dimension`, `description`, `severity`, `confidence`.
4. **Fuse** — merge DSP candidates and LLM findings, labelling each by source
   and confidence, then compute the decision in application code.

Fusion rule:

| DSP | LLM | Result | `status` |
|---|---|---|---|
| detected | detected | confirmed | `confirmed` |
| detected | not | retain for review | `unconfirmed` |
| not | detected | suspected perceptual defect | `suspected` |
| not | not | no defect | — |
| failed | any | incomplete analysis | `error` report |

DSP and LLM both assess the whole recording.

Decision table (in effect today):

| Condition | Decision |
|---|---|
| no defects | `accept` |
| defects, attempt 1–2 | `regenerate` |
| defects, attempt 3 | `stop_limit_reached` |
| invalid input / API / response | `error` |

## Project structure

```text
noise-detection/
├── cmd/inspector/main.go
├── internal/{audio,detector,gemini,fusion,report}/
└── go.mod
```

## CLI

```bash
# detectors: rule (default, local placeholder), dsp, gemini, hybrid
# with no -report, each run writes reports/runN/report.json (N auto-increments)
inspector -request-id <id> -audio in.mp3 -detector dsp     [-report out.json]
inspector -request-id <id> -audio in.mp3 -detector gemini  [-report out.json]
inspector -request-id <id> -audio in.mp3 -detector hybrid  [-report out.json]   # DSP + Gemini fusion
```

`GEMINI_API_KEY` comes from the environment; `GEMINI_MODEL` selects the model
(default `gemini-3.8-flash`). Model, prompt version, and ffprobe duration are
recorded in every report.

## Output (JSON)

```json
{
  "request_id": "example-001",
  "assessment_status": "completed",
  "has_defect": true,
  "defects": [
    {
      "type": "clipping",
      "start_seconds": 42.10,
      "end_seconds": 42.30,
      "description": "Sustained near-full-scale samples indicate clipping.",
      "source": "both",
      "status": "confirmed"
    },
    {
      "type": "click_pop",
      "dimension": "click_pop",
      "start_seconds": 116.22,
      "end_seconds": 116.24,
      "description": "A cluster of abrupt waveform discontinuities suggests a click or pop.",
      "source": "dsp",
      "status": "unconfirmed",
      "severity": "medium",
      "confidence": "low"
    }
  ],
  "timestamps_are_approximate": true,
  "signal_observations": [
    {"name": "spectral_centroid_hz", "value": 660.5, "unit": "Hz"},
    {"name": "high_band_energy_db", "value": -36.9, "unit": "dB"}
  ],
  "decision": "regenerate",
  "reason": "An audible defect was detected; another attempt is allowed.",
  "audio_filename": "sample.mp3",
  "duration_seconds": 141.5,
  "model": "dsp+gemini-3.8-flash",
  "prompt_version": "dsp-v2+gemini-audio-v2",
  "assessed_at": "2026-01-01T00:00:00Z"
}
```

- `source` ∈ `dsp` | `gemini` | `both`; `status` ∈ `confirmed` | `unconfirmed` | `suspected`.
- Timbre findings also carry `dimension` (`dullness` | `muffling` | `loss_of_clarity` | `loss_of_transients` | `smearing` | `harshness` | `distortion` | `codec_artifacts` | an artifact type) plus `severity` and `confidence` (each `low` | `medium` | `high`); severity and confidence are distinct.
- `signal_observations` are supporting numeric measurements; they never appear in `defects` and never drive a decision on their own.
- A defect interval satisfies `0 <= start_seconds < end_seconds <= duration_seconds`.
- Error reports set `has_defect: null` and `decision: "error"`.
- LLM timestamps are approximate, not sample-accurate; DSP timestamps are exact
  sample positions.

## Principles

1. **DSP measures, LLM judges** — combine objective signal data with perceptual
   assessment; never let the model compute the decision.
2. **Evidence-based** — preserve each finding's source and confidence; an
   ambiguous sound alone is not a defect.
3. **Modular** — audio, detection, LLM, fusion, and reporting stay independent.
4. **Incremental** — CLI MVP first; add infrastructure only when needed.
