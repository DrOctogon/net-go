/**
 * speciesDictionary.svelte.ts
 *
 * Per-visitor species-name dictionary store — PARKED.
 *
 * The client-side per-locale dictionary was fetched from a backend per-locale
 * species dictionary endpoint, which was removed in the human-voice pivot. The
 * feature was already gated off (PER_VISITOR_SPECIES_LOCALE_ENABLED),
 * so nothing relied on it at runtime; with the endpoint gone the store now
 * degrades permanently to an empty dictionary.
 *
 * The public API is retained because speciesDisplay.localizeSpeciesName (a live
 * consumer) imports localizeScientific. With empty maps that helper falls back
 * through the chain: dictionary miss -> server-provided common name -> scientific
 * name — the same behavior the gated-off feature always produced.
 */

import { getLocale } from '$lib/i18n/store.svelte';
import { normalizeForLookup } from '$lib/utils/speciesNames';
import type { Locale } from '$lib/i18n/config';

// ---------------------------------------------------------------------------
// Feature gate
// ---------------------------------------------------------------------------

/**
 * Master switch for per-visitor, client-side species-name localization.
 *
 * PARKED (false). The backend dictionary endpoint was removed, so this can only
 * be re-enabled together with a NEW per-visitor species-language backend that is
 * SEPARATE from the UI locale and DEFAULTS to the server-side species language
 * (never the browser locale).
 */
export const PER_VISITOR_SPECIES_LOCALE_ENABLED = false;

// ---------------------------------------------------------------------------
// Reactive state (always empty while parked)
// ---------------------------------------------------------------------------

interface DictionaryState {
  /** The locale last requested. */
  locale: Locale;
  /** scientific name -> localized common name. Always empty while parked. */
  forward: Map<string, string>;
  /** NFC-folded common name -> scientific names. Always empty while parked. */
  reverse: Map<string, string[]>;
}

const EMPTY_FORWARD: Map<string, string> = new Map();
const EMPTY_REVERSE: Map<string, string[]> = new Map();

let current = $state<DictionaryState>({
  locale: getLocale(),
  forward: EMPTY_FORWARD,
  reverse: EMPTY_REVERSE,
});

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Load the species-name dictionary for the given locale.
 *
 * PARKED no-op: the backend endpoint was removed, so this records the requested
 * locale and leaves the maps empty. Kept async so existing call sites
 * (App.svelte) that `.catch()` it continue to type-check.
 */
export async function loadDictionary(locale: Locale = getLocale()): Promise<void> {
  if (current.locale !== locale) {
    current = { locale, forward: EMPTY_FORWARD, reverse: EMPTY_REVERSE };
  }
  return Promise.resolve();
}

/**
 * Look up the localized common name for a scientific name. Always undefined
 * while parked, so callers fall back to the server-provided common name.
 */
export function localizeScientific(scientificName: string): string | undefined {
  return current.forward.get(scientificName);
}

/** Scientific names for a localized common name. Always [] while parked. */
export function resolveCommonToScientific(text: string): string[] {
  return current.reverse.get(normalizeForLookup(text)) ?? [];
}

/**
 * Unique scientific name for an unambiguous common name. Always undefined while
 * parked, so settings pickers fall back to the typed text.
 */
export function resolveCommonToScientificUnique(text: string): string | undefined {
  const matches = current.reverse.get(normalizeForLookup(text));
  return matches?.length === 1 ? matches[0] : undefined;
}

/** Substring search over localized common names. Always [] while parked. */
export function searchScientificByCommon(_text: string): string[] {
  return [];
}

// ---------------------------------------------------------------------------
// Test helper (not for production use)
// ---------------------------------------------------------------------------

/**
 * Reset internal state.
 * Exported ONLY for use in Vitest tests. Do not call from application code.
 * @internal
 */
export function resetDictionaryForTest(): void {
  current = { locale: getLocale(), forward: EMPTY_FORWARD, reverse: EMPTY_REVERSE };
}
