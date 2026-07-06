/**
 * Unit tests for VoiceActivityCard.
 *
 * Verifies:
 * - Loading state is shown immediately on render (before the fetch resolves)
 * - Data state renders an SVG chart when API returns hourly detections
 * - Empty state is shown when all hourly counts are zero
 * - Error state is shown when the API call rejects
 *
 * The API (`$lib/utils/api`) is mocked per-test via vi.mock / vi.mocked to keep
 * tests isolated from network activity.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { cleanup } from '@testing-library/svelte';

// Mock api before importing the component so Svelte picks up the stub.
vi.mock('$lib/utils/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

// Mock date util so tests are deterministic regardless of the current date.
vi.mock('$lib/utils/date', () => ({
  getLocalDateString: vi.fn(() => '2026-06-26'),
  getLocalTimeString: vi.fn(() => '12:00:00'),
  getDateInTimezone: vi.fn(() => '2026-06-26'),
  parseHour: vi.fn((time: string) => parseInt(time.split(':')[0] ?? '0', 10)),
  parseLocalDateString: vi.fn((s: string) => new Date(s)),
}));

import VoiceActivityCard from './VoiceActivityCard.svelte';
import { api } from '$lib/utils/api';

// ── Helpers ───────────────────────────────────────────────────────────────

/** Build a full 24-entry hourly data array for the API mock. */
function makeHourlyData(countFn: (hour: number) => number = () => 0) {
  return Array.from({ length: 24 }, (_, hour) => ({ hour, count: countFn(hour) }));
}

// ── Test Suite ────────────────────────────────────────────────────────────

describe('VoiceActivityCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('shows a loading indicator while the fetch is in-flight', () => {
    // Never-resolving promise keeps the card in the loading state for the duration of the test.
    vi.mocked(api.get).mockReturnValue(new Promise(() => {}));

    render(VoiceActivityCard);

    // The loading container has role="status"; its aria-label contains the loading key.
    const status = screen.getByRole('status');
    expect(status).toBeInTheDocument();
  });

  it('renders an SVG chart when the API returns detections', async () => {
    const mockData = makeHourlyData(hour => (hour >= 8 && hour <= 17 ? 5 : 0));
    vi.mocked(api.get).mockResolvedValue(mockData);

    render(VoiceActivityCard);

    // Wait for the fetch to complete and the chart to appear.
    const svg = await waitFor(() => {
      const el = document.querySelector('svg[role="img"]');
      if (!el) throw new Error('SVG not rendered yet');
      return el;
    });

    expect(svg).toBeInTheDocument();
    // aria-label is set to the chartAriaLabel i18n key (mock returns key as text)
    expect(svg.getAttribute('aria-label')).toContain('dashboard.voiceActivity.chartAriaLabel');
  });

  it('shows the empty state when all hourly counts are zero', async () => {
    vi.mocked(api.get).mockResolvedValue(makeHourlyData(() => 0));

    render(VoiceActivityCard);

    await waitFor(() => {
      // The empty-state paragraph contains the "empty" i18n key (mock returns key as text)
      const empty = screen.queryByText('dashboard.voiceActivity.empty');
      if (!empty) throw new Error('Empty state not rendered yet');
      return empty;
    });

    expect(screen.getByText('dashboard.voiceActivity.empty')).toBeInTheDocument();
  });

  it('shows the error state when the API call rejects', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('Network error'));

    render(VoiceActivityCard);

    await waitFor(() => {
      const alert = screen.queryByRole('alert');
      if (!alert) throw new Error('Error alert not rendered yet');
      return alert;
    });

    const alert = screen.getByRole('alert');
    expect(alert).toBeInTheDocument();
    expect(alert.textContent).toContain('Network error');
  });

  it('calls the hourly distribution endpoint with today as start and end date', async () => {
    vi.mocked(api.get).mockResolvedValue(makeHourlyData(() => 1));

    render(VoiceActivityCard);

    await waitFor(() => expect(vi.mocked(api.get)).toHaveBeenCalledOnce());

    const [url] = vi.mocked(api.get).mock.calls[0] as [string];
    expect(url).toContain('/api/v2/analytics/time/distribution/hourly');
    expect(url).toContain('start_date=2026-06-26');
    expect(url).toContain('end_date=2026-06-26');
  });
});
