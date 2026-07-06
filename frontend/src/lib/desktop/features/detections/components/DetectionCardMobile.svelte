<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import type { Detection } from '$lib/types/detection.types';
  import ConfidenceBadge from '$lib/desktop/features/dashboard/components/ConfidenceBadge.svelte';
  import WeatherBadge from '$lib/desktop/features/dashboard/components/WeatherBadge.svelte';
  import MoonBadge from '$lib/desktop/features/dashboard/components/MoonBadge.svelte';
  import SourceBadge from '$lib/desktop/features/dashboard/components/SourceBadge.svelte';
  import PlayOverlay from '$lib/desktop/features/dashboard/components/PlayOverlay.svelte';
  import ActionMenu from '$lib/desktop/components/ui/ActionMenu.svelte';
  import SpeakerAttributeChips from '$lib/desktop/components/data/SpeakerAttributeChips.svelte';
  import { cn } from '$lib/utils/cn';
  import { downloadDetectionAudio } from '$lib/utils/audioDownload';
  import { createSpectrogramLoader } from '$lib/utils/spectrogramLoader.svelte';
  import { DEFAULT_PLAYBACK_SPEED } from '$lib/utils/audio';
  import { get } from 'svelte/store';
  import { dashboardSettings } from '$lib/stores/settings';
  import { navigation } from '$lib/stores/navigation.svelte';
  import { t } from '$lib/i18n';
  import { Flag, Mic } from '@lucide/svelte';
  import { highlightKeywords } from '$lib/utils/highlightKeywords';

  const getDefaultAudioGain = () => get(dashboardSettings)?.defaultAudioGain ?? 0;
  const DEFAULT_AUDIO_FILTER_FREQ = 20;

  // Presentational card: the parent (DetectionsList) owns the action handlers
  // and the ConfirmModal via the shared useDetectionActions composable, and
  // passes them in as callbacks.
  interface Props {
    detection: Detection;
    onDetailsClick?: (_id: number) => void;
    onReview?: () => void;
    onMarkCorrect?: () => void;
    onMarkFalsePositive?: () => void;
    onToggleLock?: () => void;
    onDelete?: () => void;
  }

  let {
    detection,
    onDetailsClick,
    onReview,
    onMarkCorrect,
    onMarkFalsePositive,
    onToggleLock,
    onDelete,
  }: Props = $props();

  const loader = createSpectrogramLoader({ size: 'md', raw: true });

  let cardElement = $state<HTMLElement | undefined>(undefined);
  let isVisible = $state(false);
  let isMenuOpen = $state(false);

  let audioGainValue = $state(getDefaultAudioGain());
  let audioFilterFreq = $state(DEFAULT_AUDIO_FILTER_FREQ);
  let audioPlaybackSpeed = $state(DEFAULT_PLAYBACK_SPEED);

  $effect(() => {
    if (isVisible) {
      loader.start(detection.id);
    } else {
      loader.stop();
    }
  });

  function handleMenuOpen() {
    isMenuOpen = true;
  }

  function handleMenuClose() {
    isMenuOpen = false;
  }

  function handleViewDetails() {
    if (onDetailsClick) {
      onDetailsClick(detection.id);
    } else {
      navigation.navigate(`/ui/detections/${detection.id}`);
    }
  }

  // eslint-disable-next-line no-undef -- browser global
  let observer: IntersectionObserver | undefined;

  onMount(() => {
    if (!cardElement) return;

    // eslint-disable-next-line no-undef -- browser global
    observer = new IntersectionObserver(
      entries => {
        for (const entry of entries) {
          isVisible = entry.isIntersecting;
        }
      },
      { rootMargin: '200px 0px' }
    );
    observer.observe(cardElement);
  });

  onDestroy(() => {
    observer?.disconnect();
    loader.destroy();
  });
</script>

<article
  bind:this={cardElement}
  class={cn('detection-card group relative rounded-xl', isMenuOpen && 'z-[60]')}
