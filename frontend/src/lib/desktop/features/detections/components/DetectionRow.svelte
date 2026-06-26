<!--
  DetectionRow.svelte

  A comprehensive row component for displaying individual bird detection records with interactive features.
  Includes confidence indicators, status badges, weather information, and action controls.

  Usage:
  - Detection listings and tables
  - Search results display
  - Administrative detection management
  - Any context requiring detailed detection information

  Features:
  - Confidence circle visualization
  - Status badges (verified, false positive, etc.)
  - Weather condition display
  - Action menu wired to parent-owned handlers (review/lock/ignore/delete)
  - Thumbnail image support
  - Responsive design

  Props:
  - detection: Detection - The detection data object
  - onDetailsClick?: (id: number) => void - Handler for detail view
  - onReview / onMarkCorrect / onMarkFalsePositive / onToggleLock / onDelete -
    action callbacks supplied by the parent (DetectionsList) via useDetectionActions
-->
<script lang="ts">
  import ConfidenceCircle from '$lib/desktop/components/data/ConfidenceCircle.svelte';
  import VerificationBadges from '$lib/desktop/components/ui/VerificationBadges.svelte';
  import WeatherMetrics from '$lib/desktop/components/data/WeatherMetrics.svelte';
  import Checkbox from '$lib/desktop/components/forms/Checkbox.svelte';
  import SourceBadge from '$lib/desktop/features/dashboard/components/SourceBadge.svelte';
  import SpectrogramPlayer from '$lib/desktop/components/media/SpectrogramPlayer.svelte';
  import ActionMenu from '$lib/desktop/components/ui/ActionMenu.svelte';
  import { t } from '$lib/i18n';
  import { Flag, Mic } from '@lucide/svelte';
  import type { Detection } from '$lib/types/detection.types';
  import { navigation } from '$lib/stores/navigation.svelte';
  import { buildAppUrl } from '$lib/utils/urlHelpers';

  // Presentational row: the parent (DetectionsList) owns the action handlers
  // and the ConfirmModal via the shared useDetectionActions composable, and
  // passes them in as callbacks.
  interface Props {
    detection: Detection;
    onDetailsClick?: (_id: number) => void;
    selectionActive?: boolean;
    selected?: boolean;
    onToggleSelect?: (_id: string, _shiftKey: boolean) => void;
    onReview?: () => void;
    onMarkCorrect?: () => void;
    onMarkFalsePositive?: () => void;
    onToggleLock?: () => void;
    onDelete?: () => void;
  }

  let {
    detection,
    onDetailsClick,
    selectionActive = false,
    selected = false,
    onToggleSelect,
    onReview,
    onMarkCorrect,
    onMarkFalsePositive,
    onToggleLock,
    onDelete,
  }: Props = $props();

  function handleDetailsClick(e: Event) {
    e.preventDefault();
    if (onDetailsClick) {
      onDetailsClick(detection.id);
    } else {
      // Default navigation to detection detail page
      navigation.navigate(`/ui/detections/${detection.id}`);
    }
  }
</script>

<!-- DetectionRow now returns table cells for proper table structure -->
{#if selectionActive}
  <td class="w-10 text-center" onclick={e => e.stopPropagation()}>
    <Checkbox
      checked={selected}
      size="sm"
      variant="primary"
      onchange={(_checked, event) =>
        onToggleSelect?.(String(detection.id), (event as MouseEvent).shiftKey ?? false)}
    />
  </td>
{/if}

<!-- Date & Time -->
<td class="text-sm">
  <span>{detection.date} {detection.time}</span>
</td>

<!-- Weather Column -->
<td class="text-sm hidden md:table-cell">
  {#if detection.weather}
    <div class="flex flex-col gap-1">
      <WeatherMetrics
        weatherIcon={detection.weather.weatherIcon}
        weatherDescription={detection.weather.description}
        temperature={detection.weather.temperature}
        windSpeed={detection.weather.windSpeed}
        windGust={detection.weather.windGust}
        units={detection.weather.units}
        size="md"
        className="ml-1"
      />
    </div>
  {:else}
    <div class="text-[var(--color-base-content)] opacity-50 text-xs">
      {t('detections.weather.noData')}
    </div>
  {/if}
</td>

<!-- Source -->
<td class="text-sm hidden lg:table-cell">
  <SourceBadge {detection} variant="inline" />
</td>

<!-- Detection (mic icon + transcript) -->
<td class="text-sm">
  <div class="sp-detection-container">
    <!-- Static mic icon: replaces the bird thumbnail (decorative) -->
    <div class="sp-mic-icon-wrapper" aria-hidden="true">
      <Mic class="h-5 w-5" aria-hidden="true" />
    </div>
    <!-- Transcript text, single-line truncated; full text on hover -->
    <button
      onclick={handleDetailsClick}
      class="sp-transcript-text hover:text-primary transition-colors"
      title={detection.transcript}
    >
      {#if detection.transcript}
        {detection.transcript}
      {:else}
        <span class="sp-no-transcript">{t('detections.noTranscript')}</span>
      {/if}
    </button>
  </div>
</td>

<!-- Confidence -->
<td class="text-sm">
  <ConfidenceCircle confidence={detection.confidence} size="md" />
</td>

<!-- Status -->
<td>
  <div class="flex items-start gap-1.5 flex-wrap">
    {#if detection.flagged}
      {@const flagLabel = detection.keywordsHit?.length
        ? t('detections.flaggedByKeywords', { keywords: detection.keywordsHit.join(', ') })
        : t('detections.detail.transcript.flaggedNotice')}
      <span
        role="img"
        aria-label={flagLabel}
        title={flagLabel}
        class="inline-flex items-center text-[var(--color-warning)] shrink-0"
      >
        <Flag class="w-3.5 h-3.5" aria-hidden="true" />
      </span>
    {/if}
    <VerificationBadges {detection} />
  </div>
</td>

<!-- Recording/Spectrogram -->
<td class="hidden md:table-cell">
  <SpectrogramPlayer
    audioUrl={buildAppUrl(`/api/v2/audio/${detection.id}`)}
    detectionId={detection.id.toString()}
    spectrogramSize="md"
  />
</td>

<!-- Action Menu -->
<td onclick={e => e.stopPropagation()}>
  <ActionMenu
    {detection}
    {onMarkCorrect}
    {onMarkFalsePositive}
    {onReview}
    {onToggleLock}
    {onDelete}
  />
</td>

<style>
  /* Row container: mic icon + transcript side by side */
  .sp-detection-container {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  /* Fixed-size mic icon box matching the old thumbnail footprint */
  .sp-mic-icon-wrapper {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 30px;
    border-radius: 0.375rem;
    background-color: color-mix(in srgb, var(--color-base-200) 30%, transparent);
    color: var(--color-base-content);
    opacity: 0.45;
  }

  /* Transcript button: single-line, ellipsis overflow */
  .sp-transcript-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: inherit;
    cursor: pointer;
    text-align: left;
  }

  /* Muted italic fallback when there is no transcript */
  .sp-no-transcript {
    opacity: 0.4;
    font-style: italic;
    font-size: 0.875em;
  }
</style>
