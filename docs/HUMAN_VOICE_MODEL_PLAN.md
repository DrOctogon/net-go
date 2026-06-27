# Pivot Plan: BirdNET-Go → Human Voice Detector

Status: IN PROGRESS — backend pivot (Waves 1–3) + frontend voice features done, branch `human-voice-pivot` 56 commits ahead of `main`, all green. Remaining: VAD accuracy verify (needs ORT env), VoiceWatch logo asset, optional deep dead-code (`EventDetectionNewSpecies` path). See WAVE 4 below.
Date: 2026-06-23 (updated 2026-06-26)
Branch: `human-voice-pivot`
Strategy: **Neutralize + repurpose** — strip bird/animal brain, keep reusable
acoustic pipeline, retarget to human-voice-only detection.

---

## PROGRESS CHECKPOINT (read this first)

| Commit | Phase | What landed |
|---|---|---|
| `262bbc73` | 1 | `internal/classifier/humanvoice/` — Silero VAD `ModelInstance` wrapper, `RegistryIDHumanVoice` registry entry, `HumanVoiceConfig` + `conf.ModelIDHumanVoice`. Additive; no bird code touched. |
| `34091881` | 2a | Deleted `internal/birdweather/` + `internal/speciesdict/` and ALL callers (processor `BwClient` + upload action, `control_monitor` reconfigure, api/v2 integrations routes/handlers, species-dictionary endpoint, app DTO field). Fixed 3 stale tests. |
| `a3c4f9b4` | 2b | Deleted `internal/imageprovider/` (~10.5k LOC) + ALL callers (APIServerService, Processor, SSEAction/MqttAction, pending_broadcast, api/server, api/v2/api, conf/defaults, mqtt/dto). Stubbed `GetSpeciesThumbnail`→404, `getThumbnailURL`→placeholder. Dropped `BirdImage` SSE/MQTT fields + image-cache analytics calls. Fixed sse_contract/analytics/species/datastore-guard tests. |
| _(pending)_ | 2c-1 | **api/v2 bird-endpoint peel + ebird decouple.** Deleted `species.go`, `species_taxonomy.go`, `range.go`, `heatmap.go` (+ dedicated tests). Removed range/heatmap/species route registrations + `EBirdClient` field + ebird init/import from `api.go`. Neutralized `getExpectedTodayRegionalImpl` (eBird-backed) → always `Available:false`. Dropped range-filter health check from `diagnostics.go`. Moved `daysPerWeek` const to `support.go`. README bird sections removed. api/v2 build+tests green. |

### Phase 2c decomposition (decided 2026-06-24)
Plan said "one rewrite unit" but that detonates the whole pipeline at once. **Peel leaves first**, build green each step:
- **2c-1** (done): api/v2 bird endpoints + ebird decouple. ✅
- **2c-2**: range-filter peripherals — `cmd/rangefilter/`, `internal/health/checks/range_filter_check.go`, observability range metric, notification range-rebuild key, conf `range_filter.go` helpers where safe. Also fold in the **species-analytics deletion** (phenology/accumulation/distribution endpoints in analytics.go + new-species notification) — these are datastore-only (zero classifier coupling) so they don't block, but user chose to delete them.
- **2c-3 (MILESTONE)**: welded core swap — reduce `internal/classifier` to a `ModelInstance`-only facade, delete `internal/ebird` + bird impls (birdnet/perch/bat/bsg) + range filter + taxonomy/genus + embeds + model gallery, retarget ~50 analysis consumers, construct `humanvoice.Model` in the analysis layer.
- **2d** — DONE (code, build/vet/tests green; 2026-06-25). Stateful Silero VAD runner in `inference/onnx/silero.go` (DynamicAdvancedSession; inputs `input`/`state`/`sr`, outputs `output`/`stateN`; 512-sample frames @ 16 kHz; recurrent state carried per clip, reset between clips; `sr` fed as shape `[1]` since the wrapper cannot build a rank-0 scalar). Facade `inference.NewSileroVAD` returns the `Classifier` interface; `humanvoice.New` swapped off the generic classifier. Silero VAD v5 `silero_vad.onnx` (~2.3 MB) vendored at `classifier/humanvoice/data/` and `go:embed`-ed; `humanvoice.WriteEmbeddedModel` extracts it to `<configDir>/models/` on first run; `birdnet_service.Start` uses it when `BirdNET.ModelPath` is unset. IO contract verified against the model via onnx introspection. **Not executed in-repo** (ONNX Runtime shared lib absent here — `task download-onnxruntime`); real speech-detection accuracy is verified in an ORT-enabled env / QA.

