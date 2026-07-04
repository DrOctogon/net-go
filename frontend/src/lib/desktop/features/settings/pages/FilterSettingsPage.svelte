<!--
  Filter Settings Page Component

  Purpose: Configure filtering settings for VoiceWatch including the privacy filter
  and the daylight filter.

  Features:
  - Privacy filter configuration with confidence threshold
  - Daylight filter configuration with window offset
  - Real-time validation and change detection

  Props: None - This is a page component that uses global settings stores

  @component
-->
<script lang="ts">
  import Checkbox from '$lib/desktop/components/forms/Checkbox.svelte';
  import NumberField from '$lib/desktop/components/forms/NumberField.svelte';
  import SettingsSection from '$lib/desktop/features/settings/components/SettingsSection.svelte';
  import SettingsTabs from '$lib/desktop/features/settings/components/SettingsTabs.svelte';
  import type { TabDefinition } from '$lib/desktop/features/settings/components/SettingsTabs.svelte';
  import {
    settingsStore,
    settingsActions,
    privacyFilterSettings,
    daylightFilterSettings,
    realtimeSettings,
  } from '$lib/stores/settings';
  import { hasSettingsChanged } from '$lib/utils/settingsChanges';
  import { t } from '$lib/i18n';
  import { Filter } from '@lucide/svelte';

  // Daylight filter offset slider bounds (hours)
  const DAYLIGHT_OFFSET_MIN = -12;
  const DAYLIGHT_OFFSET_MAX = 12;
  const DAYLIGHT_OFFSET_STEP = 1;

  // PERFORMANCE OPTIMIZATION: Reactive settings with proper defaults
  let settings = $derived(
    (() => {
      const privacyBase = $privacyFilterSettings || {
        enabled: false,
        confidence: 0.5,
        debug: false,
      };

      const daylightBase = $daylightFilterSettings || {
        enabled: false,
        debug: false,
        offset: 0,
      };

      return {
        privacy: privacyBase,
        daylight: daylightBase,
      };
    })()
  );

  let store = $derived($settingsStore);

  // PERFORMANCE OPTIMIZATION: Reactive change detection with $derived
  let privacyFilterHasChanges = $derived(
    hasSettingsChanged(
      store.originalData.realtime?.privacyFilter,
      store.formData.realtime?.privacyFilter
    )
  );

  let daylightFilterHasChanges = $derived(
    hasSettingsChanged(
      store.originalData.realtime?.daylightFilter,
      store.formData.realtime?.daylightFilter
    )
  );

  // Tab state
  let activeTab = $state('filters');

  // Tab definitions
  let tabs = $derived<TabDefinition[]>([
    {
      id: 'filters',
      label: t('settings.filters.title'),
      icon: Filter,
      content: filtersTabContent,
      hasChanges: privacyFilterHasChanges || daylightFilterHasChanges,
    },
  ]);

  // Privacy filter update handlers
  function updatePrivacyEnabled(enabled: boolean) {
    settingsActions.updateSection('realtime', {
      ...$realtimeSettings,
      privacyFilter: { ...settings.privacy, enabled },
    });
  }

  function updatePrivacyConfidence(confidence: number) {
    settingsActions.updateSection('realtime', {
      ...$realtimeSettings,
      privacyFilter: { ...settings.privacy, confidence },
    });
  }

  // Daylight filter update handlers
  function updateDaylightEnabled(enabled: boolean) {
    settingsActions.updateSection('realtime', {
      ...$realtimeSettings,
      daylightFilter: { ...settings.daylight, enabled },
    });
  }

  function updateDaylightOffset(offset: number) {
    settingsActions.updateSection('realtime', {
      ...$realtimeSettings,
      daylightFilter: { ...settings.daylight, offset },
    });
  }
</script>

{#snippet filtersTabContent()}
  <div class="space-y-6">
    <!-- Privacy Filter Section -->
    <SettingsSection
      title={t('settings.filters.privacyFiltering.title')}
      description={t('settings.filters.privacyFiltering.description')}
      defaultOpen={true}
      hasChanges={privacyFilterHasChanges}
    >
      <div class="space-y-4">
        <!-- Enable Privacy Filtering -->
        <Checkbox
          checked={settings.privacy.enabled}
          label={t('settings.filters.privacyFiltering.enable')}
          disabled={store.isLoading || store.isSaving}
          onchange={enabled => updatePrivacyEnabled(enabled)}
        />

        <!-- Fieldset for accessible disabled state - all inputs greyed out when feature disabled -->
        <fieldset
          disabled={!settings.privacy.enabled || store.isLoading || store.isSaving}
          class="contents"
          aria-describedby="privacy-filter-status"
        >
          <span id="privacy-filter-status" class="sr-only">
            {settings.privacy.enabled
              ? t('settings.filters.privacyFiltering.enable')
              : t('settings.filters.privacyFiltering.disabled')}
          </span>
          <div class="transition-opacity duration-200" class:opacity-50={!settings.privacy.enabled}>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-x-6">
              <!-- Confidence Threshold -->
              <NumberField
                label={t('settings.filters.privacyFiltering.confidenceLabel')}
                value={settings.privacy.confidence}
                onUpdate={updatePrivacyConfidence}
                min={0}
                max={1}
                step={0.01}
                disabled={!settings.privacy.enabled || store.isLoading || store.isSaving}
                helpText={t('settings.filters.privacyFiltering.confidenceHelp')}
              />
            </div>
          </div>
        </fieldset>
      </div>
    </SettingsSection>

    <!-- Daylight Filter Section -->
    <SettingsSection
      title={t('settings.filters.daylightFilter.title')}
      description={t('settings.filters.daylightFilter.description')}
      defaultOpen={true}
      hasChanges={daylightFilterHasChanges}
    >
      <div class="space-y-4">
        <!-- Enable Daylight Filter -->
        <Checkbox
          checked={settings.daylight.enabled}
          label={t('settings.filters.daylightFilter.enable')}
          disabled={store.isLoading || store.isSaving}
          onchange={enabled => updateDaylightEnabled(enabled)}
        />

        <!-- Fieldset for accessible disabled state - all inputs greyed out when feature disabled -->
        <fieldset
          disabled={!settings.daylight.enabled || store.isLoading || store.isSaving}
          class="contents"
          aria-describedby="daylight-filter-status"
        >
          <span id="daylight-filter-status" class="sr-only">
            {settings.daylight.enabled
              ? t('settings.filters.daylightFilter.enable')
              : t('settings.filters.daylightFilter.disabled')}
          </span>
          <div
            class="space-y-4 transition-opacity duration-200"
            class:opacity-50={!settings.daylight.enabled}
          >
            <div class="grid grid-cols-1 md:grid-cols-2 gap-x-6">
              <!-- Daylight Window Offset -->
              <NumberField
                label={t('settings.filters.daylightFilter.offsetLabel')}
                value={settings.daylight.offset}
                onUpdate={updateDaylightOffset}
                min={DAYLIGHT_OFFSET_MIN}
                max={DAYLIGHT_OFFSET_MAX}
                step={DAYLIGHT_OFFSET_STEP}
                disabled={!settings.daylight.enabled || store.isLoading || store.isSaving}
                helpText={t('settings.filters.daylightFilter.offsetHelp')}
              />
            </div>
          </div>
        </fieldset>
      </div>
    </SettingsSection>
  </div>
{/snippet}

<!-- Main Content -->
<main class="settings-page-content" aria-label="Filter settings configuration">
  <SettingsTabs {tabs} bind:activeTab />
</main>
