<!--
  Analysis Settings Page Component

  Purpose: Configure VoiceWatch analysis settings including detection thresholds,
  false positive filtering, dynamic threshold, and transcription.

  Features:
  - Detection settings for the built-in Human Voice (Silero VAD) model
  - Bat detection settings (shown when bat detection is enabled)
  - Locale selector with flag icons for species labels
  - False positive filter with colored level badge
  - Dynamic threshold with enable/disable and parameter tuning
  - Advanced section with processing threads and custom classifier paths
  - Transcription and keyword flagging

  Props: None - This is a page component that uses global settings stores

  @component
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import SettingsSection from '$lib/desktop/features/settings/components/SettingsSection.svelte';
  import SettingsNote from '$lib/desktop/features/settings/components/SettingsNote.svelte';
  import SettingsPageActions from '$lib/desktop/features/settings/components/SettingsPageActions.svelte';
  import NumberField from '$lib/desktop/components/forms/NumberField.svelte';
  import FalsePositiveFilterControl, {
    type FilterLevel,
  } from '$lib/desktop/components/forms/FalsePositiveFilterControl.svelte';
  import Checkbox from '$lib/desktop/components/forms/Checkbox.svelte';
  import SelectDropdown from '$lib/desktop/components/forms/SelectDropdown.svelte';
  import type { SelectOption } from '$lib/desktop/components/forms/SelectDropdown.types';
  import FlagIcon, { type FlagLocale } from '$lib/desktop/components/ui/FlagIcon.svelte';
  import TextInput from '$lib/desktop/components/forms/TextInput.svelte';
  import {
    settingsStore,
    settingsActions,
    voicewatchSettings,
    dynamicThresholdSettings,
    realtimeSettings,
    batSettings,
    transcriptionSettings,
    type TranscriptionSettings,
  } from '$lib/stores/settings';
  import { api, ApiError } from '$lib/utils/api';
  import { toastActions } from '$lib/stores/toast';
  import { safeArrayAccess } from '$lib/utils/security';
  import { t } from '$lib/i18n';
  import { AlertTriangle, X, Plus } from '@lucide/svelte';

  // ── Store-derived state ───────────────────────────────────────────────
  let store = $derived($settingsStore);
  let birdnet = $derived($voicewatchSettings);
  let dynamicThreshold = $derived(
    $dynamicThresholdSettings ?? {
      enabled: false,
      debug: false,
      trigger: 0.8,
      min: 0.3,
      validHours: 24,
    }
  );
  let falsePositiveFilter = $derived($realtimeSettings?.falsePositiveFilter ?? { level: 0 });
  let bat = $derived(
    $batSettings ?? {
      enabled: false,
      threshold: 0.5,
      filterEnabled: false,
      nighttimeOnly: true,
      falsePositiveFilter: { level: 0 },
      ultrasonicFilter: { enabled: true },
    }
  );

  const batFPLevel = $derived(bat.falsePositiveFilter?.level ?? 0);

  // ── VoiceWatch locale loading ────────────────────────────────────────────
  interface BirdnetLocaleOption extends SelectOption {
    localeCode: FlagLocale;
  }

  let birdnetLocales = $state<{
    loading: boolean;
    error: string | null;
    data: Array<{ value: string; label: string }>;
  }>({
    loading: true,
    error: null,
    data: [],
  });

  let birdnetLocaleOptions = $derived<BirdnetLocaleOption[]>(
    birdnetLocales.data.map(locale => ({
      value: locale.value,
      label: locale.label,
      localeCode: locale.value as FlagLocale,
    }))
  );

  async function loadBirdnetLocales() {
    birdnetLocales.loading = true;
    birdnetLocales.error = null;

    try {
      const localesData = await api.get<Record<string, string>>('/api/v2/settings/locales');
      birdnetLocales.data = Object.entries(localesData || {}).map(([value, label]) => ({
        value,
        label: label as string,
      }));
    } catch (err) {
      if (err instanceof ApiError) {
        toastActions.warning(t('settings.main.errors.localesLoadFailed'));
      }
      birdnetLocales.error = t('settings.main.errors.localesLoadFailed');
      birdnetLocales.data = [{ value: 'en', label: 'English' }];
    } finally {
      birdnetLocales.loading = false;
    }
  }

  // ── False Positive Filter helpers ─────────────────────────────────────
  const OVERLAP_COMPARISON_TOLERANCE = 0.001;

  const falsePositiveFilterLevels = [
    {
      value: 0,
      descriptionKey: 'settings.main.sections.falsePositiveFilter.levels.off',
      minOverlap: 0.0,
      threshold: 0.0,
    },
    {
      value: 1,
      descriptionKey: 'settings.main.sections.falsePositiveFilter.levels.lenient',
      minOverlap: 2.0,
      threshold: 0.2,
    },
    {
      value: 2,
      descriptionKey: 'settings.main.sections.falsePositiveFilter.levels.moderate',
      minOverlap: 2.2,
      threshold: 0.3,
    },
    {
      value: 3,
      descriptionKey: 'settings.main.sections.falsePositiveFilter.levels.balanced',
      minOverlap: 2.4,
      threshold: 0.5,
    },
    {
      value: 4,
      descriptionKey: 'settings.main.sections.falsePositiveFilter.levels.strict',
      minOverlap: 2.7,
      threshold: 0.6,
    },
    {
      value: 5,
      descriptionKey: 'settings.main.sections.falsePositiveFilter.levels.maximum',
      minOverlap: 2.8,
      threshold: 0.7,
    },
  ];

  // Constants matching backend: internal/analysis/processor/processor.go
  const CHUNK_DURATION_SECONDS = 3.0;
  const REFERENCE_WINDOW_SECONDS = 6.0;
  const MIN_SEGMENT_LENGTH = 0.1;
  const FLOAT_EPSILON = 1e-9;

  function calculateMinDetections(level: number, overlap: number): number {
    if (level === 0) return 1;

    const levelData = safeArrayAccess(falsePositiveFilterLevels, level);
    if (!levelData) return 1;

    const segmentLength = Math.max(MIN_SEGMENT_LENGTH, CHUNK_DURATION_SECONDS - overlap);
    const maxDetectionsIn6s = REFERENCE_WINDOW_SECONDS / segmentLength;
    const required = maxDetectionsIn6s * levelData.threshold - FLOAT_EPSILON;
    return Math.max(1, Math.ceil(required));
  }

  function getFalsePositiveFilterDescription(level: number, overlap: number): string {
    const levelData = safeArrayAccess(falsePositiveFilterLevels, level);
    if (!levelData) return '';

    const minDet = calculateMinDetections(level, overlap);
    const baseDescription = t(levelData.descriptionKey);

    if (level === 0) return baseDescription;

    return t('settings.main.sections.falsePositiveFilter.detectionCount', {
      count: minDet.toString(),
      description: baseDescription,
    });
  }

  function getMinimumOverlapForLevel(level: number): number {
    return safeArrayAccess(falsePositiveFilterLevels, level)?.minOverlap ?? 0.0;
  }

  function updateFalsePositiveFilterLevel(newLevel: number) {
    const oldLevel = falsePositiveFilter.level;
    const oldMinOverlap = getMinimumOverlapForLevel(oldLevel);
    const newMinOverlap = getMinimumOverlapForLevel(newLevel);
    const currentOverlap = birdnet?.overlap ?? 0;

    settingsActions.updateSection('realtime', {
      falsePositiveFilter: { level: newLevel },
    });

    if (currentOverlap < newMinOverlap) {
      settingsActions.updateSection('voicewatch', { overlap: newMinOverlap });
      toastActions.info(
        t('settings.main.sections.falsePositiveFilter.overlapAdjusted', {
          overlap: newMinOverlap.toFixed(1),
        })
      );
    } else if (
      newMinOverlap < oldMinOverlap &&
      Math.abs(currentOverlap - oldMinOverlap) < OVERLAP_COMPARISON_TOLERANCE
    ) {
      settingsActions.updateSection('voicewatch', { overlap: newMinOverlap });
      toastActions.info(
        t('settings.main.sections.falsePositiveFilter.overlapReduced', {
          overlap: newMinOverlap.toFixed(1),
        })
      );
    }
  }

  // ── Update handlers ───────────────────────────────────────────────────
  function updateBirdnetSetting(key: string, value: string | number) {
    settingsActions.updateSection('voicewatch', { [key]: value });
  }

  function updateDynamicThreshold(key: string, value: number | boolean) {
    settingsActions.updateSection('realtime', {
      dynamicThreshold: { ...dynamicThreshold, [key]: value },
    });
  }

  function updateBatThreshold(value: number) {
    settingsActions.updateSection('bat', { threshold: value });
  }

  function updateBatNighttimeOnly(value: boolean) {
    settingsActions.updateSection('bat', { nighttimeOnly: value });
  }

  function updateBatUltrasonicFilter(value: boolean) {
    settingsActions.updateSection('bat', {
      ultrasonicFilter: { ...bat.ultrasonicFilter, enabled: value },
    });
  }

  function updateBatFalsePositiveFilterLevel(newLevel: number) {
    settingsActions.updateSection('bat', {
      falsePositiveFilter: { level: newLevel },
    });
  }

  // ── Transcription & keyword-flagging state ────────────────────────────
  const defaultTranscription: TranscriptionSettings = {
    enabled: false,
    model: '',
    binary: 'whisper-cli',
    language: 'en',
    keywords: [],
    keywordCaseSensitive: false,
    includeTranscriptInAlerts: false,
  };

  let transcription = $derived($transcriptionSettings ?? defaultTranscription);

  /** Indicates a configuration error: enabled but no model path set. */
  let transcriptionModelMissing = $derived(transcription.enabled && !transcription.model.trim());

  function updateTranscription<K extends keyof TranscriptionSettings>(
    key: K,
    value: TranscriptionSettings[K]
  ) {
    settingsActions.updateSection('realtime', {
      transcription: { ...transcription, [key]: value },
    });
  }

  // Local state for the keyword input field
  let keywordInput = $state('');

  function addKeyword() {
    const trimmed = keywordInput.trim();
    if (!trimmed) return;
    const current = Array.isArray(transcription.keywords) ? transcription.keywords : [];
    if (current.includes(trimmed)) {
      keywordInput = '';
      return;
    }
    updateTranscription('keywords', [...current, trimmed]);
    keywordInput = '';
  }

  function removeKeyword(index: number) {
    const current = Array.isArray(transcription.keywords) ? transcription.keywords : [];
    updateTranscription(
      'keywords',
      current.filter((_, i) => i !== index)
    );
  }

  function handleKeywordKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      addKeyword();
    }
  }

  // ── FP filter level definitions for the shared component ─────────────
  const BADGE_OFF = 'bg-black/5 dark:bg-white/5 text-[var(--color-base-content)]';
  const BADGE_SUCCESS = 'bg-[var(--color-success)] text-[var(--color-success-content)]';
  const BADGE_INFO = 'bg-[var(--color-info)] text-[var(--color-info-content)]';
  const BADGE_WARNING = 'bg-[var(--color-warning)] text-[var(--color-warning-content)]';
  const BADGE_ERROR = 'bg-[var(--color-error)] text-[var(--color-error-content)]';

  const BIRD_FP_LEVELS: FilterLevel[] = [
    {
      value: 0,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.off',
      badgeClass: BADGE_OFF,
    },
    {
      value: 1,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.lenient',
      badgeClass: BADGE_SUCCESS,
    },
    {
      value: 2,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.moderate',
      badgeClass: BADGE_INFO,
    },
    {
      value: 3,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.balanced',
      badgeClass: BADGE_WARNING,
    },
    {
      value: 4,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.strict',
      badgeClass: BADGE_ERROR,
    },
    {
      value: 5,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.maximum',
      badgeClass: BADGE_ERROR,
    },
  ];

  // Bat has only 3 meaningful levels (fixed 50% overlap, 4 detections in window):
  // Off=bypass (1 det), Moderate=2 det, Strict=3 det.
  // Lenient(1 det) is functionally identical to Off, so it's excluded.
  const BAT_FP_LEVELS: FilterLevel[] = [
    {
      value: 0,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.off',
      badgeClass: BADGE_OFF,
    },
    {
      value: 2,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.moderate',
      badgeClass: BADGE_INFO,
    },
    {
      value: 4,
      nameKey: 'settings.main.sections.falsePositiveFilter.levelNames.strict',
      badgeClass: BADGE_ERROR,
    },
  ];

  // Bat FP filter calculation helpers.
  // The bat model uses a fixed 50% overlap (1.5s step for 3s clip),
  // yielding 4 possible detections in a 6-second reference window.
  const BAT_MAX_DETECTIONS_IN_WINDOW = 4;

  function calculateBatMinDetections(level: number): number {
    if (level === 0) return 1;
    const levelData = safeArrayAccess(falsePositiveFilterLevels, level);
    if (!levelData) return 1;
    const required = BAT_MAX_DETECTIONS_IN_WINDOW * levelData.threshold - FLOAT_EPSILON;
    return Math.max(1, Math.ceil(required));
  }

  const BAT_FP_DESCRIPTION_KEYS: Record<number, string> = {
    0: 'analysis.detection.batFalsePositiveFilter.levels.off',
    2: 'analysis.detection.batFalsePositiveFilter.levels.moderate',
    4: 'analysis.detection.batFalsePositiveFilter.levels.strict',
  };

  function getBatFalsePositiveFilterDescription(level: number): string {
    // eslint-disable-next-line security/detect-object-injection
    const descKey = BAT_FP_DESCRIPTION_KEYS[level];
    if (!descKey) return '';

    const baseDescription = t(descKey);
    if (level === 0) return baseDescription;

    const minDet = calculateBatMinDetections(level);
    return t('analysis.detection.batFalsePositiveFilter.detectionCount', {
      count: minDet.toString(),
      description: baseDescription,
    });
  }

  function updateThreshold(value: number) {
    settingsActions.updateSection('voicewatch', { threshold: value });
  }

  onMount(() => {
    loadBirdnetLocales();
  });