**Build command (always use this tag set — frontend `dist` not built, no TFLite C lib):**
`go build -tags "notflite skipfrontend" ./...` → currently exit 0.

**Known deferred debt:**
- ~~`conf` dead `BirdweatherSettings` struct + `Realtime.Birdweather` field~~ — DONE (commit `20d07123`, 2026-06-26): removed struct, field, validators, `birdweatherIDPattern`, viper defaults, config.yaml blocks, dead notification message constants, logger default, and the api/v2 test refs. `RetrySettings` kept (MQTT consumer). Still-deferred bird conf (EBird/range/perch/bat/bsg structs) have live readers — separate pass.
- Phase 1 `humanvoice.Predict` runs the GENERIC `inference.Classifier` — it does NOT yet implement Silero VAD's real stateful streaming I/O (state tensors + `sr` input + 512-sample frames). Real inference is unbuilt. See §3 / Phase 2c.

## WAVE 1 BACKEND CLEANUP — DONE (2026-06-26)

Orchestrated backend pivot cleanup, all build/test green (`-tags "notflite skipfrontend"`):
- BirdWeather dead config removed (`20d07123`), eBird integration removed (`b1aae90a`).
- Dog-bark + bat-ultrasonic filters removed (`6bd6917a`, −664 LOC): `dogbarkfilter.go`, `internal/audiocore/ultrasonic/`, `unlikely_comments.go`, `UltrasonicCV`/`DogBarkFilter` conf + api/v2 metrics. Kept `isHumanVocalization` (privacy filter), `SourceTypeUltrasonic` (hw routing), `Unlikely` DB column.
- Hot-reload coverage declared for continuous-recording (`restart`) + transcription (`fresh`) settings (`717300bb`); full api/v2 suite green.
- **Store-all-clips (Phase 3): SATISFIED** by the continuous full-audio recorder (`dfc92a80`) — records everything in rolling chunks; the per-detection force-save in the original plan is superseded.

Wave 2 frontend retarget — DONE (2026-06-26): BirdWeather/eBird UI removed, keyword/transcript surfaced + detection list/detail/dashboard/daily-summary retargeted to voice (mic + transcript), new-species widget + species settings page/search/range removed, Transcription/Keyword settings UI built.

Wave 3 rebrand — VoiceWatch product name already applied in UI/page-title/About; remaining = docs sweep + a redesigned logo asset (`BirdNET-Go-logo.webp`). Keep upstream BirdNET attribution.

Privacy-gate (Phase 3) — **SUPERSEDED, no change**: the privacy filter is already opt-in, `enabled: false` by default → all voice clips stored by default; the toggle is the privacy opt-out. Removing it would be a privacy regression for a speech recorder. Keep as-is.

Restart-toast for continuous-recording — DONE (`eed1d087`): real change-detection + RestartBanner reason wired; field moved restartExempt→restartCovered.

## WAVE 4 — VOICE FEATURES (feature-dev) — DONE (2026-06-26)

Four user-facing voice features + follow-ups, each gated (go build + datastore/alerting tests + `npm run check:all` 0 errors, ast:security clean + `npm test` ~2432 green) and pushed:
- **Transcript search** — wired into BOTH search surfaces (GET `/api/v2/detections` → `AdvancedSearchFilters.Transcript`, AND POST `/api/v2/search` → `SearchFilters.Transcript`; the two use SEPARATE query builders — both required). Parameterized LIKE with `escapeLikePattern` (escapes `\ % _`), `ESCAPE '\\'`. No SQL injection.
- **Flagged-only filter** — `SearchFilters.Flagged *bool` + `SearchRequest.FlaggedOnly`, applied to result AND count queries; Search.svelte "Flagged only" toggle (sends `flagged:true` only when on).
- **Voice-activity dashboard widget** — `VoiceActivityCard.svelte` (D3 24h bar chart from `GET /api/v2/analytics/time/distribution/hourly`, no species param).
- **Keyword-flag built-in alert rule** — `RuleKeyKeywordFlag*`/`ObjectTypeKeywordFlag`/`EventKeywordMatched` (60s cooldown, bell action) in defaults/constants/schema/dispatcher; removed dead new-species + BirdWeather rules (`9ba3ff3f`).
- **Keyword highlighting** — `highlightKeywords.ts` util (word-boundary `\b`, case-insensitive, `escapeRegExp`, dedup, cap 100, round-trip-safe, never throws); XSS-safe segment `{#each}` + `<mark>` render (NO `{@html}`) in DetectionDetail + DetectionRow + DetectionCardMobile + dashboard DetectionCard. 31 util tests.

