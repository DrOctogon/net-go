<!--
  SpeakerAttributeChips.svelte

  Renders estimated speaker-attribute chips (gender + relative age band) with
  their confidence percentage, e.g. "Female · 87%". These are opt-in demographic
  *estimates*, not identity recognition.

  A chip is shown only when the corresponding value is present and recognized;
  when a detection has no estimates (the common case) nothing renders.

  Variants:
  - "default": light chip for use on standard surfaces (tables, detail panels)
  - "overlay": dark chip for use over the spectrogram background on cards

  Props:
  - detection: the detection (gender / age fields are read)
  - variant?: visual style ("default" | "overlay")
  - class?: extra classes for the wrapper
-->
<script lang="ts">
  import { t } from '$lib/i18n';
  import type { Detection } from '$lib/types/detection.types';
  import { getSpeakerChips } from '$lib/utils/speakerAttributes';

  interface Props {
    detection: Pick<Detection, 'gender' | 'genderConfidence' | 'ageBand' | 'ageConfidence'>;
    variant?: 'default' | 'overlay';
    class?: string;
  }

  let { detection, variant = 'default', class: className = '' }: Props = $props();

  // Resolve label keys and build the display/aria text inside $derived so the
  // chips re-localize when the active locale changes.
  let chips = $derived(
    getSpeakerChips(detection).map(chip => {
      const label = t(chip.labelKey);
      const text = chip.confidencePct != null ? `${label} · ${chip.confidencePct}%` : label;
      const groupLabel =
        chip.kind === 'gender'
          ? t('detections.speaker.genderAria')
          : t('detections.speaker.ageAria');
      return { kind: chip.kind, text, aria: `${groupLabel}: ${text}` };
    })
  );
</script>

{#if chips.length > 0}
  <div class={`sp-attr-chips ${className}`}>
    {#each chips as chip (chip.kind)}
      <span class="sp-attr-chip sp-attr-chip-{variant}" title={chip.aria} aria-label={chip.aria}>
        {chip.text}
      </span>
    {/each}
  </div>
{/if}

<style>
  .sp-attr-chips {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.25rem;
  }

  .sp-attr-chip {
    display: inline-flex;
    align-items: center;
    padding: 0.0625rem 0.375rem;
    border-radius: 0.375rem;
    font-size: 0.6875rem;
    font-weight: 500;
    line-height: 1.3;
    white-space: nowrap;
  }

  /* Light chip mirrors the matched-keyword chip styling (rounded, bordered). */
  .sp-attr-chip-default {
    border: 1px solid color-mix(in srgb, var(--color-info) 40%, transparent);
    background-color: color-mix(in srgb, var(--color-info) 12%, transparent);
    color: var(--color-base-content);
  }

  /* Dark chip for use over the spectrogram, matching the overlay flag badge. */
  .sp-attr-chip-overlay {
    background-color: rgb(0 0 0 / 0.5);
    color: white;
  }
</style>
