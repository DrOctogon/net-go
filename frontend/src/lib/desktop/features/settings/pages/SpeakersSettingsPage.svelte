<!--
  Speakers Settings Page Component

  Purpose: Household speaker roster management. Lists every voice-print
  speaker (named and unnamed) with its detection count and lets the user
  assign, change, or clear display names inline.

  Data: GET /api/v2/speakers (auth-gated) returns
  [{speakerId, name, detections}]; PUT /api/v2/speakers/:id/name renames
  (empty name clears). Renames apply immediately - no save/reset bar.

  Props: None - This is a page component

  @component
-->
<script lang="ts">
  import SettingsSection from '$lib/desktop/features/settings/components/SettingsSection.svelte';
  import SettingsTabs from '$lib/desktop/features/settings/components/SettingsTabs.svelte';
  import type { TabDefinition } from '$lib/desktop/features/settings/components/SettingsTabs.svelte';
  import { Users, SquarePen } from '@lucide/svelte';
  import { t } from '$lib/i18n';
  import { fetchWithCSRF } from '$lib/utils/api';
  import { toastActions } from '$lib/stores/toast';
  import { isAuthenticated } from '$lib/utils/auth';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { loggers } from '$lib/utils/logger';

  const logger = loggers.ui;

  interface SpeakerRosterEntry {
    speakerId: string;
    name: string;
    detections: number;
  }

  let roster = $state<SpeakerRosterEntry[]>([]);
  let loading = $state(true);
  let loadError = $state(false);

  // Inline rename state: at most one row is being edited at a time.
  let renameId = $state<string | null>(null);
  let renameValue = $state('');
  let renameSaving = $state(false);

  // Most-detected speakers first; ties broken by cluster id for stable order.
  let sortedRoster = $derived(
    [...roster].sort(
      (a, b) => b.detections - a.detections || a.speakerId.localeCompare(b.speakerId)
    )
  );

  $effect(() => {
    roster = [];
    loadError = false;
    if (!$isAuthenticated) {
      loading = false;
      return;
    }
    loading = true;

    const controller = new AbortController();
    fetch(buildAppUrl('/api/v2/speakers'), { signal: controller.signal })
      .then(response => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((data: unknown) => {
        if (controller.signal.aborted) return;
        roster = Array.isArray(data) ? (data as SpeakerRosterEntry[]) : [];
        loading = false;
      })
      .catch(error => {
        if (controller.signal.aborted) return;
        loadError = true;
        loading = false;
        logger.error('Failed to load speaker roster:', error);
      });

    return () => {
      controller.abort();
    };
  });

  function openRename(entry: SpeakerRosterEntry): void {
    renameId = entry.speakerId;
    renameValue = entry.name;
  }

  async function saveSpeakerName(speakerId: string): Promise<void> {
    const name = renameValue.trim();
    renameSaving = true;
    try {
      await fetchWithCSRF(`/api/v2/speakers/${speakerId}/name`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      });
      roster = roster.map(entry => (entry.speakerId === speakerId ? { ...entry, name } : entry));
      renameId = null;
    } catch (error) {
      toastActions.error(t('detections.speaker.renameFailed'));
      logger.error('Failed to save speaker name:', error);
    } finally {
      renameSaving = false;
    }
  }

  let activeTab = $state('roster');

  let tabs = $derived<TabDefinition[]>([
    {
      id: 'roster',
      label: t('settings.speakers.title'),
      icon: Users,
      content: rosterTabContent,
      hasChanges: false,
    },
  ]);
</script>

{#snippet rosterTabContent()}
  <div class="space-y-6">
    <SettingsSection
      title={t('settings.speakers.title')}
      description={t('settings.speakers.description')}
      defaultOpen={true}
    >
      {#if !$isAuthenticated}
        <p class="py-4 text-sm opacity-70">{t('settings.speakers.loginRequired')}</p>
      {:else if loading}
        <p class="py-4 text-sm opacity-70">{t('common.ui.loadingSettings')}</p>
      {:else if loadError}
        <p class="py-4 text-sm text-[var(--color-error)]">
          {t('settings.speakers.loadFailed')}
        </p>
      {:else if sortedRoster.length === 0}
        <p class="py-4 text-sm opacity-70">{t('settings.speakers.empty')}</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="table w-full">
            <thead>
              <tr>
                <th>{t('detections.speaker.identityLabel')}</th>
                <th class="text-right">{t('navigation.detections')}</th>
                <th><span class="sr-only">{t('detections.speaker.renameAction')}</span></th>
              </tr>
            </thead>
            <tbody>
              {#each sortedRoster as entry (entry.speakerId)}
                <tr>
                  <td>
                    {#if renameId === entry.speakerId}
                      <form
                        class="flex items-center gap-2"
                        onsubmit={e => {
                          e.preventDefault();
                          void saveSpeakerName(entry.speakerId);
                        }}
                      >
                        <input
                          type="text"
                          class="input input-sm flex-1 min-w-0"
                          bind:value={renameValue}
                          maxlength="64"
                          placeholder={t('detections.speaker.renamePlaceholder')}
                          aria-label={t('detections.speaker.renamePlaceholder')}
                          disabled={renameSaving}
                        />
                        <button
                          type="submit"
                          class="btn btn-sm btn-primary"
                          disabled={renameSaving}
                        >
                          {t('common.save')}
                        </button>
                        <button
                          type="button"
                          class="btn btn-sm"
                          onclick={() => (renameId = null)}
                          disabled={renameSaving}
                        >
                          {t('common.cancel')}
                        </button>
                      </form>
                    {:else if entry.name}
                      <span class="font-medium">{entry.name}</span>
                      <span class="ml-2 text-xs opacity-60">{entry.speakerId}</span>
                    {:else}
                      <span class="font-medium">{entry.speakerId}</span>
                      <span class="ml-2 text-xs opacity-60">({t('settings.speakers.unnamed')})</span
                      >
                    {/if}
                  </td>
                  <td class="text-right tabular-nums">{entry.detections}</td>
                  <td class="text-right">
                    {#if renameId !== entry.speakerId}
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs"
                        onclick={() => openRename(entry)}
                        aria-label={t('detections.speaker.renameAction')}
                      >
                        <SquarePen class="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </SettingsSection>
  </div>
{/snippet}

<main class="settings-page-content" aria-label={t('settings.speakers.description')}>
  <SettingsTabs {tabs} bind:activeTab showActions={false} />
</main>
