# Pivot Plan: BirdNET-Go → Human Voice Detector

Status: PLAN (no code written yet)
Date: 2026-06-23
Strategy: **Neutralize + repurpose** — strip bird/animal brain, keep reusable
acoustic pipeline, retarget to human-voice-only detection.

---

## 1. Decisions locked

| Topic | Decision |
|---|---|
| Product | Human-voice-only acoustic detector. Birds/animals removed. |
| Audio clips | **Store ALL voice clips** (for model accuracy / future training). No privacy gating now — privacy addressed later. |
| Rip-out depth | Neutralize bird-specific code + integrations; keep pipeline scaffold; repurpose. Build stays green incrementally. |
| Model | Silero VAD (ONNX) as the single classifier, via existing ONNX backend. Speaker-ID deferred. |

---

## 2. Keep vs. Remove

### KEEP (reusable skeleton)
- `internal/audiocore/` — capture, buffer, resample.
- `internal/inference/onnx.go` — ONNX backend.
- `internal/classifier/` orchestrator + `ModelInstance` interface (repurposed to host one model).
- `internal/datastore/` core + `v2only/` (label_type already supports non-bird).
- `internal/analysis/` pipeline: `process.go`, `processor/` (esp. `save_audio_action.go`, `privacy_filter.go`, `false_positive_filter.go`).
- `internal/api/v2/` framework, auth, `detections.go`, settings, SSE.
- `internal/conf/` config system (hot-reload).
- `frontend/` shell, settings, detection list, auth.

### REMOVE / NEUTRALIZE (bird & animal)
- Models/data: `internal/classifier/data/*.tflite`, `*.onnx` (BirdNET/Perch/MData), `eBird_taxonomy_codes_*.json`, `genus_taxonomy.json`, `data/labels/`.
- Registry: BirdNET v2.4/v3, Perch, Bat, BSG entries in `model_registry.go`.
- `internal/ebird/` — eBird taxonomy client.
- `internal/imageprovider/` — avicommons/wikipedia bird images.
- `internal/birdweather/` — BirdWeather upload integration.
- `internal/speciesdict/` — species dictionary.
- `internal/labels/` bird label files (keep mechanism, drop bird content).
- Range filter: `cmd/rangefilter/`, eBird geo range filter in classifier/config.
- Processor bird filters: `dogbarkfilter.go`, `daylight_filter.go` (bat nighttime), species exclusion/config lookup, `vocalization_labels.go` (bird call/song types).
- API v2 bird endpoints: `species.go`, `range.go`, `media.go` (bird image), taxonomy routes.
- Frontend: species cards, taxonomy/rarity/phenology UI, bird imagery.
- Config: `BirdNETConfig` bird fields, `RangeFilterSettings`, `PerchConfig`, `BatConfig`, `BSGConfig`.
- QA: bird-specific specs in `~/src/birdnet-go-qa/`.

---

## 3. Phased execution (build green at each phase end)

### Phase 0 — Branch + baseline
- Branch from updated main: `git pull origin main && git checkout -b human-voice-pivot`.
- Snapshot current `go build ./...` + test baseline.

### Phase 1 — Human Voice model (the new brain)
- New pkg `internal/classifier/humanvoice/`: implement `ModelInstance` wrapping
  Silero VAD ONNX. `Predict`: resample window 48k→16k, run VAD frames, aggregate
  (max/mean) → single `Human Voice` result. `NumSpecies()=1`.
- Add `RegistryIDHumanVoice` to `model_registry.go` (Backend ONNX, DetectionName
  `HumanVoice`, ConfigAlias `human_voice`, Spec 16k internal / window-matched).
- Add `HumanVoiceConfig` to `conf/config.go` (`ModelPath`, `Threshold`); enablement
  via `ModelsConfig.Enabled`. Hot-reloadable.
- Unit tests: silence→none, speech→detect, noise→no false positive.

### Phase 2 — Make it the only model
- `orchestrator.go` / `birdnet_service.go` (`analysis/birdnet_init.go`): load
  HumanVoice as the primary (and only) model. Remove BirdNET/Perch/Bat/BSG load paths.
- Delete embedded bird models from `classifier/data/`; remove from registry/catalog.
- Resolve compile breaks by deleting bird-only branches, not stubbing.

### Phase 3 — Datastore: voice detections + store-all-clips
- Detections write with `label_type=human-voice` (v2only). Drop `Sci_Common_Code`
  parser dependence (`detection/species.go`).
- **Store all clips:** in `processor/save_audio_action.go`, force clip save on every
  human-voice detection regardless of threshold-for-save / privacy settings. Remove
  privacy gate (`privacy_filter.go`) from the save path (keep file for later re-add).
- Strip species/taxonomy columns usage from queries; keep generic detection schema.

### Phase 4 — Remove bird integrations
- Delete `ebird/`, `imageprovider/`, `birdweather/`, `speciesdict/`, range filter,
  bird processor filters. Fix all references (config, API, processor, frontend).
- Remove bird API v2 endpoints (`species.go`, `range.go`, `media.go` bird routes);
  keep `detections.go` (now voice). **No v1 changes.**

### Phase 5 — Config cleanup
- Remove `BirdNETConfig` bird fields, `RangeFilterSettings`, `Perch/Bat/BSGConfig`.
- Provide migration default so existing configs don't crash (ignore/strip unknown
  bird keys on load).

### Phase 6 — Frontend retarget
- Replace species UI with voice-event list (timestamp, confidence, clip playback,
  duration). Remove taxonomy/rarity/phenology/imagery. Settings: VAD threshold,
  enable, clip storage info. `npm run check:all`.

### Phase 7 — Branding / docs / QA
- Rename app + docs (BirdNET-Go → e.g. VoiceNet-Go). Update `CLAUDE.md` family,
  README, `internal/api/v2/CLAUDE.md`.
- Replace QA specs with voice-detection E2E.
- `golangci-lint run -v` + `go test -race` green.

---

## 4. Risks

| Risk | Mitigation |
|---|---|
| Removing core packages cascades compile breaks | Phase order keeps a green build; delete branches not stub |
| Storing all clips = disk growth | Documented tradeoff (accuracy first); add retention later |
| Legal/privacy of recording speech | Explicitly deferred per user; flag in README before any release |
| Config back-compat | Strip unknown bird keys on load (Phase 5) |
| Frontend tightly coupled to species types | Phase 6 isolated; ship backend-first |

---

## 5. Open questions

1. New project name? (placeholder: VoiceNet-Go.)
2. Keep `birdnet-go-qa` repo or fork a `voicenet-qa`?
3. Embed Silero VAD in binary, or gallery download? (Plan: embed — it's ~1.8MB, only model now.)
4. Voice detection granularity for v1: presence-per-window only, or also segment
   start/end timestamps within the clip?

---

## 6. Compliance checklist

- [ ] API additions v2-only.
- [ ] Settings hot-reload (atomic/per-request).
- [ ] No magic numbers (thresholds/frame sizes as constants).
- [ ] `golangci-lint run -v` + `go test -race` green per phase.
- [ ] Reuse `ModelInstance`/orchestrator/datastore — no parallel pipeline.
