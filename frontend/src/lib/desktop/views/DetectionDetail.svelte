<!--
  DetectionDetail.svelte - Single Detection View Component

  Purpose: Display comprehensive details for a single human voice detection

  Features:
  - Hero section with mic icon, detection title, confidence indicator
  - Metadata card (date/time, source, weather, download)
  - Audio player with large spectrogram visualization
  - Tabbed content: overview, history, notes, review
  - Transcript with keyword highlighting
  - Weather and environmental context

  Props:
  - detectionId: string - The ID of the detection to display
-->
<script lang="ts">
  import ConfidenceCircle from '$lib/desktop/components/data/ConfidenceCircle.svelte';
  import WeatherDetails from '$lib/desktop/components/data/WeatherDetails.svelte';
  import AudioPlayer from '$lib/desktop/components/media/AudioPlayer.svelte';
  import VerificationBadges from '$lib/desktop/components/ui/VerificationBadges.svelte';
  import ErrorAlert from '$lib/desktop/components/ui/ErrorAlert.svelte';
  import { t } from '$lib/i18n';
  import type { Detection } from '$lib/types/detection.types';
  import { hasReviewPermission, isAuthenticated } from '$lib/utils/auth';
  import { formatLocalDateTime } from '$lib/utils/date';
  import { buildAppUrl, getCurrentPathWithQuery } from '$lib/utils/urlHelpers';
  import { loggers } from '$lib/utils/logger';
  import { highlightKeywords } from '$lib/utils/highlightKeywords';
  import SourceBadge from '$lib/desktop/features/dashboard/components/SourceBadge.svelte';
  import SpeakerAttributeChips from '$lib/desktop/components/data/SpeakerAttributeChips.svelte';
  import { getSpeakerChips } from '$lib/utils/speakerAttributes';
  import {
    Download,
    Mic,
    Clock,
    Flag,
    History,
    StickyNote,
    Sun,
    Moon,
    Sunrise,
    Sunset,
  } from '@lucide/svelte';

  // ReviewCard component type (Svelte 5 component)
  type ReviewCardComponent =
    typeof import('$lib/desktop/components/review/ReviewCard.svelte').default;

  const logger = loggers.ui;

  // Constants
  const TAB_FOCUS_DELAY_MS = 50;
  type TabType = 'overview' | 'history' | 'notes' | 'review';

  interface Props {
    detectionId?: string;
  }

  const { detectionId: detectionIdProp }: Props = $props();

  // Resolved detection ID - initialized by $effect below, not directly from prop
  // to ensure reactive updates work correctly
  let resolvedDetectionId = $state<string | undefined>(undefined);

  // State
  let activeTab = $state<TabType>('overview');

  // Dynamic review component loading
  let ReviewCard: ReviewCardComponent | null = $state(null);

  // Use the existing auth store pattern (same as DesktopSidebar)
  let canReview = $derived($hasReviewPermission);
  let clipExtractionEnabled = $derived($isAuthenticated);
  let detection = $state<Detection | null>(null);
  // Pre-compute highlighted segments so the template stays declarative.
  // Reactively recomputed whenever `detection` changes.
  let transcriptSegments = $derived(
    detection !== null && (detection.transcript ?? '') !== ''
      ? highlightKeywords(detection.transcript ?? '', detection.keywordsHit)
      : []
  );
  let isLoadingDetection = $state(true);
  let detectionError = $state<string | null>(null);

  // AbortController for preventing race conditions
  let detectionController: AbortController | null = null;

  // Validate detection ID to prevent path traversal attacks
  // Only allow alphanumeric characters, hyphens, and underscores
  function isValidDetectionId(id: string): boolean {
    return /^[a-zA-Z0-9_-]+$/.test(id);
  }

  // Resolve detection ID from URL if not provided via prop
  $effect(() => {
    if (!detectionIdProp) {
      const pathParts = window.location.pathname.split('/');
      const detectionIndex = pathParts.indexOf('detections');
      if (detectionIndex !== -1 && pathParts[detectionIndex + 1]) {
        const candidateId = pathParts[detectionIndex + 1];
        if (isValidDetectionId(candidateId)) {
          resolvedDetectionId = candidateId;
        } else {
          detectionError = t('detections.errors.noIdProvided');
        }
      }
    } else if (isValidDetectionId(detectionIdProp)) {
      resolvedDetectionId = detectionIdProp;
    } else {
      detectionError = t('detections.errors.noIdProvided');
    }
  });

  // Initialize tab from URL query parameter (with permission check for review tab)
  $effect(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const tabParam = urlParams.get('tab');
    const validTabs: TabType[] = ['overview', 'history', 'notes', 'review'];
    if (tabParam && validTabs.includes(tabParam as TabType)) {
      activeTab = tabParam === 'review' && !canReview ? 'overview' : (tabParam as TabType);
    }
  });

  // Fetch detection data when resolvedDetectionId changes
  $effect(() => {
    if (resolvedDetectionId) {
      fetchDetection();
    }

    return () => {
      detectionController?.abort();
    };
  });

  // Fetch detection data
  async function fetchDetection() {
    if (!resolvedDetectionId) {
      detectionError = t('detections.errors.noIdProvided');
      isLoadingDetection = false;
      return;
    }

    detectionController?.abort();
    const controller = new AbortController();
    detectionController = controller;
    const { signal } = controller;

    isLoadingDetection = true;
    detectionError = null;
    // Reset the detail so a newer detection never briefly shows the
    // previously viewed one while the fetch is still in flight.
    detection = null;

    try {
      const response = await fetch(buildAppUrl(`/api/v2/detections/${resolvedDetectionId}`), {
        signal,
      });

      // Check the captured signal, not the shared controller: a newer request may
      // have replaced detectionController with a fresh, non-aborted instance, and
      // checking that would let this stale response overwrite newer data.
      if (signal.aborted) return;

      if (response.ok) {
        const data = (await response.json()) as Detection;
        if (signal.aborted) return;
        detection = data;
      } else {
        let errorMessage: string;
        switch (response.status) {
          case 404:
            errorMessage = t('detections.errors.notFound');
            break;
          case 403:
            errorMessage = t('detections.errors.noPermission');
            break;
          case 401:
            errorMessage = t('detections.errors.loginRequired');
            break;
          case 500:
          case 502:
          case 503:
            errorMessage = t('detections.errors.serverError');
            break;
          default:
            errorMessage = t('detections.errors.loadFailed', { status: response.status });
        }
        throw new Error(errorMessage);
      }
    } catch (error) {
      if (signal.aborted || (error instanceof Error && error.name === 'AbortError')) {
        return;
      }
      detectionError =
        error instanceof Error ? error.message : t('detections.errors.loadFailed', { status: '' });
      logger.error('Error fetching detection:', error);
    } finally {
      // Only the request that still owns the controller may reset shared state; a
      // superseded request must not clear a newer request's loading flag/controller.
      if (detectionController === controller) {
        isLoadingDetection = false;
        detectionController = null;
      }
    }
  }

  // Dynamically load review component when user has review permission
  $effect(() => {
    if (canReview && !ReviewCard) {
      import('$lib/desktop/components/review/ReviewCard.svelte')
        .then(module => {
          ReviewCard = module.default;
          logger.debug('ReviewCard component loaded for authenticated user');
        })
        .catch(error => {
          logger.error('Failed to load ReviewCard component:', error);
        });
    }
  });

  // Handle review card completion
  function handleReviewComplete() {
    if (resolvedDetectionId) {
      fetchDetection();
    }
  }

  // Keyboard navigation handler for tab buttons
  function handleTabKeydown(e: KeyboardEvent) {
    const tabs: TabType[] = ['overview', 'history', 'notes'];
    if (canReview) tabs.push('review');

    const currentIndex = tabs.indexOf(activeTab);
    if (currentIndex === -1) return;

    if (e.key === 'ArrowRight') {
      e.preventDefault();
      activeTab = tabs[(currentIndex + 1) % tabs.length];
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      activeTab = tabs[(currentIndex - 1 + tabs.length) % tabs.length];
    }
  }

  // Focus management for tab switching
  $effect(() => {
    const activePanel = document.getElementById(`tab-panel-${activeTab}`);
    let timeoutId: ReturnType<typeof setTimeout> | null = null;

    if (activePanel && document.activeElement?.getAttribute('role') === 'tab') {
      timeoutId = setTimeout(() => {
        activePanel.focus();
      }, TAB_FOCUS_DELAY_MS);
    }

    return () => {
      if (timeoutId) clearTimeout(timeoutId);
    };
  });

  // URL state management - update URL when tab changes
  $effect(() => {
    if (typeof window !== 'undefined' && resolvedDetectionId) {
      const url = new URL(window.location.href);

      if (activeTab === 'overview') {
        url.searchParams.delete('tab');
      } else {
        url.searchParams.set('tab', activeTab);
      }

      const newUrl = url.pathname + url.search;
      if (newUrl !== getCurrentPathWithQuery()) {
        window.history.replaceState(null, '', newUrl);
      }
    }
  });

  // Handle browser back/forward navigation
  $effect(() => {
    if (typeof window !== 'undefined') {
      function handlePopState() {
        const urlParams = new URLSearchParams(window.location.search);
        const tabParam = urlParams.get('tab');
        const validTabs = ['overview', 'history', 'notes', 'review'] as const;

        if (tabParam && validTabs.includes(tabParam as typeof activeTab)) {
          if (tabParam === 'review' && !canReview) {
            activeTab = 'overview';
          } else {
            activeTab = tabParam as typeof activeTab;
          }
        } else {
          activeTab = 'overview';
        }
      }

      window.addEventListener('popstate', handlePopState);

      return () => {
        window.removeEventListener('popstate', handlePopState);
      };
    }
  });
