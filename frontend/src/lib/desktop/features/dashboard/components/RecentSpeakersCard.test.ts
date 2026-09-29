/**
 * Unit tests for RecentSpeakersCard.
 *
 * Verifies:
 * - Loading state is shown immediately on render (before the fetch resolves)
 * - Roster state renders the top 5 speakers by detections with spk_N name fallback
 * - Empty state is shown when the roster is empty
 * - Error state is shown when the API call rejects
 * - Guest state shows a sign-in hint and never calls the auth-gated API
 * - Row click navigates to the speaker settings page
 *
 * The API (`$lib/utils/api`) is mocked per-test via vi.mock / vi.mocked to keep
 * tests isolated from network activity (same idiom as VoiceActivityCard.test.ts).
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent, cleanup } from '@testing-library/svelte';

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

// Mock navigation so row clicks can be asserted without a History API.
vi.mock('$lib/stores/navigation.svelte', () => ({
  navigation: {
    navigate: vi.fn(),
    currentPath: '/ui/dashboard',
  },
}));

import RecentSpeakersCard from './RecentSpeakersCard.svelte';
import { api } from '$lib/utils/api';
import { navigation } from '$lib/stores/navigation.svelte';

// ── Helpers ───────────────────────────────────────────────────────────────

interface RosterEntry {
  speakerId: string;
  name: string;
  detections: number;
}

function makeRoster(): RosterEntry[] {
  return [
    { speakerId: 'spk_1', name: 'Alice', detections: 12 },
    { speakerId: 'spk_2', name: '', detections: 40 },
    { speakerId: 'spk_3', name: 'Bob', detections: 7 },
    { speakerId: 'spk_4', name: 'Carol', detections: 25 },
    { speakerId: 'spk_5', name: 'Dave', detections: 3 },
    { speakerId: 'spk_6', name: 'Eve', detections: 1 },
  ];
}

// ── Test Suite ────────────────────────────────────────────────────────────

describe('RecentSpeakersCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('shows a loading indicator while the fetch is in-flight', () => {
    // Never-resolving promise keeps the card in the loading state.
    vi.mocked(api.get).mockReturnValue(new Promise(() => {}));

    render(RecentSpeakersCard);

    expect(screen.getByRole('status')).toBeInTheDocument();
  });

  it('renders the top 5 speakers by detections with spk_N fallback for unnamed speakers', async () => {
    vi.mocked(api.get).mockResolvedValue(makeRoster());

    render(RecentSpeakersCard);

    await waitFor(() => {
      if (!screen.queryByText('Alice')) throw new Error('Roster not rendered yet');
    });

    // Top 5 by detections: spk_2 (40, unnamed → id fallback), Carol (25),
    // Alice (12), Bob (7), Dave (3). Eve (1) is cut by the top-5 limit.
    expect(screen.getByText('spk_2')).toBeInTheDocument();
    expect(screen.getByText('Carol')).toBeInTheDocument();
    expect(screen.getByText('Alice')).toBeInTheDocument();
    expect(screen.getByText('Bob')).toBeInTheDocument();
    expect(screen.getByText('Dave')).toBeInTheDocument();
    expect(screen.queryByText('Eve')).not.toBeInTheDocument();

    // Rows are ordered by detections descending.
    const rows = screen.getAllByRole('listitem');
    expect(rows).toHaveLength(5);
    expect(rows[0]?.textContent).toContain('spk_2');
    expect(rows[1]?.textContent).toContain('Carol');
  });

  it('shows the empty state when the roster is empty', async () => {
    vi.mocked(api.get).mockResolvedValue([]);

    render(RecentSpeakersCard);

    await waitFor(() => {
      if (!screen.queryByText('dashboard.recentSpeakers.empty')) {
        throw new Error('Empty state not rendered yet');
      }
    });

    expect(screen.getByText('dashboard.recentSpeakers.empty')).toBeInTheDocument();
  });

  it('shows the error state when the API call rejects', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('Network error'));

    render(RecentSpeakersCard);

    await waitFor(() => {
      if (!screen.queryByRole('alert')) throw new Error('Error alert not rendered yet');
    });

    expect(screen.getByRole('alert').textContent).toContain('Network error');
  });

  it('shows a sign-in hint and skips the fetch for guests', () => {
    render(RecentSpeakersCard, { props: { isGuest: true } });

    expect(screen.getByText('dashboard.recentSpeakers.signInHint')).toBeInTheDocument();
    expect(vi.mocked(api.get)).not.toHaveBeenCalled();
  });

  it('calls the speakers roster endpoint once', async () => {
    vi.mocked(api.get).mockResolvedValue(makeRoster());

    render(RecentSpeakersCard);

    await waitFor(() => expect(vi.mocked(api.get)).toHaveBeenCalledOnce());

    const [url] = vi.mocked(api.get).mock.calls[0] as [string];
    expect(url).toBe('/api/v2/speakers');
  });

  it('navigates to speaker settings when a row is clicked', async () => {
    vi.mocked(api.get).mockResolvedValue(makeRoster());

    render(RecentSpeakersCard);

    await waitFor(() => {
      if (!screen.queryByText('Alice')) throw new Error('Roster not rendered yet');
    });

    const rows = screen.getAllByRole('button');
    await fireEvent.click(rows[0] as HTMLElement);

    expect(vi.mocked(navigation.navigate)).toHaveBeenCalledWith('/ui/settings/speakers');
  });
});
