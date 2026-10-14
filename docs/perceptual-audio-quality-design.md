# Design Direction: Timbre and Perceptual Audio Quality Assessment

**Status: proposed extension; not implemented or validated.**

## Purpose and Current Gap

Extend the detector to assess whether generated music has an unnatural or degraded timbre, including problems that persist across a passage or an entire recording. For example, a piano may remain free of clicks and dropouts while sounding muffled, blurred, or unusually harsh.

The current DSP checks cover clipping, internal low-energy intervals, waveform discontinuities, and channel imbalance. The Gemini prompt focuses on clearly audible, unintended artifacts. Neither path explicitly assesses the broader dimensions below. The existing transient check detects abrupt discontinuities; it does not measure lost attack detail.

This extension should complement artifact detection. It should not grade composition, performance, genre, or personal taste, and it should not assume that every piano must sound bright.

## Assessment Dimensions

These categories describe listening observations. They are not mutually exclusive, and overlapping labels should not inflate the number of reported problems.

| Dimension | Audible evidence to assess |
| --- | --- |
| Dullness | Reduced brilliance or missing upper-frequency detail relative to an appropriate comparison. |
| Muffling | A veiled, obscured quality, as though the sound passes through a barrier. |
| Loss of clarity | Reduced note definition or separation within a passage. |
| Loss of transients | Weakened attack detail or reduced immediacy of note onset. |
| Smearing | Blurred attacks or temporal detail that makes successive sounds less distinct. |
| Harshness | Unnaturally abrasive or piercing sound. |
| Distortion | Audible breakup, roughness, or added tonal components inconsistent with the intended sound. |
| Suspected compression artifacts | Warbling, metallic texture, or blurred detail potentially associated with lossy encoding. |

Descriptions should state what is audible before suggesting a cause. A listening judgment alone should not establish that a codec, processing stage, or generation model caused the problem.

## Reference and Context

Support two assessment modes:

- **With a reference:** compare against an appropriate source recording or approved version. Check comparability, align corresponding passages, and account for playback-level differences. If the performances or arrangements differ substantially, report that limitation rather than interpreting every difference as degradation.
- **Without a reference:** assess audible plausibility and consistency within the recording, using instrument context and declared production effects. Report suspected quality problems with explicit uncertainty; do not claim that detail was lost from an unavailable original.

An intended warm piano, soft playing, room reverberation, or lo-fi texture should not automatically become a defect. Context informs the assessment but should not suppress unrelated audible problems.

## Proposed Assessment Flow

1. **Validate and decode.** Preserve the original file and its timeline. Collect duration, sample rate, and channel metadata. Any alignment or level adjustment for comparison must use analysis copies and retain a mapping to original timestamps.
2. **Collect signal evidence.** Retain existing artifact checks. Experiment with spectral balance, onset-envelope shape, and temporal changes in these measurements. Treat them as supporting observations, not universal quality thresholds. Evaluate their usefulness separately for each dimension.
3. **Assess perceptual quality.** Extend the Gemini prompt and structured response to cover the proposed dimensions, intentional effects, reference availability, severity, and uncertainty. Assess both overall quality and affected passages. Require a concrete listening description for every finding. Model suitability must be established through evaluation.
4. **Combine evidence.** Associate findings by time, dimension, and described behavior. Time overlap alone must not establish agreement. A lack of DSP evidence must not automatically dismiss a perceptual concern, and DSP evidence alone must not establish audible degradation.
5. **Validate and decide.** Check response consistency and timestamp bounds, then apply an explicit decision policy in application code.

## Reporting and Decisions

Represent both localized findings and recording-wide concerns. Proposed fields include dimension, scope, interval where applicable, audible evidence, severity, evidence sources, reference mode, uncertainty, and review status. Distinguish severity from confidence; model confidence is not a calibrated probability.

The current pipeline treats every retained finding as grounds for regeneration, including unconfirmed findings. This policy should change before adding broader perceptual categories. Initially, uncertain or unsupported concerns should request review, while automatic regeneration should require an actionable finding that meets evaluation-backed criteria. This requires an explicit review outcome in the report and caller contract. Detection failures remain errors, never clean results. Retain the default three-generation limit; the caller remains responsible for regeneration.

## Validation and Initial Scope

Begin with piano recordings and a small set of dimensions: muffling, attack-detail loss, and harshness. Build a listening set containing clean examples, deliberate production effects, controlled degradations, and naturally occurring generation failures. Keep related variants of the same source in the same evaluation split.

Use independent human judgments to assess category agreement, severity, false alarms, missed problems, and timestamp usefulness. Compare DSP-only, Gemini-only, and combined assessments, with and without references. Include recording-wide degradation, where internal change detection may provide little evidence.

Release the first version as an advisory assessment. Enable automatic rejection only after held-out evaluation demonstrates acceptable false-positive rates, especially for intentional effects. Passing means no actionable problem was detected, not guaranteed fidelity.
