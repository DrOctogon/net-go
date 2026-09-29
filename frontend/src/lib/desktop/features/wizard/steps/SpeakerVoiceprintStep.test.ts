import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';
import SpeakerVoiceprintStep from './SpeakerVoiceprintStep.svelte';

// Mock i18n with passthrough keys — this test only cares about structure/behavior,
// not translated copy (covered by i18n:validate:full in CI).
vi.mock('$lib/i18n', () => ({
  t: vi.fn((key: string) => key),
}));

describe('SpeakerVoiceprintStep', () => {
  it('renders without throwing', () => {
    expect(() => render(SpeakerVoiceprintStep, { props: {} })).not.toThrow();
  });

  it('is non-blocking: reports valid=true immediately so onboarding is never stuck here', () => {
    const onValidChange = vi.fn();
    render(SpeakerVoiceprintStep, { props: { onValidChange } });
    expect(onValidChange).toHaveBeenCalledWith(true);
  });

  it('adds no required input — there is nothing to fill in or check', () => {
    const { container } = render(SpeakerVoiceprintStep, { props: {} });
    expect(container.querySelectorAll('input, select, textarea')).toHaveLength(0);
  });

  it('links to the audio settings Speaker Attributes section and the Speakers roster page', () => {
    const { container } = render(SpeakerVoiceprintStep, { props: {} });
    const hrefs = Array.from(container.querySelectorAll('a')).map(a => a.getAttribute('href'));
    expect(hrefs.some(href => href?.includes('/settings/audio'))).toBe(true);
    expect(hrefs.some(href => href?.includes('/settings/speakers'))).toBe(true);
  });
});
