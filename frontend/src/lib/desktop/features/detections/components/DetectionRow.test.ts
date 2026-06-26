import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import DetectionRow from './DetectionRow.svelte';
import type { Detection } from '$lib/types/detection.types';

// DetectionRow is presentational: opening the action menu and clicking an item
// must invoke the callback the parent passed (the parent owns the actual
// API/modal logic via useDetectionActions). These tests assert that wiring.

vi.mock('$lib/stores/navigation.svelte', () => ({
  navigation: {
    currentPath: '/ui/detections',
    navigate: vi.fn(),
    handlePopState: vi.fn(),
  },
}));

function createMockDetection(overrides: Partial<Detection> = {}): Detection {
  return {
    id: 123,
    date: '2024-01-15',
    time: '10:30:00',
    commonName: 'American Robin',
    scientificName: 'Turdus migratorius',
    confidence: 0.85,
    locked: false,
    sourceType: 'microphone',
    sourceName: 'default',
    clipName: 'clip_001.wav',
    spectrogramPath: '/spectrograms/clip_001.png',
    ...overrides,
  } as Detection;
}

async function openMenuAndClick(itemName: RegExp) {
  const menuButton = screen.getByRole('button', { name: /actions menu/i });
  await fireEvent.click(menuButton);
  const item = screen.getByRole('menuitem', { name: itemName });
  await fireEvent.click(item);
}

describe('DetectionRow transcript presentation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders the transcript text when detection.transcript is set', () => {
    const detection = createMockDetection({ transcript: 'package at the door' });
    render(DetectionRow, { props: { detection } });
    expect(screen.getByText('package at the door')).toBeInTheDocument();
  });

  it('shows the noTranscript fallback when transcript is absent', () => {
    const detection = createMockDetection({ transcript: undefined });
    render(DetectionRow, { props: { detection } });
    // t() is mocked to return its key
    expect(screen.getByText('detections.noTranscript')).toBeInTheDocument();
  });

  it('does not render a species thumbnail image element', () => {
    const detection = createMockDetection({ transcript: 'hello world' });
    render(DetectionRow, { props: { detection } });
    // The old thumbnail img had src containing the species-image API path.
    // After the pivot only the spectrogram img remains; no species thumbnail.
    expect(document.querySelector('img[src*="species-image"]')).toBeNull();
  });
});

describe('DetectionRow action callbacks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('invokes onReview when the review menu item is clicked', async () => {
    const onReview = vi.fn();
    render(DetectionRow, { props: { detection: createMockDetection({ id: 456 }), onReview } });

    await openMenuAndClick(/review detection/i);

    expect(onReview).toHaveBeenCalledTimes(1);
  });

  it('invokes onDelete when the delete menu item is clicked', async () => {
    const onDelete = vi.fn();
    render(DetectionRow, { props: { detection: createMockDetection({ id: 100 }), onDelete } });

    await openMenuAndClick(/delete detection/i);

    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it('invokes onToggleLock when the lock menu item is clicked', async () => {
    const onToggleLock = vi.fn();
    render(DetectionRow, {
      props: { detection: createMockDetection({ id: 200, locked: false }), onToggleLock },
    });

    await openMenuAndClick(/lock detection/i);

    expect(onToggleLock).toHaveBeenCalledTimes(1);
  });

  it('invokes onToggleSpecies when the ignore-species menu item is clicked', async () => {
    const onToggleSpecies = vi.fn();
    render(DetectionRow, {
      props: { detection: createMockDetection({ id: 300 }), onToggleSpecies },
    });

    await openMenuAndClick(/ignore species/i);

    expect(onToggleSpecies).toHaveBeenCalledTimes(1);
  });

  it('invokes onMarkCorrect when the mark-correct menu item is clicked', async () => {
    const onMarkCorrect = vi.fn();
    render(DetectionRow, {
      props: { detection: createMockDetection({ id: 400 }), onMarkCorrect },
    });

    // Anchored so "Correct" does not also match "Incorrect".
    await openMenuAndClick(/^Correct$/);

    expect(onMarkCorrect).toHaveBeenCalledTimes(1);
  });

  it('invokes onMarkFalsePositive when the mark-false-positive menu item is clicked', async () => {
    const onMarkFalsePositive = vi.fn();
    render(DetectionRow, {
      props: { detection: createMockDetection({ id: 500 }), onMarkFalsePositive },
    });

    await openMenuAndClick(/^Incorrect$/);

    expect(onMarkFalsePositive).toHaveBeenCalledTimes(1);
  });
});
