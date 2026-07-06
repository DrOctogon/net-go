<!--
  Analysis Settings Page Component

  Purpose: Configure VoiceWatch analysis settings including detection thresholds,
  false positive filtering, dynamic threshold, and manage the
  model gallery (install/uninstall additional classifier models).

  Features:
  - Two main tabs: Settings and Models
  - Settings tab: Detection settings, false positive filter,
    dynamic threshold, and advanced options
  - Models tab: Model gallery with Installed and Available tabs
  - Confidence threshold slider for bird detection
  - Bat detection threshold slider (visible when a bat model is installed)
  - Locale selector with flag icons for species labels
  - False positive filter with colored level badge
  - Dynamic threshold with enable/disable and parameter tuning
  - Advanced section with processing threads and custom classifier paths
  - License acceptance dialog for model installation
  - Remove confirmation dialog for model uninstallation
  - Real-time download progress via SSE

  Props: None - This is a page component that uses global settings stores

  @component
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { CatalogEntry, DownloadProgress } from '$lib/types/models';
  import {
    fetchCatalog,
    installModel,
    reinstallModel,
    uninstallModel,
    subscribeInstallProgress,
  } from '$lib/utils/modelsApi';
  import { invalidateModels } from '$lib/stores/models.svelte';
  import SettingsTabs from '$lib/desktop/features/settings/components/SettingsTabs.svelte';
  import type { TabDefinition } from '$lib/desktop/features/settings/components/SettingsTabs.svelte';
  import SettingsSection from '$lib/desktop/features/settings/components/SettingsSection.svelte';
  import SettingsNote from '$lib/desktop/features/settings/components/SettingsNote.svelte';
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
  import { cn } from '$lib/utils/cn.js';
  import { api, ApiError } from '$lib/utils/api';
  import { toastActions } from '$lib/stores/toast';
  import { formatBytes } from '$lib/utils/formatters';
  import { safeArrayAccess } from '$lib/utils/security';
  import { t } from '$lib/i18n';
  import {
    Download,
    Trash2,
    Shield,
    ShieldAlert,
    Package,
    BrainCircuit,
    AlertTriangle,
    TriangleAlert,
    Loader2,
    RefreshCw,
    Radar,
    Globe,
    XCircle,
    X,
    Check,
    Plus,
    Settings as SettingsIcon,
  } from '@lucide/svelte';

  import logoBirdnet from '$lib/assets/logos/logo-birdnet.png';
  import logoGoogle from '$lib/assets/logos/logo-google.png';
  import logoJyu from '$lib/assets/logos/logo-jyu.jpeg';

  const MODEL_LOGOS: Record<string, string> = {
    human_voice: logoBirdnet,
    perch: logoGoogle,
    bsg: logoJyu,
  };

  function getModelLogo(id: string): string | null {
    for (const [prefix, logo] of Object.entries(MODEL_LOGOS)) {
      if (id.startsWith(prefix)) return logo;
    }
    return null;
  }

  // ── Page-level tab state ──────────────────────────────────────────────
  type PageTab = 'settings' | 'models';
  let pageTab = $state<PageTab>('settings');

  // ── Gallery (Models tab) state ────────────────────────────────────────
  let catalog = $state<CatalogEntry[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let installingId = $state<string | null>(null);
  let deletingId = $state<string | null>(null);
  let reinstallingId = $state<string | null>(null);
  let downloadProgress = $state<DownloadProgress | null>(null);
  let completionTimer: ReturnType<typeof setTimeout> | undefined;

  let licenseModel = $state<CatalogEntry | null>(null);
  let removeConfirmModel = $state<CatalogEntry | null>(null);

  // Element bindings should NOT use $state - causes showModal() to fail
  let licenseDialogRef: HTMLDialogElement | null = null;
  let removeDialogRef: HTMLDialogElement | null = null;

  type GalleryTab = 'installed' | 'available';
  let galleryTab = $state<GalleryTab>('installed');

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

  // Check if a bat model is installed
  const hasBatModel = $derived(catalog.some(e => e.installed && e.category === 'bat'));
  const batFPLevel = $derived(bat.falsePositiveFilter?.level ?? 0);

  // ── Derived catalog views ─────────────────────────────────────────────
  const installedEntries = $derived(catalog.filter(e => e.installed));
  const availableWildlife = $derived(
    catalog.filter(e => !e.installed && e.category === 'wildlife')
  );
  const availableBirds = $derived(catalog.filter(e => !e.installed && e.category === 'bird'));
  const availableBats = $derived(catalog.filter(e => !e.installed && e.category === 'bat'));
  const availableGeomodels = $derived(
    catalog.filter(e => !e.installed && e.category === 'geomodel')
  );

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

  // ── Gallery tab definitions ───────────────────────────────────────────
  const galleryTabs: TabDefinition[] = $derived([
    {
      id: 'installed',
      label: t('analysis.gallery.tabs.installed'),
      icon: Package,
      content: installedTabContent,
    },
    {
      id: 'available',
      label: t('analysis.gallery.tabs.available'),
      icon: Download,
      content: availableTabContent,
    },
  ]);

  // ── Page-level tab definitions ────────────────────────────────────────
  const pageTabs: TabDefinition[] = $derived([
    {
      id: 'settings',
      label: t('analysis.tabs.settings'),
      icon: SettingsIcon,
      content: settingsTabContent,
    },
    {
      id: 'models',
      label: t('analysis.tabs.models'),
      icon: Package,
      content: modelsTabContent,
    },
  ]);

  // ── SSE cleanup handle ────────────────────────────────────────────────
  let progressCleanup: (() => void) | null = null;

  onMount(() => {
    loadCatalog();
    loadBirdnetLocales();
    return () => {
      if (progressCleanup) progressCleanup();
      clearTimeout(completionTimer);
    };
  });

  // ── Gallery functions ─────────────────────────────────────────────────
  async function loadCatalog() {
    loading = true;
    error = null;
    try {
      const response = await fetchCatalog();
      catalog = response.catalog;
    } catch (e) {
      error = e instanceof Error ? e.message : t('analysis.gallery.errors.catalogLoadFailed');
    } finally {
      loading = false;
    }
  }

  function openLicenseDialog(entry: CatalogEntry) {
    licenseModel = entry;
    licenseDialogRef?.showModal();
  }

  function closeLicenseDialog() {
    licenseDialogRef?.close();
    licenseModel = null;
  }

  async function handleInstall() {
    if (!licenseModel) return;
    const modelId = licenseModel.id;
    closeLicenseDialog();
    installingId = modelId;
    downloadProgress = null;

    try {
      await installModel(modelId);

      if (progressCleanup) progressCleanup();
      progressCleanup = subscribeInstallProgress(
        modelId,
        (progress: DownloadProgress) => {
          downloadProgress = progress;
        },
        () => {
          downloadProgress = {
            catalogId: modelId,
            status: 'complete',
            downloadedBytes: 0,
            totalBytes: 0,
            currentFile: 0,
            totalFiles: 0,
          };
          progressCleanup = null;
          clearTimeout(completionTimer);
          completionTimer = setTimeout(() => {
            if (installingId === modelId) {
              installingId = null;
              downloadProgress = null;
            }
            invalidateModels();
            loadCatalog();
          }, 2000);
        },
        (err: string) => {
          error = err;
          installingId = null;
          downloadProgress = null;
          progressCleanup = null;
        }
      );
    } catch (e) {
      error = e instanceof Error ? e.message : t('analysis.gallery.errors.installFailed');
      installingId = null;
    }
  }

  function openRemoveDialog(entry: CatalogEntry) {
    removeConfirmModel = entry;
    removeDialogRef?.showModal();
  }

  function closeRemoveDialog() {
    removeDialogRef?.close();
    removeConfirmModel = null;
  }

  async function handleUninstall() {
    if (!removeConfirmModel) return;
    const modelId = removeConfirmModel.id;
    closeRemoveDialog();
    deletingId = modelId;

    try {
      await uninstallModel(modelId);
      invalidateModels();
      await loadCatalog();
    } catch (e) {
      error = e instanceof Error ? e.message : t('analysis.gallery.errors.removeFailed');
    } finally {
      deletingId = null;
    }
  }

  async function handleReinstall(entry: CatalogEntry) {
    if (reinstallingId || installingId) return;
    reinstallingId = entry.id;
    downloadProgress = null;

    try {
      await reinstallModel(entry.id);

      if (progressCleanup) progressCleanup();
      progressCleanup = subscribeInstallProgress(
        entry.id,
        (progress: DownloadProgress) => {
          downloadProgress = progress;
        },
        () => {
          downloadProgress = {
            catalogId: entry.id,
            status: 'complete',
            downloadedBytes: 0,
            totalBytes: 0,
            currentFile: 0,
            totalFiles: 0,
          };
          progressCleanup = null;
          clearTimeout(completionTimer);
          completionTimer = setTimeout(() => {
            if (reinstallingId === entry.id) {
              reinstallingId = null;
              downloadProgress = null;
            }
            invalidateModels();
            loadCatalog();
          }, 2000);
        },
        (err: string) => {
          error = err;
          reinstallingId = null;
          downloadProgress = null;
          progressCleanup = null;
        }
      );
    } catch (e) {
      error = e instanceof Error ? e.message : t('analysis.gallery.errors.installFailed');
      reinstallingId = null;
    }
  }

  /** Compute download percentage for progress bar */
  function progressPercent(p: DownloadProgress): number {
    if (p.totalBytes <= 0) return 0;
    return Math.min(100, Math.round((p.downloadedBytes / p.totalBytes) * 100));
  }

  /** Human-readable status label */
  function statusLabel(status: DownloadProgress['status']): string {
    switch (status) {
      case 'downloading':
        return t('analysis.gallery.progress.downloading');
      case 'verifying':
        return t('analysis.gallery.progress.verifying');
      case 'loading':
        return t('analysis.gallery.progress.loading');
      case 'complete':
        return t('analysis.gallery.progress.complete');
      case 'failed':
        return t('analysis.gallery.progress.failed');
      default:
        return '';
    }
  }
</script>

<!-- ── Settings Tab Content ──────────────────────────────────────────── -->
{#snippet settingsTabContent()}
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
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
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

    <!-- 2. Bat Detection (only when a bat model is installed) -->
    {#if hasBatModel}
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
    </SettingsSection>
  </div>
{/snippet}

<!-- ── Models Tab Content ────────────────────────────────────────────── -->
{#snippet modelsTabContent()}
  <SettingsSection
    title={t('analysis.gallery.title')}
    description={t('analysis.gallery.description')}
    defaultOpen={true}
  >
    <SettingsTabs tabs={galleryTabs} bind:activeTab={galleryTab} showActions={false} />
  </SettingsSection>
{/snippet}

<!-- ── Gallery: Installed Tab ────────────────────────────────────────── -->
{#snippet installedTabContent()}
  <div class="space-y-4">
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Loader2 class="size-6 animate-spin text-[var(--color-primary)]" />
        <span class="ml-3 text-sm text-[var(--color-base-content)]/80"
          >{t('analysis.gallery.loading')}</span
        >
      </div>
    {:else if error}
      <div
        class="flex items-center gap-3 rounded-lg border border-[var(--color-error)]/30 bg-[var(--color-error)]/10 px-4 py-3 text-sm"
        role="alert"
      >
        <AlertTriangle class="size-5 shrink-0 text-[var(--color-error)]" />
        <span class="text-[var(--color-base-content)]">{error}</span>
        <button
          onclick={loadCatalog}
          class="ml-auto flex items-center gap-1.5 rounded-md bg-[var(--color-base-200)] px-3 py-1.5 text-xs font-medium text-[var(--color-base-content)] hover:bg-[var(--color-base-300)] transition-colors"
        >
          <RefreshCw class="size-3.5" />
          {t('analysis.gallery.retry')}
        </button>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <!-- Built-in VoiceWatch model (always present) -->
        <div
          class="rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-200)] p-4"
        >
          <div class="flex items-start gap-3">
            <img src={logoBirdnet} alt="" class="size-10 shrink-0 rounded-lg" />
            <div class="min-w-0 flex-1">
              <h4 class="text-sm font-semibold text-[var(--color-base-content)]">BirdNET v2.4</h4>
              <p class="mt-0.5 line-clamp-2 text-xs text-[var(--color-base-content)]/80">
                {t('analysis.gallery.builtInDescription')}
              </p>
              <p class="mt-1 text-xs text-[var(--color-base-content)]/80">
                Cornell Lab of Ornithology / Chemnitz University
              </p>
            </div>
          </div>
          <div
            class="mt-3 flex items-center justify-between border-t border-[var(--color-base-300)] pt-3"
          >
            <div class="flex items-center gap-2 text-xs text-[var(--color-base-content)]/80">
              <span>v2.4</span>
              <span>{t('analysis.gallery.species', { count: '6,000+' })}</span>
            </div>
            <span
              class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/15 px-2.5 py-0.5 text-xs font-medium text-[var(--color-primary)]"
            >
              {t('analysis.gallery.builtIn')}
            </span>
          </div>
        </div>

        <!-- Installed additional models -->
        {#each installedEntries as entry (entry.id)}
          {@const isDeleting = deletingId === entry.id}
          {@const isReinstalling = reinstallingId === entry.id}
          {@const reinstallProgress = isReinstalling ? downloadProgress : null}
          {@const logo = getModelLogo(entry.id)}
          <div
            class="rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-200)] p-4"
          >
            <div class="flex items-start gap-3">
              {#if logo}
                <img src={logo} alt="" class="size-10 shrink-0 rounded-lg" />
              {:else}
                <div class="shrink-0 rounded-lg bg-[var(--color-primary)]/10 p-2.5">
                  {#if entry.category === 'geomodel'}
                    <Globe size={24} class="text-[var(--color-primary)]" />
                  {:else if entry.category === 'bat'}
                    <Radar size={24} class="text-[var(--color-primary)]" />
                  {:else}
                    <BrainCircuit size={24} class="text-[var(--color-primary)]" />
                  {/if}
                </div>
              {/if}
              <div class="min-w-0 flex-1">
                <h4 class="text-sm font-semibold text-[var(--color-base-content)]">{entry.name}</h4>
                <p class="mt-0.5 line-clamp-2 text-xs text-[var(--color-base-content)]/80">
                  {entry.description}
                </p>
                {#if entry.upstreamUrl}
                  <a
                    href={entry.upstreamUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="mt-1 inline-block text-xs text-[var(--color-primary)]/80 hover:text-[var(--color-primary)] transition-colors"
                  >
                    {entry.author}
                  </a>
                {:else}
                  <p class="mt-1 text-xs text-[var(--color-base-content)]/80">{entry.author}</p>
                {/if}
              </div>
            </div>
            <!-- Progress bar (shown during reinstall, not for companion entries) -->
            {#if reinstallProgress}
              <div class="mt-3 space-y-1.5">
                {#if reinstallProgress.status === 'complete'}
                  <div
                    class="flex items-center gap-2 text-sm font-medium text-[var(--color-success)]"
                  >
                    <Check class="h-4 w-4" />
                    <span>{t('analysis.gallery.reinstallComplete')}</span>
                  </div>
                {:else}
                  <div class="h-2 w-full overflow-hidden rounded-full bg-[var(--color-base-300)]">
                    <div
                      class="h-full rounded-full bg-[var(--color-primary)] transition-all duration-300"
                      style:width="{progressPercent(reinstallProgress)}%"
                    ></div>
                  </div>
                  <div
                    class="flex items-center justify-between text-xs text-[var(--color-base-content)]/80"
                  >
                    <span>
                      {statusLabel(
                        reinstallProgress.status
                      )}{#if reinstallProgress.status === 'downloading' && reinstallProgress.totalFiles > 1}
                        ({reinstallProgress.currentFile}/{reinstallProgress.totalFiles})
                      {/if}
                    </span>
                    {#if reinstallProgress.status === 'downloading' && reinstallProgress.totalBytes > 0}
                      <span>
                        {formatBytes(reinstallProgress.downloadedBytes)} / {formatBytes(
                          reinstallProgress.totalBytes
                        )}
                      </span>
                    {/if}
                  </div>
                {/if}
              </div>
            {/if}
            <!-- Incompatible warning for installed models -->
            {#if !entry.compatible}
              <div
                class="mt-3 flex items-start gap-2 rounded-lg bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-400"
              >
                <XCircle class="h-4 w-4 shrink-0 mt-0.5" />
                <span>{entry.incompatibleReason || t('analysis.gallery.onnxRuntimeMissing')}</span>
              </div>
            {/if}
            <!-- Metadata grid -->
            <div
              class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 border-t border-[var(--color-base-300)] pt-3 text-xs"
            >
              {#if entry.region}
                <div class="text-[var(--color-base-content)]/80">
                  {t('analysis.gallery.regionLabel')}
                </div>
                <div class="text-[var(--color-base-content)]/80">{entry.region}</div>
              {/if}
              <div class="text-[var(--color-base-content)]/80">
                {t('analysis.gallery.speciesLabel')}
              </div>
              <div class="text-[var(--color-base-content)]/80">
                {t('analysis.gallery.species', { count: entry.speciesCount })}
              </div>
              <div class="text-[var(--color-base-content)]/80">
                {t('analysis.gallery.license.license')}
              </div>
              <div>
                {#if entry.commercialUse}
                  <span
                    class="inline-flex items-center gap-1 rounded-full bg-[var(--color-success)]/15 px-2 py-0.5 text-xs text-[var(--color-success)]"
                    title={t('analysis.gallery.license.commercialUseAllowed')}
                  >
                    <Shield class="size-3" />
                    {entry.license}
                  </span>
                {:else}
                  <span
                    class="inline-flex items-center gap-1 rounded-full bg-[var(--color-warning)]/15 px-2 py-0.5 text-xs text-[var(--color-warning)]"
                    title={t('analysis.gallery.license.nonCommercialOnly')}
                  >
                    <ShieldAlert class="size-3" />
                    {entry.license}
                  </span>
                {/if}
              </div>
            </div>
            <!-- Geomodel badge (for acoustic classifiers that bundle a geomodel) -->
            {#if entry.hasGeomodel && entry.category !== 'geomodel'}
              <div class="mt-2">
                <span
                  class="inline-flex items-center gap-1 rounded-full bg-[var(--color-info)]/15 px-2.5 py-0.5 text-xs font-medium text-[var(--color-info)]"
                >
                  {t('analysis.gallery.geomodelBadge')}
                </span>
              </div>
            {/if}
            <!-- Action footer -->
            <div class="mt-3 flex items-center justify-end gap-2">
              <button
                onclick={() => handleReinstall(entry)}
                disabled={reinstallingId !== null || installingId !== null || isDeleting}
                class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-[var(--color-base-content)]/80 hover:bg-[var(--color-base-300)] transition-colors disabled:opacity-50"
                aria-label="{t('analysis.gallery.reinstall')} {entry.name}"
              >
                {#if isReinstalling}
                  <Loader2 class="size-3.5 animate-spin" />
                  {t('analysis.gallery.reinstalling')}
                {:else}
                  <RefreshCw class="size-3.5" />
                  {t('analysis.gallery.reinstall')}
                {/if}
              </button>
              <button
                onclick={() => openRemoveDialog(entry)}
                disabled={isDeleting || isReinstalling || installingId !== null}
                class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-[var(--color-error)] hover:bg-[var(--color-error)]/10 transition-colors disabled:opacity-50"
                aria-label="{t('analysis.gallery.remove')} {entry.name}"
              >
                {#if isDeleting}
                  <Loader2 class="size-3.5 animate-spin" />
                  {t('analysis.gallery.removing')}
                {:else}
                  <Trash2 class="size-3.5" />
                  {t('analysis.gallery.remove')}
                {/if}
              </button>
            </div>
          </div>
        {/each}
      </div>

      {#if installedEntries.length === 0}
        <p class="py-4 text-center text-sm text-[var(--color-base-content)]/80">
          {t('analysis.gallery.noInstalledModels')}
        </p>
      {/if}
    {/if}
  </div>
{/snippet}

{#snippet modelCard(entry: CatalogEntry)}
  {@const isInstalling = installingId === entry.id}
  {@const progress = isInstalling ? downloadProgress : null}
  {@const logo = getModelLogo(entry.id)}
  <div
    class={cn(
      'flex h-full flex-col rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-200)] p-4',
      !entry.compatible && 'opacity-60'
    )}
  >
    <!-- Header: logo + name/description/author -->
    <div class="flex items-start gap-3">
      {#if logo}
        <img src={logo} alt="" class="size-10 shrink-0 rounded-lg" />
      {:else}
        <div class="shrink-0 rounded-lg bg-[var(--color-primary)]/10 p-2.5">
          {#if entry.category === 'geomodel'}
            <Globe size={24} class="text-[var(--color-primary)]" />
          {:else if entry.category === 'bat'}
            <Radar size={24} class="text-[var(--color-primary)]" />
          {:else}
            <BrainCircuit size={24} class="text-[var(--color-primary)]" />
          {/if}
        </div>
      {/if}
      <div class="min-w-0 flex-1">
        <h4 class="text-sm font-semibold text-[var(--color-base-content)]">
          {entry.name}
        </h4>
        <p class="mt-0.5 line-clamp-2 text-xs text-[var(--color-base-content)]/80">
          {entry.description}
        </p>
        {#if entry.upstreamUrl}
          <a
            href={entry.upstreamUrl}
            target="_blank"
            rel="noopener noreferrer"
            class="mt-1 inline-block text-xs text-[var(--color-primary)]/80 hover:text-[var(--color-primary)] transition-colors"
          >
            {entry.author}
          </a>
        {:else}
          <p class="mt-1 text-xs text-[var(--color-base-content)]/80">{entry.author}</p>
        {/if}
      </div>
    </div>

    <!-- Progress bar (shown during install, not for companion entries) -->
    {#if progress}
      <div class="mt-3 space-y-1.5">
        {#if progress.status === 'complete'}
          <div class="flex items-center gap-2 text-sm font-medium text-[var(--color-success)]">
            <Check class="h-4 w-4" />
            <span>{t('analysis.gallery.progress.complete')}</span>
          </div>
        {:else}
          <div class="h-2 w-full overflow-hidden rounded-full bg-[var(--color-base-300)]">
            <div
              class="h-full rounded-full bg-[var(--color-primary)] transition-all duration-300"
              style:width="{progressPercent(progress)}%"
            ></div>
          </div>
          <div
            class="flex items-center justify-between text-xs text-[var(--color-base-content)]/80"
          >
            <span>
              {statusLabel(
                progress.status
              )}{#if progress.status === 'downloading' && progress.totalFiles > 1}
                ({progress.currentFile}/{progress.totalFiles})
              {/if}
            </span>
            {#if progress.status === 'downloading' && progress.totalBytes > 0}
              <span>
                {formatBytes(progress.downloadedBytes)} / {formatBytes(progress.totalBytes)}
              </span>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    <!-- Incompatible warning banner -->
    {#if !entry.compatible}
      <div
        class="mt-3 flex items-start gap-2 rounded-lg bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-400"
      >
        <TriangleAlert class="h-4 w-4 shrink-0 mt-0.5" />
        <span>{entry.incompatibleReason || t('analysis.gallery.onnxRuntimeRequired')}</span>
      </div>
    {/if}

    <!-- Metadata grid -->
    <div
      class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 border-t border-[var(--color-base-300)] pt-3 text-xs"
    >
      {#if entry.region}
        <div class="text-[var(--color-base-content)]/80">{t('analysis.gallery.regionLabel')}</div>
        <div class="text-[var(--color-base-content)]">{entry.region}</div>
      {/if}
      <div class="text-[var(--color-base-content)]/80">{t('analysis.gallery.speciesLabel')}</div>
      <div class="text-[var(--color-base-content)]">
        {t('analysis.gallery.species', { count: entry.speciesCount })}
      </div>
      <div class="text-[var(--color-base-content)]/80">{t('analysis.gallery.license.license')}</div>
      <div>
        {#if entry.commercialUse}
          <span
            class="inline-flex items-center gap-1 rounded-full bg-[var(--color-success)]/15 px-2 py-0.5 text-xs text-[var(--color-success)]"
            title={t('analysis.gallery.license.commercialUseAllowed')}
          >
            <Shield class="size-3" />
            {entry.license}
          </span>
        {:else}
          <span
            class="inline-flex items-center gap-1 rounded-full bg-[var(--color-warning)]/15 px-2 py-0.5 text-xs text-[var(--color-warning)]"
            title={t('analysis.gallery.license.nonCommercialOnly')}
          >
            <ShieldAlert class="size-3" />
            {entry.license}
          </span>
        {/if}
      </div>
    </div>

    <!-- Geomodel badge (for acoustic classifiers that bundle a geomodel) -->
    {#if entry.hasGeomodel && entry.category !== 'geomodel'}
      <div class="mt-2">
        <span
          class="inline-flex items-center gap-1 rounded-full bg-[var(--color-info)]/15 px-2.5 py-0.5 text-xs font-medium text-[var(--color-info)]"
        >
          {t('analysis.gallery.geomodelBadge')}
        </span>
      </div>
    {/if}

    <!-- Action footer (pushed to bottom via mt-auto) -->
    <div class="mt-auto flex items-center justify-end pt-3">
      <button
        onclick={() => openLicenseDialog(entry)}
        disabled={!entry.compatible || isInstalling || installingId !== null}
        class="inline-flex items-center gap-1.5 rounded-md bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-content)] hover:bg-[var(--color-primary)]/80 transition-colors disabled:opacity-50"
        aria-label="{t('analysis.gallery.install')} {entry.name}"
      >
        {#if isInstalling}
          <Loader2 class="size-3.5 animate-spin" />
          {t('analysis.gallery.installing')}
        {:else}
          <Download class="size-3.5" />
          {t('analysis.gallery.install')}
        {/if}
      </button>
    </div>
  </div>
{/snippet}

{#snippet availableTabContent()}
  <div class="space-y-6">
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Loader2 class="size-6 animate-spin text-[var(--color-primary)]" />
        <span class="ml-3 text-sm text-[var(--color-base-content)]/80"
          >{t('analysis.gallery.loading')}</span
        >
      </div>
    {:else if error}
      <div
        class="flex items-center gap-3 rounded-lg border border-[var(--color-error)]/30 bg-[var(--color-error)]/10 px-4 py-3 text-sm"
        role="alert"
      >
        <AlertTriangle class="size-5 shrink-0 text-[var(--color-error)]" />
        <span class="text-[var(--color-base-content)]">{error}</span>
        <button
          onclick={loadCatalog}
          class="ml-auto flex items-center gap-1.5 rounded-md bg-[var(--color-base-200)] px-3 py-1.5 text-xs font-medium text-[var(--color-base-content)] hover:bg-[var(--color-base-300)] transition-colors"
        >
          <RefreshCw class="size-3.5" />
          {t('analysis.gallery.retry')}
        </button>
      </div>
    {:else}
      <!-- Acoustic Classifiers section -->
      {#if availableWildlife.length > 0 || availableBirds.length > 0 || availableBats.length > 0}
        <div class="space-y-4">
          <h2 class="text-sm font-bold uppercase tracking-wider text-[var(--color-base-content)]">
            {t('analysis.gallery.sections.acoustic')}
          </h2>

          {#if availableWildlife.length > 0}
            <div>
              <h3
                class="mb-3 text-sm font-semibold uppercase tracking-wider text-[var(--color-base-content)]/80"
              >
                {t('analysis.gallery.categories.wildlife')}
              </h3>
              <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                {#each availableWildlife as entry (entry.id)}
                  {@render modelCard(entry)}
                {/each}
              </div>
            </div>
          {/if}

          {#if availableBirds.length > 0}
            <div>
              <h3
                class="mb-3 text-sm font-semibold uppercase tracking-wider text-[var(--color-base-content)]/80"
              >
                {t('analysis.gallery.categories.bird')}
              </h3>
              <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                {#each availableBirds as entry (entry.id)}
                  {@render modelCard(entry)}
                {/each}
              </div>
            </div>
          {/if}

          {#if availableBats.length > 0}
            <div>
              <h3
                class="mb-3 text-sm font-semibold uppercase tracking-wider text-[var(--color-base-content)]/80"
              >
                {t('analysis.gallery.categories.bat')}
              </h3>
              <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                {#each availableBats as entry (entry.id)}
                  {@render modelCard(entry)}
                {/each}
              </div>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Geomodels section -->
      {#if availableGeomodels.length > 0}
        <div class="space-y-4">
          <h2 class="text-sm font-bold uppercase tracking-wider text-[var(--color-base-content)]">
            {t('analysis.gallery.sections.geomodel')}
          </h2>
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {#each availableGeomodels as entry (entry.id)}
              {@render modelCard(entry)}
            {/each}
          </div>
        </div>
      {/if}

      {#if availableWildlife.length === 0 && availableBirds.length === 0 && availableBats.length === 0 && availableGeomodels.length === 0}
        <p class="py-8 text-center text-sm text-[var(--color-base-content)]/80">
          {t('analysis.gallery.noAvailableModels')}
        </p>
      {/if}
    {/if}
  </div>
{/snippet}

<!-- ── Main Content ──────────────────────────────────────────────────── -->
<main class="settings-page-content space-y-6" aria-label={t('analysis.title')}>
  <SettingsTabs tabs={pageTabs} bind:activeTab={pageTab} />
</main>

<!-- License Acceptance Dialog -->
<dialog
  bind:this={licenseDialogRef}
  class="m-auto w-full max-w-md rounded-xl border border-[var(--color-base-300)] bg-[var(--color-base-100)] p-0 shadow-xl backdrop:bg-black/50"
  aria-labelledby="license-dialog-title"
>
  {#if licenseModel}
    <div class="p-6">
      <h3 id="license-dialog-title" class="text-lg font-semibold text-[var(--color-base-content)]">
        {t('analysis.gallery.license.title')}
      </h3>
      <div class="mt-4 space-y-3">
        <table
          class="w-full overflow-hidden rounded-lg border-separate border-spacing-0 bg-[var(--color-base-200)] text-sm"
        >
          <tbody>
            <tr>
              <th
                scope="row"
                class="px-4 pt-4 pb-1 text-left font-normal align-top text-[var(--color-base-content)]/80"
                >{t('analysis.gallery.license.model')}</th
              >
              <td
                class="px-4 pt-4 pb-1 text-right align-top font-medium text-[var(--color-base-content)]"
                >{licenseModel.name}</td
              >
            </tr>
            <tr>
              <th
                scope="row"
                class="px-4 py-1 text-left font-normal align-top text-[var(--color-base-content)]/80"
                >{t('analysis.gallery.license.author')}</th
              >
              <td class="px-4 py-1 text-right align-top text-[var(--color-base-content)]"
                >{licenseModel.author}</td
              >
            </tr>
            <tr>
              <th
                scope="row"
                class="px-4 py-1 text-left font-normal align-top text-[var(--color-base-content)]/80"
                >{t('analysis.gallery.license.license')}</th
              >
              <td class="px-4 py-1 text-right align-top text-[var(--color-base-content)]"
                >{licenseModel.license}</td
              >
            </tr>
            <tr>
              <th
                scope="row"
                class="px-4 py-1 text-left font-normal align-top text-[var(--color-base-content)]/80"
                >{t('analysis.gallery.license.commercialUse')}</th
              >
              <td class="px-4 py-1 text-right align-top">
                {#if licenseModel.commercialUse}
                  <span class="inline-flex items-center gap-1 text-[var(--color-success)]">
                    <Shield class="size-3.5" />
                    {t('analysis.gallery.license.allowed')}
                  </span>
                {:else}
                  <span class="inline-flex items-center gap-1 text-[var(--color-warning)]">
                    <ShieldAlert class="size-3.5" />
                    {t('analysis.gallery.license.notAllowed')}
                  </span>
                {/if}
              </td>
            </tr>
            <tr>
              <th
                scope="row"
                class="px-4 pt-1 pb-4 text-left font-normal align-top text-[var(--color-base-content)]/80"
                >{t('analysis.gallery.license.downloadSize')}</th
              >
              <td class="px-4 pt-1 pb-4 text-right align-top text-[var(--color-base-content)]"
                >{formatBytes(licenseModel.totalSizeBytes)}</td
              >
            </tr>
          </tbody>
        </table>

        {#if !licenseModel.commercialUse}
          <div
            class="flex items-start gap-2 rounded-lg border border-[var(--color-warning)]/30 bg-[var(--color-warning)]/10 px-3 py-2.5 text-sm"
          >
            <ShieldAlert class="mt-0.5 size-4 shrink-0 text-[var(--color-warning)]" />
            <p class="text-[var(--color-base-content)]">
              {t('analysis.gallery.license.nonCommercialWarning')}
            </p>
          </div>
        {/if}
      </div>

      <div class="mt-6 flex justify-end gap-3">
        <button
          onclick={closeLicenseDialog}
          class="rounded-lg border border-[var(--color-base-300)] px-4 py-2 text-sm font-medium text-[var(--color-base-content)] hover:bg-[var(--color-base-200)] transition-colors"
        >
          {t('common.cancel')}
        </button>
        <button
          onclick={handleInstall}
          class="inline-flex items-center gap-2 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-content)] hover:bg-[var(--color-primary)]/80 transition-colors"
        >
          <Download class="size-4" />
          {t('analysis.gallery.license.acceptAndInstall')}
        </button>
      </div>
    </div>
  {/if}
</dialog>

<!-- Remove Confirmation Dialog -->
<dialog
  bind:this={removeDialogRef}
  class="m-auto rounded-xl border border-[var(--color-base-300)] bg-[var(--color-base-100)] p-0 shadow-xl backdrop:bg-black/50"
  aria-labelledby="remove-dialog-title"
>
  {#if removeConfirmModel}
    <div class="w-full max-w-md p-6">
      <div class="flex items-start gap-3">
        <div class="shrink-0 rounded-full bg-[var(--color-error)]/10 p-2">
          <AlertTriangle class="size-5 text-[var(--color-error)]" />
        </div>
        <div>
          <h3
            id="remove-dialog-title"
            class="text-lg font-semibold text-[var(--color-base-content)]"
          >
            {t('analysis.gallery.removeDialog.title', { name: removeConfirmModel.name })}
          </h3>
          <p class="mt-2 text-sm text-[var(--color-base-content)]/80">
            {t('analysis.gallery.removeDialog.confirmation')}
          </p>
        </div>
      </div>

      <div class="mt-6 flex justify-end gap-3">
        <button
          onclick={closeRemoveDialog}
          class="rounded-lg border border-[var(--color-base-300)] px-4 py-2 text-sm font-medium text-[var(--color-base-content)] hover:bg-[var(--color-base-200)] transition-colors"
        >
          {t('common.cancel')}
        </button>
        <button
          onclick={handleUninstall}
          class="inline-flex items-center gap-2 rounded-lg bg-[var(--color-error)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--color-error)]/80 transition-colors"
        >
          <Trash2 class="size-4" />
          {t('analysis.gallery.remove')}
        </button>
      </div>
    </div>
  {/if}
</dialog>