Orphan i18n `settings.alerts.builtInRules.newSpecies` removed (en.json + types.generated.ts). Build-time `EventDetectionNewSpecies` const + `detection_bridge.go` path remain (dead-but-harmless for single-class voice; never fires) — optional future removal.

## WAVE 5 — SPEAKER ATTRIBUTES: GENDER + AGE (backend + frontend DONE; real model pending)

**Scaffold landed (2026-06-27, build+vet+tests green):** new `internal/speaker/` pkg (`Analyzer` iface + `NoopAnalyzer` + `New` factory + `Cosine` + `PCMS16LEToFloat32` + gender/age consts/validators, unit-tested); additive nullable `Note` columns (`gender`, `gender_confidence`, `age_band`, `age_confidence`, `speaker_id`, `voice_print_embedding` JSON) + `detection.Result` fields + `NoteFromResult` mapping; `conf.SpeakerAttributesSettings` (default off) + config.yaml + restart wiring (detector + settingsChangeChecks + restartRequiringChecks + hot-reload registry + coverage tests); processor seam `analyzeSpeakerAttributes` (gated, s16le→f32, attach, alert); `gender`/`ageBand` search filters on BOTH surfaces (result+count, allowlist-validated, parameterized); `GET /api/v2/detections/:id/similar` (auth-gated cosine, top-10, min-score floor); alerting `EventSpeakerAttributeMatched`/`ObjectTypeSpeakerAttr` rule+schema; i18n keys (all 15 locales + generated types). Reviewed (go-reviewer): auth-gated the similar endpoint, 64-bit ID parse, qualified SQL columns, dropped clip-name leak.

**Post-save alert + ctx threading DONE (commit `7fcdd766`, 2026-06-27):** alert emission moved out of the pre-save seam into `DatabaseAction.ExecuteContext` (after NoteID store) via `emitSpeakerAttributeAlert(settings, *Result)` — carries a valid detection ID; `analyzeSpeakerAttributes` now takes the flusher lifecycle `ctx` (threaded through `processApprovedDetection`, nil-guarded); new `(*detection.Result).HasSpeakerAttributes()` + test. Seam is now estimate+attach only.

**Frontend DONE (commit `a651f55d`, 2026-06-27, `npm run check:all` clean):** Detection TS type +`gender/genderConfidence/ageBand/ageConfidence`; shared `SpeakerAttributeChips.svelte` (default+overlay, hidden when empty, XSS-safe) + pure `utils/speakerAttributes.ts`; chips on DetectionRow, DetectionCardMobile, dashboard DetectionCard, DetectionDetail; Search.svelte gender+ageBand selectors (sent only when set); AudioSettingsPage "Speaker Attributes" section (master+per-attr toggles+thresholds+privacy caveat, hot-reload via updateSection) + `stores/settings.ts` shape; i18n across all 16 locales + regenerated types; chip/util tests.

**Still pending:** real ONNX gender/age/voice-print model (Noop → estimates always empty, similarity empty); speaker clustering to populate `speaker_id`. Both need external model assets / embeddings — not buildable in-repo. `golangci-lint` not yet run (binary absent locally).

Goal: enrich each human-voice detection with **estimated speaker gender** and **estimated age band**, surfaced in the UI and searchable/alertable. Additive on top of the existing VAD pipeline — VAD gates *whether* a clip has speech; these models classify *who* is speaking. Privacy-sensitive: estimates are demographic inferences, gate behind an opt-in setting and document the accuracy/bias caveats before any release.

