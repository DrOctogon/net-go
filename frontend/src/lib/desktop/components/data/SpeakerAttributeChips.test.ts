import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import SpeakerAttributeChips from './SpeakerAttributeChips.svelte';
import type { Detection } from '$lib/types/detection.types';

// The global i18n mock (src/test/setup.ts) returns the key for unknown keys, so
// chip labels render as their i18n key. We assert on those keys plus the
// confidence formatting the component adds.

function makeDetection(overrides: Partial<Detection> = {}): Detection {
  return { id: 1, ...overrides } as Detection;
}

describe('SpeakerAttributeChips', () => {
  it('renders gender and age chips with formatted confidence when present', () => {
    render(SpeakerAttributeChips, {
      props: {
        detection: makeDetection({
          gender: 'female',
          genderConfidence: 0.87,
          ageBand: 'adult',
          ageConfidence: 0.62,
        }),
      },
    });

    expect(screen.getByText('detections.speaker.gender.female · 87%')).toBeInTheDocument();
    expect(screen.getByText('detections.speaker.age.adult · 62%')).toBeInTheDocument();
  });

  it('shows only the label (no percentage) when confidence is absent', () => {
    render(SpeakerAttributeChips, {
      props: { detection: makeDetection({ gender: 'male' }) },
    });

    const chip = screen.getByText('detections.speaker.gender.male');
    expect(chip).toBeInTheDocument();
    expect(chip.textContent).not.toContain('%');
  });

  it('renders nothing when the detection has no speaker estimates', () => {
    const { container } = render(SpeakerAttributeChips, {
      props: { detection: makeDetection({}) },
    });
    expect(container.querySelector('.sp-attr-chip')).toBeNull();
  });

  it('hides a chip whose value is empty or unrecognized', () => {
    render(SpeakerAttributeChips, {
      props: {
        detection: makeDetection({ gender: '', ageBand: 'teen', ageConfidence: 0.5 }),
      },
    });

    expect(screen.queryByText(/detections\.speaker\.gender/)).toBeNull();
    expect(screen.getByText('detections.speaker.age.teen · 50%')).toBeInTheDocument();
  });

  it('sets an accessible label combining the group label and value', () => {
    render(SpeakerAttributeChips, {
      props: { detection: makeDetection({ gender: 'female', genderConfidence: 0.87 }) },
    });

    expect(
      screen.getByLabelText('detections.speaker.genderAria: detections.speaker.gender.female · 87%')
    ).toBeInTheDocument();
  });
});
