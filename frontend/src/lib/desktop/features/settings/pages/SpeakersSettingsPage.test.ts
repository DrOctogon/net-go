/**
 * Tests for SpeakersSettingsPage.
 *
 * Common mocks (logger, i18n, toast) come from src/test/setup.ts. This file
 * follows the AudioSettingsPage.test.ts idiom: stub global.fetch directly for
 * the plain GET calls the component makes, and mock $lib/utils/api's
 * fetchWithCSRF for the rename PUT. Auth gating uses the real auth store
 * (see src/lib/stores/auth.test.ts) rather than a mock, since isAuthenticated
 * is a thin derived wrapper around it.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, within, waitFor, fireEvent, cleanup } from '@testing-library/svelte';
import SpeakersSettingsPage from './SpeakersSettingsPage.svelte';
import { auth } from '$lib/stores/auth';

vi.mock('$lib/utils/api', () => ({
  fetchWithCSRF: vi.fn(),
}));

import { fetchWithCSRF } from '$lib/utils/api';

const mockFetchWithCSRF = vi.mocked(fetchWithCSRF);

interface RosterEntry {
  speakerId: string;
  name: string;
  detections: number;
}

interface ActivityEntry {
  speakerId: string;
  date: string;
  count: number;
}

function jsonResponse(data: unknown, ok = true, status = 200): Response {
  return {
    ok,
    status,
    json: () => Promise.resolve(data),
  } as unknown as Response;
}

/** Wires global.fetch to answer GET /api/v2/speakers and /api/v2/speakers/activity independently. */
function stubFetch({
  roster,
  rosterError,
  activity,
  activityError,
  activityPending,
}: {
  roster?: RosterEntry[];
  rosterError?: boolean;
  activity?: ActivityEntry[];
  activityError?: boolean;
  activityPending?: boolean;
}) {
  global.fetch = vi.fn((url: string) => {
    if (url.includes('/api/v2/speakers/activity')) {
      if (activityPending) return new Promise(() => {});
      if (activityError) return Promise.reject(new Error('network error'));
      return Promise.resolve(jsonResponse(activity ?? []));
    }
    if (url.includes('/api/v2/speakers')) {
      if (rosterError) return Promise.reject(new Error('network error'));
      return Promise.resolve(jsonResponse(roster ?? []));
    }
    return Promise.reject(new Error(`unexpected fetch: ${url}`));
  }) as unknown as typeof fetch;
}