### Model approach
- **Reuse the ONNX backend** (`internal/inference/onnx`) — same pattern as Silero VAD. No new runtime.
- Candidate models (small, ONNX-exportable, 16 kHz mono — matches VAD output):
  - **Gender**: binary/probabilistic classifier (e.g. wav2vec2 / ECAPA-TDNN gender head, or a lightweight CNN on log-mel). Output: `{male, female, unknown}` + confidence.
  - **Age**: regression → age (years) bucketed into bands, OR direct multi-class band classifier. Bands (proposed, named constants — no magic strings): `child` / `teen` / `adult` / `senior`. Output: band + confidence.
  - Prefer a **single multi-task model** (shared encoder, two heads) if one exists — one inference pass, lower CPU on the Pi. Otherwise two small sequential models.
- Only run attribute inference **when VAD fires** (speech present) — never on silence/noise. Reuse the already-resampled 16 kHz frames; don't re-decode.
- Gate behind `Realtime.Audio.SpeakerAttributes.Enabled` (hot-reloadable, default `false`). Separate sub-toggles `Gender.Enabled` / `Age.Enabled` so each can run independently.

### Backend
- New pkg `internal/classifier/speakerattr/` (parallel to `humanvoice/`): `ModelInstance`-style wrappers; `go:embed` the model(s) + `WriteEmbeddedModel` extract-on-first-run, same as VAD.
- `conf/config.go`: add `SpeakerAttributesConfig` (`Enabled`, `Gender{Enabled,Threshold,ModelPath}`, `Age{Enabled,Threshold,ModelPath}`). Hot-reload via per-request checks; add to `settingsChangeChecks` (model construction at pipeline startup → likely **restart-required**, mirror continuous-recording wiring + `restartCovered` test entry).
- Detection schema: add nullable `gender`, `gender_confidence`, `age_band`, `age_confidence` columns to the v2only datastore (migration; nullable so existing rows + VAD-only mode are valid). Wire through `analysis/processor` save path after the VAD result, before save.
- API v2 (NEVER v1): extend detection DTO with the attribute fields; add filters to BOTH search surfaces (GET `/api/v2/detections` `AdvancedSearchFilters`, POST `/api/v2/search` `SearchFilters`) — `gender:`, `age_band:`. Apply to result AND count queries (same dual-builder gotcha as transcript search).

### Alerting
- New built-in rule scaffolding mirroring keyword-flag: `EventSpeakerAttributeMatched` + `ObjectTypeSpeakerAttr` + rule keys, so users can alert on e.g. `gender=female AND age_band=child`. Constants/defaults/schema/dispatcher.

### Frontend
- Detection list/detail/card: show gender + age-band chips with confidence (reuse the keyword-chip pattern). Hide chips when attributes disabled or null.
- Search.svelte: gender + age-band filter selectors (only send when set).
- Settings: SpeakerAttributes section under audio — master enable + per-attribute toggles + thresholds + model-status info. i18n keys for all labels.
- `npm run check:all` 0 errors; util/component tests for chip render + null-safety.

### Privacy + compliance (must address before release)
- Demographic inference on recorded speech is **higher-risk than VAD presence**. Default OFF; README + UI must state these are *estimates*, document known bias (accuracy varies by accent/language/recording quality), and clarify no identity/biometric ID is performed (this is attribute estimation, not speaker recognition).
- Estimates stored alongside clips inherit the existing store-all-clips retention; note in privacy docs.

### Open questions
1. Single multi-task model vs. two models? (CPU budget on Pi decides.)
2. Age as regression+bucketing or direct band classifier? Band boundaries?
3. Report per-clip single estimate, or per-VAD-segment when multiple speakers? (v1: per-clip single, like current detection granularity.)
4. Confidence-threshold default + whether to store low-confidence estimates as `unknown` vs. drop.

## REMAINING WORK (do mechanical parts on Sonnet, not Opus)

