import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { waitFor, cleanup, screen, fireEvent } from '@testing-library/svelte';
import { createComponentTestFactory } from '../../../test/render-helpers';
import DetectionDetail from './DetectionDetail.svelte';
import type { Detection } from '$lib/types/detection.types';

vi.mock('$lib/utils/api', () => ({
  fetchWithCSRF: vi.fn(),
}));

import { fetchWithCSRF } from '$lib/utils/api';

const mockFetchWithCSRF = vi.mocked(fetchWithCSRF);

// Heavy / context-dependent children are not relevant to the fetch-race logic.
vi.mock('$lib/desktop/components/media/AudioPlayer.svelte');
vi.mock('$lib/desktop/components/data/ConfidenceCircle.svelte');
vi.mock('$lib/desktop/components/data/WeatherDetails.svelte');
vi.mock('$lib/desktop/features/dashboard/components/SourceBadge.svelte');
vi.mock('$lib/desktop/components/ui/VerificationBadges.svelte');

const detailTest = createComponentTestFactory(DetectionDetail);

/** Build a minimal valid Detection for the detail view. */
function makeDetection(overrides: Partial<Detection>): Detection {
  return {
    id: 1,
    date: '2024-01-01',
    time: '10:00:00',
    timestamp: '2024-01-01T10:00:00Z',
    beginTime: '2024-01-01T10:00:00Z',
    endTime: '2024-01-01T10:00:03Z',
    speciesCode: 'spc',
    scientificName: 'Default scientific',
    commonName: 'Default common',
    confidence: 0.9,
    verified: 'unverified',
    locked: false,
    ...overrides,
  };
}

// Sentinel dates referenced by both the fixtures and the assertions.
// The hero renders det.date directly, so dates are the most visible
// unique field now that scientific names are no longer shown in the UI.
const FRESH_DATE = '2024-12-31';
const STALE_DATE = '2020-01-01';

/** Minimal fetch Response stub carrying a JSON body. */
function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    headers: new Headers({ 'content-type': 'application/json' }),
    json: () => Promise.resolve(body),
    // Serialize lazily and reject (never throw synchronously) so this
    // Promise-returning method honors its contract even on a non-serializable body.
    text: () => {
      try {
        return Promise.resolve(JSON.stringify(body));
      } catch (error) {
        return Promise.reject(error instanceof Error ? error : new Error(String(error)));
      }
    },
  } as unknown as Response;
}

describe('DetectionDetail stale-response race (#978)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  // Regression: navigating from detection A to B while A's request is still in
  // flight must not let A's late response overwrite B. The fix captures the
  // AbortController signal locally and checks the captured signal (not the shared
  // controller reference, which by then points at B's non-aborted controller).
  it('does not let a stale detection response overwrite a newer one', async () => {
    let resolveStale!: (r: Response) => void;
    const staleResponse = new Promise<Response>(resolve => {
      resolveStale = resolve;
    });

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        // Detection A: held in flight until we resolve it manually (after switching to B).
        if (url.includes('/api/v2/detections/det-a')) {
          return staleResponse;
        }
        // Detection B: resolves immediately and becomes the current detection.
        if (url.includes('/api/v2/detections/det-b')) {
          return Promise.resolve(jsonResponse(makeDetection({ id: 2, date: FRESH_DATE })));
        }
        // Secondary species/taxonomy/attribution endpoints: irrelevant here.
        return Promise.resolve(jsonResponse({}));
      })
    );

    const { container, rerender } = detailTest.render({ detectionId: 'det-a' });

    // Switch to detection B before A resolves.
    await rerender({ detectionId: 'det-b' });
    await waitFor(() => {
      expect(container.textContent).toContain(FRESH_DATE);
    });

    // A's response now arrives late; the captured-signal guard must drop it.
    resolveStale(jsonResponse(makeDetection({ id: 1, date: STALE_DATE })));
    // Flush the production stale-handling path: await the promise it awaits, then
    // a macrotask so every microtask hop (response.json, the captured-signal
    // guard) and the Svelte DOM flush complete before asserting. A microtask-only
    // flush (await tick) under-drains and lets the negative assertion fire early.
    await staleResponse;
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(container.textContent).toContain(FRESH_DATE);
    expect(container.textContent).not.toContain(STALE_DATE);
  });
});

describe('DetectionDetail similar-voices merge', () => {
  const DETECTION_ID = 'det-1';

  /**
   * Stub the three endpoints the detail view reads. The detection's own cluster
   * is spk_1 (the merge TARGET) and the similar-voices row is spk_2 (the source).
   */
  function stubFetch({ rowSpeakerId = 'spk_2' }: { rowSpeakerId?: string } = {}) {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/similar')) {
          return Promise.resolve(
            jsonResponse([{ id: 2, score: 0.93, speakerId: rowSpeakerId, date: '2024-06-01' }])
          );
        }
        if (url.includes('/api/v2/speakers')) {
          return Promise.resolve(
            jsonResponse([
              { speakerId: 'spk_1', name: 'Alice' },
              { speakerId: 'spk_2', name: 'Bob' },
            ])
          );
        }
        if (url.includes(`/api/v2/detections/${DETECTION_ID}`)) {
          return Promise.resolve(
            jsonResponse(makeDetection({ id: 1, date: FRESH_DATE, speakerId: 'spk_1' }))
          );
        }
        return Promise.resolve(jsonResponse({}));
      })
    );
  }

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it('merges the row cluster INTO the detection cluster (row = source, detection = target)', async () => {
    mockFetchWithCSRF.mockResolvedValue(undefined);
    stubFetch();
    detailTest.render({ detectionId: DETECTION_ID });

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'detections.speaker.merge.actionAria' })
      ).toBeInTheDocument();
    });

    await fireEvent.click(
      screen.getByRole('button', { name: 'detections.speaker.merge.actionAria' })
    );

    // The confirmation names the surviving speaker, so it must be the "named" variant.
    expect(screen.getByText('detections.speaker.merge.confirmNamed')).toBeInTheDocument();

    await fireEvent.click(
      screen.getByRole('button', { name: 'detections.speaker.merge.confirmLabel' })
    );

    await waitFor(() => {
      expect(mockFetchWithCSRF).toHaveBeenCalledWith('/api/v2/speakers/spk_1/merge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sourceId: 'spk_2' }),
      });
    });
  });

  it('hides the merge action when the row belongs to the same cluster', async () => {
    stubFetch({ rowSpeakerId: 'spk_1' });
    detailTest.render({ detectionId: DETECTION_ID });

    await waitFor(() => {
      expect(screen.getByText('detections.detail.similarVoices.title')).toBeInTheDocument();
    });

    expect(
      screen.queryByRole('button', { name: 'detections.speaker.merge.actionAria' })
    ).not.toBeInTheDocument();
  });
});
