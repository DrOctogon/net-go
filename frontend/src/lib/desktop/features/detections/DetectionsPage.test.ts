/**
 * Regression test for a stale-response race in fetchDetections().
 *
 * Bug: fetchDetections() had no AbortController and no staleness check, so a
 * fast follow-up request (sort/page/search/numResults change) could have its
 * response overwritten by an earlier, slower request that resolved later.
 *
 * Fix: fetchDetections() now aborts the previous in-flight request and checks
 * the captured AbortSignal before applying a response, following the pattern
 * in DetectionDetail.svelte's fetchDetection().
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';

const fetchWithCSRFMock = vi.fn();
vi.mock('$lib/utils/api', () => ({
  fetchWithCSRF: (...args: unknown[]) => fetchWithCSRFMock(...args),
}));

vi.mock('./components/DetectionsCard.svelte', async () => ({
  default: (await import('./components/__tests__/StubDetectionsCard.svelte')).default,
}));

import DetectionsPage from './DetectionsPage.svelte';

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(res => {
    resolve = res;
  });
  return { promise, resolve };
}

function makeResponse(id: number, total: number) {
  return {
    data: [{ id }],
    total,
    limit: 25,
    current_page: 1,
    total_pages: 1,
  };
}

describe('DetectionsPage - stale fetch race', () => {
  beforeEach(() => {
    fetchWithCSRFMock.mockReset();
    window.history.replaceState({}, '', '/ui/detections');
  });

  afterEach(() => {
    cleanup();
  });

  it('keeps the most recently requested sort result even when it resolves before the stale one', async () => {
    const initial = deferred<unknown>();
    fetchWithCSRFMock.mockReturnValueOnce(initial.promise);

    render(DetectionsPage);
    initial.resolve(makeResponse(0, 0));
    await waitFor(() => expect(screen.getByTestId('total-results').textContent).toBe('0'));

    // "sort-a" fires first (stale, slow) and "sort-b" fires second (fresh, fast).
    const stale = deferred<unknown>();
    fetchWithCSRFMock.mockReturnValueOnce(stale.promise);
    await fireEvent.click(screen.getByTestId('trigger-sort-a'));

    const fresh = deferred<unknown>();
    fetchWithCSRFMock.mockReturnValueOnce(fresh.promise);
    await fireEvent.click(screen.getByTestId('trigger-sort-b'));

    // Resolve the newer request first...
    fresh.resolve(makeResponse(2, 2));
    await waitFor(() => expect(screen.getByTestId('total-results').textContent).toBe('2'));

    // ...then let the older, superseded request resolve after it.
    stale.resolve(makeResponse(1, 1));
    await new Promise(r => setTimeout(r, 0));

    // The stale response must never overwrite the fresher one.
    expect(screen.getByTestId('total-results').textContent).toBe('2');
  });
});