- **2b** — delete `internal/imageprovider/` (~20 api/v2 files reference it: analytics, sse, settings, insights, media, app DTOs, processor actions, mqtt/dto, main.go). Bounded, mechanical.
- **2c (MILESTONE)** — classifier brain swap. The bird brain is ONE welded subsystem: `internal/classifier` (~11.6k LOC) + `internal/ebird` (imported by `classifier/genus.go`→`birdnet.go`) + embedded model data (unconditional embeds in `genus.go`/`taxonomy.go`/`label_files.go`) + range filter + species/heatmap/taxonomy api/v2 endpoints + 30+ consumers in `analysis`. Deleting model data detonates the embeds. Must be done as ONE rewrite unit: construct `humanvoice.Model` in the `analysis` layer (avoids `humanvoice`→`classifier` import cycle), introduce a reduced `ModelInstance`-only classifier facade, re-type consumers, AND build the real Silero VAD inference path + fetch `silero_vad.onnx`.
- **3+** — datastore voice label_type + store-all-clips (force `save_audio_action.go`), frontend retarget (voice-event list, drop species/taxonomy/rarity UI), rebrand, dead-conf cleanup.

---

## 2c-3 STATUS — DONE (build GREEN, tests pass) — 2026-06-25

Slim facade welded through all consumers. `go build -tags "notflite skipfrontend" ./...`
clean; `go vet` clean; tests pass for `classifier`, `humanvoice`, `inferencestats`,
`api/v2`, `analysis`, `analysis/processor`.

Consumer retarget completed:

- `analysis/birdnet_service.go`: builds `humanvoice.New(&Config{ModelPath,ONNXRuntimePath,Threads,Threshold})` then `classifier.NewOrchestrator(settings, model)`; deleted ModelManager field/method, `loadModelCatalog`, `initModelManager`.
- `analysis/control_monitor.go`: dropped `BuildRangeFilter` (×2), `OpenFaunaResolver` (→`installNameResolver(nil,...)`), `ReloadSecondaryModels`; `handleRebuildRangeFilter` now maintenance-cleanup only.
- `analysis/{database_migration,database_service}.go`: `LoadTaxonomyData("")` → `var sciIndex map[string]string` (empty).
- `analysis/{buffer_manager.go:IsModelActive skip removed, process.go:ModelSpecFor now (ModelSpec,error), audio_pipeline_service.go:SetSunCalc drop, api_service.go:WithModelManager drop}`.
- `processor/processor.go`: dropped `taxonomyDB` field, bat threshold/ultrasonic branches, `EnrichResultWithTaxonomy`→`detection.ParseSpeciesString`, `GetSpeciesOccurrenceAtTime`→0, `DetectionNamePerch` clause, `UpdateRangeFilterAction` scheduling; deleted orphaned `applyUltrasonicFilter`.
- `processor/{extended_capture,daylight_filter,false_positive_filter,actions_*}.go`: `resolveSpeciesFilter` now 4-arg (no taxonomy); kept daylight filter (generic suncalc, taxonomy-free) — diverged from spec's delete-list; deleted `UpdateRangeFilterAction` type/Execute/GetDescription.
- `api/server.go` + `api/v2/api.go`: removed `modelManager`/`ModelManager`/`WithModelManager`/`TaxonomyDB`/`LoadTaxonomyDatabase`; dropped `model routes` registration.
- `api/v2/models.go`: DELETED. `system_models.go`/`inference_status.go`/`diagnostics.go`/`metrics_history.go`: retargeted to single-model facade (orchestrator `ModelInfos`/`ModelID`; inference counters + per-model RSS not tracked until 2d).
- `classifier/orchestrator.go`: added `PrimaryModelID`/`PrimaryModelInfo`.
- `cmd/benchmark/benchmark.go`: injects `humanvoice` model.
- Test files retargeted to single model; bird/bat/perch/taxonomy-specific cases deleted.

Deferred-OK (compile/run fine, clean later): conf range/perch/bat/bsg structs + bat
false-positive-filter helpers (read conf.Bat, kept per §E); observability
`RecordRangeFilter`; notification rebuild key.

Known non-blocker: `TestExecuteCommandAction_EnvironmentIsolation` hangs in this sandbox
(runs a temp shell script via untouched `execute.go`; zero classifier coupling) — skip it.

Lint (`golangci-lint`) not run — binary absent from this environment; run `task lint` before PR.