>
  <!-- Inner container with overflow-hidden for spectrogram clipping -->
  <div class="detection-card-inner">
    <!-- Spectrogram Background -->
    <div class="spectrogram-container">
      {#if loader.showSpinner}
        <div class="spectrogram-loading">
          <span class="loading loading-spinner loading-md text-[var(--color-base-content)]/50"
          ></span>
          {#if loader.isQueued}
            <span class="text-xs text-[var(--color-base-content)]/40 mt-1"
              >{t('components.audio.waiting')}</span
            >
          {:else if loader.isGenerating}
            <span class="text-xs text-[var(--color-base-content)]/40 mt-1"
              >{t('components.audio.generating')}</span
            >
          {/if}
        </div>
      {/if}

      {#if loader.error}
        <div class="spectrogram-error">
          <span class="text-sm text-[var(--color-base-content)]/50"
            >{t('components.audio.spectrogramUnavailable')}</span
          >
        </div>
      {:else if loader.spectrogramUrl}
        <img
          src={loader.spectrogramUrl}
          alt={t('components.audio.spectrogramForSpecies', { species: detection.commonName })}
          class="spectrogram-image"
          class:opacity-0={loader.state === 'loading'}
          decoding="async"
          onload={() => loader.handleImageLoad()}
          onerror={() => loader.handleImageError()}
        />
      {/if}
    </div>

    <!-- Top-Left Badges: Confidence + Weather -->
    <div class="absolute top-3 left-3 flex items-center gap-2 z-10">
      <ConfidenceBadge confidence={detection.confidence} />
      {#if detection.weather?.weatherIcon}
        <WeatherBadge
          weatherIcon={detection.weather.weatherIcon}
          description={detection.weather.description}
          temperature={detection.weather.temperature}
          units={detection.weather.units}
          timeOfDay={detection.timeOfDay}
        />
      {/if}
      {#if detection.weather?.moonPhaseName && detection.timeOfDay === 'night'}
        <MoonBadge moonPhaseName={detection.weather.moonPhaseName} />
      {/if}
      <SourceBadge {detection} variant="overlay" />
      {#if detection.flagged}
        {@const flagLabel = detection.keywordsHit?.length
          ? t('detections.flaggedByKeywords', { keywords: detection.keywordsHit.join(', ') })
          : t('detections.detail.transcript.flaggedNotice')}
        <span
          role="img"
          aria-label={flagLabel}
          title={flagLabel}
          class="inline-flex items-center justify-center w-6 h-6 rounded bg-black/50 text-[var(--color-warning)]"
        >
          <Flag class="w-3.5 h-3.5" aria-hidden="true" />
        </span>
      {/if}
      <SpeakerAttributeChips {detection} variant="overlay" />
    </div>

    <!-- Center Play Button -->
    <PlayOverlay
      detectionId={detection.id}
      gainValue={audioGainValue}
      filterFreq={audioFilterFreq}
      playbackSpeed={audioPlaybackSpeed}
    />

    <!-- Bottom Voice Info Bar: tappable for all auth levels to view details -->
    <button
      type="button"
      class="absolute inset-x-0 bottom-0 z-[11] text-left"
      onclick={handleViewDetails}
      aria-label={t('detections.row.viewDetails', { species: detection.commonName })}
    >
      <div class="voice-info-bar">
        <!-- Decorative mic icon: replaces bird thumbnail -->
        <div class="voice-mic-icon" aria-hidden="true">
          <Mic class="w-4 h-4 text-white" aria-hidden="true" />
        </div>
        <!-- Transcript (single-line, ellipsis) with full text on hover -->
        <div class="voice-transcript-area">
          {#if detection.transcript}
            <span class="voice-transcript-text" title={detection.transcript}
              >{#each highlightKeywords(detection.transcript, detection.keywordsHit) as seg, i (i)}{#if seg.match}<mark
                    class="rounded-sm bg-[var(--color-warning)]/20 text-[var(--color-base-content)] border-b border-[var(--color-warning)]/60 font-medium"
                    >{seg.text}</mark
                  >{:else}{seg.text}{/if}{/each}</span
            >
          {:else}
            <span class="voice-no-transcript">{t('detections.noTranscript')}</span>
          {/if}
        </div>
        <!-- Detection time -->
        <div class="voice-time">
          <span class="voice-time-text">{detection.time}</span>
        </div>
      </div>
    </button>
  </div>

  <!-- Top-Right Action Menu - OUTSIDE overflow-hidden container -->
  <div class="absolute top-2 right-2 z-50">
    <ActionMenu
      {detection}
      variant="overlay"
      {onMarkCorrect}
      {onMarkFalsePositive}
      {onReview}
      {onToggleLock}
      {onDelete}
      onDownload={() => downloadDetectionAudio(detection)}
      onMenuOpen={handleMenuOpen}
      onMenuClose={handleMenuClose}
    />
  </div>
</article>

<style>
  .detection-card {
    background-color: var(--color-base-100);
  }

  .detection-card-inner {
    position: relative;
    height: 15rem;
    border-radius: 0.75rem;
    overflow: hidden;
  }

  .spectrogram-container {
    position: absolute;
    inset: 0;
    overflow: hidden;
  }

  .spectrogram-image {
    position: absolute;
    left: 0;
    bottom: 0;
    width: 100%;
    min-height: 100%;
    object-fit: cover;
    object-position: center bottom;
    image-rendering: pixelated;
    transition: opacity 0.3s ease;
  }

  .spectrogram-loading,
  .spectrogram-error {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    background: linear-gradient(
      135deg,
      color-mix(in srgb, var(--color-base-200) 80%, transparent) 0%,
      color-mix(in srgb, var(--color-base-300) 60%, transparent) 100%
    );
  }

  :global([data-theme='dark']) .spectrogram-loading,
  :global([data-theme='dark']) .spectrogram-error {
    background: linear-gradient(135deg, rgb(30 41 59 / 0.9) 0%, rgb(15 23 42 / 0.95) 100%);
  }

  /* Voice info bar: replaces SpeciesInfoBar at the bottom of the card */
  .voice-info-bar {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.75rem 1rem;
    z-index: 10;
    background: linear-gradient(to top, rgb(0 0 0 / 0.65), transparent);
  }

  .voice-mic-icon {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border-radius: 0.5rem;
    background-color: rgb(255 255 255 / 0.12);
  }

  .voice-transcript-area {
    flex: 1;
    min-width: 0;
  }

  .voice-transcript-text {
    display: block;
    font-weight: 600;
    font-size: 0.9375rem;
    color: white;
    line-height: 1.3;
    text-shadow: 0 1px 2px rgb(0 0 0 / 0.5);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .voice-no-transcript {
    font-size: 0.875rem;
    color: rgb(255 255 255 / 0.45);
    font-style: italic;
    text-shadow: 0 1px 2px rgb(0 0 0 / 0.5);
  }

  .voice-time {
    flex-shrink: 0;
  }

  .voice-time-text {
    font-size: 0.875rem;
    font-weight: 500;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    color: white;
    text-shadow: 0 1px 2px rgb(0 0 0 / 0.5);
  }
</style>
