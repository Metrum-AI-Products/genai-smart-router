---
name: narrated-video-production
description: Use this skill when the user needs a narrated product demo, tutorial, launch video, training video, or informational video that combines voice-over with slides, text, images, screen recordings, or clips. Trigger on any request to plan or produce a video with a speech narrative.
---

# Produce an approval-gated narrated video

The approved narration and its rendered speech audio are the source of truth. Do **not** start speech synthesis or final video assembly until the human has explicitly approved the complete narration.

## Brand

Apply the Metrum AI visual system by default. Read [`references/metrum-ai-brand.md`](references/metrum-ai-brand.md) before creating visuals. Use the official logo only (never recreate as text); apply the dark technical-interface composition, approved type roles, and red-to-blue gradient.

## Workflow

### 1. Prepare the approval package

Read the instructions and background material. Identify audience, objective, platform, aspect ratio, target duration, brand constraints, available assets, and claims that need source support. Ask only for missing information that materially affects the video.

Draft a package containing:

- Complete, speakable narration divided into numbered scenes/beats
- Visual intent per beat (screen recording, clip, image, diagram, slide, text, generated visual)
- On-screen wording and source notes for factual claims
- Metrum-branded visual direction and expected duration including pauses
- Inter-scene pause config (default `0.75s` after every non-final scene; allow `0.0`–`2.0s`)
- Estimated duration and open questions/asset gaps

Present the package and ask for **explicit approval of the full narration**. Comments or a general go-ahead count only when they clearly approve the complete narration. If any word changes after approval, re-present the full narration and obtain approval again.

### 2. Lock narration and generate speech

Save the approved narration as a durable project artifact with stable scene IDs. Record the approved text **verbatim** in the production manifest.

After approval, ask which TTS provider to use and request credentials only for the active run (env vars or provider secure mechanism — never put secrets in source, manifests, logs, or commits).

Generate separate audio clips per scene ID, then concatenate them without rewriting speech. Insert the configured silence between non-final clips; do not append silence to the final clip. Record measured speech durations, pause durations, combined duration, provider, model, voice ID, and non-secret synthesis settings.

Use rendered audio durations (not estimates) to create the timeline. Prefer word-/phrase-level timestamps when available; otherwise mark lower timestamp precision.

If synthesis needs a text change, return to approval — do not patch wording in post.

### 3. Build visuals against the audio timeline

Create a timeline artifact with exact start/end/duration, narration text, visual asset, on-screen text, pause, and transition for every scene. Fit visuals to audio (extend, trim, freeze, transition) — never speed up, cut, or pad approved speech to fit an asset.

Default pause split: hold outgoing visual `0.30s`, crossfade remaining `0.45s`; start next narration only after its slide is fully visible. Scale proportionally if the user changes the pause. Never place a transition over spoken words by default.

Use supplied or licensed assets; label generated visuals when provenance matters. Keep on-screen text consistent with the approved narration. Generate captions from the final rendered audio.

### 4. Assemble and verify

Render with the final narration as the primary audio track. Align all clips, images, slides, transitions, captions, and overlays to the measured timeline. Include music/effects only if requested; duck them beneath speech.

Before delivery, verify:

- [ ] Every spoken claim is in the approved narration and source-supported where factual
- [ ] Final audio starts at zero, has no accidental gaps/overlaps, and matches the manifest duration
- [ ] Visual boundaries, captions, and on-screen text match final audio timestamps
- [ ] Metrum AI logo, colors, fonts, contrast, and gradient match the brand reference
- [ ] Render has the requested dimensions, frame rate, codec/container, and playable audio

Deliver the final video, approved narration, final audio, captions, and non-secret timeline/manifest. State any approximation or timing limitation plainly.

## Reference

- [`references/metrum-ai-brand.md`](references/metrum-ai-brand.md) — Brand system, palette, type, logo rules