</script>

<main class="settings-page-content" aria-label={t('analysis.title')}>
  <div class="space-y-6">
    <!-- 1. Bird Detection -->
    <SettingsSection
      title={t('analysis.bird.title')}
      description={t('analysis.bird.description')}
      defaultOpen={true}
      originalData={{
        threshold: store.originalData.voicewatch?.threshold,
        locale: store.originalData.voicewatch?.locale,
        fpFilter: store.originalData.realtime?.falsePositiveFilter?.level ?? 0,
      }}
      currentData={{
        threshold: birdnet?.threshold,
        locale: birdnet?.locale,
        fpFilter: falsePositiveFilter.level,
      }}
    >
      <!-- Names and credits the bundled detector. VoiceWatch ships Silero VAD,
           not a BirdNET classifier; see the Credits section of README.md.
           Upstream BirdNET/Cornell/Chemnitz credit lives on the About page. -->
      <SettingsNote><span>{t('analysis.detection.builtInModel')}</span></SettingsNote>

      <div class="mt-4 grid grid-cols-1 md:grid-cols-2 gap-6">
        <NumberField
          label={t('analysis.detection.confidenceThreshold.label')}
          value={birdnet?.threshold ?? 0.3}
          onUpdate={updateThreshold}
          min={0}
          max={1}
          step={0.05}
          disabled={store.isLoading || store.isSaving}
          helpText={t('analysis.detection.confidenceThreshold.helpText')}
        />

        <SelectDropdown
          options={birdnetLocaleOptions}
          value={birdnet?.locale ?? 'en'}
          label={t('analysis.detection.locale.label')}
          helpText={t('analysis.detection.locale.helpText')}
          disabled={store.isLoading || store.isSaving || birdnetLocales.loading}
          variant="select"
          groupBy={false}
          searchable={true}
          onChange={value => updateBirdnetSetting('locale', value as string)}
        >
          {#snippet renderOption(option)}
            {@const localeOption = option as BirdnetLocaleOption}
            <div class="flex items-center gap-2">
              <FlagIcon locale={localeOption.localeCode} className="size-4" />
              <span>{localeOption.label}</span>
            </div>
          {/snippet}
          {#snippet renderSelected(options)}
            {#if options[0]}
              {@const localeOption = options[0] as BirdnetLocaleOption}
              <span class="flex items-center gap-2">
                <FlagIcon locale={localeOption.localeCode} className="size-4" />
                <span>{localeOption.label}</span>
              </span>
            {:else}
              <span>{birdnet?.locale ?? 'en'}</span>
            {/if}
          {/snippet}
        </SelectDropdown>
      </div>

      <!-- Bird False Positive Filter -->
      <div class="mt-6">
        <FalsePositiveFilterControl
          id="false-positive-filter-level"
          level={falsePositiveFilter.level}
          levels={BIRD_FP_LEVELS}
          onUpdate={updateFalsePositiveFilterLevel}
          getDescription={level => getFalsePositiveFilterDescription(level, birdnet?.overlap ?? 0)}
          disabled={store.isLoading || store.isSaving}
        />
      </div>

      {#if falsePositiveFilter.level === 0}
        <SettingsNote>
          {#snippet icon()}<AlertTriangle class="size-4 text-[var(--color-warning)]" />{/snippet}
          <span>{t('settings.main.sections.falsePositiveFilter.warningOff')}</span>
        </SettingsNote>
      {:else if falsePositiveFilter.level >= 4}
        <SettingsNote>
          <span>{t('settings.main.sections.falsePositiveFilter.hardwareNote')}</span>
        </SettingsNote>
      {/if}
    </SettingsSection>

    <!-- 2. Bat Detection (only when bat detection is enabled) -->
    {#if bat.enabled}
      <SettingsSection
        title={t('analysis.bat.title')}
        description={t('analysis.bat.description')}
        defaultOpen={true}
        originalData={{
          batThreshold: store.originalData.bat?.threshold,
          batNighttimeOnly: store.originalData.bat?.nighttimeOnly,
          batUltrasonicFilter: store.originalData.bat?.ultrasonicFilter?.enabled ?? true,
          batFPFilter: store.originalData.bat?.falsePositiveFilter?.level ?? 0,
        }}
        currentData={{
          batThreshold: bat.threshold,
          batNighttimeOnly: bat.nighttimeOnly,
          batUltrasonicFilter: bat.ultrasonicFilter?.enabled ?? true,
          batFPFilter: batFPLevel,
        }}
      >
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <NumberField
            label={t('analysis.detection.batThreshold.label')}
            value={bat.threshold}
            onUpdate={updateBatThreshold}
            min={0.01}
            max={0.99}
            step={0.01}
            disabled={store.isLoading || store.isSaving}
            helpText={t('analysis.detection.batThreshold.helpText')}
          />
          <div></div>

          <Checkbox
            checked={bat.nighttimeOnly ?? true}
            label={t('analysis.detection.batNighttimeOnly.label')}
            helpText={t('analysis.detection.batNighttimeOnly.helpText')}
            disabled={store.isLoading || store.isSaving}
            onchange={updateBatNighttimeOnly}
          />
          <Checkbox
            checked={bat.ultrasonicFilter?.enabled ?? true}
            label={t('analysis.detection.batUltrasonicFilter.label')}
            helpText={t('analysis.detection.batUltrasonicFilter.helpText')}
            disabled={store.isLoading || store.isSaving}
            onchange={updateBatUltrasonicFilter}
          />
        </div>

        <!-- Bat False Positive Filter -->
        <div class="mt-6">
          <FalsePositiveFilterControl
            id="bat-false-positive-filter-level"
            level={batFPLevel}
            levels={BAT_FP_LEVELS}
            onUpdate={updateBatFalsePositiveFilterLevel}
            getDescription={level => getBatFalsePositiveFilterDescription(level)}
            disabled={store.isLoading || store.isSaving}
          />
        </div>

        {#if batFPLevel === 0}
          <SettingsNote>
            {#snippet icon()}<AlertTriangle class="size-4 text-[var(--color-warning)]" />{/snippet}
            <span>{t('analysis.detection.batFalsePositiveFilter.warningOff')}</span>
          </SettingsNote>
        {/if}
      </SettingsSection>
    {/if}

    <!-- 4. Dynamic Threshold -->
    <SettingsSection
      title={t('settings.main.sections.dynamicThreshold.title')}
      description={t('settings.main.sections.dynamicThreshold.description')}
      originalData={store.originalData.realtime?.dynamicThreshold}
      currentData={store.formData.realtime?.dynamicThreshold}
    >
      <SettingsNote><span>{t('analysis.dynamicThreshold.birdOnlyNote')}</span></SettingsNote>

      <div class="mt-4">
        <Checkbox
          checked={dynamicThreshold.enabled}
          label={t('settings.main.sections.dynamicThreshold.enable.label')}
          helpText={t('settings.main.sections.dynamicThreshold.enable.helpText')}
          disabled={store.isLoading || store.isSaving}
          onchange={value => updateDynamicThreshold('enabled', value)}
        />
      </div>

      {#if dynamicThreshold.enabled}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mt-4">
          <NumberField
            label={t('settings.main.sections.dynamicThreshold.trigger.label')}
            value={dynamicThreshold.trigger}
            onUpdate={value => updateDynamicThreshold('trigger', value)}
            min={0.0}
            max={1.0}
            step={0.01}
            helpText={t('settings.main.sections.dynamicThreshold.trigger.helpText')}
            disabled={store.isLoading || store.isSaving}
          />

          <NumberField
            label={t('settings.main.sections.dynamicThreshold.minimum.label')}
            value={dynamicThreshold.min}
            onUpdate={value => updateDynamicThreshold('min', value)}
            min={0.0}
            max={0.99}
            step={0.01}
            helpText={t('settings.main.sections.dynamicThreshold.minimum.helpText')}
            disabled={store.isLoading || store.isSaving}
          />

          <NumberField
            label={t('settings.main.sections.dynamicThreshold.expireTime.label')}
            value={dynamicThreshold.validHours}
            onUpdate={value => updateDynamicThreshold('validHours', value)}
            min={0}
            max={1000}
            step={1}
            helpText={t('settings.main.sections.dynamicThreshold.expireTime.helpText')}
            disabled={store.isLoading || store.isSaving}
          />
        </div>
      {/if}
    </SettingsSection>

    <!-- 5. Advanced (collapsed by default) -->
    <SettingsSection
      title={t('analysis.advanced.title')}
      description={t('analysis.advanced.description')}
      defaultOpen={false}
      originalData={{
        threads: store.originalData.voicewatch?.threads,
        modelPath: store.originalData.voicewatch?.modelPath,
        labelPath: store.originalData.voicewatch?.labelPath,
      }}
      currentData={{
        threads: birdnet?.threads,
        modelPath: birdnet?.modelPath,
        labelPath: birdnet?.labelPath,
      }}
    >
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <NumberField
          label={t('settings.main.fields.tensorflowThreads.label')}
          value={birdnet?.threads ?? 0}
          onUpdate={value => updateBirdnetSetting('threads', value)}
          min={0}
          max={32}
          step={1}
          helpText={t('settings.main.fields.tensorflowThreads.helpText')}
          disabled={store.isLoading || store.isSaving}
        />
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mt-6">
        <TextInput
          id="model-path"
          value={birdnet?.modelPath ?? ''}
          label={t('settings.main.sections.customClassifier.modelPath.label')}
          placeholder={t('settings.main.sections.customClassifier.modelPath.placeholder')}
          helpText={t('settings.main.sections.customClassifier.modelPath.helpText')}
          disabled={store.isLoading || store.isSaving}
          onchange={value => updateBirdnetSetting('modelPath', value)}
        />

        <TextInput
          id="label-path"
          value={birdnet?.labelPath ?? ''}
          label={t('settings.main.sections.customClassifier.labelPath.label')}
          placeholder={t('settings.main.sections.customClassifier.labelPath.placeholder')}
          helpText={t('settings.main.sections.customClassifier.labelPath.helpText')}
          disabled={store.isLoading || store.isSaving}
          onchange={value => updateBirdnetSetting('labelPath', value)}
        />
      </div>
    </SettingsSection>

    <!-- 6. Transcription & Keyword Flagging -->
    <SettingsSection
      title={t('analysis.transcription.title')}
      description={t('analysis.transcription.description')}
      defaultOpen={false}
      originalData={store.originalData.realtime?.transcription}
      currentData={store.formData.realtime?.transcription}
    >
      <!-- Enable toggle -->
      <Checkbox
        checked={transcription.enabled}
        label={t('analysis.transcription.enable.label')}
        helpText={t('analysis.transcription.enable.helpText')}
        disabled={store.isLoading || store.isSaving}
        onchange={value => updateTranscription('enabled', value)}
      />

      <!-- Model path + language -->
      <div class="mt-4 grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <TextInput
            id="transcription-model-path"
            value={transcription.model}
            label={t('analysis.transcription.modelPath.label')}
            placeholder={t('analysis.transcription.modelPath.placeholder')}
            helpText={transcriptionModelMissing
              ? undefined
              : t('analysis.transcription.modelPath.helpText')}
            disabled={store.isLoading || store.isSaving}
            onchange={value => updateTranscription('model', value)}
          />
          {#if transcriptionModelMissing}
            <p
              id="transcription-model-required"
              class="mt-1 text-sm text-[var(--color-error)]"
              role="alert"
              aria-live="polite"
            >
              {t('analysis.transcription.modelPath.required')}
            </p>
          {/if}
        </div>

        <TextInput
          id="transcription-language"
          value={transcription.language}
          label={t('analysis.transcription.language.label')}
          placeholder={t('analysis.transcription.language.placeholder')}
          helpText={t('analysis.transcription.language.helpText')}
          disabled={store.isLoading || store.isSaving}
          onchange={value => updateTranscription('language', value)}
        />
      </div>

      <!-- Advanced: binary path -->
      <details
        class="mt-4 rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-200)]/50"
      >
        <summary
          class="cursor-pointer select-none px-4 py-3 text-sm font-medium text-[var(--color-base-content)] hover:bg-[var(--color-base-200)] rounded-lg transition-colors"
        >
          {t('analysis.transcription.advanced.title')}
        </summary>
        <div class="px-4 pb-4 pt-2">
          <TextInput
            id="transcription-binary"
            value={transcription.binary}
            label={t('analysis.transcription.advanced.binary.label')}
            placeholder={t('analysis.transcription.advanced.binary.placeholder')}
            helpText={t('analysis.transcription.advanced.binary.helpText')}
            disabled={store.isLoading || store.isSaving}
            onchange={value => updateTranscription('binary', value)}
          />
        </div>
      </details>

      <!-- Keyword list editor -->
      <div class="mt-6 space-y-3">
        <div>
          <label class="label justify-start" for="transcription-keyword-input">
            <span class="label-text capitalize">{t('analysis.transcription.keywords.label')}</span>
          </label>
          <div class="flex gap-2">
            <input
              id="transcription-keyword-input"
              type="text"
              class="input input-sm flex-1"
              placeholder={t('analysis.transcription.keywords.inputPlaceholder')}
              bind:value={keywordInput}
              disabled={store.isLoading || store.isSaving}
              onkeydown={handleKeywordKeydown}
              aria-label={t('analysis.transcription.keywords.label')}
              aria-describedby="transcription-keyword-help"
            />
            <button
              type="button"
              class="inline-flex items-center justify-center gap-1.5 h-8 px-3 text-sm font-medium rounded-lg bg-[var(--color-primary)] text-[var(--color-primary-content)] hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)] focus-visible:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              disabled={store.isLoading || store.isSaving || !keywordInput.trim()}
              onclick={addKeyword}
              aria-label={t('analysis.transcription.keywords.addButton')}
            >
              <Plus class="size-4" />
              {t('analysis.transcription.keywords.addButton')}
            </button>
          </div>
          <span id="transcription-keyword-help" class="help-text mt-1 block">
            {t('analysis.transcription.keywords.helpText')}
          </span>
        </div>

        <!-- Keyword chips -->
        {#if transcription.keywords.length === 0}
          <p class="text-sm text-[var(--color-base-content)]/60 italic">
            {t('analysis.transcription.keywords.emptyState')}
          </p>
        {:else}
          <div
            class="flex flex-wrap gap-2"
            role="list"
            aria-label={t('analysis.transcription.keywords.label')}
          >
            {#each transcription.keywords as keyword, i (keyword)}
              <span
                class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/15 pl-3 pr-1.5 py-1 text-sm font-medium text-[var(--color-primary)]"
                role="listitem"
              >
                {keyword}
                <button
                  type="button"
                  class="inline-flex items-center justify-center size-5 rounded-full hover:bg-[var(--color-primary)]/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)] focus-visible:ring-offset-1 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={store.isLoading || store.isSaving}
                  onclick={() => removeKeyword(i)}
                  aria-label={t('analysis.transcription.keywords.removeAriaLabel', { keyword })}
                >
                  <X class="size-3.5" />
                </button>
              </span>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Case-sensitive toggle -->
      <div class="mt-4">
        <Checkbox
          checked={transcription.keywordCaseSensitive}
          label={t('analysis.transcription.caseSensitive.label')}
          helpText={t('analysis.transcription.caseSensitive.helpText')}
          disabled={store.isLoading || store.isSaving}
          onchange={value => updateTranscription('keywordCaseSensitive', value)}
        />
      </div>

      <!-- Transcript-in-alerts opt-in (privacy: default off, verbatim speech
           leaves the device via external alert channels when enabled) -->
      <div class="mt-4">
        <Checkbox
          checked={transcription.includeTranscriptInAlerts}
          label={t('analysis.transcription.includeTranscriptInAlerts.label')}
          helpText={t('analysis.transcription.includeTranscriptInAlerts.helpText')}
          disabled={store.isLoading || store.isSaving}
          onchange={value => updateTranscription('includeTranscriptInAlerts', value)}
        />
      </div>
    </SettingsSection>
  </div>
  <SettingsPageActions />
</main>
