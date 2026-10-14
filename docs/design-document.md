# Audio Defect Detector: Technical Design

**Core idea:** the project follows the CRISP-DM lifecycle. Audio is understood
and prepared once (decode + deterministic signal features), one or more
**models** turn those prepared features into findings, an **evaluation** layer
aggregates the evidence and decides, and the CLI deploys the result as a JSON
report. Decisions are computed in code, never by a model.

> Aligned with the `noise-detection` repository.

## Layering: a base layer and the methods above it

The signal layer and the judgment layer are deliberately separate:

- **Base layer (Data Preparation, `internal/features`)** — pure signal work:
  decode once, measure the signal, extract candidate anomaly intervals. It
  produces *data*, not verdicts.
- **Methods (Modeling, `internal/model`)** — every detection method decides what
  the prepared data means. The deterministic **DSP** decision, a local **rule**
  pass, a future **ML** model, and an **LLM** (Gemini) are peers here; none of
  them re-reads the signal, and none is privileged over another.

This is why DSP appears in two places: its *measurement* is the base layer, its
*decision* is one method among the rest.

## CRISP-DM phases

| Phase | Artifact | Responsibility |
|---|---|---|
| Business Understanding | `docs/` | the objective, the report contract, and the decision policy |
| Data Understanding | `internal/audio` | validate the caller request; probe metadata; decode to PCM |
| **Data Preparation** | `internal/features` | decode once; measure, extract candidate intervals, build a stable feature vector |
| **Modeling** | `internal/model` (+ `model/{rule,dsp,llm}`) | the `Model` contract, a registry, and the detection methods |
| Evaluation | `internal/evaluation` | aggregate evidence by source, validate the response, compute the decision |
| Deployment | `cmd/inspector`, `internal/report` | the CLI entry point and the JSON report contract |

```text
Business Understanding ── docs: contract + decision policy
        |
Data Understanding ────── internal/audio       validate · ffprobe · ffmpeg PCM
        |
Data Preparation ──────── internal/features    decode once · metrics · candidates · feature vector
        |
Modeling ──────────────── internal/model       rule | dsp | llm  (any combination, shared features)
        |
Evaluation ────────────── internal/evaluation  aggregate evidence by source · validate · decide
        |
Deployment ────────────── cmd/inspector + internal/report   JSON report
```

## Phase detail

### Data Understanding (`internal/audio`)

Validate the caller request (request id, attempt, path, optional duration) and
read the file: `ffprobe` for metadata, `ffmpeg` for interleaved float32 PCM. The
original file is preserved; nothing is denoised, normalised, or trimmed.

### Data Preparation (`internal/features`)

`Extract` decodes the candidate **once** and returns a `Set`:

- `Metrics` — named measurements over the whole recording (spectral centroid,
  high-band energy, onset rate, high-band segment range).
- `Candidates` — anomalous intervals from signal statistics (clipping,
  silence/dropout, click/pop bursts, channel imbalance). A threshold yields a
  *candidate*, never a verdict.
- `Vector` / `VectorNames` — a stable-order numeric representation, ready for a
  machine-learning model.

Every model consumes this one prepared `Set`; no model repeats the decode.

### Modeling (`internal/model`)

A model is anything implementing `model.Model`:

```go
Predict(ctx context.Context, in report.Input, prepared features.Set) (report.Assessment, error)
```

| Model | Package | Reads | Method |
|---|---|---|---|
| `rule` | `internal/model/rule` | the filename | local heuristic (placeholder) |
| `dsp` | `internal/model/dsp` | `prepared.Candidates` | deterministic: one finding per candidate |
| `llm` | `internal/model/llm` | the audio + `prepared` features | Gemini, structured JSON (perceptual + artifact dimensions) |

The registry maps names to factories; the CLI selects any combination by
repeating `-detector`. Adding an ML model means implementing `Model` and
registering a factory — the pipeline is unchanged. The LLM receives the prepared
metrics and candidates as context, so it reasons on top of the base layer rather
than in isolation.