describe('SpeakersSettingsPage', () => {
  let originalFetch: typeof global.fetch;

  beforeEach(() => {
    vi.clearAllMocks();
    originalFetch = global.fetch;
    // Default: security disabled -> isAuthenticated is true (see auth.ts).
    auth.setSecurity(false, true);
  });

  afterEach(() => {
    global.fetch = originalFetch;
    auth.setSecurity(false, true);
    cleanup();
  });

  describe('Roster tab', () => {
    it('renders speaker names, raw spk_N fallback, and detection counts, most-detected first', async () => {
      stubFetch({
        roster: [
          { speakerId: 'spk_1', name: 'Alice', detections: 5 },
          { speakerId: 'spk_2', name: '', detections: 12 },
        ],
      });

      render(SpeakersSettingsPage);

      await waitFor(() => {
        expect(screen.getByText('Alice')).toBeInTheDocument();
      });

      const rows = screen.getAllByRole('row');
      // rows[0] is the header row; data rows follow in detections-descending order.
      expect(within(rows[1]).getByText('spk_2')).toBeInTheDocument();
      expect(within(rows[1]).getByText('12')).toBeInTheDocument();
      expect(rows[1].textContent).toContain('settings.speakers.unnamed');

      expect(within(rows[2]).getByText('Alice')).toBeInTheDocument();
      expect(within(rows[2]).getByText('5')).toBeInTheDocument();
      // The named row also shows the raw id as secondary text.
      expect(within(rows[2]).getByText('spk_1')).toBeInTheDocument();
    });

    it('shows a login-required message and skips fetching for unauthenticated (guest) users', async () => {
      auth.setSecurity(true, false);
      stubFetch({ roster: [{ speakerId: 'spk_1', name: 'Alice', detections: 1 }] });

      render(SpeakersSettingsPage);

      expect(screen.getByText('settings.speakers.loginRequired')).toBeInTheDocument();
      expect(global.fetch).not.toHaveBeenCalled();
    });

    it('shows an empty state when the roster has no speakers', async () => {
      stubFetch({ roster: [] });

      render(SpeakersSettingsPage);

      await waitFor(() => {
        expect(screen.getByText('settings.speakers.empty')).toBeInTheDocument();
      });
    });

    it('shows an error state when the roster fetch fails', async () => {
      stubFetch({ rosterError: true });

      render(SpeakersSettingsPage);

      await waitFor(() => {
        expect(screen.getByText('settings.speakers.loadFailed')).toBeInTheDocument();
      });
    });

    describe('Rename flow', () => {
      beforeEach(() => {
        stubFetch({ roster: [{ speakerId: 'spk_1', name: 'Alice', detections: 5 }] });
      });

      it('renames a speaker: opens the editor, submits, PUTs the new name, and updates the UI', async () => {
        mockFetchWithCSRF.mockResolvedValue(undefined);
        render(SpeakersSettingsPage);

        await waitFor(() => {
          expect(screen.getByText('Alice')).toBeInTheDocument();
        });

        await fireEvent.click(
          screen.getByRole('button', { name: 'detections.speaker.renameAction' })
        );

        const input = screen.getByLabelText('detections.speaker.renamePlaceholder');
        await fireEvent.input(input, { target: { value: 'Bob' } });
        await fireEvent.click(screen.getByRole('button', { name: 'common.save' }));

        await waitFor(() => {
          expect(mockFetchWithCSRF).toHaveBeenCalledWith('/api/v2/speakers/spk_1/name', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: 'Bob' }),
          });
        });

        await waitFor(() => {
          expect(screen.getByText('Bob')).toBeInTheDocument();
        });
        expect(screen.queryByText('Alice')).not.toBeInTheDocument();
        expect(
          screen.queryByLabelText('detections.speaker.renamePlaceholder')
        ).not.toBeInTheDocument();
      });

      it('cancel restores the original name and issues no request', async () => {
        render(SpeakersSettingsPage);

        await waitFor(() => {
          expect(screen.getByText('Alice')).toBeInTheDocument();
        });

        await fireEvent.click(
          screen.getByRole('button', { name: 'detections.speaker.renameAction' })
        );

        const input = screen.getByLabelText('detections.speaker.renamePlaceholder');
        await fireEvent.input(input, { target: { value: 'Charlie' } });
        await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

        expect(mockFetchWithCSRF).not.toHaveBeenCalled();
        expect(screen.getByText('Alice')).toBeInTheDocument();
        expect(screen.queryByText('Charlie')).not.toBeInTheDocument();
        expect(
          screen.queryByLabelText('detections.speaker.renamePlaceholder')
        ).not.toBeInTheDocument();
      });
    });
  });

  describe('Activity tab', () => {
    async function openActivityTab() {
      await fireEvent.click(screen.getByRole('tab', { name: 'settings.speakers.activityTab' }));
    }

    it('shows a loading state while the activity fetch is in flight', async () => {
      stubFetch({ roster: [], activityPending: true });
      render(SpeakersSettingsPage);

      await waitFor(() => {
        expect(screen.getByRole('tab', { name: 'settings.speakers.activityTab' })).toBeVisible();
      });
      await openActivityTab();

      expect(screen.getByText('common.ui.loadingSettings')).toBeInTheDocument();
    });

    it('renders a populated activity table joined with roster display names, newest day first', async () => {
      stubFetch({
        roster: [{ speakerId: 'spk_1', name: 'Alice', detections: 5 }],
        activity: [
          { speakerId: 'spk_1', date: '2026-09-01', count: 3 },
          { speakerId: 'spk_2', date: '2026-09-02', count: 7 },
        ],
      });
      render(SpeakersSettingsPage);

      await waitFor(() => {
        expect(screen.getByText('Alice')).toBeInTheDocument();
      });
      await openActivityTab();

      await waitFor(() => {
        expect(screen.getByText('2026-09-02')).toBeInTheDocument();
      });

      const rows = screen.getAllByRole('row');
      // Newest date first: 2026-09-02 (unnamed spk_2) before 2026-09-01 (named Alice).
      expect(within(rows[1]).getByText('2026-09-02')).toBeInTheDocument();
      expect(within(rows[1]).getByText('spk_2')).toBeInTheDocument();
      expect(within(rows[1]).getByText('7')).toBeInTheDocument();

      expect(within(rows[2]).getByText('2026-09-01')).toBeInTheDocument();
      expect(within(rows[2]).getByText('Alice')).toBeInTheDocument();
      expect(within(rows[2]).getByText('3')).toBeInTheDocument();
    });

    it('shows an empty state when there is no activity', async () => {
      stubFetch({ roster: [], activity: [] });
      render(SpeakersSettingsPage);

      await waitFor(() => {
        expect(screen.getByRole('tab', { name: 'settings.speakers.activityTab' })).toBeVisible();
      });
      await openActivityTab();

      await waitFor(() => {
        expect(screen.getByText('settings.speakers.activityEmpty')).toBeInTheDocument();
      });
    });

    it('shows an error state when the activity fetch fails', async () => {
      stubFetch({ roster: [], activityError: true });
      render(SpeakersSettingsPage);

      await waitFor(() => {
        expect(screen.getByRole('tab', { name: 'settings.speakers.activityTab' })).toBeVisible();
      });
      await openActivityTab();

      await waitFor(() => {
        expect(screen.getByText('settings.speakers.activityLoadFailed')).toBeInTheDocument();
      });
    });
  });
});