---

## 2c-3 EXECUTION SPEC (turn-key — read before cutting)

Atomic commit; no green intermediate. Order: slim the facade → delete bird files → delete ebird → retarget consumers → build/fix/test. Verified against the codebase 2026-06-24.

### A. classifier facade — KEEP (edit in place)
- `model.go` — keep `ModelSpec`, `ModelInstance`, `ModelInfo`-adjacent consts. Drop `NameResolver` iff no remaining consumer (control_monitor's `OpenFaunaResolver()` use is removed below).
- `queue.go` — keep verbatim (`Results` struct, `ResultsQueue`, `ResizeQueue`, `DefaultQueueSize`). This is the pipeline's data carrier; consumers read `classifier.Results` off `ResultsQueue`.
- `logger.go` — keep.
- `model_registry.go` — SLIM to: `ModelInfo` struct + `DisplayName`/`ToDetectionModelInfo`; consts `BackendTFLite/ONNX/OpenVINO`, `Quantization*`, `RegistryIDHumanVoice`, `DetectionNamePerch` (drop if no consumer after retarget); `ModelRegistry` map with **only** the `RegistryIDHumanVoice` entry; helpers consumers call: `ResolveConfigModelID`, `ConfigAliasForRegistry`, `GetModelSpec`, `DetectionModelInfoForID`, `KnownConfigIDs`. DROP: all BirdNET/Perch/Bat/BSG entries, `ResolveBirdNETVersion`, `DetermineModelInfo`, `filenamePatterns`, `detectQuantization`, `remapV24ForONNXOnly`, `stockBirdNETV24ONNXVariant`, `customBirdNETV24ModelInfo`, `defaultClassifierModelInfo`, `defaultRangeFilterONNXPath`, `IsLocaleSupported`, `birdnetVersionToRegistryID`, refs to `DefaultModelVersion`/`DefaultBirdNETINT8ONNXModelName`/`DefaultRangeFilterV2ONNXModelName` (those consts live in deleted files).
- `orchestrator.go` — REWRITE to a single-model host (~80 LOC). Import cycle: `humanvoice`→`classifier`, so the Orchestrator must NOT construct the model. Inject it:
  ```go
  type Orchestrator struct {
      settingsAtomic atomic.Pointer[conf.Settings]
      inferenceMu    sync.Mutex      // serialize Predict (model not goroutine-safe)
      primary        ModelInstance
  }
  func NewOrchestrator(settings *conf.Settings, primary ModelInstance) (*Orchestrator, error)
  func (o *Orchestrator) PredictModel(ctx, modelID string, sample [][]float32) ([]datastore.Results, error) // ignore modelID; use primary
  func (o *Orchestrator) Predict(ctx, sample) ([]datastore.Results, error)
  func (o *Orchestrator) ModelSpecFor(modelID string) (ModelSpec, error) // primary.Spec()
  func (o *Orchestrator) NumSpecies() int        // primary.NumSpecies()
  func (o *Orchestrator) Labels() []string       // primary.Labels()
  func (o *Orchestrator) AllLabels() []string    // primary.Labels()
  func (o *Orchestrator) ModelInfos() []ModelInfo
  func (o *Orchestrator) Delete()                // primary.Close()
  ```
  Keep `tracing.go`/`threads.go` only if the rewritten Predict uses them; otherwise delete.

### B. classifier — DELETE files
birdnet.go, perch_onnx.go, bat_onnx.go, orchestrator_bat_onnx.go, orchestrator_perch_onnx.go, orchestrator_memory.go, orchestrator_notifications.go, range_filter.go, mapped_range_filter.go, model_onnx.go, model_openvino.go, model_manager.go, model_catalog.go, catalog_loader.go, heatmap_service.go, genus.go, taxonomy.go, taxonomy_resolver.go, label_files.go, names.go, models_embedded.go, models_external.go, nighttime_scheduler.go, analyze.go, tflite_available.go, tflite_unavailable.go, openvino_available.go, openvino_unavailable.go, `data/*` (.tflite/.json), + all `*_test.go` for the above. **Delete `internal/ebird/` whole.**

### C. analysis consumers — RETARGET (file:line from 2c map; re-verify, lines drift)
- `birdnet_service.go`: replace `classifier.NewOrchestrator(settings)` → build `humanvoice.New(&humanvoice.Config{ModelPath, ONNXRuntimePath, Threads, Threshold})` then `classifier.NewOrchestrator(settings, model)`. DELETE `classifier.BuildRangeFilter`, `classifier.LoadCatalog`, `classifier.NewModelManager`+`ScanInstalled`. Keep `bn.NumSpecies()`.
- `process.go`: keep `PredictModel`, `ModelSpecFor`, `ResultsQueue`. No bird logic.
- `processor/processor.go` (heaviest, 23 sites): DROP `EnrichResultWithTaxonomy`, `GetSpeciesOccurrenceAtTime`, `taxonomyDB` field + `LoadTaxonomyDatabase`, `RegistryIDBat`/`DetectionNamePerch` branches (bat/perch special-casing), `ModelRegistry[item.ModelID].Spec.ClipLength` → use `ModelSpecFor`/`GetModelSpec`. Keep the `ResultsQueue` consume loop + `classifier.Results`/`DetectionModelInfoForID`/`ResolveConfigModelID`. `detection.ParseSpeciesString` stays (generic).
- `control_monitor.go`: DROP `BuildRangeFilter` (×2), `OpenFaunaResolver()`; `AllLabels()` → ["Human Voice"].
- `buffer_manager.go`: keep `*Orchestrator`/`ModelSpec`/`ModelInfo` types — no change beyond compile.
- `audio_pipeline_service.go`: `ResolveConfigModelID`/`ModelRegistry` keep; with single model the per-model fan-out collapses to one entry — simplify model-set join.
- `extended_capture.go`: DROP `LoadTaxonomyDatabase`; `AllLabels()` trivial.
- `daylight_filter.go`, `dogbarkfilter.go`, `vocalization_labels.go`: DELETE (bird/bat filters) + unregister from processor.
- `database_service.go`/`database_migration.go`: DROP `LoadTaxonomyData("")`.
- `false_positive_filter.go`, `actions_types.go`: DROP `RegistryIDBat` branch / `UpdateRangeFilterAction`.

### D. api/v2 — remaining classifier coupling (deferred from 2c-1, MUST handle here)
`api.go` still has `TaxonomyDB *classifier.TaxonomyDatabase` (field + `LoadTaxonomyDatabase` at ~531) and `ModelManager *classifier.ModelManager` (field + `WithModelManager` + `initModelRoutes`). Both types are deleted here. Remove the fields, the load call, the option, and the model-gallery routes (`models.go` + tests). Re-grep `c.TaxonomyDB` / `ModelManager` for stragglers.

### E. conf + observability + notification (range/secondary peripherals) — DONE (commit 6cefa1a2, 2026-06-25)
- `conf/config.go`: drop `RangeFilterSettings`, `PerchConfig`, `BatConfig`, `BSGConfig` + their `Settings` fields + bird fields on `BirdNETConfig` (keep struct for lat/long/threads/threshold/onnxruntimepath that humanvoice config reads). `conf/range_filter.go` helpers + `conf/clone.go`/`validate_services.go` refs.
- `observability/metrics/birdnet.go`: drop `RangeFilterDuration`/`RecordRangeFilter`.
- `notification/message_keys.go`: drop `MsgSettingsRebuildingRangeFilter`.

### F. species-analytics deletion (user-chosen, datastore-only, no classifier coupling) — DONE (commit 73b40ffe, 2026-06-25)
`analytics.go`: remove `GetSpeciesAccumulation`, `GetSpeciesPhenology`, `GetSpeciesHourlyDistribution` handlers + routes. `notifications.go`: remove `CreateTestNewSpeciesNotification` + route. Delete tests: analytics_species_accumulation/distribution/phenology_test.go, notifications_new_species_test.go. (Can be its own small commit.)

### G. Verify
`go build -tags "notflite skipfrontend" ./...` → 0; `go vet` the touched pkgs; `go test -tags "notflite skipfrontend" ./internal/api/v2/ ./internal/analysis/... ./internal/classifier/...`. humanvoice `Predict` still uses the generic ONNX path (real Silero VAD = 2d).

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