### Evaluation (`internal/evaluation`)

Aggregate the findings into events. Overlapping findings from *different*
sources join one event: the first source supplies its type and description and
every observing source is recorded in `sources`; same-source findings are never
merged. Status follows from the sources:

| Sources observing the event | `status` | Meaning |
|---|---|---|
| two or more | `confirmed` | models agree on the event |
| exactly one | `unconfirmed` | a single model observed it |
| none | — | no defect |

Then validate the response (`0 <= start < end <= duration`) and compute the
decision in code.

Decision table:

| Condition | Decision |
|---|---|
| no defects | `accept` |
| defects, attempt 1–2 | `regenerate` |
| defects, attempt 3 | `stop_limit_reached` |
| invalid input / API / response | `error` |

### Deployment (`cmd/inspector`, `internal/report`)

The CLI runs one assessment per invocation and writes one JSON report per
candidate into a numbered run directory.

```bash
# models: rule (default, placeholder), dsp (signal), llm (Gemini)
# repeat -detector to run several models over the same candidate
inspector -request-id <id> -audio in.mp3 -detector dsp                  [-report out.json]
inspector -request-id <id> -audio in.mp3 -detector llm                  [-report out.json]
inspector -request-id <id> -audio in.mp3 -detector dsp -detector llm    [-report out.json]
```

`GEMINI_API_KEY` comes from the environment; `GEMINI_MODEL` selects the model
(default `gemini-3.8-flash`). Model, prompt version, and duration are recorded
in every report.

## Project structure

```text
noise-detection/
├── cmd/inspector/main.go          # Deployment
├── internal/audio/{input,probe,decode,findings}.go        # Data Understanding
├── internal/features/{features,candidates,metrics}.go     # Data Preparation (base layer)
├── internal/model/{model,registry}.go                     # Modeling (contract + registry)
├── internal/model/{rule,dsp,llm}/…                        # Modeling (methods)
├── internal/evaluation/{aggregate,decide,pipeline}.go     # Evaluation
├── internal/report/report.go                              # contract
└── go.mod
```

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
      "sources": ["dsp", "gemini"],
      "status": "confirmed"
    },
    {
      "type": "click_pop",
      "dimension": "click_pop",
      "start_seconds": 116.22,
      "end_seconds": 116.24,
      "description": "A cluster of abrupt waveform discontinuities suggests a click or pop.",
      "sources": ["dsp"],
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
  "prompt_version": "dsp-v2+gemini-audio-v5",
  "assessed_at": "2026-01-01T00:00:00Z"
}
```

- `sources` lists every model family (`dsp` | `gemini` | …) that observed the
  event; `status` is `confirmed` for two or more sources and `unconfirmed` for one.
- Timbre findings also carry `dimension` (`dullness` | `muffling` | `loss_of_clarity` | `loss_of_transients` | `smearing` | `harshness` | `distortion` | `codec_artifacts` | an artifact type) plus `severity` and `confidence` (each `low` | `medium` | `high`); severity and confidence are distinct.
- `signal_observations` are the prepared measurements; they never appear in `defects` and never drive a decision on their own.
- A defect interval satisfies `0 <= start_seconds < end_seconds <= duration_seconds`.
- Error reports set `has_defect: null` and `decision: "error"`.
- LLM timestamps are approximate, not sample-accurate; DSP timestamps are exact
  sample positions.

## Principles

1. **Measure in the base layer, decide in the methods** — signal processing is
   deterministic and shared; judgment belongs to the models.
2. **Models measure and judge, code decides** — never let a model compute the
   decision.
3. **Evidence by source** — preserve each finding's sources and status; an
   ambiguous sound alone is not a defect.
4. **Pluggable models** — a model is anything implementing `model.Model`; adding
   one registers a factory and changes no pipeline.
5. **One pass over the data** — decode once, prepare once, share the result.
