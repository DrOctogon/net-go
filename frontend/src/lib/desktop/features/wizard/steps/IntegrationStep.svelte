<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { t } from '$lib/i18n';
  import { settingsActions, settingsStore, type RealtimeSettings } from '$lib/stores/settings';
  import { get } from 'svelte/store';
  import { getLogger } from '$lib/utils/logger';
  import { ShieldCheck, HeartHandshake } from '@lucide/svelte';
  import type { WizardStepProps } from '../types';

  const logger = getLogger('IntegrationStep');

  let { onValidChange }: WizardStepProps = $props();

  let privacyEnabled = $state(true);
  let sentryEnabled = $state(false);
  let dirty = $state(false);

  let isValid = $derived(true);

  $effect(() => {
    const valid = isValid;
    untrack(() => onValidChange?.(valid));
  });

  onMount(() => {
    // Load current settings
    const store = get(settingsStore);
    const realtime = store?.formData?.realtime;
    if (realtime?.privacyFilter) {
      privacyEnabled = realtime.privacyFilter.enabled ?? true;
    }
    const sentry = store?.formData?.sentry;
    if (sentry) {
      sentryEnabled = sentry.enabled ?? false;
    }
  });

  function togglePrivacy() {
    privacyEnabled = !privacyEnabled;
    dirty = true;
  }

  function toggleSentry() {
    sentryEnabled = !sentryEnabled;
    dirty = true;
  }

  // Save on unmount — only if user made changes
  $effect(() => {
    return () => {
      if (!dirty) return;
      settingsActions.updateSection('realtime', {
        privacyFilter: { enabled: privacyEnabled } as RealtimeSettings['privacyFilter'],
      });
      settingsActions.updateSection('sentry', {
        enabled: sentryEnabled,
      });
      settingsActions.saveSettings().catch(err => {
        logger.error('Failed to save integration settings', err);
      });
    };
  });
</script>

<div class="space-y-3">
  <button
    type="button"
    class="flex w-full cursor-pointer items-start gap-3 rounded-lg border-2 p-4 text-left transition-colors {privacyEnabled
      ? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5'
      : 'border-[var(--border-200)] hover:border-[var(--border-300)]'}"
    onclick={togglePrivacy}
    aria-pressed={privacyEnabled}
  >
    <ShieldCheck class="mt-0.5 size-5 shrink-0 text-[var(--color-base-content)]" />
    <div class="flex-1">
      <span class="text-sm font-medium text-[var(--color-base-content)]">
        {t('wizard.steps.integration.privacyFilterLabel')}
      </span>
      <p class="mt-0.5 text-sm text-[var(--color-base-content)] opacity-80">
        {t('wizard.steps.integration.privacyFilterHelp')}
      </p>
    </div>
    <span
      class="mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors {privacyEnabled
        ? 'bg-[var(--color-primary)]'
        : 'bg-[var(--color-base-300)]'}"
      aria-hidden="true"
    >
      <span
        class="inline-block size-3.5 rounded-full bg-white shadow transition-transform {privacyEnabled
          ? 'translate-x-5'
          : 'translate-x-0.5'}"
      ></span>
    </span>
  </button>

  <button
    type="button"
    class="flex w-full cursor-pointer items-start gap-3 rounded-lg border-2 p-4 text-left transition-colors {sentryEnabled
      ? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5'
      : 'border-[var(--border-200)] hover:border-[var(--border-300)]'}"
    onclick={toggleSentry}
    aria-pressed={sentryEnabled}
  >
    <HeartHandshake class="mt-0.5 size-5 shrink-0 text-[var(--color-base-content)]" />
    <div class="flex-1">
      <span class="text-sm font-medium text-[var(--color-base-content)]">
        {t('wizard.steps.integration.errorReportingLabel')}
      </span>
      <p class="mt-0.5 text-sm text-[var(--color-base-content)] opacity-80">
        {t('wizard.steps.integration.errorReportingHelp')}
      </p>
    </div>
    <span
      class="mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors {sentryEnabled
        ? 'bg-[var(--color-primary)]'
        : 'bg-[var(--color-base-300)]'}"
      aria-hidden="true"
    >
      <span
        class="inline-block size-3.5 rounded-full bg-white shadow transition-transform {sentryEnabled
          ? 'translate-x-5'
          : 'translate-x-0.5'}"
      ></span>
    </span>
  </button>
</div>