</script>

<!-- Snippets for better organization -->

{#snippet heroSection(det: Detection)}
  <section class="detection-hero-grid" aria-labelledby="species-heading">
    <!-- Identity Card -->
    <div class="hero-card hero-identity-card">
      <div class="hero-identity-row">
        <!-- Mic icon - decorative -->
        <div class="hero-icon" aria-hidden="true">
          <Mic class="hero-mic-icon" />
        </div>

        <!-- Detection identity -->
        <div class="hero-species">
          <h1 id="species-heading" class="species-display-name">
            {t('detections.humanVoice')}
          </h1>
          <div class="mt-3" aria-label={t('detections.detail.aria.classificationBadges')}>
            <VerificationBadges detection={det} size="sm" />
          </div>
        </div>

        <!-- Confidence -->
        <div
          class="hero-confidence"
          aria-label={t('detections.detail.aria.confidence', {
            confidence: Math.round((det.confidence ?? 0) * 100),
          })}
        >
          <ConfidenceCircle confidence={det.confidence} size="xl" />
        </div>
      </div>
    </div>

    <!-- Metadata Card -->
    <div
      class="hero-card hero-metadata-card"
      role="region"
      aria-label={t('detections.detail.aria.metadata')}
    >
      <h3 class="section-heading">{t('detections.detail.observation')}</h3>
      <!-- Date & Time -->
      <div class="meta-section">
        <div class="meta-date">{det.date}</div>
        <div class="meta-time-row">
          <Clock class="w-3.5 h-3.5" />
          <span>{det.time}</span>
          {#if det.timeOfDay}
            {@const tod = det.timeOfDay.toLowerCase()}
            <span class="time-of-day-badge tod-{tod}">
              {#if tod === 'day'}
                <Sun size={12} />
              {:else if tod === 'night'}
                <Moon size={12} />
              {:else if tod === 'sunrise'}
                <Sunrise size={12} />
              {:else if tod === 'sunset'}
                <Sunset size={12} />
              {/if}
              <span>{t(`detections.timeOfDay.${tod}`)}</span>
            </span>
          {/if}
        </div>
      </div>

      <!-- Audio Source -->
      {#if det.source}
        <div class="meta-section">
          <SourceBadge detection={det} variant="inline" />
        </div>
      {/if}

      <!-- Weather -->
      {#if det.weather}
        <div
          class="meta-section hero-weather"
          aria-label={t('detections.detail.aria.weatherConditions')}
        >
          <WeatherDetails
            weatherIcon={det.weather.weatherIcon}
            weatherDescription={det.weather.description}
            timeOfDay={det.timeOfDay?.toLowerCase() === 'night' ? 'night' : 'day'}
            temperature={det.weather.temperature}
            windSpeed={det.weather.windSpeed}
            windGust={det.weather.windGust}
            units={det.weather.units}
            size="md"
          />
        </div>
      {/if}

      <!-- Speaker Attributes (estimated gender + age band; opt-in) -->
      {#if getSpeakerChips(det).length > 0}
        <div class="meta-section" aria-label={t('detections.speaker.sectionLabel')}>
          <div class="speaker-attr-label">{t('detections.speaker.sectionLabel')}</div>
          <SpeakerAttributeChips detection={det} variant="default" />
        </div>
      {/if}

      <!-- Download -->
      {#if det.clipName}
        <div class="meta-section">
          <a
            href={buildAppUrl(`/api/v2/media/audio/${det.clipName}`)}
            download
            class="meta-download"
            aria-label={t('detections.detail.aria.downloadAudioClip', {
              name: t('detections.humanVoice'),
            })}
          >
            <Download class="w-4 h-4" />
            <span>{t('media.audio.download')}</span>
          </a>
        </div>
      {/if}
    </div>
  </section>
{/snippet}

{#snippet overviewTab(det: Detection)}
  <!-- Transcript Section: only rendered when the backend returns transcript text -->
  {#if det.transcript}
    <section aria-labelledby="transcript-heading" class="mb-8">
      <h3 id="transcript-heading" class="section-heading">
        {t('detections.detail.transcript.title')}
      </h3>
      <div class="content-panel">
        {#if det.flagged}
          <div
            class="flex items-center gap-2 mb-3 pb-3 border-b border-[var(--border-100)]"
            role="status"
          >
            <Flag class="w-4 h-4 text-[var(--color-warning)] shrink-0" aria-hidden="true" />
            <span class="text-sm font-medium text-[var(--color-base-content)]/70">
              {t('detections.detail.transcript.flaggedNotice')}
            </span>
          </div>
        {/if}

        <p class="text-sm leading-relaxed whitespace-pre-wrap">
          {#each transcriptSegments as seg, i (i)}{#if seg.match}<mark
                class="rounded-sm bg-[var(--color-warning)]/20 text-[var(--color-base-content)] border-b border-[var(--color-warning)]/60 font-medium"
                >{seg.text}</mark
              >{:else}{seg.text}{/if}{/each}
        </p>

        {#if det.keywordsHit && det.keywordsHit.length > 0}
          <div class="mt-3 pt-3 border-t border-[var(--border-100)]">
            <p
              class="text-xs font-semibold uppercase tracking-wider text-[var(--color-base-content)]/50 mb-2"
            >
              {t('detections.detail.transcript.matchedKeywords')}
            </p>
            <ul
              class="flex flex-wrap gap-1.5 list-none p-0 m-0"
              aria-label={t('detections.detail.transcript.matchedKeywords')}
            >
              {#each det.keywordsHit as keyword (keyword)}
                <li
                  class="px-2 py-0.5 rounded text-xs font-medium border border-[var(--color-warning)]/40 bg-[var(--color-warning)]/10 text-[var(--color-base-content)]/80"
                >
                  {keyword}
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </div>
    </section>
  {/if}

  <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
    <!-- Species Tracking -->
    {#if det.isNewSpecies || det.isNewThisYear || det.isNewThisSeason || (det.daysSinceFirstSeen != null && det.daysSinceFirstSeen > 0)}
      <section aria-labelledby="tracking-heading">
        <h3 id="tracking-heading" class="section-heading">{t('species.tracking.title')}</h3>
        <div class="content-panel">
          <dl class="metadata-list">
            {#if det.isNewSpecies}
              <div class="metadata-row">
                <dt>{t('species.tracking.newSpecies')}</dt>
                <dd class="text-[var(--color-success)] font-medium">{t('common.values.yes')}</dd>
              </div>
            {/if}
            {#if det.isNewThisYear}
              <div class="metadata-row">
                <dt>{t('species.tracking.newThisYear')}</dt>
                <dd class="text-[var(--color-success)] font-medium">{t('common.values.yes')}</dd>
              </div>
            {/if}
            {#if det.isNewThisSeason && det.currentSeason}
              <div class="metadata-row">
                <dt>{t('species.tracking.newThisSeason')}</dt>
                <dd class="capitalize">{det.currentSeason}</dd>
              </div>
            {/if}
            {#if det.daysSinceFirstSeen != null && det.daysSinceFirstSeen > 0}
              <div class="metadata-row">
                <dt>{t('species.tracking.daysSinceFirst')}</dt>
                <dd>{det.daysSinceFirstSeen}</dd>
              </div>
            {/if}
          </dl>
        </div>
      </section>
    {/if}
  </div>
{/snippet}

{#snippet historyTab()}
  <section class="empty-state-section" aria-labelledby="history-heading">
    <History class="empty-state-icon" />
    <h3 id="history-heading" class="empty-state-heading">{t('detections.history.title')}</h3>
    <p class="empty-state-text" role="status">
      {t('detections.history.comingSoon')}
    </p>
  </section>
{/snippet}

{#snippet notesTab(det: Detection)}
  <section aria-labelledby="notes-heading">
    <h3 id="notes-heading" class="section-heading">{t('detections.notes.title')}</h3>
    {#if det.comments && det.comments.length > 0}
      <div class="space-y-3" role="list" aria-label={t('detections.detail.aria.comments')}>
        {#each det.comments as comment (comment.id ?? comment.createdAt)}
          <article class="content-panel" role="listitem">
            <p class="text-sm leading-relaxed">
              <span class="sr-only"
                >{t('detections.detail.aria.commentText')}:
              </span>{comment.entry}
            </p>
            <p class="text-xs text-[var(--color-base-content)]/40 mt-2">
              <span class="sr-only"
                >{t('detections.detail.aria.commentTimestamp')}:
              </span>{formatLocalDateTime(new Date(comment.createdAt))}
            </p>
          </article>
        {/each}
      </div>
    {:else}
      <div class="empty-state-section">
        <StickyNote class="empty-state-icon" />
        <p class="empty-state-text" role="status">
          {t('detections.notes.noComments')}
        </p>
      </div>
    {/if}
  </section>
{/snippet}

<!-- Main component -->
<main class="col-span-12 detection-detail" aria-label={t('detections.detail.aria.mainRegion')}>
  <!-- Loading state with live region -->
  <div role="status" aria-live="polite" class="sr-only">
    {#if isLoadingDetection}
      {t('detections.aria.loading')}
    {:else if detection}
      {t('detections.aria.loaded', {
        species: t('detections.humanVoice'),
      })}
    {:else if detectionError}
      {t('detections.aria.error', { error: detectionError })}
    {/if}
  </div>

  {#if isLoadingDetection}
    <!-- Loading skeleton matching real layout structure (decorative, AT uses live region above) -->
    <div class="detection-hero-grid" aria-hidden="true">
      <!-- Identity card skeleton -->
      <div class="hero-card hero-identity-card">
        <div class="hero-identity-row">
          <div class="hero-icon skeleton-block" style:width="64px" style:height="64px"></div>
          <div class="hero-species">
            <div class="skeleton-line w-48 h-6 mb-2"></div>
            <div class="flex gap-2 mt-3">
              <div class="skeleton-line w-16 h-5 rounded-full"></div>
              <div class="skeleton-line w-16 h-5 rounded-full"></div>
            </div>
          </div>
          <div class="hero-confidence">
            <div class="skeleton-block w-20 h-20 rounded-full"></div>
          </div>
        </div>
      </div>
      <!-- Metadata card skeleton -->
      <div class="hero-card hero-metadata-card">
        <div class="skeleton-line w-24 h-3 mb-4"></div>
        <div class="space-y-3">
          <div class="skeleton-line w-28 h-5"></div>
          <div class="skeleton-line w-20 h-4"></div>
          <div class="skeleton-line w-full h-12 mt-4"></div>
        </div>
      </div>
    </div>
    <!-- Media section skeleton -->
    <div class="surface-card">
      <div class="p-5 md:p-6">
        <div class="skeleton-line w-32 h-4 mb-4"></div>
        <div class="skeleton-block w-full" style:aspect-ratio="2 / 1"></div>
      </div>
    </div>
    <!-- Tabs skeleton -->
    <div class="surface-card">
      <div class="p-5 md:p-6">
        <div class="flex gap-4 mb-6">
          <div class="skeleton-line w-20 h-8 rounded-md"></div>
          <div class="skeleton-line w-20 h-8 rounded-md"></div>
          <div class="skeleton-line w-20 h-8 rounded-md"></div>
        </div>
        <div class="space-y-3">
          <div class="skeleton-line w-full h-4"></div>
          <div class="skeleton-line w-3/4 h-4"></div>
          <div class="skeleton-line w-1/2 h-4"></div>
        </div>
      </div>
    </div>
  {:else if detectionError}
    <!-- Error state -->
    <div class="surface-card p-6">
      <div role="alert" aria-live="assertive">
        <ErrorAlert message={detectionError} />
      </div>
    </div>
  {:else if detection}
    <!-- Hero Section -->
    {@render heroSection(detection)}

    <!-- Media Section -->
    <section class="surface-card" aria-labelledby="media-heading">
      <div class="p-5 md:p-6">
        <h2 id="media-heading" class="section-heading !mb-0">
          {t('detections.media.title')}
        </h2>
        {#if clipExtractionEnabled}
          <p class="text-sm text-[var(--color-base-content)]/60 mt-0.5 mb-4">
            {t('detections.media.clipHint')}
          </p>
        {:else}
          <div class="mb-3"></div>
        {/if}
        <div
          role="region"
          aria-label={t('detections.detail.aria.audioRecordingFor', {
            name: t('detections.humanVoice'),
          })}
        >
          <div class="detail-audio-container">
            <AudioPlayer
              audioUrl={buildAppUrl(`/api/v2/audio/${detection.id}`)}
              detectionId={detection.id.toString()}
              showSpectrogram={true}
              spectrogramSize="lg"
              spectrogramRaw={false}
              responsive={true}
              className="w-full"
              enableClipExtraction={clipExtractionEnabled}
              clipLabel={`${detection.commonName}_${detection.date}_${detection.time.replace(/:/g, '-')}`}
            />
          </div>
        </div>
      </div>
    </section>

    <!-- Tabbed Content -->
    <section class="surface-card" aria-labelledby="tabs-heading">
      <div class="p-5 md:p-6">
        <h2 id="tabs-heading" class="sr-only">{t('detections.detail.aria.tabsHeading')}</h2>

        <!-- Tab Navigation -->
        <div class="tab-nav" role="tablist" aria-label={t('detections.detail.aria.tabList')}>
          {#each ['overview', 'history', 'notes'] as tab (tab)}
            <button
              id="tab-{tab}"
              role="tab"
              class="tab-button"
              class:tab-active={activeTab === tab}
              aria-selected={activeTab === tab}
              aria-controls="tab-panel-{tab}"
              tabindex={activeTab === tab ? 0 : -1}
              onclick={() => (activeTab = tab as TabType)}
              onkeydown={handleTabKeydown}
            >
              {t(`detections.tabs.${tab}`)}
            </button>
          {/each}
          {#if canReview}
            <button
              id="tab-review"
              role="tab"
              class="tab-button"
              class:tab-active={activeTab === 'review'}
              aria-selected={activeTab === 'review'}
              aria-controls="tab-panel-review"
              tabindex={activeTab === 'review' ? 0 : -1}
              onclick={() => (activeTab = 'review')}
              onkeydown={handleTabKeydown}
            >
              {t('common.actions.review')}
            </button>
          {/if}
        </div>

        <!-- Tab Content -->
        <div class="mt-6">
          {#if activeTab === 'overview'}
            <div
              role="tabpanel"
              id="tab-panel-overview"
              aria-labelledby="tab-overview"
              aria-hidden="false"
              tabindex="0"
            >
              {@render overviewTab(detection)}
            </div>
          {:else if activeTab === 'history'}
            <div
              role="tabpanel"
              id="tab-panel-history"
              aria-labelledby="tab-history"
              aria-hidden="false"
              tabindex="0"
            >
              {@render historyTab()}
            </div>
          {:else if activeTab === 'notes'}
            <div
              role="tabpanel"
              id="tab-panel-notes"
              aria-labelledby="tab-notes"
              aria-hidden="false"
              tabindex="0"
            >
              {@render notesTab(detection)}
            </div>
          {:else if activeTab === 'review' && canReview && ReviewCard}
            <div
              role="tabpanel"
              id="tab-panel-review"
              aria-labelledby="tab-review"
              aria-hidden="false"
              tabindex="0"
            >
              <ReviewCard {detection} onSaveComplete={handleReviewComplete} />
            </div>
          {/if}
        </div>
      </div>
    </section>
  {/if}
</main>

<style>
  /* ===========================================
     DETECTION DETAIL - Editorial Design System
     =========================================== */

  .detection-detail {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
    width: 100%;
  }

  /* ----- Surface card (replaces DaisyUI card) ----- */
  .surface-card {
    background-color: var(--color-base-100);
    border-radius: var(--radius-box);
    border: 1px solid var(--border-100);
  }

  /* ----- Hero Section: Two-card grid ----- */
  .detection-hero-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 1rem;
  }

  /* Two columns: identity + metadata */
  @media (min-width: 1024px) {
    .detection-hero-grid {
      grid-template-columns: 1fr minmax(220px, 280px);
    }

    .hero-identity-card {
      grid-column: 1;
      grid-row: 1;
    }

    .hero-metadata-card {
      grid-column: 2;
      grid-row: 1;
    }
  }

  .hero-card {
    background-color: var(--color-base-100);
    border-radius: var(--radius-box);
    border: 1px solid var(--border-100);
    padding: 1.5rem;
  }

  @media (min-width: 768px) {
    .hero-card {
      padding: 2rem;
    }
  }

  /* Identity card row: thumbnail + species + confidence */
  .hero-identity-row {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  @media (min-width: 768px) {
    .hero-identity-row {
      flex-direction: row;
      align-items: flex-start;
      gap: 1.5rem;
    }
  }

  .hero-confidence {
    flex-shrink: 0;
  }

  @media (min-width: 768px) {
    .hero-confidence {
      margin-left: auto;
      padding-left: 1.5rem;
      border-left: 1px solid var(--border-100);
    }
  }

  /* Metadata card - vertical stacked sections */
  .hero-metadata-card {
    display: flex;
    flex-direction: column;
    justify-content: center;
  }

  .meta-section {
    padding: 0.875rem 0;
  }

  .meta-section:first-child {
    padding-top: 0;
  }

  .meta-section:last-child {
    padding-bottom: 0;
  }

  .meta-section + .meta-section {
    border-top: 1px solid var(--border-100);
  }

  /* Date - prominent visual anchor for the time section */
  .meta-date {
    font-size: 1rem;
    font-weight: 600;
    color: var(--color-base-content);
    letter-spacing: -0.01em;
  }

  /* Time + time-of-day badge - inline, secondary */
  .meta-time-row {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    margin-top: 0.25rem;
    font-size: 0.9375rem;
    font-weight: 500;
    color: var(--color-base-content);
    opacity: 0.7;
  }

  .meta-time-row .time-of-day-badge {
    margin-top: 0;
    margin-left: 0.125rem;
  }

  /* Label above the speaker-attribute chips in the metadata card */
  .speaker-attr-label {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-base-content);
    opacity: 0.5;
    margin-bottom: 0.375rem;
  }

  .meta-download {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.8125rem;
    font-weight: 500;
    color: var(--color-base-content);
    opacity: 0.6;
    transition: opacity 0.15s ease;
  }

  .meta-download:hover {
    opacity: 1;
  }

  /* Mic icon container - decorative hero placeholder */
  .hero-icon {
    width: 64px;
    height: 64px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 0.75rem;
    background: linear-gradient(135deg, var(--color-base-200), var(--color-base-300));
    flex-shrink: 0;
  }

  .hero-mic-icon {
    width: 2rem;
    height: 2rem;
    color: var(--color-base-content);
    opacity: 0.35;
  }

  /* Species identity - takes available space */
  .hero-species {
    flex: 1;
    min-width: 0;
    padding-top: 0.125rem;
  }

  /* Species display name - refined Inter typography */
  .species-display-name {
    font-family: 'Inter Variable', Inter, ui-sans-serif, sans-serif;
    font-size: 1.5rem;
    font-weight: 650;
    line-height: 1.2;
    letter-spacing: -0.025em;
    color: var(--color-base-content);
    margin: 0;
  }

  @media (min-width: 768px) {
    .species-display-name {
      font-size: 1.875rem;
    }
  }

  /* Time of day badge - theme-safe via alpha transparency */
  .time-of-day-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.6875rem;
    font-weight: 550;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.2rem 0.5rem;
    border-radius: 9999px;
    margin-top: 0.5rem;
    line-height: 1;
    color: var(--color-base-content);
  }

  .tod-day {
    background: oklch(80% 0.1 85deg / 0.4);
  }

  .tod-night {
    background: oklch(50% 0.08 270deg / 0.25);
  }

  .tod-sunrise {
    background: oklch(80% 0.12 55deg / 0.4);
  }

  .tod-sunset {
    background: oklch(78% 0.12 30deg / 0.4);
  }

  /* Weather in metadata card - consistent typographic scale */
  .hero-weather :global(.wd-container) {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.25rem;
  }

  .hero-weather :global(.wd-weather-row),
  .hero-weather :global(.wd-temperature-row),
  .hero-weather :global(.wd-wind-row) {
    gap: 0.375rem;
    font-size: 0.8125rem;
  }

  .hero-weather :global(.wd-weather-icon) {
    font-size: 1.125rem;
  }

  .hero-weather :global(.wd-wind-row) {
    opacity: 0.55;
  }

  /* ----- Section Heading ----- */
  .section-heading {
    font-size: 0.8125rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--color-base-content);
    opacity: 0.65;
    margin-bottom: 0.75rem;
  }

  /* ----- Content Panel (inner blocks) ----- */
  .content-panel {
    background-color: var(--color-base-200);
    border-radius: var(--radius-field);
    border: 1px solid var(--border-100);
    padding: 1rem;
  }

  /* ----- Metadata list (key-value pairs) ----- */
  .metadata-list {
    display: flex;
    flex-direction: column;
    gap: 0.625rem;
  }

  .metadata-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    font-size: 0.875rem;
  }

  .metadata-row dt {
    color: var(--color-base-content);
    opacity: 0.5;
  }

  .metadata-row dd {
    font-weight: 500;
    color: var(--color-base-content);
  }

  /* ----- Tab Navigation ----- */
  .tab-nav {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--border-100);
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: none;
  }

  .tab-nav::-webkit-scrollbar {
    display: none;
  }

  .tab-button {
    position: relative;
    padding: 0.625rem 1rem;
    font-size: 0.8125rem;
    font-weight: 500;
    color: var(--color-base-content);
    opacity: 0.55;
    background: none;
    border: none;
    cursor: pointer;
    white-space: nowrap;
    transition:
      opacity 0.15s ease,
      color 0.15s ease;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
  }

  .tab-button:hover {
    opacity: 0.85;
  }

  .tab-button:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: -2px;
    border-radius: var(--radius-selector);
  }

  .tab-button.tab-active {
    opacity: 1;
    color: var(--color-primary);
    border-bottom-color: var(--color-primary);
    font-weight: 600;
  }

  /* ----- Audio Container ----- */
  .detail-audio-container {
    width: 100%;
  }

  .detail-audio-container :global(.group) {
    position: relative;
  }

  .detail-audio-container :global(img) {
    width: 100%;
    height: auto;
    display: block;
  }

  /* ----- Empty States ----- */
  .empty-state-section {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 3rem 1.5rem;
    text-align: center;
  }

  .empty-state-icon {
    width: 2.5rem;
    height: 2.5rem;
    color: var(--color-base-content);
    opacity: 0.2;
    margin-bottom: 1rem;
  }

  .empty-state-heading {
    font-size: 0.9375rem;
    font-weight: 600;
    color: var(--color-base-content);
    opacity: 0.55;
    margin-bottom: 0.5rem;
  }

  .empty-state-text {
    font-size: 0.875rem;
    color: var(--color-base-content);
    opacity: 0.4;
    font-style: italic;
  }

  /* ----- Skeleton loading primitives ----- */
  .skeleton-line,
  .skeleton-block {
    background: var(--color-base-300);
    border-radius: var(--radius-selector);
    animation: skeleton-pulse 1.5s ease-in-out infinite;
  }

  .skeleton-block {
    border-radius: var(--radius-box);
  }

  @keyframes skeleton-pulse {
    0%,
    100% {
      opacity: 1;
    }

    50% {
      opacity: 0.5;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .skeleton-line,
    .skeleton-block {
      animation: none;
    }
  }
</style>
