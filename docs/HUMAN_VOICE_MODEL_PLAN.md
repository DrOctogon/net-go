# Pivot Plan: BirdNET-Go → Human Voice Detector

Status: IN PROGRESS — Phases 1 + 2a committed, green.
Date: 2026-06-23 (updated 2026-06-24)
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
- **2d**: real stateful Silero VAD inference (state tensors, `sr` input, 512-sample frames) + fetch `silero_vad.onnx`. Deferred from 2c-3 so the structural swap stays reviewable; humanvoice `Predict` uses the current generic path / clearly-marked stub until then.

**Build command (always use this tag set — frontend `dist` not built, no TFLite C lib):**
`go build -tags "notflite skipfrontend" ./...` → currently exit 0.

**Known deferred debt:**
- `conf` still has a dead `BirdweatherSettings` struct + `Realtime.Birdweather` field (retained to keep 2a bounded; the hot-reload coverage test maps it with no action). Remove in the config-cleanup pass.
- Phase 1 `humanvoice.Predict` runs the GENERIC `inference.Classifier` — it does NOT yet implement Silero VAD's real stateful streaming I/O (state tensors + `sr` input + 512-sample frames). Real inference is unbuilt. See §3 / Phase 2c.

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

### E. conf + observability + notification (range/secondary peripherals)
- `conf/config.go`: drop `RangeFilterSettings`, `PerchConfig`, `BatConfig`, `BSGConfig` + their `Settings` fields + bird fields on `BirdNETConfig` (keep struct for lat/long/threads/threshold/onnxruntimepath that humanvoice config reads). `conf/range_filter.go` helpers + `conf/clone.go`/`validate_services.go` refs.
- `observability/metrics/birdnet.go`: drop `RangeFilterDuration`/`RecordRangeFilter`.
- `notification/message_keys.go`: drop `MsgSettingsRebuildingRangeFilter`.

### F. species-analytics deletion (user-chosen, datastore-only, no classifier coupling)
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
