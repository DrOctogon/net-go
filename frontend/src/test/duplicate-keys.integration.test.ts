/**
 * Integration Tests: Duplicate Key Validation with Real Database Data
 *
 * These tests run in a real browser against a real VoiceWatch backend
 * to validate that API responses won't cause duplicate key errors when
 * rendered by Svelte components.
 *
 * Svelte 5 throws `each_key_duplicate` runtime errors when {#each} blocks
 * encounter non-unique keys. These tests catch data-level issues that
 * synthetic unit tests cannot — for example, the v2 database schema
 * producing rows with duplicate scientific names or detection IDs.
 *
 * Prerequisites:
 *   - Backend running on http://localhost:8080
 *   - Start with: task integration-backend
 *
 * Usage:
 *   npm run test:integration
 */

import { describe, expect, it } from 'vitest';
import { apiCall } from './integration-setup';
import { getLocalDateString } from '$lib/utils/date';

// ============================================================================
// Helper: Check array for duplicate values in a specific field
// ============================================================================

/**
 * Finds duplicate values for a given key in an array of objects.
 * Returns an array of { value, indexes } for each duplicate found.
 */
function findDuplicateKeys<T>(
  items: T[],
  keyFn: (item: T) => string | number | undefined
): Array<{ value: string | number; indexes: number[] }> {
  const seen = new Map<string | number, number[]>();

  items.forEach((item, index) => {
    const key = keyFn(item);
    if (key === undefined) return;

    const existing = seen.get(key);
    if (existing) {
      existing.push(index);
    } else {
      seen.set(key, [index]);
    }
  });

  return Array.from(seen.entries())
    .filter(([, indexes]) => indexes.length > 1)
    .map(([value, indexes]) => ({ value, indexes }));
}

// ============================================================================
// Daily Species Summary — DailySummaryCard uses (item.scientific_name) as key
// ============================================================================

describe('Duplicate Keys: Daily Species Summary', () => {
  it('daily species summary has unique scientific_name keys', async () => {
    // DailySummaryCard.svelte:1029 — {#each sortedData as item (item.scientific_name)}
    // If the v2 schema returns multiple rows for the same species on a given day,
    // the component will crash with each_key_duplicate
    const today = getLocalDateString();
    const response = await apiCall(`/analytics/species/daily?date=${today}&limit=100`);

    if (!response.ok) {
      // No data for today is fine — skip test
      console.log(`No daily summary for ${today} (status ${response.status}), skipping`);
      return;
    }

    const data = await response.json();

    // Response may be an array or wrapped in an object
    const species: Array<{ scientific_name: string }> = Array.isArray(data)
      ? data
      : (data.species ?? data.data ?? []);

    if (species.length === 0) {
      console.log('No species data for today, skipping');
      return;
    }

    const duplicates = findDuplicateKeys(species, item => item.scientific_name);

    expect(duplicates).toEqual([]);
  });

  it('daily species summary across multiple dates has unique keys per date', async () => {
    // Test the last 7 days to catch schema issues
    const dates: string[] = [];
    for (let i = 0; i < 7; i++) {
      const d = new Date();
      d.setDate(d.getDate() - i);
      dates.push(getLocalDateString(d));
    }

    for (const date of dates) {
      const response = await apiCall(`/analytics/species/daily?date=${date}&limit=200`);

      if (!response.ok) continue;

      const data = await response.json();
      const species: Array<{ scientific_name: string }> = Array.isArray(data)
        ? data
        : (data.species ?? data.data ?? []);

      if (species.length === 0) continue;

      const duplicates = findDuplicateKeys(species, item => item.scientific_name);

      expect(
        duplicates,
        `Duplicate scientific_name in daily summary for ${date}: ${JSON.stringify(duplicates)}`
      ).toEqual([]);
    }
  });
});

// ============================================================================
// Species Summary — Species.svelte uses (species.scientific_name) as key
// ============================================================================

describe('Duplicate Keys: Species Summary', () => {
  it('species summary has unique scientific_name keys', async () => {
    // Species.svelte:509,518,539,593 — {#each filteredSpecies as species (species.scientific_name)}
    const today = getLocalDateString();
    const weekAgo = getLocalDateString(new Date(Date.now() - 7 * 24 * 60 * 60 * 1000));

    const response = await apiCall(
      `/analytics/species/summary?start_date=${weekAgo}&end_date=${today}`
    );

    if (!response.ok) {
      console.log(`Species summary not available (status ${response.status}), skipping`);
      return;
    }

    const data = await response.json();
    const species: Array<{ scientific_name: string }> = Array.isArray(data)
      ? data
      : (data.species ?? data.data ?? []);

    if (species.length === 0) {
      console.log('No species summary data, skipping');
      return;
    }

    const duplicates = findDuplicateKeys(species, item => item.scientific_name);

    expect(
      duplicates,
      `Duplicate scientific_name in species summary: ${JSON.stringify(duplicates)}`
    ).toEqual([]);
  });
});

// ============================================================================
// Detections — DetectionsList, DetectionCardGrid use (detection.id) as key
// ============================================================================

