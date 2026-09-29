<script lang="ts">
  import { untrack } from 'svelte';
  import { t } from '$lib/i18n';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import SettingsNote from '$lib/desktop/features/settings/components/SettingsNote.svelte';
  import type { WizardStepProps } from '../types';

  // Purely informational step — never blocks onboarding. Speaker/voice-print
  // features are opt-in and configured later in Settings, so there is nothing
  // to validate here.
  let { onValidChange }: WizardStepProps = $props();

  $effect(() => {
    untrack(() => onValidChange?.(true));
  });
</script>

<div class="space-y-5">
  <p class="text-sm leading-relaxed text-[var(--color-base-content)]">
    {t('wizard.steps.speakerVoiceprint.intro')}
  </p>

  <ul class="list-disc space-y-2 pl-5 text-sm text-[var(--color-base-content)] opacity-90">
    <li>{t('wizard.steps.speakerVoiceprint.point1')}</li>
    <li>{t('wizard.steps.speakerVoiceprint.point2')}</li>
    <li>{t('wizard.steps.speakerVoiceprint.point3')}</li>
    <li>{t('wizard.steps.speakerVoiceprint.point4')}</li>
  </ul>

  <SettingsNote>
    <p>{t('wizard.steps.speakerVoiceprint.caveat')}</p>
    <p class="mt-2">
      {t('wizard.steps.speakerVoiceprint.settingsHint')}
      <a href={buildAppUrl('/ui/settings/audio')} class="link link-primary">
        {t('wizard.steps.speakerVoiceprint.settingsLinkLabel')}
      </a>.
    </p>
    <p class="mt-2">
      {t('wizard.steps.speakerVoiceprint.rosterHint')}
      <a href={buildAppUrl('/ui/settings/speakers')} class="link link-primary">
        {t('wizard.steps.speakerVoiceprint.rosterLinkLabel')}
      </a>.
    </p>
  </SettingsNote>
</div>
