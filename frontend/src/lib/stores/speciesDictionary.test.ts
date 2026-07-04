/**
 * Tests for the speciesDictionary store.
 *
 * The store is PARKED: its backend per-locale species dictionary endpoint was
 * removed in the human-voice pivot, so loadDictionary is a no-op and every
 * lookup degrades to empty. These tests pin that parked behavior.
 *
 * Note: Common mocks (logger, i18n, toast) are defined in src/test/setup.ts.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// --- Module mocks (must be hoisted above imports) ---

vi.mock('$lib/i18n/store.svelte', () => ({
  getLocale: vi.fn(() => 'fi' as const),
}));

// Import after mocks are declared
import {
  loadDictionary,
  resolveCommonToScientificUnique,
  localizeScientific,
  resolveCommonToScientific,
  searchScientificByCommon,
  resetDictionaryForTest,
  PER_VISITOR_SPECIES_LOCALE_ENABLED,
} from './speciesDictionary.svelte';

describe('per-visitor species locale gate', () => {
  // The per-visitor client-side localization overlay is PARKED: its backend
  // endpoint was removed, so it can only be re-enabled with a new per-visitor
  // species-language backend that is separate from the UI locale.
  it('stays disabled after the backend dictionary endpoint was removed', () => {
    expect(PER_VISITOR_SPECIES_LOCALE_ENABLED).toBe(false);
  });
});

describe('speciesDictionary store (parked)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetDictionaryForTest();
  });

  afterEach(() => {
    vi.clearAllMocks();
    resetDictionaryForTest();
  });

  it('loadDictionary resolves without populating the dictionary', async () => {
    await loadDictionary('fi');
    expect(localizeScientific('Turdus merula')).toBeUndefined();
  });

  it('every lookup degrades to an empty result', async () => {
    await loadDictionary('fi');
    expect(resolveCommonToScientific('Mustarastas')).toEqual([]);
    expect(resolveCommonToScientificUnique('Mustarastas')).toBeUndefined();
    expect(searchScientificByCommon('lepakko')).toEqual([]);
  });

  it('switching locale keeps the dictionary empty', async () => {
    await loadDictionary('fi');
    await loadDictionary('fr');
    expect(localizeScientific('Turdus merula')).toBeUndefined();
  });
});