describe('Duplicate Keys: Detections', () => {
  it('detections list has unique id keys', async () => {
    // DetectionsList.svelte:345 — {#each data.notes as detection (detection.id)}
    // DetectionCardGrid.svelte:275 — {#each data.slice(0, selectedLimit) as detection (detection.id)}
    const response = await apiCall('/detections?limit=500');

    if (!response.ok) {
      console.log(`Detections not available (status ${response.status}), skipping`);
      return;
    }

    const data = await response.json();
    const detections: Array<{ id: string | number }> = Array.isArray(data)
      ? data
      : (data.notes ?? data.detections ?? data.data ?? []);

    if (detections.length === 0) {
      console.log('No detection data, skipping');
      return;
    }

    const duplicates = findDuplicateKeys(detections, item => item.id);

    expect(duplicates, `Duplicate detection IDs: ${JSON.stringify(duplicates)}`).toEqual([]);
  });

  it('recent detections have unique id keys', async () => {
    // Analytics.svelte:1348 — {#each recentDetections as detection, index (detection.id ?? index)}
    const response = await apiCall('/detections/recent?limit=50');

    if (!response.ok) {
      console.log(`Recent detections not available (status ${response.status}), skipping`);
      return;
    }

    const data = await response.json();
    const detections: Array<{ id: string | number }> = Array.isArray(data)
      ? data
      : (data.notes ?? data.detections ?? data.data ?? []);

    if (detections.length === 0) {
      console.log('No recent detection data, skipping');
      return;
    }

    const duplicates = findDuplicateKeys(detections, item => item.id);

    expect(duplicates, `Duplicate recent detection IDs: ${JSON.stringify(duplicates)}`).toEqual([]);
  });
});

// ============================================================================
// Audio Devices — AudioSettingsPage uses device values in SelectDropdown
// ============================================================================

describe('Duplicate Keys: Audio Devices', () => {
  it('audio devices have unique id values', async () => {
    // AudioSettingsPage derives audioSourceOptions from device data
    // SelectDropdown.svelte uses (option.value) as key (now fixed to composite key)
    // But verifying data uniqueness is still valuable for correct behavior
    const response = await apiCall('/system/audio/devices');

    if (!response.ok) {
      console.log(`Audio devices not available (status ${response.status}), skipping`);
      return;
    }

    const data = await response.json();
    const devices: Array<{ id: string; name: string }> = Array.isArray(data)
      ? data
      : (data.devices ?? data.data ?? []);

    if (devices.length === 0) {
      console.log('No audio devices found, skipping');
      return;
    }

    const duplicates = findDuplicateKeys(devices, item => item.id);

    expect(duplicates, `Audio devices with duplicate IDs: ${JSON.stringify(duplicates)}`).toEqual(
      []
    );
  });
});

// ============================================================================
// Hourly Distribution — used by Analytics time charts
// ============================================================================

describe('Duplicate Keys: Hourly Distribution', () => {
  it('hourly distribution has unique time slots', async () => {
    const today = getLocalDateString();
    const weekAgo = getLocalDateString(new Date(Date.now() - 7 * 24 * 60 * 60 * 1000));

    const response = await apiCall(
      `/analytics/time/distribution/hourly?start_date=${weekAgo}&end_date=${today}`
    );

    if (!response.ok) {
      console.log(`Hourly distribution not available (status ${response.status}), skipping`);
      return;
    }

    const data = await response.json();
    const distribution: Array<{ hour: number }> = Array.isArray(data)
      ? data
      : (data.distribution ?? data.data ?? []);

    if (distribution.length === 0) {
      console.log('No hourly distribution data, skipping');
      return;
    }

    const duplicates = findDuplicateKeys(distribution, item => item.hour);

    expect(
      duplicates,
      `Duplicate hours in hourly distribution: ${JSON.stringify(duplicates)}`
    ).toEqual([]);
  });
});

// ============================================================================
// New Species Detections — Analytics page
// ============================================================================

describe('Duplicate Keys: New Species Detections', () => {
  it('new species detections have unique identifiers', async () => {
    const today = getLocalDateString();
    const monthAgo = getLocalDateString(new Date(Date.now() - 30 * 24 * 60 * 60 * 1000));

    const response = await apiCall(
      `/analytics/species/detections/new?start_date=${monthAgo}&end_date=${today}`
    );

    if (!response.ok) {
      console.log(`New species not available (status ${response.status}), skipping`);
      return;
    }

    const data = await response.json();
    const newSpecies: Array<{ scientific_name?: string; scientificName?: string }> = Array.isArray(
      data
    )
      ? data
      : (data.species ?? data.data ?? []);

    if (newSpecies.length === 0) {
      console.log('No new species data, skipping');
      return;
    }

    const duplicates = findDuplicateKeys(
      newSpecies,
      item => item.scientific_name ?? item.scientificName
    );

    expect(duplicates, `Duplicate new species: ${JSON.stringify(duplicates)}`).toEqual([]);
  });
});
