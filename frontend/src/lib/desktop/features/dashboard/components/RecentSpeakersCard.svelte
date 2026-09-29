<!--
RecentSpeakersCard.svelte — Dashboard widget: top household speakers by detection count.

Purpose:
- Fetches GET /api/v2/speakers once on mount (mirroring VoiceActivityCard's
  single-fetch-on-mount cadence) and renders the top 5 roster entries by
  detection count. Unnamed speakers fall back to their cluster id (spk_N).
- The speakers endpoint is auth-gated: guests skip the fetch entirely and see
  a sign-in hint instead of a 401 error.
- Clicking a row navigates to the speaker settings page where names are managed.
- Mirrors the dashboard card pattern of VoiceActivityCard (section container,
  header with title+subtitle, loading / error / empty states).

Props: isGuest — whether the viewer is unauthenticated (dashboard already derives this)
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { Users, ChevronRight, LogIn } from '@lucide/svelte';
  import { t } from '$lib/i18n';
  import { api } from '$lib/utils/api';
  import { getLogger } from '$lib/utils/logger';
  import { navigation } from '$lib/stores/navigation.svelte';

  const logger = getLogger('dashboard');

  // Shape of each entry returned by GET /api/v2/speakers
  interface SpeakerRosterEntry {
    speakerId: string;
    name: string;
    detections: number;
  }

  interface Props {
    isGuest?: boolean;
  }

  let { isGuest = false }: Props = $props();

  // Number of roster entries shown on the card
  const TOP_SPEAKERS_LIMIT = 5;
  // Destination for row clicks: the speaker settings page (name management)
  const SPEAKERS_SETTINGS_PATH = '/ui/settings/speakers';

  let speakers = $state<SpeakerRosterEntry[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Top entries by detection count, descending
  let topSpeakers = $derived(
    [...speakers].sort((a, b) => b.detections - a.detections).slice(0, TOP_SPEAKERS_LIMIT)
  );

  // Unnamed speakers fall back to their cluster id (e.g. "spk_3")
  function displayName(speaker: SpeakerRosterEntry): string {
    return speaker.name || speaker.speakerId;
  }

  function openSpeakerSettings(): void {
    navigation.navigate(SPEAKERS_SETTINGS_PATH);
  }

  async function fetchRoster(): Promise<void> {
    loading = true;
    error = null;
    try {
      const result = await api.get<SpeakerRosterEntry[]>('/api/v2/speakers');
      speakers = Array.isArray(result) ? result : [];
    } catch (err) {
      error = err instanceof Error ? err.message : t('dashboard.recentSpeakers.errors.load');
      logger.error('RecentSpeakersCard: failed to fetch speaker roster', err);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    // The speakers API requires authentication; guests get a sign-in hint
    // instead of a doomed request.
    if (isGuest) {
      loading = false;
      return;
    }
    void fetchRoster();
  });
</script>

<section
  class="card col-span-12 flex h-full flex-col rounded-2xl border border-[var(--color-base-200)] bg-[var(--color-base-100)] shadow-sm"
>
  <!-- Card Header -->
  <div class="flex items-center gap-3 border-b border-[var(--color-base-200)] px-6 py-4">
    <Users class="size-5 shrink-0 text-[var(--color-primary)]" aria-hidden="true" />
    <div class="flex flex-col">
      <h3 class="font-semibold">{t('dashboard.recentSpeakers.title')}</h3>
      <p class="text-sm text-[var(--color-base-content)]/60">
        {t('dashboard.recentSpeakers.subtitle')}
      </p>
    </div>
  </div>

  <!-- Card Content -->
  <div class="flex flex-1 flex-col px-4 pb-4 pt-3">
    {#if isGuest}
      <!-- Guest state: explain that signing in unlocks the roster -->
      <div class="flex flex-1 flex-col items-center justify-center gap-2 py-6">
        <LogIn class="size-5 text-[var(--color-base-content)]/40" aria-hidden="true" />
        <p class="text-center text-sm text-[var(--color-base-content)]/60">
          {t('dashboard.recentSpeakers.signInHint')}
        </p>
      </div>
    {:else if loading}
      <!-- Loading state: spinner + labeled text -->
      <div
        class="flex flex-1 items-center justify-center gap-2 py-6"
        role="status"
        aria-live="polite"
        aria-label={t('dashboard.recentSpeakers.loading')}
      >
        <div
          class="h-5 w-5 animate-spin rounded-full border-2 border-[var(--color-primary)] border-t-transparent"
          aria-hidden="true"
        ></div>
        <span class="text-sm text-[var(--color-base-content)]/60">
          {t('dashboard.recentSpeakers.loading')}
        </span>
      </div>
    {:else if error}
      <!-- Error state: alert with descriptive message -->
      <div class="flex flex-1 items-center justify-center py-6" role="alert" aria-live="assertive">
        <p
          class="rounded-lg bg-[var(--color-error)]/10 px-4 py-3 text-sm text-[var(--color-error)]"
        >
          {error}
        </p>
      </div>
    {:else if topSpeakers.length === 0}
      <!-- Empty state: explains how the roster gets populated -->
      <div class="flex flex-1 items-center justify-center py-6">
        <p class="text-center text-sm text-[var(--color-base-content)]/40">
          {t('dashboard.recentSpeakers.empty')}
        </p>
      </div>
    {:else}
      <!-- Roster: top speakers by detections; rows navigate to speaker settings -->
      <ul class="flex flex-col">
        {#each topSpeakers as speaker (speaker.speakerId)}
          <li>
            <button
              type="button"
              onclick={openSpeakerSettings}
              aria-label={t('dashboard.recentSpeakers.rowAriaLabel', {
                name: displayName(speaker),
              })}
              class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left transition-colors hover:bg-[var(--color-base-200)]"
            >
              <span class="min-w-0 flex-1 truncate text-sm font-medium">
                {displayName(speaker)}
              </span>
              <span class="shrink-0 text-sm text-[var(--color-base-content)]/60">
                {t('dashboard.recentSpeakers.detectionsCount', { count: speaker.detections })}
              </span>
              <ChevronRight
                class="size-4 shrink-0 text-[var(--color-base-content)]/40"
                aria-hidden="true"
              />
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</section>
